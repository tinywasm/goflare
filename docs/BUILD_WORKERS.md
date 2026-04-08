# Building for Cloudflare Workers

GoFlare enables you to run Go applications on Cloudflare Workers by compiling them to WebAssembly and providing a JavaScript bridge.

## Build Process

When `ENTRY` is set in your configuration, `goflare build` will:

1. Compile your Go source code to WASM using `tinywasm` (delegating to `tinygo` or `go`).
2. Generate `worker.js`, which serves as the entry point for the Cloudflare Worker.
3. Prepare `wasm_exec.js`, the standard Go/WASM bridge.
4. Output all artifacts to the `.goflare/` directory.

## Deployment

Deployment is done via a multipart upload to the Cloudflare Workers API.

### Upload Fields
- **`metadata`**: A JSON object specifying the `main_module` (e.g., `worker.js`).
- **`worker.js`**: The JavaScript wrapper script.
- **`worker.wasm`**: The compiled WebAssembly binary.
- **`wasm_exec.js`**: The Go WASM runtime support.

## workers.dev Only

GoFlare currently targets deployment to the `*.workers.dev` subdomain. It does not automatically configure custom routes or zones. After deployment, your worker will be live at `https://<worker-name>.<your-subdomain>.workers.dev`.
