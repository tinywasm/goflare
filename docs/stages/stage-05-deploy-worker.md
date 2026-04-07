# Stage 05 — Deploy: Worker

## Goal
Full implementation of `(g *Goflare) DeployWorker(store Store) error`.
Uploads the Worker script via Cloudflare API. Worker is accessible at `*.workers.dev` only.

---

## Tasks

### 5.1 — Implement `(g *Goflare) DeployWorker(store Store) error` (`cloudflare.go`)

**Pre-conditions:**
- `g.Config.Entry` is set (deploy decision is based on Config, not file presence)
- Artifacts exist: `<OutputDir>/worker.wasm`, `<OutputDir>/worker.js`, `<OutputDir>/wasm_exec.js`
- Token available via `g.GetToken(store)`

**Steps:**

1. Call `g.GetToken(store)` — error if token missing (Auth not called)
2. Verify artifact files exist — error with specific missing file name
3. Build multipart body:
   - Field `metadata`: JSON `{"main_module": "worker.js"}`
   - File `worker.js` (content-type: `application/javascript+module`)
   - File `worker.wasm` (content-type: `application/wasm`)
   - File `wasm_exec.js` (content-type: `application/javascript`)
4. `PUT https://api.cloudflare.com/client/v4/accounts/<AccountID>/workers/scripts/<WorkerName>`
5. Return nil on success.

Worker is live at: `https://<WorkerName>.<AccountSubdomain>.workers.dev`

No zone lookup. No route configuration.

### 5.2 — Tests (`tests/deploy_worker_test.go`)

Uses mock HTTP server and `MemoryStore` with pre-seeded token.

- `TestDeployWorker_UploadScript` — verifies PUT to correct URL with correct multipart fields
- `TestDeployWorker_MetadataField` — metadata JSON contains `"main_module": "worker.js"`
- `TestDeployWorker_MissingArtifact` — error names the specific missing file
- `TestDeployWorker_TokenMissing` — error when GetToken fails (Auth not called)
- `TestDeployWorker_APIError` — CF returns error response, propagated with CF message

---

## Files Changed
- `cloudflare.go` — replace DeployWorker stub with full implementation as method
- `tests/deploy_worker_test.go` — new file
