# PLAN — Pages Functions (Go) support

> Objetivo: que `goflare` permita desarrollar proyectos Go para **Cloudflare Pages Functions** con un flujo simple: el dev escribe Go (sobre `tinywasm/*`, no stdlib pesada — D12), corre `goflare build` localmente, **commitea los artefactos compilados** (`functions/[[path]].mjs` + `functions/edge.wasm`), y al hacer `git push` Cloudflare detecta el cambio vía Git Integration y despliega lo que está en el repo, **sin tokens distribuidos a devs ni pipelines de CI**. El modo del proyecto se **infiere de los imports de `edge/main.go`** (D11) — `edge/main.go` sigue siendo el único entrypoint, igual que en Workers; el contenido (qué API de goflare importa) decide el modo.

## 1. Contexto y motivación

Hoy `goflare` soporta dos modos:

- **Pages estático** (`PUBLIC_DIR=web/public`) — assets sin lógica edge.
- **Workers** (`ENTRY=edge`) — un solo `edge/main.go` desplegado como Worker. Usa una API propia `workers.Handle(w, r)` con `Request/Response` custom.

Cloudflare ofrece un tercer modelo, **Pages Functions**: archivos JS dentro de un directorio `functions/` que actúan como endpoints serverless integrados con el sitio Pages. Convención de Cloudflare:

- `functions/api/hello.js` → endpoint en `/api/hello`.
- `functions/[[path]].js` → catch-all que recibe todas las rutas (lo que vamos a usar).
- Cada archivo exporta `onRequest(context)` o variantes por método (`onRequestPost`, `onRequestGet`).
- `context.env` da acceso a env vars y secrets configurados en el dashboard de Cloudflare.
- `context.request` es un `Request` estándar de Fetch API.

Es el modelo más común para sitios estáticos con un backend mínimo (forms, webhooks, proxy a APIs externas). Hoy se escribe en JS/TS; este plan lo automatiza para Go.

`goflare` debe automatizar este flujo para que el dev escriba **solo Go**:

```
my-project/                  ← raíz del proyecto = raíz del repo git
├── .env                     # credenciales y metadata (sin MODE — se infiere de imports, D11)
├── .env.example
├── routes/
│   └── routes.go            # Build-agnóstico — func Register(r router.Router) (aggregator de URLs)
├── modules/
│   └── contact/
│       ├── model.go         # Build-agnóstico — modelo + Validate()
│       └── handler.go       # Build-agnóstico — func Handle(w, r)
├── web/                     # FRONTEND — propiedad del dev / framework tinywasm
│   ├── client.go            # //go:build wasm   — fuente del frontend (dev)
│   ├── server.go            # //go:build !wasm  — dev local, llama routes.Register (dev)
│   └── public/              # ← build output directory (config one-time en CF)
│       ├── index.html       #   dev (o stub del framework tinywasm)
│       ├── client.wasm      #   producido por el framework tinywasm al compilar web/client.go
│       ├── script.js        #   producido por el framework tinywasm (assetmin)
│       └── style.css        #   producido por el framework tinywasm (assetmin)
├── edge/
│   └── main.go              # //go:build wasm — ÚNICO entrypoint (mismo que Workers)
│                            #   Su contenido (qué importa) decide el modo (D11)
└── functions/               # EDGE FUNCTIONS — ÚNICOS artefactos generados por goflare
    ├── [[path]].mjs         # ← generado por goflare build (bundle JS pegamento)
    └── edge.wasm            # ← generado por goflare build (TinyGo binary de edge/main.go)
```

> **Responsabilidad de cada directorio**:
> - **`web/public/`** lo construye el **framework tinywasm** (compilador de `web/client.go`, assetmin). goflare **no** lo produce; solo lo despliega tal cual.
> - **`functions/`** es el **único output propio de goflare**: el bundle JS + el WASM compilado del edge.
>
> Ambos directorios deben commitearse porque CF Git Integration despliega lo que está en el repo (D8). Pero conceptualmente vienen de orígenes distintos.

**Importante**:

- **Artefactos compilados se commitean.** No hay CI/CD que los regenere — lo que está en el repo es lo que se despliega. Beneficios: el dev ve el tamaño del binario en `git diff` *antes* del push, no hay tokens en máquinas ni en GitHub Secrets, cero pipeline que falle.
- **`functions/` y `web/public/` son hermanos en la raíz** del repo. Cloudflare Git Integration despliega el contenido del repo tal cual: `web/public/` como estáticos (configurado one-time en el dashboard como build output directory), y `functions/` se detecta automáticamente por convención.
- **Único entrypoint `edge/main.go`** — el mismo nombre que usa hoy el modo Workers. Lo que cambia entre modo Workers y Pages-Functions es qué API de goflare importa el dev (`workers.Handle` vs `pages.Serve(r)`); de ese import `goflare build` infiere el modo (D11) y empaqueta la salida correspondiente. No se ensucia el repo con `pages/`.

**Clave del diseño**: la lógica de los endpoints vive en `routes/` + `modules/*/` **sin build tags**. Tanto `edge/main.go` (edge, wasm) como `web/server.go` (dev local, nativo) importan el mismo `routes.Register(mux)`. Una sola fuente de verdad para las rutas; cero drift dev↔prod; tests con `httptest` sin tocar wasm.

## 2. Decisiones tomadas (gates resueltos)

| # | Decisión | Justificación |
|---|---|---|
| D1 | **Single WASM + router interno** (no file-based routing por endpoint) | Un solo binario, menor overhead de cold start, mapea 1:1 con catch-all `[[path]]` de Cloudflare. |
| D2 | **Compilación: solo TinyGo** | Binarios chicos (kB), Cloudflare tiene límites de 1 MiB sin paid plan. Go stdlib produce binarios de 2-10 MB. |
| D3 | **Layout de salida: `functions/[[path]].mjs` + `functions/edge.wasm`** — exactamente 2 archivos | Catch-all en raíz captura todas las rutas. El handler Go enruta internamente. Máxima simplicidad. |
| D4 | **API edge: tipo `workers.Handle` + router custom, todo sobre `tinywasm/*`** (no `net/http`, no stdlib pesada) | Restricción crítica de tamaño binario (D12): stdlib `net/http` arrastra `fmt`/`strings`/`errors`/`encoding/*`/`bufio`/`crypto/*` ⇒ binario +80% y excede límite de 1 MiB de Cloudflare Free. Además TinyGo **no soporta net/http completo** en target `js/wasm`. API custom mantiene el binario chico y se alinea con el ecosistema tinywasm. |
| D4b | **Patrón `routes.Register(r router.Router)`** sobre **interfaz mínima compartida** | Preserva "una sola fuente de verdad para las rutas" entre edge (wasm) y dev local. La interfaz `router.Router` es pequeña (Get/Post/etc.) y se implementa distinto en cada target: en wasm el router de `tinywasm/goflare` (sin stdlib pesada); en dev local un adapter envuelve `http.ServeMux` (stdlib OK ahí porque no se despliega). Tests con un mock de `Context` trivial. |
| D4c | **Output WASM se llama `edge.wasm`** (consistente con el modo Workers actual) | El nombre `edge` ya es la convención de goflare para artefactos de edge. Evita inventar nombres nuevos como `app.wasm`. |
| D12 | **Restricción crítica de tamaño**: código en `//go:build wasm` NO puede importar stdlib pesada (`fmt`, `strings`, `errors`, `encoding/*`, `net/http`, `log`, etc.). Solo `syscall/js`, `bytes`, primitivas mínimas, y `tinywasm/*` (`tinywasm/fmt`, `tinywasm/json`, `tinywasm/strings`, `tinywasm/fetch`, etc.). | Stdlib infla el binario ~80% y excede 1 MiB. Aplica también al código existente en `workers/` — `workers/request.go:6` importa `"fmt"` y debe ser refactorizado como parte de Fase 1. Cada PR que toque código wasm debe verificar imports antes de mergear. |
| D5 | **Reutilizar el bridging JS↔Go ya existente en [workers/](../workers/)**. Solo agregar un adapter `net/http` encima. | El paquete `workers/` ya hace el trabajo duro: lee JS Request, construye JS Response, registra `binding.handleRequest` vía `syscall/js`. El mismo `binding` lo invocan tanto `fetch` (Workers) como `onRequest` (Pages) en [assets/worker.mjs](../assets/worker.mjs) — sirve para ambos modos sin cambios. No hay código externo a migrar; todo el bridging vive en este repo. |
| D6 | **Alcance MVP**: routing + acceso a `context.env` + fetch outbound. Validado con un demo end-to-end en `goflare-demo` (ver su PLAN.md). | Cualquier alcance menor no permite reemplazar un endpoint real tipo `contacto.js`. |
| D7 | **Cloudflare Git Integration es el flujo de despliegue recomendado.** Conexión one-time vía dashboard de Cloudflare; cada `git push` despliega automáticamente. | Sin tokens en local, sin tokens en GH Secrets, sin pipeline de CI que mantener. La GitHub App de Cloudflare se instala una sola vez por repo (acto manual en el dashboard de CF) y a partir de ahí cualquier dev con push access despliega con `git push`. Modelo más simple posible — el repo *es* el estado desplegado. |
| D8 | **Commitear los artefactos generados** (`functions/[[path]].mjs`, `functions/edge.wasm`, `web/public/client.wasm`, `web/public/script.js`, `web/public/style.css`). | Es lo que habilita D7: CF Git Integration despliega lo commiteado tal cual, sin build step en infra de CF (no necesita TinyGo). Beneficio adicional: el dev ve el **tamaño del binario en `git diff`** antes de pushear — si crece demasiado lo detecta inmediatamente. Trade-off aceptado: historial de git crece con cada cambio del binario (con TinyGo el binario es <1 MiB, manejable; si crece mucho a futuro, `git lfs`). |
| D9 | **`goflare deploy` (Direct Upload v2): código presente en el repo pero NO verificado.** Estado actual: existen archivos (`build.go`, `cloudflare.go`, etc.) pero el flujo end-to-end nunca se ejecutó. Tratar como **placeholder a validar**, no como funcionalidad garantizada. | No usarlo como fallback "seguro" en docs ni en demos hasta validarlo. Si en algún punto del desarrollo del MVP se necesita desplegar sin CF Git Integration, agendar una tarea explícita: "validar `goflare deploy` end-to-end". Si tras la validación funciona, queda como fallback genuino; si no funciona y nadie lo necesita aún, postergarlo. Decisión binaria, sin pretender que existe algo que no se sabe si está roto. |
| D10 | **Único entrypoint: `edge/main.go`** (mismo nombre que en modo Workers). No se crea `pages/main.go`. | Reduce ruido en el repo: un solo lugar para el código del edge, sin importar el target. El contenido de `edge/main.go` varía según `MODE`: con `workers.Handle(fn)` para Workers, con `pages.Serve(mux)` para Pages Functions. goflare elige cómo empaquetar la salida según `MODE`. |
| D11 | **Mode del proyecto se infiere de los `import` de `edge/main.go`.** Sin variable `MODE` en `.env`. | El código es la fuente de verdad. Si `edge/main.go` importa `tinywasm/goflare/pages` → modo `pages-functions`; si importa `tinywasm/goflare/workers` → modo `workers`; si no existe `edge/main.go` pero sí `web/public/` → modo `pages` (estático). Sin riesgo de desync entre `.env` y el código real. El flag `--mode` de `goflare init` sigue existiendo, pero ahora **scaffoldea el `edge/main.go` con los imports correctos** en vez de escribir a `.env`. |

## 3. Estado actual del código (auditoría rápida)

✅ **Reutilizable sin cambios:**
- [javascripts.go](../javascripts.go) — `generateWorkerFile()` ya bundlea y minifica `wasm_exec.js + runtime.mjs + worker.mjs` en un solo `.js`.
- [assets/worker.mjs:53-58](../assets/worker.mjs#L53-L58) — ya exporta `onRequest` para Pages.
- [assets/runtime.mjs](../assets/runtime.mjs) — `createRuntimeContext({ env, ctx, binding })` ya expone `env` al WASM.

🟡 **Necesita adaptación:**
- `generateWorkerFile()` siempre escribe a `OutputDir/edge.js`. Para Pages Functions hace falta una variante que escriba el bundle a `functions/[[path]].mjs` (el `import mod from "./edge.wasm"` se mantiene igual, ya que el `.wasm` queda al lado del `.mjs` en `functions/`).
- [build.go](../build.go) / [pages.go](../pages.go) / [workers.go](../workers.go) — bifurcación de modos: debe detectar el **tercer modo** Pages-Functions.
- [config.go](../config.go) — agregar campo de configuración (ver §5).

✅ **Ya existe el bridging JS↔Go (estructura reutilizable):**
- [workers/workers.go](../workers/workers.go) — registra `binding.handleRequest` vía `syscall/js`. **El mismo binding sirve para Workers (`fetch`) y Pages (`onRequest`)** porque [worker.mjs](../assets/worker.mjs) llama a `binding.handleRequest` en ambos casos.
- [workers/request.go](../workers/request.go) — lee método/URL/headers/body desde la JS Request.
- [workers/response.go](../workers/response.go) — construye la JS Response desde el buffer Go.

🟥 **Deuda crítica a fixear ANTES de avanzar (D12):**
- [workers/request.go:6](../workers/request.go#L6) importa stdlib `"fmt"` — viola la restricción de tamaño. Refactorizar a `tinywasm/fmt` o eliminar el uso.
- Auditar todos los archivos en [workers/](../workers/) buscando imports de stdlib pesada (`fmt`, `strings`, `errors`, `encoding/*`, etc.) y reemplazar por `tinywasm/*`.
- Comando de verificación durante CI: `grep -rE '^\s*"(fmt|strings|errors|encoding|net/http|log|io/ioutil)"' workers/ edge/ cloudflare/` debe devolver vacío.

❌ **Falta crear (encima de lo existente):**
- **`router/`** — paquete build-agnóstico con la interfaz `Context` y `Router`. Solo tipos y firmas; cero stdlib. Consumido por `routes/routes.go` (cableado de URL→handler) y por todos los `modules/*/handler.go`.
- **`pages/pages.go`** — implementación wasm del `Router`. Internamente reutiliza el `binding.handleRequest` de `workers/`. Despacha por path/method matching mínimo (sin regex, solo prefijos y matching exacto). Construido sobre `tinywasm/*`, cero stdlib pesada.
- **`pages/devserver/`** (solo `//go:build !wasm`) — implementación nativa del `Router` que envuelve `http.ServeMux`. Usada por `web/server.go`. Stdlib OK acá porque no se despliega.
- **`cloudflare/env_wasm.go` + `cloudflare/env_native.go`** — acceso dual-target a `context.env` (wasm, vía `syscall/js`) y `os.Getenv` (dev local). En el lado wasm: cero stdlib pesada.
- **Refactor de `workers/`** (D12): reemplazar stdlib por `tinywasm/*`. Aprovechar para extraer primitivas comunes compartidas por `workers/` y `pages/`.

## 4. Diseño técnico

### 4.1 Selección de modo (inferida del código)

`goflare build` inspecciona el proyecto en este orden (primera coincidencia gana):

| Señal en el proyecto | Modo inferido | Output |
|---|---|---|
| `edge/main.go` importa `github.com/tinywasm/goflare/pages` | `pages-functions` | `functions/[[path]].mjs` + `functions/edge.wasm` + assets de `web/public/` |
| `edge/main.go` importa `github.com/tinywasm/goflare/workers` | `workers` | `.build/edge.js` + `.build/edge.wasm` |
| No existe `edge/main.go` pero existe `web/public/` | `pages` (estático) | Solo estáticos + `web/public/client.wasm` (si hay `web/client.go`) |
| Ninguna de las anteriores | error con sugerencia de `goflare init` | — |

**Por qué inferir del código y no de `.env`:**

- **Source of truth**: el código siempre dice la verdad; `.env` se puede desincronizar al refactorizar imports.
- **Zero-config**: el dev no escribe `MODE=...` en ningún lado. Cambia el import, listo.
- **Confirmación post-build**: tras `goflare build`, la presencia de `functions/edge.wasm` confirma el modo. Validación cruzada gratis.

El flag `--mode=` de `goflare init` sigue existiendo pero **scaffoldea `edge/main.go` con los imports adecuados** — no escribe a `.env`.

### 4.2 Pipeline de build (modo `pages-functions`)

```
1. Detecta modo inspeccionando imports de edge/main.go (D11). Confirma que es pages-functions.
2. Verifica que PUBLIC_DIR existe y tiene los assets del frontend
   (el framework tinywasm ya los produjo: client.wasm, script.js, style.css, index.html).
   goflare NO los regenera — son input, no output.
3. Compila edge/main.go con tinygo → functions/edge.wasm   ← output goflare
4. Bundlea JS pegamento → functions/[[path]].mjs            ← output goflare
   - Reutiliza generateWorkerFile() parametrizado:
     - destino: functions/[[path]].mjs
     - import del wasm: "./edge.wasm"
     - export: const onRequest (no default { fetch })
```

**División de responsabilidades en el build**:

- El **framework tinywasm** (`tinywasm/dom`, assetmin, tinygo, etc.) construye `web/public/*` desde `web/client.go` y los sources del frontend. Eso pasa **antes** de `goflare build` (o en otro paso del Makefile/Taskfile del dev).
- `goflare build` solo produce `functions/[[path]].mjs` + `functions/edge.wasm`. Asume que `web/public/` ya está listo y lo trata como input opaco a desplegar.

Outputs van **al árbol del repo**, sin staging tipo `.build/dist/`, para que `git status` los muestre y se commiteen (D8).

### 4.3 API Go expuesta al dev — patrón `routes.Register` sobre interfaz custom

> **Naming**: el paquete se llama `routes/` (no `api/`, no `handlers/`, no `endpoints/`) porque su responsabilidad literal es registrar **rutas** URL→handler. `routes.Register(r)` describe la acción exacta. Survives growth: cuando agregues `sdk/`, `internal/`, `services/`, sigue siendo inequívoco.

**`router/router.go`** (build-agnóstico, sin stdlib pesada — solo tipos):

```go
package router

// Context es la abstracción mínima que ve un handler.
// Misma firma en wasm (edge) y en dev local (nativo).
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
    Handle(method, path string, h HandlerFunc) // catch-all
}
```

**`routes/routes.go`** (build-agnóstico — recibe `router.Router`, no sabe nada de net/http ni de wasm; aggregator central de URLs):

```go
package routes

import (
    "github.com/tinywasm/goflare/router"
    "github.com/tinywasm/goflare-demo/modules/contact"
)

func Register(r router.Router) {
    r.Post("/api/contacto", contact.Handle)
    r.Options("/api/contacto", contact.Handle) // CORS preflight
}
```

**`edge/main.go`** (edge, wasm — mismo path que en modo Workers, contenido distinto):

```go
//go:build wasm

package main

import (
    "github.com/tinywasm/goflare/pages"   // Router impl wasm sobre tinywasm/*
    "github.com/tinywasm/goflare-demo/routes"
)

func main() {
    r := pages.NewRouter()
    routes.Register(r)
    pages.Serve(r) // bloquea, registra binding.handleRequest
}
```

**`web/server.go`** (dev local, nativo — la misma `routes.Register`):

```go
//go:build !wasm

package main

import (
    "github.com/tinywasm/goflare/pages/devserver" // adapter sobre net/http (stdlib OK acá)
    "github.com/tinywasm/goflare-demo/routes"
)

func main() {
    r := devserver.NewRouter()
    routes.Register(r)
    devserver.ListenAndServe(":8080", r, "web/public")
}
```

**Reglas del patrón:**

- `router/`, `routes/` y `modules/*/handler.go` **nunca** importan stdlib pesada. Solo `router.Context`/`router.Router` y, donde haga falta, `tinywasm/fmt`, `tinywasm/json`, etc.
- `cloudflare.Env(key)` con dual-target (D11): `env_wasm.go` lee de `syscall/js`, `env_native.go` de `os.Getenv`. Mismo API pública.
- Tests: mock trivial de `router.Context` permite `go test ./routes/... ./modules/...` sin levantar servidor ni compilar wasm.
- El dev local **sí** puede usar stdlib libremente en `web/server.go` y `devserver/` — esos no se despliegan al edge.

**Escalabilidad del patrón** (informativo, no parte del MVP):

- **Patrón A (recomendado para MVP)**: aggregator central. `routes/routes.go` lista todas las rutas en un solo archivo. Bueno para 1-10 endpoints; el mapa de URLs del proyecto está concentrado y es fácil de auditar.
- **Patrón B (a futuro)**: registro distribuido. Cada `modules/<feature>/routes.go` expone su propio `Register(r router.Router)`. `edge/main.go` los llama en cadena (`contact.Register(r); newsletter.Register(r); ...`). Migrar de A→B es trivial — sin reescribir handlers.

### 4.4 Bundle de salida (`functions/[[path]].mjs`)

Conceptualmente idéntico al `edge.js` actual, pero:

- Ruta de destino: `functions/[[path]].mjs` (catch-all de Cloudflare Pages) en vez de `OutputDir/edge.js`.
- Import del WASM: `import mod from "./edge.wasm";` — sin cambios, ya que el binario queda al lado del bundle dentro de `functions/`.
- Export principal: `export const onRequest = ...` (Pages necesita `onRequest`, no `default { fetch }`).
- Sigue siendo **un solo archivo** tras minificación.

### 4.5 Acceso a `context.env` desde Go (dual-target)

`cloudflare/` expone la misma API en wasm (edge) y en nativo (dev local) usando split files:

- `cloudflare/env_wasm.go`  → `//go:build wasm` — lee de `runtimeContext.env` vía `syscall/js`. El runtime ya expone `env` en [assets/runtime.mjs:9-15](../assets/runtime.mjs#L9-L15).
- `cloudflare/env_native.go` → `//go:build !wasm` — fallback a `os.Getenv`. Permite que `web/server.go` y los tests carguen secrets desde un `.env` local sin diferencias de código.

API pública idéntica en ambos targets:

```go
func Env(key string) string                 // env var o secret
func EnvOr(key, fallback string) string
func Lookup(key string) (string, bool)
```

Resultado: `contact.Handle` puede llamar `cloudflare.Env("RESEND_API_KEY")` sin saber si está corriendo en el edge o en local — la misma línea funciona en ambos.

### 4.6 CLI y flujo de trabajo del dev

#### Selección de modo: inferida de los imports (D11)

`goflare build` lee `edge/main.go` y decide el modo según qué paquete de goflare importe. `.env` no contiene `MODE`. Si el dev cambia el import, el modo cambia automáticamente — cero desync.

El flag `--mode=` de `goflare init` sigue existiendo pero se usa solo para **scaffoldear** `edge/main.go` con los imports correctos al crear el proyecto.

#### Subcomando: `goflare init --mode=<mode>`

```bash
goflare init --mode=pages-functions
# Modos (determinan qué imports scaffoldea edge/main.go):
#   pages           — solo sitio estático (frontend WASM opcional)
#   workers         — solo Worker standalone
#   pages-functions — Pages estático + Functions Go (ESTE PLAN)   ← NUEVO
```

Sin flag → interactivo:

```bash
$ goflare init
? Project name: my-app
? Cloudflare account ID: ...
? Project mode:
  > pages-functions  (recommended for static sites with a Go API)
    workers          (single edge Worker)
    pages            (static-only site)
? Frontend WASM (web/client.go)? (Y/n)
```

#### Estructura scaffoldeada por `init --mode=pages-functions`

goflare **crea** automáticamente los directorios y archivos esqueleto. El dev no necesita acordarse de qué carpeta hace qué:

```
my-app/
├── .env                       ← PROJECT_NAME, CLOUDFLARE_ACCOUNT_ID  (sin MODE — D11)
├── .env.example
├── .gitignore                 ← ignora .env y .build/ (NO ignora functions/ ni web/public/*.wasm)
├── go.mod
├── routes/routes.go                 ← stub con 1 ruta de ejemplo
├── modules/hello/
│   ├── model.go
│   └── handler.go             ← stub net/http
├── web/
│   ├── client.go              ← stub frontend WASM
│   ├── server.go              ← dev server pre-configurado con routes.Register
│   └── public/index.html      ← stub HTML
└── edge/main.go               ← entrypoint trivial (routes.Register + pages.Serve)
```

> **No** scaffoldea `functions/` — esa carpeta aparece tras el primer `goflare build`. **Sí** debe commitearse después de cada build (no va en `.gitignore`).

#### Comandos durante desarrollo

| Comando | Qué hace | Notas |
|---|---|---|
| `goflare init --mode=pages-functions` | Crea estructura del proyecto; scaffoldea `edge/main.go` con los imports correctos para el modo. | Una sola vez. |
| `goflare dev` *(nuevo, opcional)* | Wrapper de `go run ./web` que también detecta cambios y recompila el frontend wasm. | Atajo; el dev también puede correr `go run ./web` directo. |
| `goflare build` | Infiere modo de los imports de `edge/main.go` (D11), compila TinyGo y genera `functions/[[path]].mjs` + `functions/edge.wasm`. **No** toca `web/public/` — eso es producido aparte por el framework tinywasm. | Idempotente. Tras build, hacer `git add functions/ web/public/ && git commit && git push` → CF despliega. |
| `goflare auth` | Guarda token CF en keyring. | Solo necesario si usás `goflare deploy` (estado NO verificado, D9). |
| `goflare deploy` | **NO verificado (D9)**. Comando presente en el código pero el flujo end-to-end nunca se ejecutó. No incluir en docs públicas hasta validar manualmente. | Flujo soportado del MVP es `git push` vía CF Git Integration. |

#### Configuración one-time en Cloudflare (manual, dashboard)

1. Crear proyecto Pages en `dash.cloudflare.com`.
2. **Connect to Git** → elegir el repo de GitHub. Esto instala la GitHub App de Cloudflare en el repo (autorización OAuth interactiva — no scriptable).
3. **Production branch**: `main` (o el que uses).
4. **Build command**: dejar **vacío** (no buildear en CF — el repo trae los artefactos compilados).
5. **Build output directory**: `web/public`.
6. (Opcional) Custom domain.
7. (Opcional) Environment variables / secrets: agregar acá las claves que el edge necesita en runtime (ej. `RESEND_API_KEY`). El runtime las expone vía `context.env` → `cloudflare.Env("RESEND_API_KEY")` en Go.

Tras este setup, **nadie necesita tokens en local ni GH Secrets**. Cada `git push` a la rama configurada dispara un deploy automático de CF.

**Iteración futura** (fuera del MVP): `goflare init` podría invocar la Cloudflare API para crear el proyecto Pages programáticamente y setear build output directory. La instalación de la GitHub App sigue siendo manual (OAuth interactivo) — no es totalmente automatizable.

#### Flujo end-to-end del dev

Setup inicial (una sola vez por proyecto, por el dueño del repo):

```
1. goflare init --mode=pages-functions
2. git init && git add . && git remote add origin <github-url> && git push -u
3. (dashboard CF) Connect to Git → seleccionar repo → build command vacío → output dir web/public
4. (dashboard CF) Settings → Environment variables → agregar secrets que el edge use
```

Día a día:

```
1. editar routes/routes.go, modules/<x>/handler.go, web/client.go
2. go run ./web              ← prueba local en :8080 (API + estáticos)
3. goflare build             ← regenera SOLO functions/[[path]].mjs + functions/edge.wasm
                                (web/public/* se regenera con el flujo del framework tinywasm)
4. git status                ← verás los binarios cambiados; tamaño visible
5. git add . && git commit && git push   ← CF detecta push y despliega
```

Sumar un dev nuevo:

```
1. git clone
2. go run ./web              ← funciona inmediatamente, sin secrets en local
3. (editar, build, commit, push)
```

Sin distribuir tokens. Sin configurar Secrets. Sin nada.

## 5. Cambios en configuración (`.env`)

`.env` ya no contiene `MODE` (D11: se infiere de los imports). Solo credenciales y metadata:

```bash
# Identificadores del proyecto (no secretos)
PROJECT_NAME=my-app
CLOUDFLARE_ACCOUNT_ID=...

# Opcionales (con defaults sensatos)
PUBLIC_DIR=web/public             # default: web/public
FUNCTIONS_DIR=functions           # default: functions
DOMAIN=example.com                # solo si querés que goflare configure dominio
```

**Secretos: NUNCA en `.env`.** El `CLOUDFLARE_API_TOKEN` se guarda en el **keyring del sistema** vía `goflare auth` ([store.go](../store.go) ya implementa esto). Razones:

- `.env` se puede commitear por error.
- `.env` queda en backups, shell history, snapshots de disco.
- El keyring del SO está cifrado y protegido por la sesión de usuario.

`goflare auth` corre una sola vez por máquina; el token queda persistido y los demás comandos lo leen del keyring transparentemente. **Cero secretos en disco plano.**

`.env` va al `.gitignore` igual (contiene `CLOUDFLARE_ACCOUNT_ID` que aunque no es secreto, no aporta nada commitear).

## 6. Despliegue: Cloudflare Git Integration

### Flujo recomendado: `git push` y CF despliega

CF Pages se conecta al repo de GitHub una sola vez vía dashboard (instala la GitHub App de Cloudflare). Configurás:

- **Build command**: vacío (no se buildea en CF).
- **Build output directory**: `web/public`.
- (Opcional) Environment variables para runtime.

A partir de ahí, **cada `git push` despliega**. Lo que está commiteado es lo que se despliega — sin transformaciones, sin pipeline, sin tokens.

### Por qué este modelo

1. **Cero tokens distribuidos.** Ningún dev necesita `CLOUDFLARE_API_TOKEN` — ni en keyring local, ni en `.env`, ni en GH Secrets. La única autorización es la GitHub App de CF instalada por el dueño del repo.
2. **Sumar devs es trivial.** Clonan, editan, commitean, pushean. Cero secrets, cero configuración local de credenciales.
3. **Tamaño del binario visible antes del push.** El dev ve `functions/edge.wasm | Bin +47KB` en `git diff --stat` y decide si commitea. Con CI, ese feedback llega minutos después.
4. **El repo *es* el estado desplegado.** "¿Qué está en producción?" = `git show main:functions/edge.wasm`. Sin desync entre código fuente y artefactos.
5. **Sin pipeline que fallar.** No hay workflow.yml que mantener, ni rate limits de Actions, ni "el deploy está esperando un runner". CF clona el repo y sirve.

### Trade-off principal: artefactos en git

Se acepta porque:

- TinyGo genera `<1 MiB` por binario; el crecimiento del repo es manejable durante años.
- Si crece demasiado a futuro, `git lfs` cubre el caso sin cambiar el flujo.
- El feedback inmediato del tamaño en `git diff` es un beneficio, no una molestia.

Convención recomendada para el equipo: **siempre correr `goflare build` antes de `git commit`** cuando se toca código de `edge/`, `routes/`, `modules/` o `web/`. Una pre-commit hook trivial lo automatiza si querés (opcional, no parte del MVP):

```bash
#!/bin/sh
goflare build || exit 1
git add functions/ web/public/
```

### Fallback potencial: `goflare deploy` (Direct Upload v2)

**Estado: NO verificado** (D9). El código existe en el repo (`build.go`, `cloudflare.go`, …) pero el flujo end-to-end nunca se ejecutó. No prometer este comando en README ni en demos hasta validarlo manualmente con un proyecto de prueba real.

Si en algún punto se necesita (probable durante el desarrollo de goflare mismo, para iterar sin commitear binarios al demo), abrir una tarea explícita: "validar `goflare deploy` end-to-end contra un proyecto Pages real". Tras esa validación, decidir: dejarlo documentado como fallback genuino, o quitarlo si nadie lo necesita y mantener funcionalidad no usada cuesta más que borrarla.

El **único flujo documentado y soportado** del MVP es CF Git Integration + `git push`.

### GitHub Actions: alternativa, no recomendada

Existe `docs/CI_GITHUB_ACTIONS.md` con templates de workflows para quien necesite Actions específicamente (gates de tests bloqueantes, preview deploys por PR, auditoría exhaustiva por SHA). **Para el flujo estándar no aporta nada que Git Integration no haga más simple**. Mantenido como referencia para casos avanzados.

## 7. Roadmap de implementación

### Fase 1 — Runtime Go (encima del bridging existente)
- [ ] **Fixear deuda D12 en `workers/`**: auditar imports, reemplazar `"fmt"` y cualquier otra stdlib pesada por `tinywasm/*`. Confirmar con `grep -rE '^\s*"(fmt|strings|errors|encoding|net/http|log|io/ioutil)"' workers/` → vacío.
- [ ] Crear paquete `router/` build-agnóstico con `Context` y `Router` (interfaces puras, cero stdlib).
- [ ] Crear paquete `pages/` (wasm): `NewRouter()` + `Serve(r Router)`. Router custom con path matching mínimo, construido sobre primitivas de `workers/` + `tinywasm/*`.
- [ ] Crear paquete `pages/devserver/` (`//go:build !wasm`): adapter del `router.Router` sobre `http.ServeMux` + `ListenAndServe(addr, r, staticDir)`.
- [ ] Crear `cloudflare/env_wasm.go` + `cloudflare/env_native.go` con API `Env/EnvOr/Lookup` dual-target. El lado wasm: cero stdlib pesada.
- [ ] Tests: mock de `router.Context` para `go test ./router/... ./pages/...` sin wasm. Tests del fallback nativo de `cloudflare.Env` con `go test ./cloudflare/...`.

### Fase 2 — Build pipeline
- [ ] Inferir modo en `build.go` inspeccionando imports de `edge/main.go` (D11). Error claro si no se puede determinar.
- [ ] Parametrizar `generateWorkerFile()` con `(destPath, wasmImportName, exportShape)`.
- [ ] Adaptar `assets/worker.mjs` o agregar `assets/pages.mjs` que solo exporta `onRequest`.
- [ ] Compilar `edge/main.go` (mismo path que modo Workers; D10) con tinygo cuando el modo inferido es `pages-functions`.
- [ ] `goflare build` produce `functions/[[path]].mjs` + `functions/edge.wasm` directamente en el árbol del repo (sin staging `.build/dist/`).
- [ ] Actualizar `init` para scaffoldear `edge/main.go` con los imports correctos del modo elegido (no escribe `MODE=` a `.env`); ajustar `.gitignore` para **NO** ignorar `functions/` ni `web/public/*.wasm` (D8).

### Fase 3 — Deploy
- [ ] **Validar `goflare deploy` end-to-end** (D9): estado actual NO verificado. Solo si se decide priorizar — la Fase 3 puede saltarse y dejarse para post-MVP si CF Git Integration cubre el caso de uso del demo. Si se valida: confirmar que sube `functions/` y `web/public/` correctamente.
- [ ] Documentar setup one-time de CF Git Integration en `docs/BUILD_PAGES_FUNCTIONS.md`.
- [ ] Verificar end-to-end: `git push` a una rama conectada → CF despliega → endpoint responde.

### Fase 4 — Documentación
- [ ] Nuevo `docs/BUILD_PAGES_FUNCTIONS.md`.
- [ ] Actualizar `README.md` con el tercer modo.
- [ ] Actualizar `docs/QUICK_REFERENCE.md` y `docs/ARCHITECTURE.md`.

### Fase 5 — Validación E2E
- [ ] `goflare-demo` migrado a Pages Functions — validación E2E ejecutada por el dev en local (fuera del scope de este repo).

## 8. Riesgos y preguntas abiertas

1. **Límite de tamaño 1 MiB** en Cloudflare Free para `.wasm`. D12 (solo `tinywasm/*`, no stdlib pesada) hace que se cumpla por construcción. Validación continua: D8 (commit del binario) hace que el dev vea el tamaño en `git diff --stat` antes de cada push — alerta temprana si algún import escapó la regla.
2. **Disciplina de imports**: si un dev agrega accidentalmente `"fmt"` en un archivo wasm, el binario crece de golpe. Mitigación: pre-commit hook opcional con el grep de D12, o CI gate cuando se implemente.
3. **HTTP outbound desde WASM**: Go stdlib `net/http` no funciona en `GOOS=js` (y aunque funcionara, viola D12). Hace falta `tinywasm/fetch` que invoque el `fetch` global del runtime JS. ¿Existe ya? Si no, abrir issue paralelo.
3. **Streaming / WebSockets**: fuera del MVP. Documentarlo como "not yet supported".
4. **Crecimiento del repo por binarios commiteados** (D8): a largo plazo, el historial de `functions/edge.wasm` puede pesar. Mitigación: `git lfs` si llega a doler. No bloqueante para MVP.
5. **Compat con `workers.Handle` actual**: mantener intacto. `pages.Serve(http.Handler)` es API nueva, no reemplaza la anterior. Por D10, ambas viven en `edge/main.go` según el `MODE` configurado.
6. **Connect-to-Git en CF requiere acción manual del dueño del repo** (OAuth interactivo, no scriptable). Es una sola vez por proyecto; aceptado.

## 9. Out of scope (explícito)

- File-based routing (un archivo Go por endpoint). Descartado en D1.
- Migración de sitios existentes en JS/TS a Go — el plan agrega la capacidad; cada proyecto decide si y cuándo migrar.
- Soporte para Go (stdlib) además de TinyGo. Descartado en D2.
- Durable Objects, D1, R2, KV. Fuera del MVP (no necesarios para reemplazar un `contacto.js`).
