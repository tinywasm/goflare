# GoFlare — Plan de Implementación

> Este documento es ejecutable por un agente externo.
> Cada stage incluye el código exacto a escribir o modificar.

---

## Estado actual

| Componente | Estado |
|-----------|--------|
| CLI `init / build / deploy` | ✅ |
| `buildPages()` — copia `PUBLIC_DIR` a `.goflare/dist/` | ✅ |
| `buildWorker()` — compila WASM con `tinywasm/client` | ✅ |
| `DeployPages` — Direct Upload v2 | ✅ |
| `DeployWorker` — multipart PUT | ✅ |
| Worker JS template | ⚠️ no es ES module — ver Stage A |
| `buildPages()` compila `web/client.go` antes de copiar | ❌ — ver Stage B |
| `tinywasm/workers` (reemplaza syumai/workers) | ❌ — ver Stage C |
| Demo para `gonew` | ❌ — ver Stage D |

---

## Convención de archivos (fija para todos los proyectos goflare)

```
proyecto/
├── .env                        # credenciales — gitignored
├── .env.example                # plantilla pública
├── web/
│   ├── client.go               # //go:build wasm — frontend WASM (tinywasm/dom + tinywasm/form)
│   └── public/                 # PUBLIC_DIR en .env
│       ├── index.html
│       ├── script.js           # generado por goflare build
│       └── client.wasm         # generado por goflare build
└── worker/
    └── main.go                 # //go:build wasm — Worker backend (tinywasm/workers)
```

**Reglas fijas — no configurables:**
- Frontend WASM: siempre `web/client.go`
- Assets estáticos: siempre `web/public/`
- Worker backend: siempre `worker/main.go`
- Config: siempre `.env`

`.env` mínimo:
```
PROJECT_NAME=mi-proyecto
CLOUDFLARE_ACCOUNT_ID=<account-id>
PUBLIC_DIR=web/public
```
`ENTRY` no es necesario cuando existe `worker/main.go` — goflare lo detecta automáticamente.

---

## Inconsistencias corregidas en este plan

| # | Severidad | Problema | Corrección aplicada |
|---|-----------|----------|---------------------|
| 1 | 🔴 | `twFront.Change(mode)` dispara compilación TinyGo inmediata en `New()` | Usar `twFront.SetMode(mode)` — Stage B |
| 2 | 🔴 | `js.FuncOf` en `readBodyText` nunca llama `.Release()` — memory leak por request | Liberar refs tras resolver canal — Stage C |
| 3 | 🔴 | `worker/main.go` usa `fmt.Errorf` sin import; plan muestra dos enfoques ambiguos | Eliminar nota; usar `tinywasm/fmt` + `fmt.Errf` — Stage D |
| 4 | 🔴 | `dom.SetInnerHTML` no existe en tinywasm/dom | Reemplazar por `dom.Render("result", dom.P(...))` — Stage D |
| 5 | 🟠 | `web/models.go` sin `//go:build wasm` — `web/` no compila en non-WASM (no `main()`) | Agregar build tag a `models.go` — Stage D |
| 6 | 🟠 | `fetch.Post("/contact")` apunta a Pages, no al Worker (deploys separados) | Leer `window.WORKER_URL` via `js.Global()` — Stage D |
| 7 | 🟠 | `twFront` SourceDir hardcodea `"web"` — falla si `PUBLIC_DIR` no sigue la convención | Derivar con `filepath.Dir(cfg.PublicDir)` — Stage B |
| 8 | 🟡 | Auto-detección de `worker/main.go` mencionada pero no implementada | Agregar en `config.go` `applyDefaults()` — Stage B |
| 9 | 🟡 | `Content-Type: application/json` se setea antes del check de OPTIONS | Mover header dentro de cada branch — Stage D |
| 10 | 🟡 | `init.go` prompta por `ENTRY` — confuso con nueva auto-detección | Actualizar `init.go` para omitir si `worker/main.go` existe — Stage B |

---

## Stage A — Worker JS assets (goflare)

**Archivos a modificar:** `goflare/javascripts.go`, `goflare/cloudflare.go`
**Archivos a crear:** `goflare/assets/worker.mjs`, `goflare/assets/runtime.mjs`, `goflare/assets/wasm_exec_worker.js`

### Decisión: embed en lugar de generar

En vez de generar strings de JS en tiempo de compilación de goflare, se copian
los archivos del template probado en producción:
```
/home/cesar/Dev/Pkg/Fork/workers/_templates/cloudflare/worker-tinygo/build/
├── worker.mjs      → goflare/assets/worker.mjs       (sin cambios)
├── runtime.mjs     → goflare/assets/runtime.mjs      (cambiar app.wasm → worker.wasm)
└── wasm_exec.js    → goflare/assets/wasm_exec_worker.js
```

`goflare/javascripts.go` usa `//go:embed` para incrustarlos y `generateWorkerFile()`
los escribe directamente al `OutputDir`. Sin templates de strings, sin lógica de generación.

### Mecanismo de `binding` (de `wasm_exec.js`)

El `wasm_exec.js` del template parchea `go.run(instance, context)` con un `Proxy`:
```js
const globalProxy = new Proxy(global, {
    get(target, prop) {
        if (prop === 'context') return context;  // ← context = {env, ctx, binding}
        return Reflect.get(...arguments);
    }
})
```
Esto permite que Go acceda a `binding` con:
```go
binding := js.Global().Get("context").Get("binding")
binding.Set("handleRequest", js.FuncOf(myHandler))
```
No se usa `globalThis.goflare` — se usa el `binding` estándar del template.
Esto afecta Stage C (workers package).

### A1 — Copiar assets a `goflare/assets/`

Copiar los 3 archivos. `runtime.mjs` requiere un cambio de una línea:

**`goflare/assets/runtime.mjs`** — igual al original excepto:
```diff
-import mod from "./app.wasm";
+import mod from "./worker.wasm";
```

`worker.mjs` y `wasm_exec_worker.js` se copian sin cambios.

### A2 — Reemplazar `goflare/javascripts.go` completo

```go
package goflare

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed assets/worker.mjs
var embeddedWorkerMjs []byte

//go:embed assets/runtime.mjs
var embeddedRuntimeMjs []byte

//go:embed assets/wasm_exec_worker.js
var embeddedWasmExecWorker []byte

// generateWorkerFile copies the three pre-built JS assets for a Cloudflare Worker
// into OutputDir. Files are embedded at compile time — no generation needed.
//
//   - worker.mjs         — ES module entry, calls binding.handleRequest(req)
//   - runtime.mjs        — loads worker.wasm, exposes createRuntimeContext
//   - wasm_exec_worker.js — TinyGo runtime with context Proxy patch
func (g *Goflare) generateWorkerFile() error {
	files := []struct {
		name string
		data []byte
	}{
		{"worker.mjs", embeddedWorkerMjs},
		{"runtime.mjs", embeddedRuntimeMjs},
		{"wasm_exec.js", embeddedWasmExecWorker},
	}
	for _, f := range files {
		dest := filepath.Join(g.Config.OutputDir, f.name)
		if err := os.WriteFile(dest, f.data, 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", f.name, err)
		}
	}
	return nil
}
```

### A3 — Actualizar `goflare/cloudflare.go` — `DeployWorker()`

Reemplazar la sección que lista los archivos de artifacts (buscar los tres `filepath.Join` con `worker.js`, `worker.wasm`, `wasm_exec.js`):

**Antes:**
```go
workerJs   := filepath.Join(g.Config.OutputDir, "worker.js")
workerWasm := filepath.Join(g.Config.OutputDir, "worker.wasm")
wasmExec   := filepath.Join(g.Config.OutputDir, "wasm_exec.js")

files := []string{workerJs, workerWasm, wasmExec}
```

**Después:**
```go
workerMjs  := filepath.Join(g.Config.OutputDir, "worker.mjs")
runtimeMjs := filepath.Join(g.Config.OutputDir, "runtime.mjs")
workerWasm := filepath.Join(g.Config.OutputDir, "worker.wasm")
wasmExec   := filepath.Join(g.Config.OutputDir, "wasm_exec.js")

files := []string{workerMjs, runtimeMjs, workerWasm, wasmExec}
```

Reemplazar la sección de metadata y `addFilePart`:

**Antes:**
```go
metadata := map[string]string{"main_module": "worker.js"}
// ...
if err := addFilePart(mw, "worker.js", workerJs); err != nil {
if err := addFilePart(mw, "worker.wasm", workerWasm); err != nil {
if err := addFilePart(mw, "wasm_exec.js", wasmExec); err != nil {
```

**Después:**
```go
metadata := map[string]string{"main_module": "worker.mjs"}
// ...
if err := addFilePart(mw, "worker.mjs", workerMjs); err != nil {
if err := addFilePart(mw, "runtime.mjs", runtimeMjs); err != nil {
if err := addFilePart(mw, "worker.wasm", workerWasm); err != nil {
if err := addFilePart(mw, "wasm_exec.js", wasmExec); err != nil {
```

### A3 — Actualizar `goflare/tests/deploy_worker_test.go`

Buscar todas las referencias a `"worker.js"` y reemplazar por `"worker.mjs"`.
Agregar verificación de `"runtime.mjs"` en los tests de multipart.
Actualizar `TestDeployWorker_MissingArtifact` para cubrir los 4 archivos nuevos.

---

## Stage B — `buildPages()` compila frontend WASM

**Archivos a modificar:** `goflare/goflare.go`, `goflare/build.go`

### Problema

`buildPages()` solo copia archivos. Si el proyecto tiene `web/client.go`,
goflare debe compilarlo a `web/public/client.wasm` + `web/public/script.js`
antes de copiar.

### B1 — Agregar `twFront` en `goflare/goflare.go`

Agregar campo en `Goflare` struct:
```go
type Goflare struct {
    tw      *client.WasmClient  // Worker compiler (Entry)
    twFront *client.WasmClient  // Frontend compiler (web/client.go) — nil si no aplica
    Config  *Config
    log     func(message ...any)
    BaseURL string
}
```

En la función `New()`, después de crear `tw`, agregar:

```go
// Si hay PublicDir, crear cliente para compilar web/client.go.
// SourceDir se deriva del padre de PublicDir (ej: "web/public" → "web").
// No llamar Change() aquí — dispara compilación inmediata.
// Usar SetMode() que solo actualiza el estado interno.
if cfg.PublicDir != "" {
    frontSourceDir := filepath.Dir(cfg.PublicDir) // fix #7
    twFront := client.New(&client.Config{
        SourceDir: func() string { return frontSourceDir },
        OutputDir: func() string { return cfg.PublicDir },
    })
    twFront.SetBuildOnDisk(true, false)
    twFront.SetMode(cfg.CompilerMode) // fix #1: SetMode, NO Change()
    g.twFront = twFront
}
```

Actualizar `SetLog` para propagar al frontend compiler:
```go
func (g *Goflare) SetLog(f func(message ...any)) {
    g.log = f
    if g.tw != nil {
        g.tw.SetLog(f)
    }
    if g.twFront != nil {
        g.twFront.SetLog(f)
    }
}
```

### B3 — Auto-detección de `worker/main.go` en `goflare/config.go` (fix #8)

Agregar al final de `applyDefaults()`:

```go
// Auto-detectar Worker entry si existe worker/main.go y Entry no está configurado.
if c.Entry == "" {
    if _, err := os.Stat(filepath.Join("worker", "main.go")); err == nil {
        c.Entry = "worker"
    }
}
```

Agregar imports necesarios en `config.go`: `"os"` y `"path/filepath"`.

### B4 — Actualizar `goflare/init.go` (fix #10)

En la función `Init()`, el prompt de `Entry` debe omitirse si `worker/main.go` ya existe,
e informar al usuario que fue detectado automáticamente:

```go
// Solo preguntar por Entry si no existe worker/main.go
if _, err := os.Stat(filepath.Join("worker", "main.go")); os.IsNotExist(err) {
    cfg.Entry, err = ask("Entry point (Worker dir, leave empty for Pages-only) [worker]:", false)
    if err != nil {
        return nil, err
    }
} else {
    fmt.Fprintln(out, "  → worker/main.go detected, Entry set to \"worker\" automatically")
    cfg.Entry = "worker"
}
```

### B2 — Actualizar `goflare/build.go` — `buildPages()`

```go
func (g *Goflare) buildPages() error {
    // 1. Verificar que PUBLIC_DIR existe
    if _, err := os.Stat(g.Config.PublicDir); os.IsNotExist(err) {
        return fmt.Errorf("public dir does not exist: %s", g.Config.PublicDir)
    }

    // 2. Compilar frontend WASM si existe web/client.go
    frontEntry := filepath.Join("web", "client.go")
    if _, err := os.Stat(frontEntry); err == nil {
        if g.twFront == nil {
            return fmt.Errorf("frontend compiler not initialized (twFront is nil)")
        }
        g.Logger("compiling frontend WASM: web/client.go →", g.Config.PublicDir)
        if err := g.twFront.Compile(); err != nil {
            return fmt.Errorf("frontend WASM compilation failed: %w", err)
        }
    }

    // 3. Copiar PUBLIC_DIR → .goflare/dist/
    distDir := filepath.Join(g.Config.OutputDir, "dist")
    if err := os.MkdirAll(distDir, 0755); err != nil {
        return fmt.Errorf("failed to create dist directory: %w", err)
    }
    return copyDir(g.Config.PublicDir, distDir)
}
```

---

## Stage C — Sub-paquete `github.com/tinywasm/goflare/workers`

**Directorio:** `goflare/workers/` — mismo módulo `github.com/tinywasm/goflare`, sin `go.mod` propio.

Todo lo relativo a Cloudflare vive en goflare. No se crea una librería nueva.

Este sub-paquete provee el runtime helper para Workers: reemplaza el patrón
raw `syscall/js` del usuario con una API limpia. Usa `tinywasm/fmt` en lugar
de `net/http`, reduciendo el binario ~80% vs `syumai/workers`.

Solo compila en WASM (`//go:build wasm`). Sin dependencias de DOM.

### Estructura de archivos

```
goflare/workers/
├── workers.go    # Handle(), Ready(), binding setup via context Proxy
├── request.go    # *Request — wrapper del JS Request
└── response.go   # *Response — io.Writer + WriteHeader + Header
```

Sin `go.mod` — hereda el módulo `github.com/tinywasm/goflare`.

### `workers.go`

El `binding` viene del runtime context inyectado por `worker.mjs` vía el `Proxy`
del `wasm_exec.js` modificado. Go lo accede con `js.Global().Get("context").Get("binding")`.

```go
//go:build wasm

package workers

import "syscall/js"

// Handle registers fn as the single request handler and blocks forever.
// fn is called for every incoming HTTP request to the Worker.
// This must be called from main(); it never returns.
//
// Uses the binding pattern from goflare/assets/worker.mjs:
//   binding.handleRequest is called per request with the JS Request object.
//   binding is accessed via js.Global().Get("context") — injected by wasm_exec.js Proxy.
func Handle(fn func(*Response, *Request)) {
    // Access the runtime context injected by worker.mjs into go.run(instance, ctx).
    // wasm_exec.js patches global with a Proxy: global.context → ctx = {env, ctx, binding}.
    binding := js.Global().Get("context").Get("binding")

    binding.Set("handleRequest", js.FuncOf(func(this js.Value, args []js.Value) any {
        req := args[0]
        return newPromise(func() (js.Value, error) {
            r, err := newRequest(req)
            if err != nil {
                return errorResponse(500, "failed to parse request"), nil
            }
            w := newResponse()
            fn(w, r)
            return w.build(), nil
        })
    }))

    Ready()
    select {}
}

// Ready signals the Workers runtime that Go initialization is complete.
// Called automatically by Handle(). Call manually only if not using Handle().
func Ready() {
    workers := js.Global().Get("workers")
    if !workers.IsNull() && !workers.IsUndefined() {
        workers.Call("ready")
    }
}

// newPromise wraps a blocking Go func in a JS Promise.
func newPromise(fn func() (js.Value, error)) js.Value {
    executor := js.FuncOf(func(this js.Value, args []js.Value) any {
        resolve, reject := args[0], args[1]
        go func() {
            result, err := fn()
            if err != nil {
                reject.Invoke(js.ValueOf(err.Error()))
                return
            }
            resolve.Invoke(result)
        }()
        return nil
    })
    return js.Global().Get("Promise").New(executor)
}

// errorResponse builds a minimal JS Response for internal errors.
func errorResponse(status int, msg string) js.Value {
    h := js.Global().Get("Headers").New()
    h.Call("set", "Content-Type", "text/plain")
    init := js.Global().Get("Object").New()
    init.Set("status", status)
    init.Set("headers", h)
    return js.Global().Get("Response").New(js.ValueOf(msg), init)
}
```

### `request.go`

```go
//go:build wasm

package workers

import (
    "fmt"
    "syscall/js"
)

// Request represents an incoming HTTP request to the Worker.
type Request struct {
    Method  string
    URL     string
    Headers map[string]string
    body    []byte
}

// Body returns the raw request body bytes.
func (r *Request) Body() []byte { return r.body }

// newRequest reads a JS Fetch Request into a Go Request.
// Blocks until the body promise resolves.
func newRequest(jsReq js.Value) (*Request, error) {
    r := &Request{
        Method:  jsReq.Get("method").String(),
        URL:     jsReq.Get("url").String(),
        Headers: map[string]string{},
    }

    // Read headers
    jsHeaders := jsReq.Get("headers")
    if !jsHeaders.IsNull() && !jsHeaders.IsUndefined() {
        entries := jsHeaders.Call("entries")
        for {
            next := entries.Call("next")
            if next.Get("done").Bool() {
                break
            }
            val := next.Get("value")
            r.Headers[val.Index(0).String()] = val.Index(1).String()
        }
    }

    // Read body — blocks via channel + promise chaining
    body, err := readBodyText(jsReq)
    if err != nil {
        return nil, fmt.Errorf("workers: read body: %w", err)
    }
    r.body = []byte(body)

    return r, nil
}

// readBodyText resolves req.text() via a blocking channel.
// js.FuncOf callbacks are released after the promise settles to avoid leaks (fix #2).
func readBodyText(jsReq js.Value) (string, error) {
    ch := make(chan string, 1)
    errCh := make(chan string, 1)

    var thenFn, catchFn js.Func

    thenFn = js.FuncOf(func(this js.Value, args []js.Value) any {
        ch <- args[0].String()
        thenFn.Release()
        catchFn.Release()
        return nil
    })
    catchFn = js.FuncOf(func(this js.Value, args []js.Value) any {
        errCh <- args[0].String()
        thenFn.Release()
        catchFn.Release()
        return nil
    })

    jsReq.Call("text").Call("then", thenFn).Call("catch", catchFn)

    select {
    case text := <-ch:
        return text, nil
    case msg := <-errCh:
        return "", fmt.Errorf("%s", msg)
    }
}
```

### `response.go`

```go
//go:build wasm

package workers

import (
    "bytes"
    "syscall/js"
)

// Response is written by the handler and converted to a JS Response.
type Response struct {
    status  int
    headers map[string]string
    buf     bytes.Buffer
}

func newResponse() *Response {
    return &Response{
        status:  200,
        headers: map[string]string{},
    }
}

// WriteHeader sets the HTTP status code.
func (w *Response) WriteHeader(code int) { w.status = code }

// Header returns the response headers map for setting values.
// Usage: w.Header()["Content-Type"] = "application/json"
func (w *Response) Header() map[string]string { return w.headers }

// Write appends bytes to the response body.
func (w *Response) Write(b []byte) (int, error) { return w.buf.Write(b) }

// WriteString appends a string to the response body.
func (w *Response) WriteString(s string) (int, error) { return w.buf.WriteString(s) }

// build converts the Go response to a JS Response object.
func (w *Response) build() js.Value {
    h := js.Global().Get("Headers").New()
    for k, v := range w.headers {
        h.Call("set", k, v)
    }

    init := js.Global().Get("Object").New()
    init.Set("status", w.status)
    init.Set("headers", h)

    return js.Global().Get("Response").New(js.ValueOf(w.buf.String()), init)
}
```

---

## Stage D — Demo project

**Repo:** `github.com/tinywasm/goflare-demo`
**Plan completo:** ver `goflare-demo/docs/PLAN.md`

> Stage D depende de A + B + C completos. Una vez terminados los stages anteriores,
> implementar el demo siguiendo el plan en el repo correspondiente.



## Orden de ejecución y dependencias

```
Stage A — independiente — modificar goflare existente
Stage B — independiente — modificar goflare existente
Stage C — independiente — crear sub-paquete goflare/workers/
Stage D — depende de A + B + C completos → ver github.com/tinywasm/goflare-demo
```

### Resumen de archivos por stage

| Stage | Archivo | Acción |
|-------|---------|--------|
| A | `goflare/assets/worker.mjs` | Copiar desde Fork sin cambios |
| A | `goflare/assets/runtime.mjs` | Copiar desde Fork, cambiar `app.wasm` → `worker.wasm` |
| A | `goflare/assets/wasm_exec_worker.js` | Copiar desde Fork sin cambios |
| A | `goflare/javascripts.go` | Reemplazar completo (`//go:embed` + `generateWorkerFile()`) |
| A | `goflare/cloudflare.go` | Cambiar nombres de artifacts en `DeployWorker()` |
| A | `goflare/tests/deploy_worker_test.go` | Actualizar nombres |
| B | `goflare/goflare.go` | Agregar `twFront`, actualizar `New()` y `SetLog()` |
| B | `goflare/build.go` | Actualizar `buildPages()` |
| C | `goflare/workers/workers.go` | Crear |
| C | `goflare/workers/request.go` | Crear |
| C | `goflare/workers/response.go` | Crear |
| D | `github.com/tinywasm/goflare-demo` | Ver `goflare-demo/docs/PLAN.md` |
