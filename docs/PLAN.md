> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.

# Plan: goflare/d1 — Cloudflare D1 ORM Adapter

## Context

`tinywasm/goflare` (module `github.com/tinywasm/goflare`) is the Go↔Cloudflare Workers bridge.
This plan adds `goflare/d1/`, a Cloudflare D1 adapter for `github.com/tinywasm/orm`, following
the same pattern as `github.com/tinywasm/indexdb`.

Module root: `tinywasm/goflare/`
`go.mod`: `module github.com/tinywasm/goflare`.
Compatibilidad: el código debe compilar con TinyGo (asyncify scheduler). Tests corren con `gotest` — no requiere instalar TinyGo.

**Do NOT use `database/sql` or `database/sql/driver`.** D1 integrates via `tinywasm/orm`.

**Prerequisites**:
- `github.com/tinywasm/jsvalue` must already export `AwaitPromise` and `Uint8ArrayClass`.
- `github.com/tinywasm/sqlt` must already export `NewCompiler()`. The compiler is NOT reimplemented here — `sqlt.NewCompiler()` is used directly.

## Goal

Create package `github.com/tinywasm/goflare/d1` that:
- Implements `orm.Executor` backed by the Cloudflare D1 JS API.
- Uses `sqlt.NewCompiler()` — does NOT reimplement SQL generation.
- Exposes `New(bindingName string) (*orm.DB, error)` as the single public constructor.
- Compiles only for TinyGo WASM (D1 is inaccessible outside a Worker).

## Public API

```go
// New opens the D1 binding named bindingName and returns an *orm.DB.
// Returns ErrDatabaseNotFound if the binding is not present in the Worker env.
func New(bindingName string) (*orm.DB, error)
```

Consumer usage:

```go
db, err := d1.New("MY_DB")
if err != nil { ... }
defer db.Close()
db.Create(&MyModel{...})
```

## ORM Pattern (mirror of indexdb)

```
orm.DB
 └── adapter            (implements orm.Executor)  ← this plan
 └── sqlt.NewCompiler() (implements orm.Compiler)  ← from tinywasm/sqlt
```

`adapter` holds the D1 `js.Value` binding and executes SQL via the D1 JS API.
SQL generation is fully delegated to `sqlt.NewCompiler()` — no local compiler.

## D1 JS API calls used

| Operation | JS call chain |
|---|---|
| Exec | `db.prepare(sql).bind(...args).run()` → `AwaitPromise` |
| QueryRow | `db.prepare(sql).bind(...args).first()` → `AwaitPromise` → single JS object |
| Query | `db.prepare(sql).bind(...args).raw({columnNames:true})` → `AwaitPromise` → `[[cols], ...rows]` |

D1 binding is at `js.Global().Get("context").Get("env").Get(bindingName)`.

## TinyWasm Constraints (mandatory)

- No `import "errors"`, `"fmt"`, `"strings"`, `"strconv"` from stdlib — use `github.com/tinywasm/fmt`.
- No `import "github.com/syumai/workers/..."`.
- No `import "database/sql"` or `"database/sql/driver"`.
- All files that use `syscall/js` must have `//go:build wasm` as first line.
- `errors.go` has no build tag (pure Go).
- D1 binding accessed via `js.Global().Get("context").Get("env").Get(name)` only.

## Code Quality (mandatory)

- Error prefix `"d1: "` is a package-level constant `errPrefix`.
- `Err(...)` / `Errf(...)` from `github.com/tinywasm/fmt` replace all `errors.New` / `fmt.Errorf`.
- `jsvalue.AwaitPromise` and `jsvalue.Uint8ArrayClass` from `github.com/tinywasm/jsvalue`.

## File Structure

```
goflare/d1/
├── errors.go        # no build tag — ErrDatabaseNotFound, errPrefix
├── adapter_wasm.go  # adapter (orm.Executor impl) + New()
└── rows_wasm.go     # d1Rows (orm.Rows impl) + scanners
```

No `compiler.go` — SQL generation delegated entirely to `sqlt.NewCompiler()`.

---

## Stage 1 — errors.go

```go
package d1

import . "github.com/tinywasm/fmt"

const errPrefix = "d1: "

var ErrDatabaseNotFound = Err(errPrefix + "database not found")
```

## Stage 2 — adapter_wasm.go

```go
//go:build wasm

package d1

import (
    "syscall/js"

    . "github.com/tinywasm/fmt"
    "github.com/tinywasm/jsvalue"
    "github.com/tinywasm/orm"
    "github.com/tinywasm/sqlt"
)

type adapter struct{ dbObj js.Value }

// New opens the named D1 binding and returns an *orm.DB.
func New(bindingName string) (*orm.DB, error) {
    v := js.Global().Get("context").Get("env").Get(bindingName)
    if v.IsUndefined() || v.IsNull() {
        return nil, ErrDatabaseNotFound
    }
    a := &adapter{dbObj: v}
    return orm.New(a, sqlt.NewCompiler()), nil
}

// Exec implements orm.Executor — runs INSERT, UPDATE, DELETE.
func (a *adapter) Exec(query string, args ...any) error {
    stmt := a.dbObj.Call("prepare", query)
    bound := bindArgs(stmt, args)
    _, err := jsvalue.AwaitPromise(bound.Call("run"))
    return err
}

// QueryRow implements orm.Executor — fetches a single row.
func (a *adapter) QueryRow(query string, args ...any) orm.Scanner {
    stmt := a.dbObj.Call("prepare", query)
    bound := bindArgs(stmt, args)
    v, err := jsvalue.AwaitPromise(bound.Call("first"))
    if err != nil {
        return &errScanner{err}
    }
    return &rowScanner{v}
}

// Query implements orm.Executor — fetches multiple rows.
func (a *adapter) Query(query string, args ...any) (orm.Rows, error) {
    stmt := a.dbObj.Call("prepare", query)
    bound := bindArgs(stmt, args)
    opts := js.Global().Get("Object").New()
    opts.Set("columnNames", true)
    p := bound.Call("raw", opts)
    arr, err := jsvalue.AwaitPromise(p)
    if err != nil {
        return nil, err
    }
    if arr.Length() == 0 {
        return &d1Rows{}, nil
    }
    // first element is column names array
    colsJS := arr.Call("shift")
    cols := make([]string, colsJS.Length())
    for i := range cols {
        cols[i] = colsJS.Index(i).String()
    }
    return &d1Rows{arr: arr, cols: cols}, nil
}

func (a *adapter) Close() error { return nil }

// bindArgs calls .bind(...args) on a prepared statement, converting []byte to Uint8Array.
func bindArgs(stmt js.Value, args []any) js.Value {
    if len(args) == 0 {
        return stmt
    }
    jsArgs := make([]any, len(args))
    for i, arg := range args {
        if b, ok := arg.([]byte); ok {
            ua := jsvalue.Uint8ArrayClass.New(len(b))
            js.CopyBytesToJS(ua, b)
            jsArgs[i] = ua
        } else {
            jsArgs[i] = arg
        }
    }
    return stmt.Call("bind", jsArgs...)
}
```

## Stage 3 — rows_wasm.go

```go
//go:build wasm

package d1

import (
    "syscall/js"
    . "github.com/tinywasm/fmt"
    "github.com/tinywasm/jsvalue"
)

// d1Rows implements orm.Rows over a raw JS array of row arrays.
type d1Rows struct {
    arr js.Value
    cols []string
    cur  int
    len  int
    once bool
}

func (r *d1Rows) rowsLen() int {
    if !r.once {
        if r.arr.Truthy() {
            r.len = r.arr.Length()
        }
        r.once = true
    }
    return r.len
}

func (r *d1Rows) Next() bool {
    if r.cur < r.rowsLen() {
        r.cur++
        return true
    }
    return false
}

func (r *d1Rows) Scan(dest ...any) error {
    if r.cur == 0 || r.cur > r.rowsLen() {
        return Err(errPrefix + "invalid row cursor")
    }
    row := r.arr.Index(r.cur - 1)
    if len(dest) != len(r.cols) {
        return Err(errPrefix + "scan destination count mismatch")
    }
    for i, ptr := range dest {
        v := row.Index(i)
        if err := scanValue(v, ptr); err != nil {
            return err
        }
    }
    return nil
}

func (r *d1Rows) Close() error { return nil }
func (r *d1Rows) Err() error   { return nil }

// scanValue copies a JS value into a Go pointer.
func scanValue(v js.Value, dest any) error {
    switch p := dest.(type) {
    case *string:
        *p = v.String()
    case *int:
        *p = v.Int()
    case *int64:
        *p = int64(v.Int())
    case *float64:
        *p = v.Float()
    case *bool:
        *p = v.Bool()
    case *[]byte:
        src := jsvalue.Uint8ArrayClass.New(v)
        b := make([]byte, src.Length())
        js.CopyBytesToGo(b, src)
        *p = b
    case *any:
        *p = jsValueToAny(v)
    default:
        return Errf(errPrefix+"unsupported scan type: %T", dest)
    }
    return nil
}

func jsValueToAny(v js.Value) any {
    switch v.Type() {
    case js.TypeNull, js.TypeUndefined:
        return nil
    case js.TypeBoolean:
        return v.Bool()
    case js.TypeNumber:
        f := v.Float()
        if f == float64(int64(f)) {
            return int64(f)
        }
        return f
    case js.TypeString:
        return v.String()
    default:
        return v.String()
    }
}

// errScanner is a Scanner that always returns an error.
type errScanner struct{ err error }
func (e *errScanner) Scan(...any) error { return e.err }

// rowScanner scans a single D1 row (JS object) using column name keys.
type rowScanner struct{ obj js.Value }
func (s *rowScanner) Scan(dest ...any) error {
    // D1 .first() returns a plain JS object keyed by column name.
    // orm callers must pass pointers in schema order — not used directly here.
    // Scan by index is not possible without column names; this scanner is for
    // internal use where the caller knows the structure.
    return Err(errPrefix + "rowScanner.Scan: use Query for multi-column reads")
}
```

> Note: `rowScanner` is a placeholder. `QueryRow` in D1 is best used for `COUNT(*)` or
> single-column results. Full row reads should use `Query`. Update if orm requires it.

## Stage 4 — go.mod

In `goflare/go.mod`, add:

```
require github.com/tinywasm/jsvalue <version-with-AwaitPromise>
require github.com/tinywasm/sqlt   <version-with-NewCompiler>
require github.com/tinywasm/orm    v0.8.1
```

## Stages Summary

| # | Archivo | Acción |
|---|---|---|
| 1 | `goflare/d1/errors.go` | Crear — `errPrefix` + `ErrDatabaseNotFound` |
| 2 | `goflare/d1/adapter_wasm.go` | Crear — `adapter`, `New()` con `sqlt.NewCompiler()`, `bindArgs` |
| 3 | `goflare/d1/rows_wasm.go` | Crear — `d1Rows`, `scanValue`, scanners |
| 4 | `goflare/go.mod` | Agregar `jsvalue`, `sqlt`, `orm` |

## Verification

```bash
gotest          # full suite: vet + stdlib tests (compiler_test.go) + wasm if detected
```
