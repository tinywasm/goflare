# PLAN — `cloudflare.go` / `DeployPages`: auto-provisión robusta del proyecto Pages

> **Repo destino:** `tinywasm/goflare` (NO el demo). Este doc es un plan para copiar
> manualmente al repo de goflare, revisarlo y ejecutarlo allí.
> **Origen del problema:** deploy de `goflare-demo` falla con
> `[-] Pages: Failed - failed to get upload token: CF API returned success=false`
> cuando el proyecto Pages **no existe** todavía en la cuenta.

---

## 1. Contexto

`goflare deploy` usa el flujo **Direct Upload** de Cloudflare Pages:

```
GET  /accounts/:id/pages/projects/:name        ← ¿existe el proyecto?
POST /accounts/:id/pages/projects              ← crearlo si no existe
POST /accounts/:id/pages/projects/:name/uploadToken   ← pedir JWT de subida
POST /pages/assets/upload                      ← subir assets (con el JWT)
POST /accounts/:id/pages/projects/:name/deployments   ← crear el deployment
```

El objetivo del usuario: **si el proyecto no existe, que se cree solo** y el deploy
continúe sin intervención manual en el dashboard.

## 2. Causa raíz (estado actual en goflare v0.2.26)

`cloudflare.go` → `DeployPages()` (líneas 88-120):

```go
// 2. Ensure Pages project exists
_, err = client.get(projectPath)
if err != nil {
    if strings.Contains(err.Error(), "404") ||
       strings.Contains(err.Error(), "not found") ||
       strings.Contains(err.Error(), "8000007") {
        // Create project
        _, err = client.post(createPath, bodyJSON)   // crea {name, production_branch:"main"}
        if err != nil {
            return fmt.Errorf("failed to create Pages project: %w", err)
        }
    } else {
        return fmt.Errorf("failed to check Pages project: %w", err)
    }
}

// 3. Get upload JWT  ← se ejecuta INMEDIATAMENTE después de crear
tokenResp, err := client.post(tokenPath, nil)
if err != nil {
    return fmt.Errorf("failed to get upload token: %w", err)   // ← el error reportado
}
```

Problemas concretos:

1. **No hay espera entre `create` y `uploadToken`.** Un proyecto Pages recién creado
   por API es **eventualmente consistente**: durante uno o dos segundos el endpoint
   `uploadToken` aún no está disponible y devuelve `success:false` (sin `errors`, porque
   no es un error de validación sino de provisión). Ese es exactamente el síntoma:
   `failed to get upload token: CF API returned success=false`.
2. **Detección de "no existe" frágil (string matching).** Depende de que el texto del
   error contenga `"404"`, `"not found"` o `"8000007"`. Si Cloudflare cambia el código o
   el mensaje, o si `errors[]` viene vacío, la rama de creación no se dispara y caería en
   `failed to check Pages project`.
3. **Create no es idempotente.** Si dos deploys corren en paralelo (o un retry), el
   segundo `create` puede devolver "already exists" y abortar.

## 3. Cambios propuestos

### 3.1 Reintentar `uploadToken` con backoff (el fix principal)

Envolver la obtención del JWT en un retry corto que absorba la latencia de provisión:

```go
// 3. Get upload JWT — reintenta: un proyecto recién creado tarda en quedar listo.
var tokenResp []byte
err = retry(5, time.Second, func() error {
    var e error
    tokenResp, e = client.post(tokenPath, nil)
    return e
})
if err != nil {
    return fmt.Errorf("failed to get upload token: %w", err)
}
```

Helper (nuevo, en `cloudflare.go` o un `retry.go`):

```go
// retry ejecuta fn hasta n veces con backoff exponencial (base, 2*base, 4*base...).
func retry(n int, base time.Duration, fn func() error) error {
    var err error
    for i := 0; i < n; i++ {
        if err = fn(); err == nil {
            return nil
        }
        if i < n-1 {
            time.Sleep(base << i) // 1s, 2s, 4s, 8s...
        }
    }
    return err
}
```

> Nota: solo conviene reintentar errores transitorios. Si el plan
> `PLAN_CF_API_CLIENT.md` se implementa (errores tipados), reintentar **solo** cuando el
> status sea 5xx o el código sea transitorio; un 403 por permisos NO debe reintentarse.

### 3.2 Detección de "no existe" basada en status/código, no en substring

Apoyarse en el error tipado introducido en `PLAN_CF_API_CLIENT.md`:

```go
_, err = client.get(projectPath)
if err != nil {
    var apiErr *cfError
    notFound := errors.As(err, &apiErr) && (apiErr.Status == http.StatusNotFound || apiErr.Code == 8000007)
    if !notFound {
        return fmt.Errorf("failed to check Pages project: %w", err)
    }
    if err := g.createPagesProject(client); err != nil {
        return err
    }
}
```

### 3.3 `create` idempotente y con log

```go
func (g *Goflare) createPagesProject(client *cfClient) error {
    g.Logger("Pages project not found — creating", g.Config.ProjectName)
    createPath := fmt.Sprintf("/accounts/%s/pages/projects", g.Config.AccountID)
    body, _ := json.Marshal(map[string]string{
        "name":              g.Config.ProjectName,
        "production_branch": "main",
    })
    _, err := client.post(createPath, body)
    if err != nil {
        var apiErr *cfError
        // 8000009 / "already exists" → otro proceso lo creó; seguimos.
        if errors.As(err, &apiErr) && apiErr.alreadyExists() {
            return nil
        }
        return fmt.Errorf("failed to create Pages project: %w", err)
    }
    return nil
}
```

## 4. Criterios de aceptación

- [ ] `goflare deploy` contra una cuenta **sin** el proyecto → lo crea y completa el
      deploy sin error en el primer intento de CI.
- [ ] Segundo `goflare deploy` (proyecto ya existe) → no intenta recrearlo y funciona.
- [ ] Si `uploadToken` falla por permisos (403), **no** se reintenta 5 veces: falla
      rápido con mensaje claro (depende de `PLAN_CF_API_CLIENT.md`).
- [ ] Tests en `goflare/tests/deploy_pages_test.go`:
  - Caso nuevo: el mock devuelve 404 en `GET project`, 200 en `POST projects`,
    `success:false` en el **primer** `POST uploadToken` y `200` en el segundo →
    el deploy debe pasar (verifica el retry).
  - Caso permisos: `uploadToken` devuelve 403 → falla sin reintentar.

## 5. Cómo el demo vuelve a probar (después de publicar goflare)

1. En `goflare-demo`: `go get github.com/tinywasm/goflare@vX.Y.Z`
2. `go generate ./workflow/`  → regenera `.github/workflows/deploy.yml`
3. Commit de `go.mod`, `go.sum`, `deploy.yml` y push a `main`.
4. El workflow corre `goflare build` + `goflare deploy`; el proyecto Pages
   `goflare-demo` debe crearse solo y el deploy quedar verde.

> Recordatorio (de `AGENTS.md`): publicar goflare con `gorelease` (tag + binario en el
> Release). Nunca `git tag` + push solo, o el `curl` del workflow da 404.
