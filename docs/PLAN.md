# Plan — Refactor de `goflare` al contrato de enrutado `tinywasm/router`

> `goflare` es hoy el **origen** de la abstracción de enrutado (define `Context`,
> `Router`, `HandlerFunc` en su subpaquete `router`). El refactor la **extrae** a la
> pieza de lego independiente `github.com/tinywasm/router` y deja a `goflare` como lo
> que debe ser: **un implementador** de ese contrato, no su dueño. Autocontenido, en
> español.

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
}
type HandlerFunc func(Context)
type Router interface {
	Get(path string, h HandlerFunc)
	Post(path string, h HandlerFunc)
	Put(path string, h HandlerFunc)
	Delete(path string, h HandlerFunc)
	Options(path string, h HandlerFunc)
	Handle(method, path string, h HandlerFunc)
	// Extensiones: streaming, WebSocket y middleware
	Stream(path string, h StreamFunc)   // StreamFunc func(Streamer)
	Socket(path string, h SocketFunc)   // SocketFunc func(Socket)
	Use(m ...Middleware)                 // Middleware func(HandlerFunc) HandlerFunc
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

1. **Extraer el contrato.** Eliminar el subpaquete `goflare/router`. Su contenido se
   convierte en la librería `github.com/tinywasm/router` (pieza aparte, con su propio
   PLAN). `goflare` deja de exportar cualquier abstracción de enrutado.
2. **`devserver` pasa a consumir el contrato externo.** Cambiar el import de
   `goflare/router` a `github.com/tinywasm/router`. `nativeRouter` sigue implementando
   `router.Router`; se le añaden las nuevas capacidades del contrato extendido:
   - `Stream` → sobre `http.Flusher` del `ResponseWriter` nativo.
   - `Socket` → upgrade WebSocket nativo, entregando un `router.Socket`.
   - `Use` → composición de `router.Middleware` alrededor de los handlers.
3. **`pages` (edge/wasm) igual.** Cambiar el import y añadir las mismas capacidades
   sobre el runtime de borde (streaming/WS según lo que el runtime soporte; lo no
   soportado debe fallar con diagnóstico ruidoso, nunca en silencio).
4. **Nada de reexportar.** `goflare` no deja un alias `goflare/router` que apunte al
   externo — una sola forma: los consumidores importan `github.com/tinywasm/router`.

---

## Pasos de implementación

1. Publicar `github.com/tinywasm/router` con el contrato (ver su propio PLAN).
2. Borrar `goflare/router`; añadir dependencia a `github.com/tinywasm/router` en
   `go.mod`.
3. Reapuntar imports en `devserver` y `pages`.
4. Implementar `Stream`/`Socket`/`Use` en `nativeRouter` y en `wasmRouter`.
5. `go build ./...` nativo y `GOOS=js GOARCH=wasm go build ./...` (el edge).

---

## Estrategia de pruebas y criterios de aceptación

- **`goflare` no exporta enrutado:** no queda ningún `type Router`/`Context` en el
  código de `goflare`; una búsqueda no debe encontrarlos.
- **Dos implementadores, un contrato:** tests de compilación
  `var _ router.Router = (*nativeRouter)(nil)` y `var _ router.Router =
  (*wasmRouter)(nil)`.
- **Paridad de capacidades:** un test de streaming (SSE) contra el `devserver`
  nativo entrega y hace `Flush`; el runtime edge que no soporte una capacidad falla
  ruidosamente, no en silencio.
- **Doble objetivo:** `devserver` compila nativo; `pages` compila
  `GOOS=js GOARCH=wasm`. Ningún módulo ni handler cambia al migrar entre ambos.
