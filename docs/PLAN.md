> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.

# Plan: goflare/d1 — Use jsvalue.ScanValue and jsvalue.ToAny

## Context

`tinywasm/jsvalue` now exports `ScanValue` and `ToAny` (added in v0.0.12 or later).
`goflare/d1/rows_wasm.go` has local implementations of these two functions that are
now duplicates. This plan removes the local code and delegates to jsvalue.

**Prerequisite**: `github.com/tinywasm/jsvalue` must be published with `ScanValue` and `ToAny`.

## Changes

### `goflare/d1/rows_wasm.go`

Remove the local `scanValue` function and `jsValueToAny` function entirely.

Replace every call to `scanValue(v, ptr)` with `jsvalue.ScanValue(v, ptr)`.

The `import "github.com/tinywasm/jsvalue"` is already present — no new imports needed.
Remove `. "github.com/tinywasm/fmt"` from `rows_wasm.go` if `Errf` is no longer used there
(it was only used inside `scanValue`). Keep it if still needed for other errors in the file.

### `goflare/go.mod`

Bump `github.com/tinywasm/jsvalue` to the version that exports `ScanValue` and `ToAny`:

```bash
go get github.com/tinywasm/jsvalue@latest
go mod tidy
```

## Stages Summary

| # | Archivo | Acción |
|---|---|---|
| 1 | `goflare/d1/rows_wasm.go` | Eliminar `scanValue` y `jsValueToAny`; reemplazar llamadas con `jsvalue.ScanValue` |
| 2 | `goflare/go.mod` | `go get jsvalue@latest` + `go mod tidy` |

## Verification

```bash
gotest
```

Sin regresiones. `go vet ./...` sin output.
