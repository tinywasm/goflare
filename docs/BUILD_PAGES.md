# Building for Cloudflare Pages

GoFlare supports deploying static sites and "Advanced Mode" Pages projects (which include a `_worker.js` for dynamic logic).

## Build Process

When `PUBLIC_DIR` is set in your configuration, `goflare build` will:

1. Create a `.build/dist/` directory.
2. Recursively copy all files from your `PUBLIC_DIR` to `.build/dist/`.
3. If `ENTRY` is also set, the Worker build artifacts will also be prepared.

## Content Type Detection

During deployment, GoFlare automatically detects the `Content-Type` for your assets based on their file extensions:

- `.html`: `text/html`
- `.css`: `text/css`
- `.js`: `application/javascript`
- `.json`: `application/json`
- `.png`: `image/png`
- `.jpg`, `.jpeg`: `image/jpeg`
- `.svg`: `image/svg+xml`
- `.ico`: `image/x-icon`
- `.wasm`: `application/wasm`
- `.txt`: `text/plain`
- Others: `application/octet-stream`

## Direct Upload v2 Flow

GoFlare implements Cloudflare's Direct Upload v2 API for Pages:

1. **Upload JWT:** GoFlare requests a short-lived upload token from Cloudflare.
2. **File Batching:** Files are hashed (SHA-256) and uploaded in batches of up to 50 files.
3. **Manifest Deployment:** A final deployment request is sent containing the mapping of all file paths to their hashes.

## Custom Domains

If `DOMAIN` is provided in the configuration, GoFlare will attempt to attach the custom domain to your Pages project during deployment. If the domain is already attached or if DNS verification is pending, it will log a warning but proceed with the deployment.
