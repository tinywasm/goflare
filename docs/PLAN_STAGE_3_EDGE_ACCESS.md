---
message: "fix!: edge adopts the typed Access contract with Authn/Authorize seams; a guarded route is no longer a permanent 403"
---

> Este plan se despacha vía el flujo CodeJob. Ver skill: agents-workflow.
> Orquestado por `tinywasm/docs/ROUTER_CONFORMANCE_MASTER_PLAN.md` — **Fase C**.
> **Requiere la Fase A publicada**: `tinywasm/router` con el paquete `conformance`.

← [Etapa 2](PLAN_STAGE_2_FILES.md) | Índice → [PLAN.md](PLAN.md)

# Etapa 3 — `edge`: una ruta con permisos deja de ser un 403 eterno

Autocontenido, en español.

## El problema: la Etapa 2 no funciona en producción

La Etapa 2 dejó `files.Store.Mount` montando la subida así, y está **bien**:

```go
func (s *Store) Mount(r router.Router) {
	r.Put(s.prefix, s.upload).Requires("files", "write")   // subir exige permiso
	r.Get(s.prefix, s.serve).Public()                      // servir es público
}
```

Pero en `goflare/edge` **esa ruta responde 403 a todo el mundo, siempre**. No hay ningún
llamante —ni uno— que pueda subir un archivo. La tercera API del ecosistema es, hoy, papel.

La causa está en `edge/edge.go`, dentro de `Serve`:

```go
if bestMatch.info.Resource != "" && ctx.UserID() == "" {
	res.WriteHeader(403)                     // ...la verja corre AQUÍ
	return
}

h := bestMatch.h
for i := len(wr.middlewares) - 1; i >= 0; i-- {
	h = wr.middlewares[i](h)                 // ...y los middlewares AQUÍ, DESPUÉS
}
h(ctx)
```

**La verja se ejecuta antes que los middlewares.** `wasmContext.uid` arranca vacío y lo único
que lo escribe es `SetUserID`, que solo puede llamar un middleware — que corre **después** de
que la verja ya haya devuelto el 403. Es un callejón cerrado por construcción: no existe orden
de registro, ni middleware, ni truco que permita autenticar a nadie.

Y sus tests no lo vieron porque `tests/files_test.go` usa un `fakeCtx` cuyo `SetUserID` es un
método vacío, y llama a `store.upload` **directamente**, sin pasar por `Serve`. El fake mentía:
verde en los tests, 403 en Cloudflare.

## La causa raíz

`edge` **nunca recibió el tratamiento que `server/httpd` sí recibió**. Compara:

| | `server/httpd` | `goflare/edge` (hoy) |
|---|---|---|
| Modelo de acceso | `model.Access` tipado | `Public bool` + `Resource string`, a mano |
| ¿Quién llama? | `Config.Authn` (middleware de identidad) | **no existe** |
| ¿Puede? | `Config.Authorize model.Authorizer` | **no existe**; comparación inline |
| Guarded sin autorizador | **falla al arrancar** | deniega en silencio, para siempre |

`edge` no implementa el contrato: lo imita. Esta etapa lo alinea, y lo pone bajo el arnés de
conformidad para que no vuelva a divergir.

## Cambios

### 1. `edge.NewRouter()` pasa a tomar una `Config` — **ROMPE API**

Los dos asientos que faltan, con los **mismos nombres** que en `httpd` (misma cosa, mismo
nombre — un consumidor que sepa uno sabe el otro):

```go
// Config declares WHO the caller is and WHAT they may do. The library supplies the
// mechanism; the policy belongs to the app (see AUTH_POLICY_MASTER_PLAN).
type Config struct {
	// Authn establishes identity. It runs BEFORE the access gate — that ordering is
	// the whole point: a gate that runs first can never be satisfied. It reads the
	// request (cookie, header, token) and calls ctx.SetUserID. Anonymous is "" and
	// is a legal outcome, not an error.
	Authn router.Middleware

	// Authorize answers whether that identity holds a permission. nil DENIES:
	// the absence of an answer is not permission.
	Authorize model.Authorizer
}

func NewRouter(cfg Config) router.Router
```

`NewRouter()` sin argumentos **desaparece**. Un router sin `Config` es exactamente el estado
actual —el que produce el 403 eterno—, y dejarlo disponible es dejar la trampa puesta.
Una app sin autenticación pasa `edge.Config{}`: es legal, explícito, y sus rutas públicas
funcionan; lo que no podrá es montar una ruta con permisos sin decir quién autoriza, que es
justo lo que queremos que sea imposible.

### 2. `Serve`: la verja **después** de la identidad

El orden correcto, y el único que satisface el contrato:

```
Authn (establece identidad)  →  verja de acceso  →  middlewares  →  handler
```

`Authn` **no** es un middleware más de la lista: corre siempre, antes de la verja, para todas
las rutas —incluidas las públicas, que legítimamente quieren saber si hay alguien detrás—.
Los `Use` del consumidor siguen corriendo **detrás** de la verja: un 403 no debe ejecutar
lógica de negocio ni filtrar trabajo.

### 3. Adoptar `model.Access` y borrar el par `Public bool` / `Resource string`

`wasmRoute.info` es un `router.RouteInfo`, que **ya trae `Access model.Access`** desde la Fase
A de AUTH_POLICY. `edge` lo estaba ignorando y llevando sus propios campos en paralelo. Bórralos
y consume el tipado:

- `Public()` → `Access = model.AccessPublic`
- `Requires(r, a)` → `Access = model.AccessGuarded` + `Resource`/`Action` tipados
- `Authenticated()` → `Access = model.AccessAuthenticated`
- zero value → `model.AccessGuarded` sin recurso = **privado por defecto**

La decisión de la verja pasa a ser un `switch` sobre `Access`, y la autorización se delega:
`model.Allowed(cfg.Authorize, uid, res, act)` — que **deniega si `Authorize` es nil**.

Sube la dependencia a `tinywasm/router` con `conformance` (Fase A) y a `tinywasm/model` con
`Access`/`Authorizer` (v0.0.12+).

### 4. Contradicciones: fallar al arrancar

Como en `httpd` (`enforce.go`). En `Serve(r)`, antes de atender la primera petición, recorre
`r.Routes()` y **entra en pánico con un mensaje explícito** si:

- una ruta es `AccessGuarded` **con** recurso y `cfg.Authorize == nil` →
  `` `route PUT /api/files/ requires resource "files" but no Authorize is configured: it would deny every caller` ``
- una ruta es `AccessGuarded` o `AccessAuthenticated` y `cfg.Authn == nil` →
  `` `route PUT /api/files/ needs an identity but no Authn is configured: no caller can ever be authorized` ``

Es el aviso ruidoso que hoy no existe: **exactamente el fallo que nos ha costado esta etapa**,
convertido en un error de arranque que nadie puede ignorar. Un pánico al arrancar en el borde
sale por el log (la Etapa 2 ya recupera pánicos y los registra), y es infinitamente mejor que
un 403 silencioso en producción.

### 5. Demostrar conformidad — y arreglar el fake que mentía

Crea `tests/conformance_test.go`:

```go
func TestEdgeConformance(t *testing.T) {
	conformance.Run(t, conformance.Factory{New: newEdgeFactory, Verify: verifyStartup})
}
```

El `serve` de la factory **debe pasar por `Serve`**, con su `js.Global()` falso, no llamar al
handler a pelo. Y arregla `tests/files_test.go`: su `fakeCtx.SetUserID` es un método vacío
—por eso los tests de la Etapa 2 pasaban mientras la subida era un 403 en producción—. Que
guarde el id y que `UserID()` lo devuelva.

**Regla, y es la lección de esta etapa: un fake que no puede fallar no está probando nada.**

## Anti-footguns

- **No hagas pública la subida** (`files.Mount` → `.Public()`) "para que funcione". Sería
  convertir un bucket de escritura en un formulario abierto a internet. La subida **debe**
  exigir permiso; lo que faltaba era que el borde supiera quién llama.
- **No metas autenticación en `goflare`.** Ni sesiones, ni tokens, ni JWT. La política es del
  consumidor: `goflare` aporta los asientos (`Authn`, `Authorize`), la app decide quién sube.
- `r2/` sigue sin importar nada salvo `syscall/js` y `fmt`. Este plan no lo toca.

## Criterios de aceptación

- `gotest` pasa, **y los tests wasm de verdad se ejecutan** (`GOOS=js`): la Etapa 2 ya arregló
  los build tags de `tests/`; no los rompas.
- `TestEdgeConformance` pasa: los 16 casos, incluido
  `guarded_route_serves_authorized_identity` — el que hoy es imposible.
- Una ruta con `Requires` + identidad autorizada devuelve **200 y ejecuta el handler**.
- Los campos a mano han desaparecido:
  ```bash
  grep -rn "info.Public\|info.Resource == \"\"" edge/   # → vacío: se consume model.Access
  ```
- `NewRouter()` sin `Config` no existe:
  ```bash
  grep -rn "func NewRouter()" edge/                      # → vacío
  ```
- El fake ya no miente:
  ```bash
  grep -rn "func (c \*fakeCtx) SetUserID(id string)      {}" tests/   # → vacío
  ```
- Una ruta guarded sin `Authorize` **hace panic al arrancar** con el mensaje literal de arriba.
