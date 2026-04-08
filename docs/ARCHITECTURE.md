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

### 3. Build Pipeline (`build.go`)
- **Worker Build:** Produces `worker.js`, `worker.wasm`, and `wasm_exec.js`.
- **Pages Build:** Copies static assets to `dist/` directory.
- **Orchestration:** `Build()` method detects which targets to build based on configuration.

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
├── init.go             # Project initialization logic
├── build.go            # Build orchestration (Workers & Pages)
├── auth.go             # Cloudflare authentication logic
├── cloudflare.go       # Cloudflare API client and deployers
├── run.go              # CLI runner functions
├── javascripts.go      # JS wrapper templates
├── wasm.go             # WASM compilation delegation
├── tests/              # Comprehensive test suite
└── cmd/goflare/        # CLI entry point (main.go)
```

## Design Principles

- **Convention over Configuration:** Default output directory is fixed to `.goflare/`.
- **Minimal Dependencies:** Prefers Go standard library; uses `tinywasm/client` for WASM.
- **Testability:** Internal HTTP client supports base URL injection for mock server testing.
- **Self-Contained:** No external tools like Node.js or Wrangler required.
