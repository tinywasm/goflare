> Este plan se despacha vía el flujo CodeJob. Ver skill: agents-workflow.
> Orquestado por `tinywasm/docs/ROUTER_ADAPTER_MASTER_PLAN.md` — **Fase 3 (propagación)**.
> Depende de la **Fase 0** (`tinywasm/router` con cookies + `Route`/`Requires` + `Routes()`)
> y de la **Fase 1** (`tinywasm/serverd`) ya publicadas.

# Plan — Refactor de `goflare` al contrato de enrutado `tinywasm/router`

> `goflare` es hoy el **origen** de la abstracción de enrutado (define `Context`,
> `Router`, `HandlerFunc` en su subpaquete `router`). El refactor la **extrae** a la
> pieza de lego independiente `github.com/tinywasm/router` y deja a `goflare` como lo
> que debe ser: **un implementador** de ese contrato, no su dueño. En concreto, tras el
> refactor `goflare` conserva **solo su implementador único: el edge/wasm (`pages`,
> Cloudflare Workers)**; el lado nativo de desarrollo (`devserver`) deja de tener su
> propio `nativeRouter` y **reutiliza `github.com/tinywasm/serverd`** (el único
> implementador nativo del ecosistema). Autocontenido, en español.

---

## Reglas de Desarrollo

Las reglas del arnés viven en el **`AGENTS.md` de la raíz de esta librería** — léelo
antes de cualquier cambio. Este PLAN no las repite: describe solo el *cómo*.

Alcance de la pieza (responsabilidad única): `goflare` es un runtime de despliegue
(servidor de desarrollo nativo + runtime edge/wasm). Su trabajo es **implementar** el
contrato de enrutado y servir; **no** definir ese contrato ni la lógica de los
módulos.

---

## El contrato (vive fuera, se reexpresa aquí para ser autocontenido)

La abstracción de enrutado deja de vivir en `goflare` y pasa a
`github.com/tinywasm/router`. Su forma:

```go
// package router (github.com/tinywasm/router)
type Context interface {
	Method() string
	Path() string
	Body() []byte
	GetHeader(key string) string
	SetHeader(key, value string)
	WriteStatus(code int)
	Write(b []byte) (int, error)
	SetValue(key string, v any)
	Value(key string) any
	// Cookies isomórficas (Fase 0): en edge se mapean a cabecera Set-Cookie / Cookie.
	SetCookie(c Cookie)
	Cookie(name string) (Cookie, bool)
}
type HandlerFunc func(Context)

// Cookie / SameSite: tipos isomórficos del contrato (sin net/http).
type Cookie struct {
	Name, Value, Path, Domain string
	MaxAge                    int
	Secure, HttpOnly          bool
	SameSite                  SameSite
}
type SameSite int
const (SameSiteDefault SameSite = iota; SameSiteLax; SameSiteStrict; SameSiteNone)

type Router interface {
	// El registro DEVUELVE Route (Fase 0) para anotar la ruta.
	Get(path string, h HandlerFunc) Route
	Post(path string, h HandlerFunc) Route
	Put(path string, h HandlerFunc) Route
	Delete(path string, h HandlerFunc) Route
	Options(path string, h HandlerFunc) Route
	Handle(method, path string, h HandlerFunc) Route
	Stream(path string, h StreamFunc) Route   // StreamFunc func(Streamer)
	Socket(path string, h SocketFunc) Route   // SocketFunc func(Socket)
	Use(m ...Middleware)                       // Middleware func(HandlerFunc) HandlerFunc
	Routes() []RouteInfo                       // introspección
}

// Route anota una ruta; RouteInfo es su vista de solo lectura. Solo RBAC — NO hay
// rate limit en el contrato (es concern de edge/gateway).
type Route interface {
	Requires(resource, action string) Route // RBAC: action string (= user.Permission.Action)
}
type RouteInfo struct {
	Method, Path, Resource, Action string
}
```

---

## Estado de partida

`goflare` define el contrato y ya tiene **dos implementadores** de él:

- `router/router.go` — la interfaz `Router` + `Context` + `HandlerFunc` (**el
  contrato**, que se va a extraer).
- `devserver/devserver.go` — implementador **nativo**: `nativeRouter{ mux
  *http.ServeMux }`, `NewRouter() router.Router`, `ListenAndServe(addr, r
  router.Router, staticDir)`.
- `pages/pages.go` — implementador **edge/wasm**: `wasmContext`, `wasmRouter`, que
  satisfacen el mismo `router.Router` sobre el runtime de borde.

Que ya existan dos implementadores del mismo contrato es justo la prueba de que la
abstracción debe ser una pieza independiente, no propiedad de `goflare`.

---

## Cambios

1. **Extraer el contrato.** Eliminar el subpaquete `goflare/router`. Su contenido ya vive
   en `github.com/tinywasm/router` (pieza aparte, con su propio PLAN). `goflare` deja de
   exportar cualquier abstracción de enrutado.
2. **Lado nativo: `devserver` reutiliza `serverd`, no un router propio.** En coherencia
   con la decisión del maestro ("serverd = único implementador nativo"), **borrar**
   `nativeRouter`, `NewRouter` y el `ListenAndServe` propios de `devserver.go`. El
   servidor de desarrollo nativo pasa a construirse sobre `github.com/tinywasm/serverd`
   (que ya trae el adaptador `net/http`→`router`, estáticos, gzip, cookies, RBAC e
   introspección). Cualquier comportamiento específico de Cloudflare en dev (bindings D1,
   etc.) se monta como rutas/middleware sobre el `router.Router` de serverd, no como un
   adaptador HTTP duplicado.
3. **Lado edge/wasm: `pages` implementa el contrato NUEVO completo.** `wasmContext` y
   `wasmRouter` son el implementador **único** que serverd no puede cubrir (serverd es
   `!wasm`). Deben satisfacer el contrato de la Fase 0 sobre el runtime de borde:
   - Cookies: `SetCookie`/`Cookie` mapeadas a las cabeceras `Set-Cookie`/`Cookie` del
     runtime (no `net/http`).
   - Registro **devuelve `Route`**; `Requires` graba el `RouteInfo`; `Routes()` los enumera.
   - `Stream`/`Socket`/`Use` según lo que el runtime soporte; lo no soportado **falla con
     diagnóstico ruidoso**, nunca en silencio.
   - RBAC: el edge aplica `Requires` con su propio autorizador/identidad (equivalente al
     `Identify`/`Authorizer` de serverd, con los mecanismos del borde).
4. **Nada de reexportar.** `goflare` no deja un alias `goflare/router` que apunte al
   externo — una sola forma: los consumidores importan `github.com/tinywasm/router`.

---

## Pasos de implementación

1. (Prerrequisito) `github.com/tinywasm/router` (Fase 0) y `github.com/tinywasm/serverd`
   (Fase 1) ya publicados.
2. Borrar el subpaquete `goflare/router`. `go.mod`: añadir `github.com/tinywasm/router` y
   `github.com/tinywasm/serverd`.
3. **Nativo:** borrar `nativeRouter`/`NewRouter`/`ListenAndServe` de `devserver.go`;
   reconstruir el servidor de desarrollo sobre `serverd` (estáticos, gzip, cookies, RBAC,
   `/_routes`). El dev específico de Cloudflare se monta como rutas sobre su `Router`.
4. **Edge:** reapuntar imports en `pages`; hacer que `wasmContext`/`wasmRouter`
   implementen el contrato de la Fase 0 — cookies vía cabeceras, registro que devuelve
   `Route` grabando `RouteInfo`, `Routes()`, y `Stream`/`Socket`/`Use`.
5. `go build ./...` nativo (devserver sobre serverd) y `GOOS=js GOARCH=wasm go build ./...`
   (el edge `pages`).

---

## Estrategia de pruebas y criterios de aceptación

- **`goflare` no exporta enrutado:** no queda ningún `type Router`/`Context` en el
  código de `goflare`; una búsqueda no debe encontrarlos.
- **Un solo implementador propio (el edge):** `var _ router.Router = (*wasmRouter)(nil)`,
  `var _ router.Context = (*wasmContext)(nil)`, `var _ router.Route = (*wasmRoute)(nil)`.
  El nativo **ya no es de goflare** — lo aporta `serverd`; no debe quedar ningún
  `nativeRouter` en el repo.
- **Contrato nuevo cubierto en el edge:** ida/vuelta de cookie sobre `wasmContext`;
  `r.Post("/x", h).Requires("res","write")` aparece en `Routes()` como
  `RouteInfo{Resource:"res", Action:"write"}`; una capacidad no soportada por el runtime
  falla ruidosamente, no en silencio.
- **Nativo sobre serverd:** un test arranca el `devserver` (que ahora construye un
  `serverd.Server`) y sirve estáticos + una ruta registrada por contrato.
- **Doble objetivo:** `devserver` compila nativo (vía serverd); `pages` compila
  `GOOS=js GOARCH=wasm`. Ningún módulo ni handler cambia al migrar entre ambos.
