# GoFlare — Implementation Plan

## Objective

Transform `goflare` into a self-contained Go tool (library + CLI) for deploying Go WASM projects
to Cloudflare Workers and Pages. No Node.js, no Wrangler, no GitHub Actions. Pure Go, direct
Cloudflare API.

## Reference Diagram

See [docs/diagrams/goflare-generic.md](diagrams/goflare-generic.md)

## Design Decisions

| Decision | Choice | Reason |
|----------|--------|--------|
| Config source | `Config` struct is source of truth; CLI loads it from `.env` | Library callers use `Config` directly; CLI users get `.env` convenience |
| Output dir | Fixed `.goflare/` (configurable via `Config.OutputDir`) | Convention over configuration; easy to gitignore |
| Auth | Direct API token, validated with `GET /user/tokens/verify` | Simpler flow; user creates token in CF dashboard once |
| Keyring key | `goflare/<ProjectName>` as the key, service name `"goflare"` | Namespaced per project; `KeyringStore.Get("goflare/myapp")` |
| Cloudflare API | Direct HTTP via internal `cfClient` struct — no cloudflare-go SDK | SDK is not mockable with `httptest.Server`; `cfClient` injects `baseURL` for tests |
| WASM build | `tinywasm/client` (`tw.RecompileMainWasm()`) | Already encapsulates tinygo + wasm_exec.js + JS templates |
| Pages upload | Cloudflare Direct Upload v2 (3-step: JWT → file batches → deployment) | Only supported API; no ZIP endpoint exists in Pages API |
| Worker route | None — workers.dev only | No zone lookup, no custom route; simpler deploy, zero DNS requirements |
| Deploy errors | Independent per target, summary at end | Partial success is useful information |
| Custom domain | Attempt API call, warn on failure, never block deploy | DNS state is outside goflare's control |
| Tests | Unit tests with mock HTTP (`httptest.Server`) + `MemoryStore`; integration behind `//go:build integration` | Fast by default; `MemoryStore` exported so library consumers can inject it |
| Build/Deploy API | Methods on `Goflare` struct | `Build` needs `tw *client.WasmClient` on the struct; consistent pattern for all operations |
| Prompt injection | `Init` and `Auth` accept `io.Reader` | Enables unit testing without terminal emulation |
| Deploy trigger | Based on `cfg.Entry/PublicDir`, not artifact file presence | Config is authoritative; file presence check is fragile after partial builds |
| CLI framework | `flag` stdlib only | No external deps |
| `.env` parser | `bufio.Scanner` stdlib | Trivial format; no external dep needed |

## cfClient — Testability Pattern

`cfClient` holds a `baseURL` that defaults to `https://api.cloudflare.com/client/v4`.
In tests, `httptest.NewServer` provides a local URL that is injected instead.
No interface, no SDK, no extra abstraction needed.

```go
type cfClient struct {
    token      string
    baseURL    string       // default: cfAPIBase; overridden in tests
    httpClient *http.Client
}
```

CLI path: `cfClient` constructed with `baseURL = cfAPIBase` after `Auth` completes.
Test path: `cfClient` constructed with `baseURL = mockServer.URL`.

## Pending Stages

| Stage | File | Title |
|-------|------|-------|
| 02 | [stage-02-init.md](stages/stage-02-init.md) | Init Command |
| 03 | [stage-03-build.md](stages/stage-03-build.md) | Build Command |
| 04 | [stage-04-deploy-auth.md](stages/stage-04-deploy-auth.md) | Deploy: Auth |
| 05 | [stage-05-deploy-worker.md](stages/stage-05-deploy-worker.md) | Deploy: Worker |
| 06 | [stage-06-deploy-pages.md](stages/stage-06-deploy-pages.md) | Deploy: Pages |
| 07 | [stage-07-cli.md](stages/stage-07-cli.md) | CLI Wiring |
| 08 | [stage-08-tests.md](stages/stage-08-tests.md) | Tests |
| 09 | [stage-09-docs.md](stages/stage-09-docs.md) | Documentation Update |

Execute in order. Stages 05 and 06 are independent and can run in parallel after Stage 04.
Stage 09 runs last.

## Current Package State

The following files already exist and must not be recreated:

| File | State |
|------|-------|
| `goflare.go` | `Config` struct, `Goflare` struct, `New()`, `Build()`/`Deploy()`/`Auth()` stubs |
| `config.go` | `LoadConfigFromEnv()`, `Validate()`, `applyDefaults()`, stdlib `.env` parser |
| `store.go` | `Store` interface, `KeyringStore`, `MemoryStore`, `NewMemoryStore()`, `NewKeyringStore()` |
| `workers.go` | `GenerateWorkerFiles()` returns error stub |
| `pages.go` | `GeneratePagesFiles()` — existing implementation, will delegate to `Build()` in Stage 03 |
| `javascripts.go` | JS template generation — do not modify |
| `events.go` | File event handling — do not modify |
| `devtui.go` | DevTUI handler with updated error for unimplemented Worker shortcut |
| `cloudflare.go` | Has old helpers (`addFilePart`, `parseCFResponse`) — refactor in Stage 04 |
| `tests/pages_test.go` | Moved, `//go:build integration` tag applied |
| `tests/helpers_test.go` | `TempDir()`, `MockHTTPServer()` helpers |

## Target Package Structure

```
goflare/
├── README.md           Project documentation (root)
├── goflare.go          Goflare struct, New(), SetLog()
├── config.go           Config struct, LoadConfigFromEnv(), Validate()
├── store.go            Store interface, KeyringStore, MemoryStore
├── init.go             Init(), WriteEnvFile(), UpdateGitignore()        ← Stage 02
├── build.go            Build(), buildWorker(), buildPages()             ← Stage 03
├── auth.go             Auth(), GetToken(), validateToken()              ← Stage 04
├── cloudflare.go       cfClient, DeployWorker, DeployPages, helpers    ← Stage 04-06
├── run.go              RunInit, RunBuild, RunDeploy, Usage, DeployResult ← Stage 07
├── pages.go            GeneratePagesFiles() delegates to buildPages()
├── workers.go          GenerateWorkerFiles() delegates to buildWorker()
├── javascripts.go      (unchanged)
├── events.go           (unchanged)
├── devtui.go           (updated)
├── tests/
│   ├── helpers_test.go
│   ├── pages_test.go        (//go:build integration)
│   ├── init_test.go         ← Stage 02
│   ├── build_test.go        ← Stage 03
│   ├── auth_test.go         ← Stage 04
│   ├── deploy_worker_test.go ← Stage 05
│   └── deploy_pages_test.go  ← Stage 06
├── docs/
└── cmd/goflare/
    └── main.go              ← Stage 07 (thin shell only)
```

## Dependencies

### Added
- none

### Removed
- none

### Unchanged
- `github.com/zalando/go-keyring` — OS keyring
- `github.com/tinywasm/wizard` — interactive prompts in `Init`
- `github.com/tinywasm/client` — WASM compilation and JS template generation
