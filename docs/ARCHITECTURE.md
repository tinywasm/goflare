# Architecture Guide

GoFlare is designed as a standalone Go library with a thin CLI wrapper. It eliminates the need for Node.js, Wrangler, or any JavaScript-based tooling in the Go WASM deployment pipeline.

## Package Structure

```
goflare/
├── goflare.go          # Core Goflare struct and New()
├── config.go           # Config struct and .env loading
├── store.go            # Store interface (KeyringStore, MemoryStore)
├── init.go             # Init() interactive wizard
├── build.go            # Build() orchestration logic
├── auth.go             # Token validation and Auth()
├── cloudflare.go       # Cloudflare API client (cfClient)
├── run.go              # CLI runner functions
├── workers.go          # Worker build convenience wrappers
├── pages.go            # Pages build convenience wrappers
├── wasm.go             # WASM compilation logic
└── javascripts.go      # Worker/Pages JS bridge generation
```

## Core Design Principles

1. **Config as Source of Truth:** All operations are driven by the `Config` struct. The CLI simply loads this struct from a `.env` file before executing library methods.
2. **Stateless Operations:** `Goflare` is initialized with a configuration, but authentication state (tokens) is externalized through the `Store` interface.
3. **Environment Isolation:** By default, artifacts are placed in a `.goflare/` directory, which is automatically added to `.gitignore` during initialization.

## Key Subsystems

### Configuration & .env
GoFlare includes a minimal, stdlib-only `.env` parser to avoid external dependencies. It supports basic key-value pairs and optional quotes.

### Build Orchestration
The `Build()` method handles:
- **Workers:** Compiles Go to WASM using `tinywasm/client` (delegating to TinyGo) and generates the JavaScript module entry point (`worker.js`).
- **Pages:** Recursively copies the static assets from the `PublicDir` into a `dist/` subdirectory within the `OutputDir`.

### Authentication
GoFlare uses a single Cloudflare API Token for all operations.
- Token is verified against `GET /user/tokens/verify`.
- Validated tokens are stored in the system keyring using the key format `goflare/<ProjectName>`.

### Deployment Pipeline
- **Workers:** Uses a single multipart/form-data `PUT` request to upload `worker.js`, `worker.wasm`, and `wasm_exec.js` to the Cloudflare API.
- **Pages:** Implements the Direct Upload v2 API:
  1. Requests a short-lived upload JWT.
  2. Computes SHA-256 hashes for all files.
  3. Uploads file assets in batches of 50.
  4. Finalizes the deployment by sending the complete file manifest.

## Testability
The library uses `io.Reader` and `io.Writer` injection for interactive prompts and console output, and a `Store` interface for keyring access, enabling robust unit testing without side effects.
