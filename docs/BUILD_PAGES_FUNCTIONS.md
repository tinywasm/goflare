# Building Cloudflare Pages Functions (Go)

GoFlare's third mode produces **Cloudflare Pages Functions** in Go — a static Pages site plus a single edge function written in Go (compiled to WASM via TinyGo). The dev writes pure Go; goflare produces exactly two artifacts under `functions/`: a `.mjs` glue bundle and the WASM binary.

See also: [BUILD_PAGES.md](BUILD_PAGES.md) (static-only) · [BUILD_WORKERS.md](BUILD_WORKERS.md) (standalone Workers).

## When to use this mode

Static site + a small backend (forms, webhooks, proxy to external APIs) written in Go. Single binary catch-all router — simpler than file-based routing per endpoint.

## Project layout

```
my-project/                  ← raíz del proyecto = raíz del repo git
├── .env                     # credenciales y metadata (sin MODE — se infiere de imports)
├── .env.example
├── routes/
│   └── routes.go            # Build-agnóstico — func Register(r router.Router)
├── modules/
│   └── contact/
│       ├── model.go         # Build-agnóstico — modelo + Validate()
│       └── handler.go       # Build-agnóstico — func Handle(ctx router.Context)
├── web/                     # FRONTEND — propiedad del dev / framework tinywasm
│   ├── client.go            # //go:build wasm
│   ├── server.go            # //go:build !wasm — dev local
│   └── public/              # ← build output directory (configurado en CF dashboard)
│       ├── index.html
│       ├── client.wasm      # producido por el framework tinywasm
│       ├── script.js        # producido por el framework tinywasm (assetmin)
│       └── style.css        # producido por el framework tinywasm (assetmin)
├── edge/
│   └── main.go              # //go:build wasm — ÚNICO entrypoint
│                            #   El import (tinywasm/goflare/pages) decide el modo
└── functions/               # ÚNICOS artefactos generados por goflare — COMMITEADOS
    ├── [[path]].mjs         # bundle JS pegamento (catch-all de CF Pages)
    └── edge.wasm            # binario TinyGo de edge/main.go
```

**Responsabilidad de cada directorio**:

- **`web/public/`** lo construye el framework tinywasm (compilador de `web/client.go`, assetmin). goflare **no** lo produce; solo lo despliega tal cual.
- **`functions/`** es el **único output propio de goflare**: el bundle JS + el WASM compilado del edge.

Ambos directorios deben commitearse porque CF Git Integration despliega lo que está en el repo.

## Mode detection (no MODE variable)

`goflare build` infiere el modo inspeccionando los `import` de `edge/main.go`:

| Import en `edge/main.go` | Modo | Output |
|---|---|---|
| `github.com/tinywasm/goflare/pages` | `pages-functions` | `functions/[[path]].mjs` + `functions/edge.wasm` |
| `github.com/tinywasm/goflare/workers` | `workers` | `.build/edge.js` + `.build/edge.wasm` (legacy) |
| (no `edge/main.go`, sí `web/public/`) | `pages` (estático) | solo estáticos |

El código es la fuente de verdad. Cambiar el import del entrypoint cambia el modo — sin tocar `.env` ni flags.

## API expuesta al dev

### `router/router.go` (build-agnóstico, sin stdlib pesada)

```go
package router

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

### `routes/routes.go` (aggregator de URLs, build-agnóstico)

```go
package routes

import (
    "github.com/tinywasm/goflare/router"
    "github.com/your-project/modules/contact"
)

func Register(r router.Router) {
    r.Post("/api/contacto", contact.Handle)
    r.Options("/api/contacto", contact.Handle) // CORS preflight
}
```

### `edge/main.go` (edge, wasm)

```go
//go:build wasm

package main

import (
    "github.com/tinywasm/goflare/pages"   // ← este import dispara MODE=pages-functions
    "github.com/your-project/routes"
)

func main() {
    r := pages.NewRouter()
    routes.Register(r)
    pages.Serve(r) // bloquea, registra binding.handleRequest
}
```

### `web/server.go` (dev local — invoca el mismo `routes.Register`)

```go
//go:build !wasm

package main

import (
    "github.com/tinywasm/goflare/pages/devserver" // adapter sobre net/http (stdlib OK)
    "github.com/your-project/routes"
)

func main() {
    r := devserver.NewRouter()
    routes.Register(r)
    devserver.ListenAndServe(":8080", r, "web/public")
}
```

**Misma URL, mismo handler, mismo `routes.Register`** — solo cambia qué implementación de `router.Router` se inyecta.

## Restricción crítica: solo `tinywasm/*` en código wasm

Código con `//go:build wasm` **NO** puede importar stdlib pesada (`fmt`, `strings`, `errors`, `encoding/*`, `net/http`, `log`, `io/ioutil`). Solo `syscall/js`, `bytes`, primitivas mínimas, y `tinywasm/*` (`tinywasm/fmt`, `tinywasm/json`, `tinywasm/strings`, `tinywasm/fetch`, etc.).

**Razón**: stdlib infla el binario ~80% y excede el límite de 1 MiB de Cloudflare Free. TinyGo además no soporta `net/http` completo en target `js/wasm`.

Aplica a `edge/`, `routes/`, `modules/*/handler.go`, `cloudflare/env_wasm.go`, `pages/pages.go`, `workers/`. El dev local **sí** puede usar stdlib libremente en `web/server.go` y `pages/devserver/` — esos no se despliegan al edge.

**Verificación** (pre-commit / CI):

```bash
grep -rE '^\s*"(fmt|strings|errors|encoding|net/http|log|io/ioutil)"' \
    workers/ pages/pages.go router/ cloudflare/env_wasm.go edge/ routes/ modules/
# Debe devolver vacío
```

## Acceso a `context.env` desde Go (dual-target)

`cloudflare/` expone la misma API en wasm (edge) y en nativo (dev local):

- `cloudflare/env_wasm.go` (`//go:build wasm`) — lee de `runtimeContext.env` vía `syscall/js`.
- `cloudflare/env_native.go` (`//go:build !wasm`) — fallback a `os.Getenv`.

```go
func Env(key string) string
func EnvOr(key, fallback string) string
func Lookup(key string) (string, bool)
```

Un handler puede llamar `cloudflare.Env("RESEND_API_KEY")` sin saber si corre en el edge o en local — la misma línea funciona en ambos.

## Build pipeline (modo `pages-functions`)

```
1. Detecta modo inspeccionando imports de edge/main.go
2. Verifica que web/public/ existe (lo produjo el framework tinywasm previamente)
3. Compila edge/main.go con TinyGo → functions/edge.wasm
4. Bundlea JS pegamento → functions/[[path]].mjs
   - import del wasm: "./edge.wasm"
   - export: const onRequest (no default { fetch })
```

Outputs van **al árbol del repo**, sin staging tipo `.build/dist/`, para que `git status` los muestre y se commiteen.

`goflare build` no toca `web/public/`. El frontend lo construye el framework tinywasm en un paso aparte (Makefile/Taskfile del dev).

## CLI

```bash
goflare init --mode=pages-functions   # scaffoldea edge/main.go con imports correctos
goflare build                          # produce functions/[[path]].mjs + functions/edge.wasm
git add functions/ web/public/         # commitear artefactos compilados
git commit && git push                 # CF Git Integration despliega
```

## Setup one-time en Cloudflare (manual)

1. Crear proyecto Pages en `dash.cloudflare.com`.
2. **Connect to Git** → elegir el repo de GitHub. Instala la GitHub App de Cloudflare (OAuth interactivo).
3. **Production branch**: `main`.
4. **Build command**: dejar **vacío** (no buildear en CF — el repo trae los artefactos).
5. **Build output directory**: `web/public`.
6. (Opcional) Custom domain.
7. (Opcional) Environment variables: agregar acá las claves que el edge usa en runtime (ej. `RESEND_API_KEY`). El runtime las expone via `context.env` → `cloudflare.Env(...)`.

Tras este setup, **nadie necesita tokens en local ni en GH Secrets**. Cada `git push` a la rama configurada dispara un deploy automático.

## Por qué CF Git Integration + artefactos commiteados

1. **Cero tokens distribuidos.** Ningún dev necesita `CLOUDFLARE_API_TOKEN`. La única autorización es la GitHub App de CF instalada por el dueño del repo.
2. **Sumar devs es trivial.** Clonan, editan, commitean, pushean. Cero secrets, cero configuración local.
3. **Tamaño del binario visible antes del push.** El dev ve `functions/edge.wasm | Bin +47KB` en `git diff --stat` y decide si commitea.
4. **El repo *es* el estado desplegado.** Sin desync entre código fuente y artefactos.
5. **Sin pipeline que fallar.** No hay workflow.yml que mantener, ni rate limits.

**Trade-off aceptado**: el historial git crece con cada cambio del binario. Con TinyGo el binario es <1 MiB, manejable; si crece a futuro, `git lfs` cubre el caso sin cambiar el flujo.

**Convención recomendada**: correr `goflare build` antes de `git commit` cuando se toca código de `edge/`, `routes/`, `modules/` o `web/`. Pre-commit hook opcional:

```bash
#!/bin/sh
goflare build || exit 1
git add functions/ web/public/
```

## Secretos: NUNCA en `.env`

`CLOUDFLARE_API_TOKEN` se guarda en el **keyring del sistema** vía `goflare auth` (ver [SECRETS_PLAN.md](SECRETS_PLAN.md)). `.env` solo contiene identificadores no-secretos (`PROJECT_NAME`, `CLOUDFLARE_ACCOUNT_ID`, `PUBLIC_DIR`, `FUNCTIONS_DIR`, `DOMAIN`).

Razones:
- `.env` se puede commitear por error.
- `.env` queda en backups, shell history, snapshots de disco.
- El keyring del SO está cifrado y protegido por la sesión de usuario.

Con CF Git Integration el token no se necesita en ningún lado (D7).

## Patrón de escalabilidad

- **MVP**: aggregator central. `routes/routes.go` lista todas las rutas en un solo archivo. Bueno para 1-10 endpoints; mapa de URLs concentrado y fácil de auditar.
- **A futuro**: registro distribuido. Cada `modules/<feature>/routes.go` expone su propio `Register(r router.Router)`. `edge/main.go` los llama en cadena. Migrar es trivial — sin reescribir handlers.

## Fallback no recomendado: `goflare deploy` (Direct Upload v2)

**Estado: NO verificado.** El comando existe en el código pero el flujo end-to-end nunca se validó. No incluir en tutoriales hasta confirmar manualmente con un proyecto real. El flujo soportado del MVP es `git push` vía CF Git Integration.
