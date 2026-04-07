# Stage 06 — Deploy: Pages

## Goal
Full implementation of `(g *Goflare) DeployPages(store Store) error`.
Uses Cloudflare Pages Direct Upload v2 API (3-step process). Optionally configures a custom domain.

---

## Background: Cloudflare Pages Direct Upload v2

The Pages API does not accept a ZIP file. The correct flow is:

1. **Get upload JWT** — `POST /pages/projects/:name/uploadToken` → returns a short-lived JWT
2. **Upload files** — `POST /pages/assets/upload` (authenticated with JWT, up to 50 files per request)
   - Each file sent as JSON: `[{"key": "<sha256-hex>", "value": "<base64-content>", "metadata": {"contentType": "..."}}]`
3. **Create deployment** — `POST /pages/projects/:name/deployments`
   - Body: `{"files": {"/<path>": "<sha256-hex>"}, "branch": "main"}`

The `zipDir` function planned earlier is not needed. Remove it.

---

## Tasks

### 6.1 — Implement `(g *Goflare) DeployPages(store Store) error` (`cloudflare.go`)

**Pre-conditions:**
- `g.Config.PublicDir` is set (deploy decision based on Config, not file presence)
- `<OutputDir>/dist/` directory exists and is non-empty
- Token available via `g.GetToken(store)`

**Steps:**

1. Call `g.GetToken(store)` — error if missing
2. Ensure Pages project exists:
   - `GET /accounts/<AccountID>/pages/projects/<ProjectName>`
   - On 404 → `POST /accounts/<AccountID>/pages/projects` with `{"name":"<ProjectName>","production_branch":"main"}`
3. Get upload JWT:
   - `POST /accounts/<AccountID>/pages/projects/<ProjectName>/uploadToken`
   - Returns `{"jwt": "..."}` — used for file uploads only
4. Walk `<OutputDir>/dist/` and collect all files:
   - Compute SHA-256 hex of each file content
   - Build file list: `[]uploadFile{path, sha256, contentType, base64Content}`
5. Upload files in batches of 50:
   - `POST https://api.cloudflare.com/client/v4/pages/assets/upload`
   - Header: `Authorization: Bearer <jwt>`
   - Body: JSON array of file objects
6. Create deployment:
   - `POST /accounts/<AccountID>/pages/projects/<ProjectName>/deployments`
   - Body: `{"files": {"/path": "sha256hex", ...}, "branch": "main"}`
7. If `g.Config.Domain` is set → call `g.configurePagesDomain(client)`

### 6.2 — Internal type `uploadFile` (`cloudflare.go`)

```go
type uploadFile struct {
    key         string // sha256 hex
    path        string // relative path from dist/ root, with leading /
    contentType string
    value       string // base64-encoded content
}
```

### 6.3 — `detectContentType(filename string) string` (`cloudflare.go`)

Minimal content type detection by file extension using a local map.
Covers: `.html`, `.css`, `.js`, `.json`, `.png`, `.jpg`, `.svg`, `.ico`, `.wasm`, `.txt`.
Falls back to `application/octet-stream`.
No dependency on `net/http.DetectContentType` (requires reading file bytes).

### 6.4 — `(g *Goflare) configurePagesDomain(client *cfClient) error`

`POST /accounts/<AccountID>/pages/projects/<ProjectName>/domains`
```json
{"name": "<g.Config.Domain>"}
```

**Error handling:**
- Domain already attached → no-op (not an error)
- DNS not verified → log warning via `g.Logger`, do NOT fail deploy
- Any other error → log warning via `g.Logger`, do NOT fail deploy

### 6.5 — Tests (`tests/deploy_pages_test.go`)

Uses mock HTTP server and `MemoryStore` with pre-seeded token.

- `TestDeployPages_CreatesProjectIfMissing` — 404 on GET project triggers POST /projects
- `TestDeployPages_SkipsCreateIfExists` — 200 on GET project skips POST /projects
- `TestDeployPages_GetsUploadJWT` — POST /uploadToken called before file upload
- `TestDeployPages_UploadsFiles` — POST /assets/upload called with correct file batches
- `TestDeployPages_CreateDeployment` — POST /deployments called with correct manifest
- `TestDeployPages_BatchesOver50Files` — 51 files → two POST /assets/upload calls
- `TestDeployPages_NoDomainConfig_WhenDomainEmpty` — POST /domains not called if Domain empty
- `TestDeployPages_DomainWarningOnFailure` — domain POST failure does not fail overall deploy
- `TestDeployPages_EmptyDist` — error when dist/ is empty or missing
- `TestDeployPages_TokenMissing` — error when GetToken fails

---

## Files Changed
- `cloudflare.go` — rewrite DeployPages as method, add uploadFile, detectContentType, configurePagesDomain
- `tests/deploy_pages_test.go` — new file
