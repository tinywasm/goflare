# Quick Reference

## CLI Commands

| Command | Description |
|---------|-------------|
| `goflare init` | Interactive project setup. Scaffolds `edge/main.go` with the imports for the chosen mode, creates `.env`, updates `.gitignore`. |
| `goflare init --mode=pages-functions` | Non-interactive scaffold for Pages Functions mode. |
| `goflare build` | Infers mode from `edge/main.go` imports and produces artifacts (see Build Outputs). |
| `goflare auth` | Stores `CLOUDFLARE_API_TOKEN` in the OS keyring. **Tokens never live in `.env`.** |
| `goflare deploy` | Direct Upload v2 to Cloudflare. ⚠️ **Status not fully verified** — primary deploy flow is CF Git Integration + `git push`. |

## Configuration (.env)

| Key | Description |
|-----|-------------|
| `PROJECT_NAME` | Cloudflare project name. |
| `CLOUDFLARE_ACCOUNT_ID` | Your Account ID (find in dash.cloudflare.com sidebar). |
| `WORKER_NAME` | (Optional) Name for your Worker. |
| `DOMAIN` | (Optional) Custom domain for Pages. |
| `ENTRY` | (Optional) Path to entry dir (default: auto-detect `edge/`). |
| `PUBLIC_DIR` | (Optional) Path to static assets (default: `web/public`). |
| `FUNCTIONS_DIR` | (Optional) Output dir for Pages Functions (default: `functions`). |
| `COMPILER_MODE` | `S` (Small), `M` (Medium), `L` (Large). |

**NEVER** put `CLOUDFLARE_API_TOKEN` in `.env`. The token lives in the OS keyring via `goflare auth`.

The project **mode** (Workers / Pages Functions / static Pages) is **inferred from `edge/main.go` imports**, not from `.env`. See [BUILD_PAGES_FUNCTIONS.md](BUILD_PAGES_FUNCTIONS.md).

## Build Outputs

### Workers mode (legacy, `.build/` — gitignored)

| File | Source | Description |
|------|--------|-------------|
| `.build/edge.js` | `edge/main.go` | Bundled & minified Worker script (`export default { fetch, ... }`). |
| `.build/edge.wasm` | `edge/main.go` | Compiled Worker WASM binary. |

### Pages Functions mode (`functions/` — committed to git)

| File | Source | Description |
|------|--------|-------------|
| `functions/[[path]].mjs` | bundle | Catch-all glue (`export const onRequest`). |
| `functions/edge.wasm` | `edge/main.go` | Compiled Pages Functions WASM binary. |

### Static Pages (`web/public/` — committed; produced by tinywasm framework, NOT by goflare)

| File | Source | Description |
|------|--------|-------------|
| `web/public/client.wasm` | `web/client.go` | Compiled frontend WASM. |
| `web/public/script.js` | (assetmin) | Minified WASM loader and app logic. |
| `web/public/style.css` | (assetmin) | Minified CSS bundle. |

## Common Scenarios

### Pages Functions (recommended for new projects)
`goflare init --mode=pages-functions`. `edge/main.go` imports `tinywasm/goflare/pages`. Commit `functions/` + `web/public/`. Push to GitHub. CF Git Integration deploys.

### Worker Only (legacy)
`edge/main.go` imports `tinywasm/goflare/workers`. Outputs to `.build/`. Deploy via `goflare deploy` (status not fully verified).

### Pages (Static) Only
No `edge/main.go`, only `web/public/`. Pure static site.

## Troubleshooting

- **TinyGo Not Found:** Ensure `tinygo` is in your `PATH` for any build that compiles wasm.
- **Heavy stdlib import in wasm code (D12 violation):** `grep -rE '^\s*"(fmt|strings|errors|encoding|net/http|log|io/ioutil)"' edge/ routes/ modules/ workers/ pages/pages.go cloudflare/env_wasm.go` must return empty. Use `tinywasm/fmt`, `tinywasm/json`, etc. instead. Heavy stdlib inflates the wasm binary ~80% and exceeds Cloudflare Free's 1 MiB limit.
- **Invalid Token:** Run `goflare auth` to refresh the token in the keyring.
- **Missing Account ID:** Find your Account ID in the right sidebar of the Cloudflare Dashboard.
