# Cloudflare Workers Build Guide

GoFlare streamlines the process of building and deploying Go applications as Cloudflare Workers.

## Worker Overview

Go applications running as Workers are compiled to WebAssembly (WASM). To facilitate communication between the Cloudflare Worker runtime (V8) and the Go runtime, GoFlare generates a JavaScript entry point (`worker.js`).

## Build Process

GoFlare executes the following steps during the Worker build:

1. **WASM Compilation:** Uses `tinywasm/client` (TinyGo) to compile the Go source into a WebAssembly module (`worker.wasm`).
2. **Support File Generation:** Creates the `wasm_exec.js` file required by the Go WASM runtime.
3. **JS Entry Point Generation:** Generates the `worker.js` ES module with:
    - Imports for `wasm_exec.js` and `worker.wasm`.
    - Lifecycle hooks: `fetch`, `scheduled`, `queue`, and `onRequest`.
    - Glue code to bridge Cloudflare Worker events to the Go runtime.

## Configuration

Relevant `.env` keys for Worker builds:

- `PROJECT_NAME`: Used to derive the default worker name.
- `WORKER_NAME`: (Optional) The name of the worker on Cloudflare. Defaults to `<ProjectName>-worker`.
- `ENTRY`: Path to your Go main file or package (e.g., `cmd/worker/main.go`).
- `COMPILER_MODE`: (Optional) Compiler optimization mode (`S`, `M`, or `L`). Default is `S` (Small).

## Deployment

GoFlare uses the Cloudflare Workers scripts API:

1. **Artifact Verification:** Ensures `worker.js`, `worker.wasm`, and `wasm_exec.js` are present in the `.goflare/` directory.
2. **Metadata Creation:** Prepares a JSON metadata block identifying the main module (`worker.js`).
3. **Multipart Upload:** Sends the metadata, the JavaScript entry point, and the WebAssembly module in a single multipart/form-data `PUT` request.
4. **Live Deployment:** The worker is deployed to your `*.workers.dev` subdomain.

## Custom Domains

For Workers, GoFlare currently only supports deployment to `*.workers.dev` subdomains. Custom domain routing is not supported for Workers in the current version of GoFlare.
