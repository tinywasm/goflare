# Quick Reference

## CLI Commands

| Command | Description |
|---------|-------------|
| `goflare init` | Interactive project setup. Creates `.env` and updates `.gitignore`. |
| `goflare build` | Compiles WASM and prepares static assets in `.build/`. |
| `goflare deploy` | Authenticates and pushes artifacts to Cloudflare. |

## Configuration (.env)

| Key | Description |
|-----|-------------|
| `PROJECT_NAME` | Cloudflare project name. |
| `CLOUDFLARE_ACCOUNT_ID` | Your Account ID (find in dash.cloudflare.com sidebar). |
| `WORKER_NAME` | (Optional) Name for your Worker. |
| `DOMAIN` | (Optional) Custom domain for Pages. |
| `ENTRY` | (Optional) Path to main Go file. |
| `PUBLIC_DIR` | (Optional) Path to static assets. |
| `COMPILER_MODE` | `S` (Small), `M` (Medium), `L` (Large). |

## Build Outputs (.build/)

| File/Dir | Source | Description |
|----------|--------|-------------|
| `edge.js` | `edge/main.go` | Bundled & minified Worker script. |
| `edge.wasm` | `edge/main.go` | Compiled Worker WASM binary. |
| `dist/` | `web/public/` | Mirror of PublicDir for Pages upload. |
| `dist/client.wasm` | `web/client.go` | Compiled frontend WASM. |
| `dist/script.js` | (assetmin) | Minified WASM loader and app logic. |
| `dist/style.css` | (assetmin) | Minified CSS bundle. |

## Common Scenarios

### Worker Only
Set `ENTRY`, leave `PUBLIC_DIR` empty.

### Pages (Static) Only
Set `PUBLIC_DIR`, leave `ENTRY` empty.

### Both (Pages with Functions)
Set both `ENTRY` and `PUBLIC_DIR`.

## Troubleshooting

- **TinyGo Not Found:** Ensure `tinygo` is in your `PATH` for Worker builds.
- **Invalid Token:** Run `goflare deploy` again; it will prompt for a new token if authentication fails.
- **Missing Account ID:** Find your Account ID in the right sidebar of the Cloudflare Dashboard.
