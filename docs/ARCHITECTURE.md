# GoFlare Architecture

GoFlare is a Go library and CLI that bridges the gap between Go source code and Cloudflare's edge platforms (Workers and Pages). It automates the compilation, wrapper generation, and deployment process.

## Component Overview

```
┌─────────────┐
│ Go Source   │
│ (main.go)   │
└──────┬──────┘
       │
       ▼ [Build]
┌─────────────┐
│  tinywasm   │ (Compiles Go to WASM)
└──────┬──────┘
       │
       ▼
┌─────────────┐
│  goflare    │ (Generates JS wrappers)
└──────┬──────┘
       │
       ▼ [Deploy]
┌─────────────┐
│ Cloudflare  │ (Workers & Pages API)
└─────────────┘
```

## Core Modules

### 1. Configuration (`config.go`)
- **Flat Struct:** Simple `Config` struct that maps directly to `.env` keys.
- **Stdlib Parser:** Loads settings from `.env` using only standard library scanners.
- **Single Source of Truth:** Library callers use the struct directly; CLI users use the `.env` file.

### 2. Storage (`store.go`)
- **Keyring Integration:** Securely stores Cloudflare API tokens in the system keyring.
- **Service Name:** Uses `goflare` as the service name.
- **Key Format:** Tokens are stored per project if needed, or globally.
- **Memory Store:** An exported `MemoryStore` is provided for testing and library consumers.

### 3. Build Pipeline (`build.go`, `mode.go`, `javascripts.go`, `wasm.go`)
- **Mode Inference (`mode.go`):** `inferMode()` reads `edge/main.go` and inspects imports — `tinywasm/goflare/pages` → Pages Functions; `tinywasm/goflare/workers` → Workers; no entry but PublicDir → static Pages. `.env` does NOT carry a `MODE` variable; the code is the source of truth.
- **Worker Build:** Produces `.build/edge.js` (bundled) and `.build/edge.wasm`.
- **Pages Functions Build:** Produces `functions/[[path]].mjs` (catch-all, exports `onRequest`) and `functions/edge.wasm`. Outputs go directly to the project tree (no `.build/` staging) so they can be committed and served via CF Git Integration.
- **Static Pages Build:** Copies/delegates static assets (frontend WASM produced by the tinywasm framework, assetmin for JS/CSS).
- **Orchestration:** `Build()` dispatches to `buildWorker` / `buildPagesFunctions` / `buildPages` based on the inferred mode.

### 4. Authentication (`auth.go`)
- **Direct Token:** Validates Cloudflare API tokens via `GET /user/tokens/verify`.
- **Interactive:** Prompts the user for a token if one is not found in the keyring.

### 5. Deployment (`cloudflare.go`)
- **Internal HTTP Client:** `cfClient` handles direct interaction with Cloudflare API v4.
- **Workers Deploy:** Performs a multipart upload of the script, WASM, and runtime files.
- **Pages Deploy:** Implements the Direct Upload v2 flow (Upload JWT -> Batched Assets -> Deployment).

## Project Structure

```
goflare/
├── goflare.go          # Core Goflare struct and entry points
├── config.go           # Configuration loading and validation
├── store.go            # Keyring and memory storage abstractions
├── init.go             # Project initialization (scaffolds edge/main.go per mode)
├── mode.go             # Mode inference from edge/main.go imports
├── build.go            # Build orchestration (Workers, Pages, Pages Functions)
├── auth.go             # Cloudflare authentication logic
├── cloudflare.go       # Cloudflare API client and deployers
├── run.go              # CLI runner functions
├── javascripts.go      # JS bundling (worker.mjs + pages [[path]].mjs variants)
├── wasm.go             # WASM compilation delegation
├── router/             # Build-agnostic Router + Context interfaces
├── pages/              # Wasm Router impl (Pages Functions) — uses workers/ bridge
│   └── devserver/      # Native Router impl (//go:build !wasm) on http.ServeMux
├── workers/            # JS↔Go bridge (Request/Response, syscall/js, binding.handleRequest)
├── cloudflare/         # Dual-target env access (env_wasm.go + env_native.go)
├── tests/              # Comprehensive test suite
└── cmd/goflare/        # CLI entry point (main.go)
```

## Three project modes

| Inferred from `edge/main.go` import | Mode | Output |
|---|---|---|
| `github.com/tinywasm/goflare/pages` | `pages-functions` | `functions/[[path]].mjs` + `functions/edge.wasm` (committed to git) |
| `github.com/tinywasm/goflare/workers` | `workers` | `.build/edge.js` + `.build/edge.wasm` (gitignored, Direct Upload) |
| (no entry, only `web/public/`) | `pages` (static) | static assets only |

See [BUILD_PAGES_FUNCTIONS.md](BUILD_PAGES_FUNCTIONS.md), [BUILD_WORKERS.md](BUILD_WORKERS.md), [BUILD_PAGES.md](BUILD_PAGES.md).

## Design Principles

- **Convention over Configuration:** Default output directories are fixed (`.build/` for Workers, `functions/` for Pages Functions).
- **Code as source of truth (D11):** The build mode is inferred from `edge/main.go` imports, not from `.env`. No risk of desync between configuration and code.
- **Minimal binary in wasm code (D12):** Files compiled to wasm (under `edge/`, `routes/`, `modules/`, `workers/`, `pages/pages.go`, `cloudflare/env_wasm.go`) NEVER import heavy stdlib (`fmt`, `strings`, `errors`, `encoding/*`, `net/http`, `log`). Only `syscall/js`, `bytes`, and `tinywasm/*` (`tinywasm/fmt`, `tinywasm/json`, `tinywasm/fetch`, etc.). This keeps the binary <1 MiB to fit Cloudflare Free's wasm limit.
- **Native code is unrestricted:** `web/server.go`, `pages/devserver/`, and any `//go:build !wasm` file can use stdlib freely — they don't run on the edge.
- **Shared logic via interfaces (D4b):** `router.Router` and `router.Context` are pure interfaces shared between wasm and native; handlers in `modules/*/handler.go` are build-agnostic.
- **Artifacts in git (D8):** `functions/` outputs and `web/public/*` assets are committed; CF Git Integration deploys what's in the repo. Tradeoff accepted: small binary growth in history, immediate size visibility in `git diff`.
- **Secrets in keyring (never in `.env`):** `CLOUDFLARE_API_TOKEN` is stored via `goflare auth` in the OS keyring (`store.go`). `.env` only carries non-secret identifiers.
- **Self-Contained:** No external tools like Node.js or Wrangler required.
