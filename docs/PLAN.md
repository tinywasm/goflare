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
| Auth | Direct API token (no scoped token creation) | Simpler flow; user creates token in CF dashboard once |
| Keyring key | `goflare/<ProjectName>` | Namespaced, supports multiple projects |
| WASM build | `tinywasm/client` (`tw.RecompileMainWasm()`) | Already encapsulates tinygo + wasm_exec.js + JS templates |
| Pages upload | Cloudflare Direct Upload v2 (3-step: JWT → file batches → deployment) | Only supported API; no ZIP endpoint exists in Pages API |
| Worker route | None — workers.dev only | No zone lookup, no custom route; simpler deploy, zero DNS requirements |
| Deploy errors | Independent per target, summary at end | Partial success is useful information |
| Custom domain | Attempt API call, warn on failure, never block deploy | DNS state is outside goflare's control |
| Tests | Unit tests with mock HTTP + `MemoryStore` (exported); integration behind build tag | Fast by default; MemoryStore exported so library consumers can use it |
| Build/Deploy API | Methods on `Goflare` struct, not standalone functions | `Build` needs `tw *client.WasmClient`; method pattern gives access without extra params |
| Prompt injection | `Init` and `Auth` accept `io.Reader` | Enables unit testing of prompt flows without terminal emulation |
| Deploy trigger | Based on `cfg.Entry/PublicDir`, not artifact file presence | Config is authoritative; file presence check is fragile after partial builds |
| CLI framework | `flag` stdlib only | No external deps, matches project philosophy |

## Stages

| Stage | Title | Status |
|-------|-------|--------|
| [01](stages/stage-01-foundation.md) | Foundation & Refactor | pending |
| [02](stages/stage-02-init.md) | Init Command | pending |
| [03](stages/stage-03-build.md) | Build Command | pending |
| [04](stages/stage-04-deploy-auth.md) | Deploy: Auth | pending |
| [05](stages/stage-05-deploy-worker.md) | Deploy: Worker | pending |
| [06](stages/stage-06-deploy-pages.md) | Deploy: Pages | pending |
| [07](stages/stage-07-cli.md) | CLI Wiring | pending |
| [08](stages/stage-08-tests.md) | Tests | pending |
| [09](stages/stage-09-docs.md) | Documentation Update | pending |

## Execution Order

Stages must be executed in order. Each stage's output is required by the next.

```
01 → 02 → 03 → 04 → 05 → 06 → 07 → 08 → 09
```

Stages 05 and 06 are independent of each other and can run in parallel after Stage 04.
Stage 09 must run last — it documents the final state of the implementation.

## Package Structure (target)

```
goflare/
├── README.md           Project documentation (root)
├── goflare.go          Goflare struct, New(), SetLog()
├── config.go           Config struct, LoadConfigFromEnv(), Validate()
├── store.go            Store interface, KeyringStore, MemoryStore
├── init.go             Init(), WriteEnvFile(), UpdateGitignore()
├── build.go            Build(), buildWorker(), buildPages()
├── auth.go             Auth(), GetToken(), validateToken()
├── run.go              RunInit, RunBuild, RunDeploy, Usage, WriteSummary, DeployResult
├── cloudflare.go       cfClient, DeployWorker, DeployPages, helpers
├── pages.go            GeneratePagesFiles() convenience wrapper
├── workers.go          GenerateWorkerFiles() convenience wrapper
├── javascripts.go      JS template generation (unchanged)
├── events.go           File event handling (unchanged)
├── devtui.go           DevTUI handler (updated shortcuts)
├── tests/
│   ├── helpers_test.go
│   ├── pages_test.go
│   ├── init_test.go
│   ├── build_test.go
│   ├── auth_test.go
│   ├── deploy_worker_test.go
│   └── deploy_pages_test.go
├── docs/
│   ├── ARCHITECTURE.md
│   ├── BUILD_PAGES.md
│   ├── BUILD_WORKERS.md
│   ├── QUICK_REFERENCE.md
│   ├── PLAN.md
│   ├── diagrams/
│   │   ├── goflare-generic.md
│   │   ├── AUTH_FLOW.md
│   │   └── DEPLOY_FLOW.md
│   └── img/
└── cmd/goflare/
    └── main.go         CLI: init / build / deploy subcommands
```

## Dependencies

### Added
- none

### Removed
- none

### Unchanged
- `github.com/zalando/go-keyring` — OS keyring
- `github.com/tinywasm/wizard` — interactive prompts

## Open Questions

None. All design decisions resolved.
