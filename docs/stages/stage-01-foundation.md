# Stage 01 — Foundation & Refactor

## Goal
Restructure the package so all logic is testable, `Config` is the single source of truth,
and the library/CLI boundary is clean.

## Status: done

Completed files: `goflare.go`, `config.go`, `store.go`, `workers.go`, `devtui.go`,
`tests/pages_test.go`, `tests/helpers_test.go`. Package compiles cleanly.

Note: `Auth` stub signature is `Auth(store Store, prompt io.Reader)` as required by Stage 04.

---

## Tasks

### 1.1 — Refactor `Config` struct (`goflare.go`)

Replace the current `Config` with a flat, serializable struct mapping directly to `.env` keys.
`APISubdomain` is intentionally absent — Workers run on `*.workers.dev` only.

```go
type Config struct {
    // Project identity
    ProjectName string // PROJECT_NAME
    AccountID   string // CLOUDFLARE_ACCOUNT_ID
    WorkerName  string // WORKER_NAME  (default: ProjectName + "-worker")

    // Routing
    Domain string // DOMAIN (optional — custom domain for Pages)

    // Build inputs
    Entry     string // ENTRY      (path to main Go file, empty = Pages only)
    PublicDir string // PUBLIC_DIR (path to static assets, empty = Worker only)

    // Build output (not in .env — always .goflare/)
    OutputDir string // default: ".goflare/"

    // Compiler
    CompilerMode string // "S" | "M" | "L"  default: "S"
}
```

**Validation rules:**
- `ProjectName` required
- `AccountID` required
- `Entry` and `PublicDir` cannot both be empty

### 1.2 — Add `LoadConfigFromEnv(path string) (*Config, error)` (`config.go`)

New file. Reads `.env` file and populates `Config`.
Falls back to OS environment variables if `.env` path is empty.
Applies defaults after loading.

```go
func LoadConfigFromEnv(path string) (*Config, error)
func (c *Config) Validate() error
func (c *Config) applyDefaults()
```

### 1.3 — Implement `.env` parser (stdlib only)

`config.go` includes a minimal parser using `bufio.Scanner`:
- Skip blank lines and lines starting with `#`
- Split on first `=`
- Strip optional surrounding quotes from values
- No external dependency

### 1.4 — Move and restructure tests

- Move `pages_test.go` → `tests/pages_test.go`
- Add `package goflare_test` (black-box testing)
- Add build tag `//go:build integration` to tests that call tinygo binary
- Add `tests/helpers_test.go` with shared test utilities (temp dir, mock HTTP server)

### 1.5 — Add `Store` interface with implementations (`store.go`)

```go
// Store abstracts keyring access for testability.
type Store interface {
    Get(key string) (string, error)
    Set(key, value string) error
}

// KeyringStore is the real implementation using go-keyring.
type KeyringStore struct{}

// MemoryStore is an in-memory Store exported for use by library consumers in tests.
// Safe for concurrent use.
type MemoryStore struct {
    mu   sync.Mutex
    data map[string]string
}

func NewMemoryStore() *MemoryStore
```

`KeyringStore` uses `go-keyring` with the key format `goflare/<ProjectName>`.
`MemoryStore` lives in the main package (not test-only) so library users can inject it
in their own tests without duplicating the implementation.

### 1.6 — Refactor `Goflare` struct and `Build` as method (`goflare.go`)

`Build` must be a method on `Goflare` because `generateWasmFile()` needs `tw *client.WasmClient`
which lives on the struct. A standalone `Build(cfg)` function cannot reach the client.

```go
type Goflare struct {
    tw     *client.WasmClient
    Config *Config  // exported so CLI can read it after LoadConfigFromEnv
    log    func(message ...any)
}

func (g *Goflare) Build() error
func (g *Goflare) Deploy(store Store) error
func (g *Goflare) Auth(store Store) error
```

`New(cfg *Config) *Goflare` constructs the WasmClient internally from cfg — callers do not
pass a `*client.WasmClient` directly.

### 1.7 — Fix `GenerateWorkerFiles()` stub

Return a real `error` instead of `nil` with a clear message:
`"worker build not yet implemented — see stage 03"`

This prevents silent failure until Stage 03 is complete.

### 1.8 — Remove `devtui.go` shortcuts that reference unimplemented paths

`BuildWorkerShortcut` handler should return the same error as above.

---

## Files Changed
- `goflare.go` — Config refactor, Goflare struct, Build/Deploy/Auth as methods
- `config.go` — new file
- `store.go` — new file (Store interface, KeyringStore, MemoryStore)
- `workers.go` — fix stub
- `devtui.go` — update shortcuts
- `tests/pages_test.go` — moved + build tag
- `tests/helpers_test.go` — new file
- `go.mod` — no new dependencies

## Files Deleted
- `pages_test.go` (moved to tests/)
