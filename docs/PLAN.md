> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.

# Plan: goflare — Fix Validate() Requiring Deploy-Only Fields for Build

## Root Cause

`RunBuild` calls `cfg.Validate()` which requires `ProjectName` and `AccountID`.
Neither field is used anywhere in the build pipeline — they are exclusively consumed
by `cloudflare.go` (Cloudflare Pages API calls) during **deploy**. Requiring them for
`build` forces CI workflows to inject deploy credentials into a step that never
contacts Cloudflare, and causes `goflare build` to fail with "AccountID is required"
when those env vars are absent.

## Evidence

All usages of `ProjectName` and `AccountID` are in `cloudflare.go`:

```
/accounts/{AccountID}/pages/projects/{ProjectName}/uploadToken   → deploy
/accounts/{AccountID}/pages/projects/{ProjectName}/deployments   → deploy
/accounts/{AccountID}/pages/projects/{ProjectName}/domains       → deploy
/accounts/{AccountID}/workers/scripts/{WorkerName}               → deploy
```

`build.go` never references either field. `RunBuild` only needs to know that
`Entry` or `PublicDir` exist (auto-detected from the project tree).

## Fix

Split `Validate()` into `ValidateBuild()` and `ValidateDeploy()`.

### Stage 1 — `goflare/config.go`

Replace the single `Validate()` with two purpose-scoped validators:

```go
// ValidateBuild checks only what goflare build requires.
// ProjectName and AccountID are deploy-only — never referenced by build.go.
func (c *Config) ValidateBuild() error {
	if c.Entry == "" && c.PublicDir == "" {
		return fmt.Errorf("nothing to build: Entry and PublicDir are both empty")
	}
	return nil
}

// ValidateDeploy checks everything required for a Cloudflare API deploy.
func (c *Config) ValidateDeploy() error {
	if c.ProjectName == "" {
		return fmt.Errorf("ProjectName is required (set PROJECT_NAME env var)")
	}
	if c.AccountID == "" {
		return fmt.Errorf("AccountID is required (set CLOUDFLARE_ACCOUNT_ID env var)")
	}
	if c.Entry == "" && c.PublicDir == "" {
		return fmt.Errorf("Entry and PublicDir cannot both be empty")
	}
	return nil
}
```

Delete the old `Validate()` method entirely.

### Stage 2 — `goflare/run.go`

Update callers:

```go
func RunBuild(envPath string, out io.Writer) error {
	cfg, err := LoadConfigFromEnv(envPath)
	if err != nil {
		return err
	}
	if err := cfg.ValidateBuild(); err != nil {
		return err
	}
	// ... rest unchanged
}

func RunDeploy(envPath string, out io.Writer) error {
	cfg, err := LoadConfigFromEnv(envPath)
	if err != nil {
		return err
	}
	if err := cfg.ValidateDeploy(); err != nil {
		return err
	}
	// ... rest unchanged
}
```

`RunAuth` does not call `Validate()` today — no change needed there.

### Stage 3 — Publish new version

Bump to **v0.2.17** (or next patch). After publishing, consumers like `goflare-demo`
can update `go.mod` and remove any workaround env vars added to their Build step.

---

## File Summary

| File | Action |
|---|---|
| `goflare/config.go` | Replace `Validate()` with `ValidateBuild()` + `ValidateDeploy()` |
| `goflare/run.go` | `RunBuild` → `ValidateBuild()`; `RunDeploy` → `ValidateDeploy()` |

---

## Verification

- `goflare build` succeeds with only `Entry`/`PublicDir` auto-detected (no `CLOUDFLARE_ACCOUNT_ID`, no `PROJECT_NAME`).
- `goflare deploy` still fails with a clear error when `AccountID` or `ProjectName` are missing.
- `goflare build` in CI without any Cloudflare secrets → compiles WASM correctly.
- Existing tests for `Validate()` are updated to target `ValidateBuild` / `ValidateDeploy`.
