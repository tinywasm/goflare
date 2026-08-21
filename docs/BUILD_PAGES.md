# Building for Cloudflare Pages

GoFlare supports deploying static sites and "Advanced Mode" Pages projects.

## Build Process

GoFlare will automatically detect the public directory if `web/public/` exists (Convention). `goflare build` will:

1. **Verify PUBLIC_DIR:** Checks that the public directory exists.
2. **Delegate to sitec Build Pipeline:** Delegates static site compilation to `sitec`'s complete build pipeline. `sitec` scans the module, extracts declared producers (`RootCSS()`, `RenderCSS()`, `RenderHTML()`, `IconSvg()`, `Fonts()`, `RenderPages()`), compiles frontend WASM if `web/client.go` exists, generates `style.css`, `script.js`, SVG sprites, fonts, and `index.html` in memory, and writes the output to `PUBLIC_DIR`.

Note: A project that declares a `css.go` file without providing a valid producer (e.g. `RootCSS()` or `RenderCSS()`) will cause the build to fail loudly.

## Deployment

Deployment requires `CLOUDFLARE_API_TOKEN` and `CLOUDFLARE_ACCOUNT_ID` environment variables. It is designed to run in CI (e.g., GitHub Actions).

## Direct Upload v2 Flow

GoFlare implements Cloudflare's Direct Upload v2 API for Pages:

1. **Upload JWT:** GoFlare requests a short-lived upload token from Cloudflare.
2. **File Batching:** Files are hashed (SHA-256) and uploaded in batches of up to 50 files.
3. **Manifest Deployment:** A final deployment request is sent containing the mapping of all file paths to their hashes.

## Custom Domains

If `DOMAIN` is provided in the configuration, GoFlare will attempt to attach the custom domain to your Pages project during deployment.
