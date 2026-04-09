# GoFlare — PLAN: Provider-Agnostic Naming

> Date: 2026-04-09
> Status: Ready to execute
> Scope: `goflare/config.go`, `goflare/init.go`, `goflare/README.md`

---

## Motivation

`tinywasm/deploy` must be agnostic to the deployment target (Cloudflare, server, AWS, etc).
The current naming ties project structure to Cloudflare specifically:

| Current | Problem |
|---------|---------|
| `worker/main.go` | "worker" is a Cloudflare term |
| `.goflare/` output dir | provider-specific name |
| `goflare init` prompts for "Worker name" | leaks provider concept |
| 4 JS files deployed separately | unnecessary complexity |

The fix is naming + bundling the 3 JS assets into a single minified `edge.js`.

---

## Changes

### `goflare/config.go` — `applyDefaults()`

Detect only `edge/main.go`. Remove `worker/main.go` detection entirely.
Change default `OutputDir` from `.goflare/` to `.build/`.

```go
func (c *Config) applyDefaults() {
    if c.WorkerName == "" && c.ProjectName != "" {
        c.WorkerName = c.ProjectName + "-worker"
    }
    if c.OutputDir == "" {
        c.OutputDir = ".build/"   // was: ".goflare/"
    }
    if c.CompilerMode == "" {
        c.CompilerMode = "S"
    }

    // Auto-detect edge function entry.
    if c.Entry == "" {
        if _, err := os.Stat(filepath.Join("edge", "main.go")); err == nil {
            c.Entry = "edge"
        }
    }
}
```

### `goflare/init.go` — `Init()` prompt + `UpdateGitignore()`

```go
// Only ask for Entry if edge/main.go does not exist
if _, err := os.Stat(filepath.Join("edge", "main.go")); err == nil {
    fmt.Fprintln(out, "  → edge/main.go detected, Entry set to \"edge\" automatically")
    cfg.Entry = "edge"
} else {
    cfg.Entry, err = ask("Entry point (edge function dir, leave empty for Pages-only) [edge]:", false)
    if err != nil {
        return nil, err
    }
}
```

Update `UpdateGitignore` to add `.build/` instead of `.goflare/`:

```go
entries := []string{".env", ".build/"}
```

### `goflare/javascripts.go` — bundle 3 files into single minified `edge.js`

**Why it works:** Cloudflare Workers only requires that `import mod from "./edge.wasm"` and
`import { connect } from "cloudflare:sockets"` are top-level static imports. Everything else
can be inlined. `wasm_exec.js` uses an IIFE with no imports — safe to inline directly.

**Bundle structure** (assembled in Go at build time):
```
import mod from "./edge.wasm";
import { connect } from "cloudflare:sockets";

[wasm_exec.js content — IIFE inlined]

[runtime.mjs functions inlined — loadModule, createRuntimeContext]

[worker.mjs body inlined — run, fetch, scheduled, queue, onRequest, export default]
```

Then minified with `github.com/tdewolff/minify/v2/js` (already used in `tinywasm/assetmin`).

**Result:** 2 files deployed instead of 4 (`edge.js` + `edge.wasm`).

Replace `generateWorkerFile()` in `goflare/javascripts.go`:

```go
package goflare

import (
    _ "embed"
    "fmt"
    "os"
    "path/filepath"
    "strings"

    "github.com/tdewolff/minify/v2"
    minjs "github.com/tdewolff/minify/v2/js"
)

//go:embed assets/wasm_exec_worker.js
var embeddedWasmExec []byte

//go:embed assets/runtime.mjs
var embeddedRuntime []byte

//go:embed assets/worker.mjs
var embeddedWorker []byte

// generateWorkerFile bundles and minifies the three JS assets into a single edge.js.
//
// Bundle order:
//  1. Static imports (top-level — required by Cloudflare module format)
//  2. wasm_exec.js  — TinyGo runtime IIFE (no imports)
//  3. runtime.mjs   — loadModule + createRuntimeContext (imports stripped, already at top)
//  4. worker.mjs    — fetch/scheduled/queue/onRequest + export default (imports stripped)
func (g *Goflare) generateWorkerFile() error {
    wasmExecBody := stripIIFEWrapper(string(embeddedWasmExec))
    runtimeBody  := stripImports(string(embeddedRuntime))
    workerBody   := stripImports(string(embeddedWorker))

    bundle := strings.Join([]string{
        `import mod from "./edge.wasm";`,
        `import { connect } from "cloudflare:sockets";`,
        wasmExecBody,
        runtimeBody,
        workerBody,
    }, "\n\n")

    m := minify.New()
    m.AddFunc("text/javascript", minjs.Minify)
    minified, err := m.String("text/javascript", bundle)
    if err != nil {
        return fmt.Errorf("failed to minify edge.js: %w", err)
    }

    dest := filepath.Join(g.Config.OutputDir, "edge.js")
    return os.WriteFile(dest, []byte(minified), 0644)
}

// stripImports removes ES module import lines from a JS source string.
func stripImports(src string) string {
    var lines []string
    for _, line := range strings.Split(src, "\n") {
        if !strings.HasPrefix(strings.TrimSpace(line), "import ") {
            lines = append(lines, line)
        }
    }
    return strings.Join(lines, "\n")
}

// stripIIFEWrapper removes the outer (() => { ... })(); from wasm_exec.js,
// leaving the inner body for inline embedding.
func stripIIFEWrapper(src string) string {
    // wasm_exec.js wraps everything in (() => { ... })();
    // Find first { and last }); and extract the inner body.
    start := strings.Index(src, "{")
    end := strings.LastIndex(src, "});")
    if start == -1 || end == -1 || end <= start {
        return src // fallback: return as-is
    }
    return src[start+1 : end]
}
```

Update `cloudflare.go` — `DeployWorker()` to use 2 files:

```go
// Before (4 files):
files := []string{workerMjs, runtimeMjs, workerWasm, wasmExec}
metadata := map[string]string{"main_module": "worker.mjs"}

// After (2 files):
edgeJs  := filepath.Join(g.Config.OutputDir, "edge.js")
edgeWasm := filepath.Join(g.Config.OutputDir, "edge.wasm")
files := []string{edgeJs, edgeWasm}
metadata := map[string]string{"main_module": "edge.js"}
```

Remove now-unused embedded vars `embeddedWorkerMjs` and `embeddedRuntimeMjs` from
`javascripts.go` (replaced by the bundle approach above). Keep `assets/` directory
with the 3 source files for bundling.

Add `github.com/tdewolff/minify/v2` to `go.mod` in `tinywasm/goflare`.

### `goflare/README.md` — project structure

Already updated to reflect `edge/` and `.build/`. Update `.build/` contents:

```
├── .build/
│   ├── edge.js    # bundled + minified JS (wasm_exec + runtime + worker entry)
│   └── edge.wasm  # compiled edge function binary
```

---

## Breaking Changes

| Before | After |
|--------|-------|
| `worker/main.go` auto-detected | `edge/main.go` — rename dir |
| `.goflare/` output dir | `.build/` — update `.gitignore` |

Existing projects must rename `worker/` → `edge/` and update `.gitignore`.
Projects with explicit `ENTRY=worker` in `.env` must change to `ENTRY=edge`.

---

## Execution Order

```
1. goflare/go.mod                        — add github.com/tdewolff/minify/v2
2. goflare/javascripts.go                — replace generateWorkerFile() with bundle+minify
3. goflare/cloudflare.go                 — DeployWorker() use edge.js + edge.wasm (2 files)
4. goflare/config.go                     — applyDefaults(): OutputDir .build/ + edge/ only
5. goflare/init.go                       — Init(): edge/ prompt + UpdateGitignore .build/
6. goflare/tests/deploy_worker_test.go   — update artifact names to edge.js + edge.wasm
7. goflare/README.md                     — update project structure + .build/ contents
8. goflare/docs/ARCHITECTURE.md          — update if references worker/ or .goflare/
9. goflare/docs/BUILD_WORKERS.md         — update artifact list and bundle explanation
10. goflare/docs/QUICK_REFERENCE.md      — update directory names and artifact names
```

Test changes required: `deploy_worker_test.go` must update artifact names from
`worker.mjs`, `runtime.mjs`, `wasm_exec.js` → `edge.js` only.
