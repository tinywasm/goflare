# PLAN — `cfClient` / `parseCFResponse`: transparencia de errores y preflight de permisos

> **Repo destino:** `tinywasm/goflare` (NO el demo). Plan para copiar manualmente a
> goflare, revisar y ejecutar allí.
> **Motivación:** el error `CF API returned success=false` no dice **nada** (ni status
> HTTP, ni código, ni cuerpo). Eso hizo que un fallo de provisión del proyecto Pages
> fuera casi imposible de diagnosticar desde el log de CI.

---

## 1. Causa raíz (estado actual en goflare v0.2.26)

`cloudflare.go` → `parseCFResponse` (líneas 348-367):

```go
func parseCFResponse(resp *http.Response) (json.RawMessage, error) {
    data, _ := io.ReadAll(resp.Body)
    var env cfEnvelope
    if err := json.Unmarshal(data, &env); err != nil {
        return nil, fmt.Errorf("parse CF envelope: %w", err)
    }
    if !env.Success {
        if len(env.Errors) > 0 {
            // ... "CF API error: <msg> (code: N)"
        }
        return nil, fmt.Errorf("CF API returned success=false")   // ← sin info
    }
    return env.Result, nil
}
```

Problemas:

1. **Ignora el status HTTP por completo.** `do()` (líneas 56-73) nunca mira
   `resp.StatusCode`; solo `env.Success`. Un 403, 404 o 5xx quedan indistinguibles.
2. **Cuando `errors[]` viene vacío, se pierde todo:** ni el cuerpo crudo ni el status.
   Justo el caso de un proyecto recién creado / no provisionado.
3. **No expone un tipo de error estructurado**, así que los callers detectan condiciones
   con `strings.Contains(err.Error(), "8000007")` — frágil (ver `PLAN_PAGES_DEPLOY.md`).
4. **El preflight de permisos es débil.** `Auth()` (`auth.go:35`) solo llama a
   `/user/tokens/verify`, que confirma que el token es válido pero **no** que tenga
   `Cloudflare Pages: Edit` sobre la cuenta objetivo. El fallo real aparece tarde, en
   `uploadToken`.

## 2. Cambios propuestos

### 2.1 Error tipado `cfError`

```go
type cfError struct {
    Status  int          // status HTTP
    Code    int          // primer errors[].code, si hay
    Message string       // resumen legible
    Errors  []cfAPIError // todos los errores del envelope
    Body    string       // cuerpo crudo truncado (fallback)
    Path    string       // método + ruta que falló
}

func (e *cfError) Error() string {
    if len(e.Errors) > 0 {
        return fmt.Sprintf("CF API %s → HTTP %d: %s", e.Path, e.Status, e.Message)
    }
    return fmt.Sprintf("CF API %s → HTTP %d, success=false, body: %s", e.Path, e.Status, e.Body)
}

func (e *cfError) alreadyExists() bool {
    for _, x := range e.Errors {
        if x.Code == 8000009 || strings.Contains(x.Message, "already exists") {
            return true
        }
    }
    return false
}
```

### 2.2 `parseCFResponse` nunca devuelve un error sin información

```go
func parseCFResponse(method, path string, resp *http.Response) (json.RawMessage, error) {
    data, _ := io.ReadAll(resp.Body)
    var env cfEnvelope
    if err := json.Unmarshal(data, &env); err != nil {
        return nil, &cfError{Status: resp.StatusCode, Path: method + " " + path,
            Body: truncate(string(data), 500)}
    }
    if !env.Success || resp.StatusCode >= 400 {
        ce := &cfError{
            Status: resp.StatusCode,
            Errors: env.Errors,
            Path:   method + " " + path,
            Body:   truncate(string(data), 500),
        }
        if len(env.Errors) > 0 {
            ce.Code = env.Errors[0].Code
            var msgs []string
            for _, e := range env.Errors {
                msgs = append(msgs, fmt.Sprintf("%s (code: %d)", e.Message, e.Code))
            }
            ce.Message = strings.Join(msgs, ", ")
        }
        return nil, ce
    }
    return env.Result, nil
}
```

> Requiere pasar `method`/`path` a `parseCFResponse`. Ajustar `do()` y `putMultipart()`
> para propagarlos (ya los tienen a mano).

### 2.3 Preflight de permisos antes del deploy

Ampliar `validateToken` (o añadir `validateDeployScopes`) para confirmar acceso real a
Pages sobre la cuenta objetivo, fallando temprano y con mensaje accionable:

```go
// Tras /user/tokens/verify, comprueba que el token puede LISTAR proyectos Pages
// de la cuenta — confirma Pages:Edit + AccountID correcto antes de tocar nada.
func (g *Goflare) validateDeployScopes(client *cfClient) error {
    path := fmt.Sprintf("/accounts/%s/pages/projects", g.Config.AccountID)
    if _, err := client.get(path); err != nil {
        return fmt.Errorf(
            "el token no puede acceder a Pages en la cuenta %s.\n"+
            "  - Verifica permiso Account → Cloudflare Pages → Edit\n"+
            "  - Verifica que CLOUDFLARE_ACCOUNT_ID es el correcto\n"+
            "Detalle: %w", g.Config.AccountID, err)
    }
    return nil
}
```

Llamarlo en `Deploy()` (`run.go`) justo después de `g.Auth()` (línea 102).

### 2.4 (Opcional) Flag `--verbose`

`goflare deploy --verbose` que loguee cada llamada CF (método, ruta, status) para
diagnóstico futuro sin tener que adivinar.

## 3. Criterios de aceptación

- [ ] Un fallo de CF en CI muestra status HTTP + código + cuerpo (o body crudo si
      `errors[]` está vacío). Nunca más un `success=false` "pelado".
- [ ] Token sin `Pages:Edit` o `ACCOUNT_ID` equivocado → falla en el **preflight**, antes
      de `build`/`uploadToken`, con mensaje que indica qué arreglar.
- [ ] Los callers de `PLAN_PAGES_DEPLOY.md` usan `errors.As(err, &cfError)` en vez de
      `strings.Contains`.
- [ ] Tests: mock que devuelve 403 → el error contiene `HTTP 403`; mock que devuelve
      `success:false` con `errors:[]` y body `{"foo":1}` → el error incluye ese body.

## 4. Relación con el otro plan

Este plan es **prerequisito recomendado** de `PLAN_PAGES_DEPLOY.md`: el error tipado
`cfError` es lo que permite (a) detectar "no existe" por status/código en vez de string,
y (b) reintentar `uploadToken` solo en fallos transitorios y no en un 403 de permisos.

Orden sugerido de implementación en goflare:
1. `PLAN_CF_API_CLIENT.md` (error tipado + preflight).
2. `PLAN_PAGES_DEPLOY.md` (auto-create + retry usando el error tipado).
