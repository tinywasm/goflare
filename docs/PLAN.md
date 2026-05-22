> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.

# Plan: goflare/d1 — Integration test via Cloudflare D1 REST API

## Context

`goflare/d1` implements `orm.Executor` over the Cloudflare D1 JS binding (WASM-only).
There are no automated tests verifying that the SQL it generates actually works against a real
D1 database. This plan adds:

1. A pure-Go D1 REST client (`goflare/d1/client.go`) — no WASM, uses `net/http`.
2. An integration test (`goflare/d1/d1_integration_test.go`) with `//go:build integration`
   that hits the real Cloudflare D1 REST API.

## Architecture

The D1 REST endpoint is:
```
POST https://api.cloudflare.com/client/v4/accounts/{account_id}/d1/database/{database_id}/query
Authorization: Bearer <token>
Content-Type: application/json

{"sql": "SELECT 1", "params": []}
```

Response envelope (same `cfEnvelope` pattern already used in `cloudflare.go`):
```json
{
  "success": true,
  "result": [{"results": [{"col": "val"}], "success": true, "meta": {...}}]
}
```

The client reuses the existing `cfClient` and `parseCFResponse` from `cloudflare.go`.
These are unexported — the new `d1Client` wrapper lives in `goflare/d1/client.go` and
calls the CF API directly using `net/http` (duplicates only the minimal HTTP logic, no
dependency on the parent `goflare` package to avoid circular imports).

## Token resolution (keyring-first, env-var fallback for CI)

```go
func resolveToken(t *testing.T) string {
    // 1. Try OS keyring (local dev — set via `goflare auth`)
    token, err := keyring.Get("goflare", "CLOUDFLARE_API_TOKEN")
    if err == nil && token != "" {
        return token
    }
    // 2. Env var fallback (CI — GitHub Secret injected as env)
    token = os.Getenv("CLOUDFLARE_API_TOKEN")
    if token != "" {
        return token
    }
    t.Skip("no token: run 'goflare auth' locally or set CLOUDFLARE_API_TOKEN in CI")
    return ""
}
```

Same pattern for `CLOUDFLARE_ACCOUNT_ID` and `D1_DATABASE_ID`:
- Local: read from the project's `.env` file (already present in any goflare project).
- CI: set as env vars in GitHub Secrets.
- Missing → `t.Skip(...)`.

## Code quality rules

- No string literals in logic — use named constants for all env key names and API paths.
- `client.go` MUST have `//go:build !wasm` — stdlib `net/http` and `encoding/json` must never enter the WASM binary. This is the same split used throughout goflare (`cloudflare.go` is host-only, `adapter_wasm.go` is WASM-only).
- `d1_integration_test.go` has `//go:build integration` — test files never compile to WASM, but explicit `!wasm` is added for clarity.
- All HTTP errors surface as typed errors via `d1APIError`.

## Changes

### Stage 1 — `goflare/d1/client.go` (new file, `//go:build !wasm`)

New file. `//go:build !wasm` keeps stdlib out of the WASM binary entirely.
No imports from parent `goflare` package (avoids circular dependency).

```go
//go:build !wasm

package d1

import (
    "bytes"
    "encoding/json"
    "fmt"
    "io"
    "net/http"

    "github.com/tinywasm/orm"
    "github.com/tinywasm/sqlt"
)

const (
    cfD1APIBase   = "https://api.cloudflare.com/client/v4"
    cfD1QueryPath = "/accounts/%s/d1/database/%s/query"
)

// directAdapter implements orm.Executor over the Cloudflare D1 REST API.
// Used only on the host (integration tests). Production uses adapter (JS binding).
type directAdapter struct {
    client *d1RestClient
}

type d1RestClient struct {
    token      string
    accountID  string
    databaseID string
    httpClient *http.Client
    baseURL    string
}

type d1QueryRequest struct {
    SQL    string `json:"sql"`
    Params []any  `json:"params"`
}

type d1QueryResult struct {
    Results []map[string]any `json:"results"`
    Success bool             `json:"success"`
}

type d1RestEnvelope struct {
    Success bool            `json:"success"`
    Errors  []d1RestError   `json:"errors"`
    Result  []d1QueryResult `json:"result"`
}

type d1RestError struct {
    Code    int    `json:"code"`
    Message string `json:"message"`
}

// NewDirect opens a D1 database via the REST API and returns an *orm.DB.
// Uses the same sqlt.NewCompiler() as the WASM adapter — identical SQL generation path.
// token, accountID, databaseID come from keyring or env vars (never hardcoded).
func NewDirect(token, accountID, databaseID string) (*orm.DB, error) {
    if token == "" || accountID == "" || databaseID == "" {
        return nil, fmt.Errorf(errPrefix + "token, accountID and databaseID are required")
    }
    a := &directAdapter{
        client: &d1RestClient{
            token:      token,
            accountID:  accountID,
            databaseID: databaseID,
            httpClient: http.DefaultClient,
            baseURL:    cfD1APIBase,
        },
    }
    return orm.New(a, sqlt.NewCompiler()), nil
}

func (c *d1RestClient) do(sql string, params []any) ([]map[string]any, error) {
    if params == nil {
        params = []any{}
    }
    body, err := json.Marshal(d1QueryRequest{SQL: sql, Params: params})
    if err != nil {
        return nil, fmt.Errorf(errPrefix+"marshal: %w", err)
    }
    path := fmt.Sprintf(cfD1QueryPath, c.accountID, c.databaseID)
    req, err := http.NewRequest(http.MethodPost, c.baseURL+path, bytes.NewReader(body))
    if err != nil {
        return nil, fmt.Errorf(errPrefix+"request: %w", err)
    }
    req.Header.Set("Authorization", "Bearer "+c.token)
    req.Header.Set("Content-Type", "application/json")

    resp, err := c.httpClient.Do(req)
    if err != nil {
        return nil, fmt.Errorf(errPrefix+"http: %w", err)
    }
    defer resp.Body.Close()

    data, err := io.ReadAll(resp.Body)
    if err != nil {
        return nil, fmt.Errorf(errPrefix+"read: %w", err)
    }
    var env d1RestEnvelope
    if err := json.Unmarshal(data, &env); err != nil {
        return nil, fmt.Errorf(errPrefix+"parse: %w", err)
    }
    if !env.Success {
        if len(env.Errors) > 0 {
            return nil, fmt.Errorf(errPrefix+"%s (code: %d)", env.Errors[0].Message, env.Errors[0].Code)
        }
        return nil, fmt.Errorf(errPrefix + "success=false")
    }
    if len(env.Result) == 0 {
        return nil, nil
    }
    return env.Result[0].Results, nil
}

// Exec implements orm.Executor for INSERT / UPDATE / DELETE.
func (a *directAdapter) Exec(query string, args ...any) error {
    _, err := a.client.do(query, args)
    return err
}

// QueryRow implements orm.Executor for single-row SELECT.
func (a *directAdapter) QueryRow(query string, args ...any) orm.Scanner {
    rows, err := a.client.do(query, args)
    if err != nil {
        return &errScanner{err}
    }
    if len(rows) == 0 {
        return &errScanner{orm.ErrNotFound}
    }
    return &directRowScanner{row: rows[0]}
}

// Query implements orm.Executor for multi-row SELECT.
func (a *directAdapter) Query(query string, args ...any) (orm.Rows, error) {
    rows, err := a.client.do(query, args)
    if err != nil {
        return nil, err
    }
    return &directRows{rows: rows}, nil
}

func (a *directAdapter) Close() error { return nil }

// directRowScanner scans a single row from the REST response (map[string]any).
type directRowScanner struct{ row map[string]any }

func (s *directRowScanner) Scan(dest ...any) error {
    i := 0
    for _, v := range s.row {
        if i >= len(dest) {
            break
        }
        if err := orm.ScanAny(v, dest[i]); err != nil {
            return err
        }
        i++
    }
    return nil
}

// directRows iterates over REST response rows.
type directRows struct {
    rows []map[string]any
    cur  int
}

func (r *directRows) Next() bool {
    if r.cur < len(r.rows) {
        r.cur++
        return true
    }
    return false
}

func (r *directRows) Scan(dest ...any) error {
    row := r.rows[r.cur-1]
    i := 0
    for _, v := range row {
        if i >= len(dest) {
            break
        }
        if err := orm.ScanAny(v, dest[i]); err != nil {
            return err
        }
        i++
    }
    return nil
}

func (r *directRows) Close() error { return nil }
func (r *directRows) Err() error   { return nil }
```

### Stage 2 — `goflare/d1/d1_integration_test.go` (new file)

The test uses `d1.NewDirect()` → `orm.DB` — the same API as the edge Worker.
The model `testItem` is defined inline (test-only); its `Schema()`/`Pointers()`/`ModelName()`
are written manually here because `ormc` is not run on test-only structs.

```go
//go:build integration && !wasm

package d1_test

import (
    "os"
    "testing"

    "github.com/tinywasm/goflare/d1"
    "github.com/tinywasm/orm"
    keyring "github.com/zalando/go-keyring"
)

const (
    envKeyToken      = "CLOUDFLARE_API_TOKEN"
    envKeyAccountID  = "CLOUDFLARE_ACCOUNT_ID"
    envKeyDatabaseID = "D1_DATABASE_ID"
    keyringService   = "goflare"
    testTable        = "_goflare_test"
)

// testItem is a minimal model for the integration test.
// ormc is not used for test-only structs — methods are written inline.
type testItem struct {
    ID   int    `json:"id"`
    Name string `json:"name"`
}

func (m *testItem) ModelName() string { return testTable }
func (m *testItem) Schema() []orm.Field {
    return []orm.Field{
        {Name: "id", PK: true},
        {Name: "name"},
    }
}
func (m *testItem) Pointers() []any { return []any{&m.ID, &m.Name} }

func resolveToken(t *testing.T) string {
    t.Helper()
    token, err := keyring.Get(keyringService, envKeyToken)
    if err == nil && token != "" {
        return token
    }
    token = os.Getenv(envKeyToken)
    if token != "" {
        return token
    }
    t.Skip("no token: run 'goflare auth' locally or set CLOUDFLARE_API_TOKEN in CI")
    return ""
}

func resolveEnv(t *testing.T, key string) string {
    t.Helper()
    v := os.Getenv(key)
    if v == "" {
        t.Skipf("env var %s not set", key)
    }
    return v
}

func TestD1Integration(t *testing.T) {
    token     := resolveToken(t)
    accountID := resolveEnv(t, envKeyAccountID)
    dbID      := resolveEnv(t, envKeyDatabaseID)

    db, err := d1.NewDirect(token, accountID, dbID)
    if err != nil {
        t.Fatalf("NewDirect: %v", err)
    }
    defer db.Close()

    // Setup table — same call as in the edge Worker
    if err := db.CreateTable(&testItem{}); err != nil {
        t.Fatalf("CreateTable: %v", err)
    }
    t.Cleanup(func() {
        db.DropTable(&testItem{}) //nolint
    })

    // Create
    item := &testItem{ID: 1, Name: "hello"}
    if err := db.Create(item); err != nil {
        t.Fatalf("Create: %v", err)
    }

    // Read one
    got := &testItem{}
    if err := db.First(got, orm.Where("id", 1)); err != nil {
        t.Fatalf("First: %v", err)
    }
    if got.Name != "hello" {
        t.Fatalf("expected name=hello, got %q", got.Name)
    }

    // Update
    item.Name = "world"
    if err := db.Save(item); err != nil {
        t.Fatalf("Save: %v", err)
    }

    // Verify update
    got2 := &testItem{}
    if err := db.First(got2, orm.Where("id", 1)); err != nil {
        t.Fatalf("First after Save: %v", err)
    }
    if got2.Name != "world" {
        t.Fatalf("expected name=world after update, got %q", got2.Name)
    }

    // Delete
    if err := db.Delete(item); err != nil {
        t.Fatalf("Delete: %v", err)
    }

    // Verify gone
    got3 := &testItem{}
    err = db.First(got3, orm.Where("id", 1))
    if err != orm.ErrNotFound {
        t.Fatalf("expected ErrNotFound after delete, got: %v", err)
    }
}
```

### Stage 3 — `goflare/go.mod`

`go-keyring` is already in `go.mod`. No new dependencies needed.

Run:
```bash
go mod tidy
```

## Stages Summary

| # | Archivo | Acción |
|---|---|---|
| 1 | `goflare/d1/client.go` | Nuevo (`//go:build !wasm`): `directAdapter`, `NewDirect`, `d1RestClient`, `directRows`, `directRowScanner` — escaneo via `orm.ScanAny` |
| 2 | `goflare/d1/d1_integration_test.go` | Nuevo (`//go:build integration && !wasm`): `TestD1Integration` usando `orm.DB` via `d1.NewDirect` |
| 3 | `goflare/go.mod` | `go mod tidy` |

## Verification

```bash
go install github.com/tinywasm/devflow/cmd/gotest@latest
gotest
```

Normal test suite passes with no regressions (`go vet`, `go test ./...`).

Integration test (requires credentials):
```bash
CLOUDFLARE_ACCOUNT_ID=xxx D1_DATABASE_ID=yyy go test -tags=integration -run TestD1Integration ./d1/ -v
```

The token is read from the OS keyring (set via `goflare auth`) or `CLOUDFLARE_API_TOKEN` env var.
If neither is set → `t.Skip` (never fails in red).

## CI/CD

Add to `.github/workflows/test.yml` to enable integration tests in CI:

```yaml
- name: Integration tests (D1)
  if: secrets.CLOUDFLARE_API_TOKEN != ''
  env:
    CLOUDFLARE_API_TOKEN: ${{ secrets.CLOUDFLARE_API_TOKEN }}
    CLOUDFLARE_ACCOUNT_ID: ${{ secrets.CLOUDFLARE_ACCOUNT_ID }}
    D1_DATABASE_ID: ${{ secrets.D1_DATABASE_ID }}
  run: go test -tags=integration -run TestD1Integration ./d1/ -v
```
