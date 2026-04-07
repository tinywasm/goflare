# Stage 02 — Init Command

## Goal
Implement `Init() (*Config, error)` as a library function and wire it to `goflare init` CLI subcommand.
Produces a `.env` file and updates `.gitignore`. Interactive only (no non-interactive mode in this stage).

---

## Tasks

### 2.1 — Implement `Init(prompt io.Reader) (*Config, error)` (`init.go`)

New file. Runs the interactive wizard and returns a populated `Config`.
Does NOT write the `.env` — the caller decides where to write it.
Accepts an `io.Reader` for the prompt source to enable testing without terminal emulation.
In production, pass `os.Stdin`.

**Prompt sequence:**
1. `Project name:` → `ProjectName` (required, no default)
2. `Cloudflare Account ID:` → `AccountID` (required, hint: `see dash.cloudflare.com → right sidebar`)
3. `Custom domain (leave empty for *.pages.dev):` → `Domain`
4. `Entry point (leave empty for Pages-only) [web/server.go]:` → `Entry`
5. `Public dir (leave empty for Worker-only) [web/public]:` → `PublicDir`
6. `Worker name [<ProjectName>-worker]:` → `WorkerName`

**Validation after all prompts:**
- If `Entry` empty AND `PublicDir` empty → return error "at least one of Entry or PublicDir is required"

Note: `APISubdomain` is not prompted — Workers run on `*.workers.dev` only.

Uses `tinywasm/wizard` for interactive prompts (already a transitive dep).

### 2.2 — `WriteEnvFile(cfg *Config, path string) error` (`init.go`)

Writes a `.env` file with all non-empty fields.

```
PROJECT_NAME=myapp
CLOUDFLARE_ACCOUNT_ID=abc123
DOMAIN=myapp.example.com
WORKER_NAME=myapp-worker
ENTRY=web/server.go
PUBLIC_DIR=web/public
```

### 2.3 — `UpdateGitignore(dir string) error` (`init.go`)

Reads `.gitignore` in `dir`. Appends `.env` and `.goflare/` if not already present.
Creates `.gitignore` if it does not exist.

### 2.4 — Tests (`tests/init_test.go`)

- `TestInit_PromptsAndReturnsConfig` — injects `io.Reader` with canned answers, asserts returned Config fields
- `TestInit_ErrorWhenBothEmpty` — Entry and PublicDir both empty → error returned
- `TestWriteEnvFile` — writes Config, reads back raw text, asserts all fields present
- `TestWriteEnvFile_OmitsEmptyFields` — optional fields with empty value are not written
- `TestUpdateGitignore_Creates` — creates new `.gitignore` with entries
- `TestUpdateGitignore_Appends` — appends to existing `.gitignore`
- `TestUpdateGitignore_Idempotent` — does not duplicate entries on second call

---

## Files Added
- `init.go`
- `tests/init_test.go`
