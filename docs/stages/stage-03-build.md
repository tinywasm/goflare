# Stage 03 — Build Command

## Goal
Implement `(g *Goflare) Build() error` as a method on `Goflare`.
Reads `g.Config`, detects what to build (Worker, Pages, or both), produces artifacts in `.goflare/`.

---

## Tasks

### 3.1 — Implement `(g *Goflare) Build() error` (`build.go`)

New file. Orchestrates the build pipeline as a method so it has access to `g.tw`.

**Logic:**
```
if g.Config.Entry == "" && g.Config.PublicDir == "" → error "nothing to build"
if g.Config.Entry != ""     → buildWorker()  — error recorded, continue
if g.Config.PublicDir != "" → buildPages()   — error recorded, continue
if any errors → return combined error
```

Both branches run independently. If Worker fails, Pages still runs.
Errors are joined and returned together.

### 3.2 — `(g *Goflare) buildWorker() error` (`build.go`)

1. Verify `g.Config.Entry` file exists — error if not
2. Ensure `g.Config.OutputDir` directory exists (`os.MkdirAll`)
3. Call `g.generateWasmFile()` — delegates to `g.tw.RecompileMainWasm()` via existing `wasm.go`
   - On error → return with tinygo stderr in message
4. Call `g.generateWorkerFile()` — writes `worker.js` and `wasm_exec.js` via existing `javascripts.go`
5. Both `wasm.go` and `javascripts.go` are updated to write to `g.Config.OutputDir` instead of hardcoded paths

### 3.3 — `(g *Goflare) buildPages() error` (`build.go`)

1. Verify `g.Config.PublicDir` exists — error if not
2. Copy all files from `g.Config.PublicDir` to `g.Config.OutputDir + "dist/"` recursively
   - Use `os.CopyFS` (Go 1.23+) or manual `filepath.Walk`
   - No content modification

### 3.4 — Update `GenerateWorkerFiles()` and `GeneratePagesFiles()` (`workers.go`, `pages.go`)

These remain as convenience wrappers that delegate to the method:

```go
func (g *Goflare) GeneratePagesFiles() error {
    return g.buildPages()
}

func (g *Goflare) GenerateWorkerFiles() error {
    return g.buildWorker()
}
```

Callers that previously used these directly continue to work.

### 3.5 — Update `wasm.go` and `javascripts.go` to use `g.Config.OutputDir`

Replace any hardcoded output paths with `g.Config.OutputDir`.
No other logic changes to these files.

### 3.6 — Tests (`tests/build_test.go`)

- `TestBuild_WorkerOnly` — `//go:build integration` — calls tinygo via tw, checks `worker.wasm` and `worker.js` exist in OutputDir
- `TestBuild_PagesOnly` — unit, copies files, checks `dist/` contents match source
- `TestBuild_Both` — `//go:build integration` — both artifact sets present
- `TestBuild_NothingToBuild` — error when both Entry and PublicDir are empty
- `TestBuild_MissingEntry` — error when Entry path does not exist
- `TestBuild_MissingPublicDir` — error when PublicDir path does not exist
- `TestBuild_WorkerFailDoesNotStopPages` — Worker build fails, Pages build still runs, combined error returned
- `TestBuildPages_CopiesFiles` — unit, verifies recursive copy preserves structure

---

## Files Added
- `build.go`
- `tests/build_test.go`

## Files Changed
- `workers.go` — GenerateWorkerFiles delegates to buildWorker
- `pages.go` — GeneratePagesFiles delegates to buildPages
- `wasm.go` — update output path to use g.Config.OutputDir
- `javascripts.go` — update output path to use g.Config.OutputDir
- `goflare.go` — tw field and New() unchanged; Build() declared here or in build.go
