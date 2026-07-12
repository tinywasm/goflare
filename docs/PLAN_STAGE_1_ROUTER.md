> Este plan se despacha vía el flujo CodeJob. Ver skill: agents-workflow.

Etapa 1 de 2 · Índice → [PLAN.md](PLAN.md) · Siguiente → [Etapa 2](PLAN_STAGE_2_FILES.md)

# Etapa 1 — `goflare` adopta el contrato de enrutado `tinywasm/router`

**Qué hace esta etapa, en una frase:** `goflare` deja de ser **dueño** de la abstracción de
enrutado y pasa a ser **un implementador** de ella.

Al terminar: no existe `goflare/router`, no existe `goflare/devserver` con router propio, el
paquete `pages/` se llama `edge/`, y la detección de modo falla ruidosamente en vez de
adivinar.

---

**Lee antes de tocar código:** [`AGENTS.md`](../AGENTS.md) en la raíz del repo — reglas del
arnés (los dos objetivos de compilación, `gotest` en vez de `go test`, fallo ruidoso) y
[`docs/TESTING.md`](TESTING.md) — cómo se prueba sin desplegar a Cloudflare.

## Contexto que necesitas (no busques fuera de este documento)

`goflare` es un runtime de despliegue para Cloudflare. Tiene dos objetivos de compilación:

- **`!wasm` (host):** herramientas que corren en la máquina del desarrollador — el pipeline
  de build, el cliente de la API de Cloudflare, el servidor de desarrollo.
- **`wasm` (borde):** el código que se despliega y corre en Cloudflare Workers.

### ⚠️ Anti-footgun: este repo SÍ usa la librería estándar

En el ecosistema `tinywasm` rige la regla "nada de stdlib en paquetes que compilan a WASM"
(se usa `tinywasm/fmt` en vez de `errors`/`strconv`/`strings`). **Esa regla NO se aplica a
los archivos `!wasm` de este repo.** `mode.go`, `build.go`, `config.go`, `cloudflare.go` y
`devserver/` son herramientas de host y usan `net/http`, `go/parser`, `os`, `strings`
legítimamente.

**NO "arregles" esos imports. NO los conviertas a `tinywasm/fmt`.** Solo los archivos con
`//go:build wasm` (`edge/`, `workers/`, `d1/`, `cloudflare/env_wasm.go`) siguen la regla
tinywasm.

### El contrato externo `github.com/tinywasm/router` (v0.1.5, ya publicado)

Esta es su forma **real y completa**. Impleméntala tal cual; no inventes métodos.

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
	SetValue(key string, v any)   // valores de ámbito petición
	Value(key string) any
	SetCookie(c Cookie)                // cookie isomórfica → cabecera Set-Cookie
	Cookie(name string) (Cookie, bool) // lee de la cabecera Cookie
	SetUserID(id string)               // identidad de ámbito petición ("" = anónimo)
	UserID() string                    // "" si no hay sesión válida
}
type HandlerFunc func(Context)

type Cookie struct {
	Name, Value, Path, Domain string
	MaxAge                    int
	Secure, HttpOnly          bool
	SameSite                  SameSite
}
type SameSite int
const (SameSiteDefault SameSite = iota; SameSiteLax; SameSiteStrict; SameSiteNone)

type Streamer interface { Context; Flush() }  // respuestas incrementales (SSE)
type StreamFunc func(Streamer)

type Socket interface {
	Read() ([]byte, error)
	Write(b []byte) error
	Close() error
}
type SocketFunc func(Socket)

type Middleware func(HandlerFunc) HandlerFunc

type Router interface {
	Get(path string, h HandlerFunc) Route
	Post(path string, h HandlerFunc) Route
	Put(path string, h HandlerFunc) Route
	Delete(path string, h HandlerFunc) Route
	Options(path string, h HandlerFunc) Route
	Handle(method, path string, h HandlerFunc) Route
	Stream(path string, h StreamFunc) Route
	Socket(path string, h SocketFunc) Route
	Use(m ...Middleware)
	Routes() []RouteInfo
}

type Route interface {
	Requires(resource, action string) Route // RBAC
	Public() Route                          // accesible sin identidad
}
type RouteInfo struct {
	Method, Path, Resource, Action string
	Public                         bool
}

type APIModule interface {
	ModelName() string   // de model.ModuleNaming
	MountAPI(r Router)
}
```

**Regla crítica del contrato — privado por defecto.** Una ruta que **no** llama `Public()`
**ni** `Requires()` responde **403** cuando `UserID()` está vacío, *aunque no haya
autorizador configurado*. Consecuencia para ti: cada ruta de desarrollo que registres debe
llamar `.Public()`, o el servidor rechazará todo.

### El implementador nativo `github.com/tinywasm/server/httpd` (ya publicado)

**`server/httpd` NO es un módulo:** es un paquete dentro del módulo
`github.com/tinywasm/server` (v0.2.25). En `go.mod` va `require github.com/tinywasm/server`;
el import es `github.com/tinywasm/server/httpd`. Arrastra el módulo del orquestador de
desarrollo entero (devtui, devwatch): **es un coste aceptado, no un error — no intentes
evitarlo.** No hay ciclo: `tinywasm/server` no depende de `goflare`.

Su API, a usar tal cual:

```go
// package httpd
type Config struct {
	Port      string // por defecto "8080"
	PublicDir string // "" = no servir estáticos
	Gzip      bool
	NoCache   bool   // el dev server lo quiere true
	Health    bool
	Logger    func(...any)

	Authn          router.Middleware                          // graba la identidad (ctx.SetUserID)
	Authorize      func(userID, resource, action string) bool // decide el RBAC
	RoutesEndpoint bool                                       // expone /_routes
	TLS            TLSConfig
}
func New(c Config) *Server
func (s *Server) Router() router.Router
func (s *Server) Mount(m ...router.APIModule) *Server
func (s *Server) Handler() (http.Handler, error)
func (s *Server) ListenAndServe() error
```

Ya trae: adaptador `net/http`→`router`, estáticos, gzip, no-cache, cookies, RBAC e
introspección. **No reimplementes nada de eso.**

---

## Paso 1 — Borrar el subpaquete `goflare/router`

Borra el archivo `router/router.go` y el directorio `router/`.

Contenía un **fork viejo** del contrato (sin cookies, sin `Route`, sin identidad). Ya no se
usa: el contrato vive en `github.com/tinywasm/router`.

**No dejes un alias ni un paquete que reexporte el externo.** Una sola forma de importar.

`go.mod`:
- `github.com/tinywasm/router` **ya está declarado como indirecta (v0.1.5)** — al importarlo
  pasa a directa sola. No la "añadas" a mano.
- Añade `github.com/tinywasm/server` (para `server/httpd`).

## Paso 2 — `devserver/` se reconstruye sobre `server/httpd`

Archivo: `devserver/devserver.go`.

**Borra** (no comentes, no dejes deprecados):

| Símbolo a borrar | Por qué |
|---|---|
| `nativeContext` | Lo aporta el adaptador de `server/httpd` |
| `nativeRouter` | Ídem |
| `NewRouter()` | Ídem |
| `ListenAndServe(addr, r, staticDir)` | Lo aporta `httpd.Server.ListenAndServe()` |
| `noCache()` | Lo cubre `httpd.Config.NoCache` |

**Reconstruye** el servidor de desarrollo sobre `httpd.New(httpd.Config{...})` con
`PublicDir` para estáticos, `Gzip`, `NoCache: true` y `RoutesEndpoint: true`, sirviendo con
`Server.ListenAndServe()`.

Lo específico de Cloudflare en dev (bindings D1, etc.) se monta como rutas sobre
`Server.Router()`, o como `router.APIModule` vía `Server.Mount(...)`. **Nunca** como un
adaptador HTTP duplicado.

**Toda ruta que registres aquí lleva `.Public()`** — ver "privado por defecto" arriba.

**Borra `devserver/devserver_test.go`.** No lo "actualices": sus dos tests
(`TestNoCacheSetsHeaders`, `TestRouterDispatchesRegisteredRoute`) prueban precisamente el
`noCache` y el `nativeRouter` que esta etapa **elimina**. Mueren con ellos.

En su lugar, escribe el test de reemplazo **en `tests/`** (`package goflare_test`, la
convención del repo): arrancar el devserver —que ahora construye un `httpd.Server`— y
comprobar que sirve un estático **y** una ruta registrada por contrato con `.Public()`.

## Paso 3 — Renombrar el paquete `pages/` → `edge/`

Mueve `pages/pages.go` → `edge/edge.go`. Cambia `package pages` → `package edge`.

El nombre `pages` describía *dónde se despliega* (un producto de Cloudflare), no *qué es*:
un implementador de enrutado. `workers/` **no es su hermano** — es la capa de bindings al
runtime JS (`Request`, `Response`, `Handle`), y `edge/` la importa. Son dos capas, no dos
alternativas.

**`workers/` no se toca en esta etapa.**

API resultante del paquete: `edge.NewRouter()`, `edge.Serve(r)`.

⚠️ **NO toques `tests/pages_test.go`.** Pese al nombre, **no prueba el paquete `pages/`**:
prueba `goflare.GeneratePagesFiles()`, la generación de artefactos estáticos de Pages en el
host. No tiene nada que ver con este rename. Déjalo como está.

Los tests **nuevos** del paquete `edge/` van en `tests/` (`package goflare_test`) — ver
[TESTING.md](TESTING.md).

## Paso 4 — `edge/` implementa el contrato completo

Los tipos `wasmContext`, `wasmRouter` y un nuevo `wasmRoute` deben satisfacer el contrato de
arriba sobre el runtime de borde:

- **Cookies:** `SetCookie` escribe la cabecera `Set-Cookie`; `Cookie(name)` parsea la
  cabecera `Cookie` de la petición. Nada de `net/http` — el borde es wasm.
- **Identidad:** `SetUserID` / `UserID` de ámbito petición. Más `SetValue` / `Value`.
- **Registro que devuelve `Route`:** `Get`/`Post`/… devuelven `router.Route`. `Requires` y
  `Public` graban en el `RouteInfo`. `Routes()` los enumera.
- **`Use(m ...Middleware)`:** encadena middlewares alrededor del handler.
- **Match por prefijo (subárbol), no solo igualdad exacta.** Hoy `wasmRouter` compara
  `rt.path == pathname` — igualdad literal. Eso hace **imposible** cualquier ruta dinámica:
  `/api/files/` nunca capturaría `/api/files/foto.jpg`. Regla nueva, **idéntica a la del
  implementador nativo** (`http.ServeMux`):

  | Patrón registrado | Qué captura |
  |---|---|
  | `/api/contacto` (sin barra final) | **solo** `/api/contacto` — igualdad exacta |
  | `/api/files/` (**con** barra final) | `/api/files/` y **todo lo que cuelgue**: `/api/files/foto.jpg`, `/api/files/a/b.png` |

  Ante varios patrones que casan, gana **el más específico** (el prefijo más largo), igual
  que `ServeMux`.

  `Context.Path()` sigue devolviendo la **ruta concreta de la petición**
  (`/api/files/foto.jpg`), nunca el patrón. Así el handler extrae la clave rebanando el
  prefijo, y **no hace falta añadir nada al contrato `tinywasm/router`** (no hay `Param()`
  ni lo habrá: se descartó por innecesario).

  En wasm, rebana con índices (`path[len(prefix):]`) — **no importes `strings`**.
- **`Stream` / `Socket`:** si el runtime de borde no los soporta, **falla con diagnóstico
  ruidoso** al registrarse — nunca en silencio, nunca devolviendo nil.
- **RBAC privado por defecto**, idéntico al nativo: `Public()` pasa siempre; `Requires()`
  consulta al autorizador del borde; sin marcador y sin identidad → **403**.

Añade las aserciones de contrato en tiempo de compilación:

```go
var _ router.Router  = (*wasmRouter)(nil)
var _ router.Context = (*wasmContext)(nil)
var _ router.Route   = (*wasmRoute)(nil)
```

## Paso 5 — Endurecer `inferMode`: el import no reconocido FALLA

Archivo: `mode.go`.

**El problema actual:** `inferMode` detecta el modo de build escaneando los imports de
`edge/main.go` con `strings.Contains` línea a línea. Si no reconoce ninguno, hace
`return ModeWorkers` ("legacy default"). Es decir: un import no migrado **no rompe la build —
produce el artefacto equivocado en silencio.**

**Reglas nuevas:**

| `edge/main.go` | Resultado |
|---|---|
| importa `github.com/tinywasm/goflare/edge` | `ModePagesFunctions` |
| importa `github.com/tinywasm/goflare/workers` | `ModeWorkers` |
| no existe, pero sí `PublicDir` | `ModePagesStatic` *(sin cambios: es convención de ficheros)* |
| existe, no importa **ninguno** de los dos | **error** |
| importa **los dos** | **error** (ambigüedad) |

- **Borra el `return ModeWorkers` final.** El modo workers ahora se **declara** importando
  `goflare/workers`; ya no se hereda por descarte.
- **Los valores de `Mode` NO cambian.** `ModePagesFunctions = "pages-functions"`,
  `ModeWorkers = "workers"`, `ModePagesStatic = "pages"` se quedan igual: nombran el
  *artefacto de Cloudflare*, no el paquete Go.
- **Detecta con `go/parser`, no con `strings.Contains`.** `mode.go` es `!wasm`, así que
  `go/parser` está disponible sin coste. El escaneo textual da falsos positivos (un import
  comentado matchea) y falsos negativos (un import con alias no matchea). Era tolerable
  cuando el no-match caía en un default; **es inaceptable ahora que el no-match rompe la
  build.**

Mensajes de error, **literales** (cópialos a constantes):

```go
const (
	ImportEdge    = "github.com/tinywasm/goflare/edge"
	ImportWorkers = "github.com/tinywasm/goflare/workers"

	ErrNoKnownImport = "cannot infer mode: edge/main.go imports neither " + ImportEdge + " (pages-functions) nor " + ImportWorkers + " (workers)"
	ErrAmbiguous     = "cannot infer mode: edge/main.go imports both " + ImportEdge + " and " + ImportWorkers + " — import exactly one"
)
```

Los import paths van en **constantes**, nunca como literales sueltos dentro de la lógica.

Actualiza los comentarios que documentan la inferencia en `mode.go` y en `build.go`
(cabecera de `Build()`), que hoy nombran `goflare/pages`.

## Paso 6 — Tests de `inferMode` (hoy no existe ninguno)

`inferMode` **no tiene un solo test**. Escríbelos, uno por fila de la tabla del paso 5 —
tres modos y dos errores. Los casos de error comprueban el mensaje literal.

Añade además un caso con el import **comentado** (`// import "…/edge"`) que debe dar error,
no `ModePagesFunctions`: es lo que prueba que usaste `go/parser` y no texto plano.

⚠️ **Dónde va este test.** La convención del repo es que **los tests viven en `tests/`**, como
`package goflare_test`. **Pero `inferMode` es minúscula (no exportada)**, así que desde
`tests/` no se puede llamar: no compilaría. Este test es la excepción — va **junto al código**,
como `mode_internal_test.go`, en `package goflare`, igual que el `build_internal_test.go` que
ya existe.

Todos los demás tests de esta etapa (router, cookies, RBAC, match de rutas) **sí** van en
`tests/`: ejercitan API pública. Ver [TESTING.md](TESTING.md).

## Paso 7 — Compilar los dos objetivos y pasar los tests

```bash
go build ./...                       # host: devserver sobre server/httpd
GOOS=js GOARCH=wasm go build ./...   # borde: edge/
gotest                               # NUNCA `go test` — ver más abajo
```

## Cómo se prueba esto (no lo improvises)

**Lee [TESTING.md](TESTING.md) antes de escribir un test.** Define los tres niveles y la
regla para elegir uno. Resumen operativo:

- **`gotest`, nunca `go test`.** Compila los dos objetivos y levanta un navegador real para
  el lado WASM.
- El código del borde habla con **`js.Global()`, no con Cloudflare**. Para probarlo se
  inyecta un `context.env` falso desde Go y se ejercita el camino real. **No se despliega
  nada, no se usa wrangler.**
- Todo lo que esta etapa toca —match de rutas, cookies, RBAC, `inferMode`— **falla por bugs
  nuestros**, así que es Nivel 1 (nativo) o Nivel 2 (navegador). **Ninguno de estos tests
  toca Cloudflare.**

---

## Reglas de calidad (obligatorias)

- **Sin strings repetidos en la lógica.** Todo import path, clave o prefijo repetido va a una
  constante con nombre (ver paso 5). Prohibidos los literales sueltos.
- **`cmd/` delgado.** `cmd/goflare/main.go` solo parsea argumentos, inyecta dependencias e
  imprime/sale. Toda condicional o validación es una función exportada de la librería.
- **Nada de stdlib en los paquetes `wasm`** (`edge/`, `workers/`, `d1/`): usa
  `tinywasm/fmt`. **En los `!wasm` la stdlib es correcta** — ver el anti-footgun de arriba.
- **Borra de verdad.** Nada de mantener el camino viejo "por compatibilidad" detrás de un
  flag o un alias. Si el plan dice borrar, se borra.

## Criterios de aceptación (verificables)

```bash
# 1. goflare ya no exporta enrutado ni conserva el fork
grep -rn "github.com/tinywasm/goflare/router" .   # → vacío
test ! -d router                                  # → el directorio no existe

# 2. No queda implementador nativo propio
grep -rn "nativeRouter\|nativeContext" .          # → vacío

# 3. El rename es total
grep -rn "goflare/pages" .                        # → vacío  (imports, strings y comentarios)
test ! -d pages                                   # → el directorio no existe

# 4. No hay adivinanza silenciosa de modo
grep -rn "return ModeWorkers" mode.go             # → solo dentro del caso que SÍ detecta el import

# 5. Los dos objetivos compilan y los tests pasan
go build ./... && GOOS=js GOARCH=wasm go build ./...
gotest
```

Además, deben pasar estos tests de comportamiento:

- **Contrato en el borde:** las tres aserciones `var _ router.X = (*wasmY)(nil)` compilan.
- **Cookie ida y vuelta** sobre `wasmContext`.
- **Anotación de ruta:** `r.Post("/x", h).Requires("res","write")` aparece en `Routes()` como
  `RouteInfo{Method:"POST", Path:"/x", Resource:"res", Action:"write"}`; y
  `r.Get("/y", h).Public()` como `RouteInfo{Public:true}`.
- **Privado por defecto en el borde:** una ruta sin `Public()` ni `Requires()` responde
  **403** ante una petición sin identidad.
- **Match por prefijo en el borde:** una ruta registrada como `/api/files/` recibe la
  petición `/api/files/foto.jpg`, y dentro del handler `ctx.Path()` devuelve
  `/api/files/foto.jpg` (la ruta concreta, no el patrón). Una registrada como
  `/api/contacto` **no** recibe `/api/contacto/extra`. Este test **falla hoy**: el matcher
  actual es de igualdad exacta.
- **Capacidad no soportada:** registrar un `Stream`/`Socket` que el runtime no soporta falla
  ruidosamente.
- **`inferMode`:** los cinco casos del paso 5, más el del import comentado.

Para los dobles de prueba existe `github.com/tinywasm/router/mock` (`mock.Router`,
`mock.Context`, `mock.Route`). Úsalo en vez de improvisar dobles a mano.

---

## Consecuencia fuera de este repo (no es tu trabajo, pero debes saberlo)

El repo `goflare-demo` importa el fork borrado y el paquete `pages/`. **Va a romperse, y es
lo esperado** — se migra en su propio plan, después de publicar esta etapa. Tú no lo tocas.

Referencia (opcional): https://github.com/tinywasm/goflare-demo/blob/main/docs/PLAN.md
