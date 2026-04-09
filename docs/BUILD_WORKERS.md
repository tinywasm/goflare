# Building for Cloudflare Workers

GoFlare enables you to run Go applications on Cloudflare Workers by compiling them to WebAssembly and providing a JavaScript bridge.

## Build Process

When `ENTRY` is set in your configuration, `goflare build` will:

1. Compile your Go source code to WASM using `tinywasm` (delegating to `tinygo`).
2. Generate `edge.js`, which bundles the entry point, runtime, and `wasm_exec.js`.
3. Output all artifacts to the `.build/` directory.

## Deployment

Deployment is done via a multipart upload to the Cloudflare Workers API.

### Upload Fields
- **`metadata`**: A JSON object specifying the `main_module` (`edge.js`).
- **`edge.js`**: The bundled JavaScript wrapper script.
- **`edge.wasm`**: The compiled WebAssembly binary.

## workers.dev Only

GoFlare currently targets deployment to the `*.workers.dev` subdomain. It does not automatically configure custom routes or zones. After deployment, your worker will be live at `https://<worker-name>.<your-subdomain>.workers.dev`.
