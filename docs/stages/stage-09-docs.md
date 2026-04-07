# Stage 09 — Documentation Update

## Goal
Bring all documentation in sync with the final implementation.
Delete obsolete files, update existing ones, and ensure the root README reflects the new CLI and library API.

---

## Files to Delete

| File | Reason |
|------|--------|
| `docs/REVIEW.md` | Superseded by `docs/PLAN.md`; was a pre-implementation review snapshot |
| `docs/ADVANCED_MODE_UPDATE.md` | Content absorbed into `docs/BUILD_PAGES.md` and `docs/ARCHITECTURE.md` |
| `cmd/goflare/README.md` | CLI documentation moved to root `README.md` |

---

## Files to Update

### 9.1 — `README.md` (root)

Full rewrite. Must cover:

**Library usage:**
```go
cfg := &goflare.Config{
    ProjectName: "myapp",
    AccountID:   "abc123",
    Entry:       "web/server.go",
    PublicDir:   "web/public",
}
g := goflare.New(cfg)

// Build
if err := g.Build(); err != nil { ... }

// Deploy
store := goflare.NewKeyringStore()
if err := g.Auth(store, os.Stdin); err != nil { ... }
if err := g.DeployWorker(store); err != nil { ... }
if err := g.DeployPages(store); err != nil { ... }
```

**CLI usage:**
```
goflare init      # interactive setup → writes .env
goflare build     # compile worker and/or copy pages assets
goflare deploy    # authenticate and push to Cloudflare
```

**Config fields table** — all fields, their `.env` key, default, and whether required.

**Store interface** — brief note that `MemoryStore` is exported for use in tests.

**Requirements** — Go version, tinygo in PATH for Worker builds.

---

### 9.2 — `docs/ARCHITECTURE.md`

Update the following sections:

- **Package structure diagram** — reflect new files (`config.go`, `store.go`, `init.go`, `build.go`, `auth.go`)
- **Build process** — `Build()` as method on `Goflare`, delegation to `tinywasm/client`
- **Deploy process** — Pages Direct Upload v2 (3-step), Workers multipart upload
- **Auth flow** — direct token (no scoped token creation), keyring key format `goflare/<ProjectName>`
- **Config** — flat struct, `LoadConfigFromEnv`, `.env` parser (stdlib only)
- Remove any references to `APISubdomain`, ZIP upload, or scoped token creation

---

### 9.3 — `docs/BUILD_PAGES.md`

Update to reflect Pages Direct Upload v2:
- Replace old multipart description with the 3-step flow: upload JWT → file batches → deployment
- Document `detectContentType` and the 50-file batch limit
- Update generated file structure: `<OutputDir>/dist/` (no longer `pages/`)
- Remove references to ZIP upload
- Remove references to `wrangler.toml`

---

### 9.4 — `docs/BUILD_WORKERS.md`

Update to reflect workers.dev-only deployment:
- Remove custom route / zone lookup section
- Document multipart upload fields: `metadata`, `worker.js`, `worker.wasm`, `wasm_exec.js`
- Update generated file structure: `<OutputDir>/worker.js`, `<OutputDir>/worker.wasm`, `<OutputDir>/wasm_exec.js`
- Remove references to `wrangler.toml` and `deploy/` directory

---

### 9.5 — `docs/QUICK_REFERENCE.md`

Replace current content with CLI-first quick reference:
- Three commands with flags: `init`, `build`, `deploy`
- `Config` fields cheat sheet (field → `.env` key → default)
- Common patterns: Worker-only, Pages-only, both
- Troubleshooting: tinygo not in PATH, token invalid, Pages project already exists

---

### 9.6 — `docs/diagrams/AUTH_FLOW.md`

Rewrite sequence diagram:
- Remove bootstrap token and scoped token creation steps
- New flow: `goflare deploy` → check keyring → prompt if missing → `GET /user/tokens/verify` → store

---

### 9.7 — `docs/diagrams/DEPLOY_FLOW.md`

Rewrite sequence diagram to match the final implementation:
- Worker: `PUT /workers/scripts/:name` multipart → live on workers.dev
- Pages: `POST /uploadToken` → `POST /assets/upload` (batched) → `POST /deployments`
- Domain: `POST /domains` (optional, warn on failure)
- Summary output with URLs

---

## Files Unchanged

| File | Reason |
|------|--------|
| `docs/diagrams/goflare-generic.md` | Already updated in prior stages |
| `docs/PLAN.md` | This document — remains as implementation record |
| `docs/stages/*.md` | Stage files remain as implementation record |

---

## Files Changed Summary

| Action | File |
|--------|------|
| Delete | `docs/REVIEW.md` |
| Delete | `docs/ADVANCED_MODE_UPDATE.md` |
| Delete | `cmd/goflare/README.md` |
| Rewrite | `README.md` |
| Update | `docs/ARCHITECTURE.md` |
| Update | `docs/BUILD_PAGES.md` |
| Update | `docs/BUILD_WORKERS.md` |
| Rewrite | `docs/QUICK_REFERENCE.md` |
| Rewrite | `docs/diagrams/AUTH_FLOW.md` |
| Rewrite | `docs/diagrams/DEPLOY_FLOW.md` |
