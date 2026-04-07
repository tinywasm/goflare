# Stage 04 — Deploy: Auth

## Goal
Implement token validation and keyring storage as a method `(g *Goflare) Auth(store Store) error`.
Called at the start of every `deploy` invocation. Accepts an `io.Reader` for the prompt to enable
testing without terminal emulation.

---

## Tasks

### 4.1 — Implement `(g *Goflare) Auth(store Store, prompt io.Reader) error` (`auth.go`)

New file.

**Logic:**
```
key = "goflare/" + g.Config.ProjectName
token, err = store.Get(key)
if err or token == "" →
    read token from prompt: "Cloudflare API Token:"
    call validateToken(token)
    if invalid → return error "invalid token: <cf error message>"
    store.Set(key, token)
return nil
```

### 4.2 — `GetToken(store Store) (string, error)` method (`auth.go`)

Reads the token from the store without prompting. Used by `DeployWorker` and `DeployPages`
after `Auth` has already validated and stored the token.

```go
func (g *Goflare) GetToken(store Store) (string, error)
```

Returns an error if the token is not in the store (i.e., `Auth` was not called first).

### 4.3 — `validateToken(token string) error` (`auth.go`)

Package-level function (not exported). Calls `GET /user/tokens/verify`.
Returns nil on HTTP 200 + `success: true`.
Returns descriptive error on failure including CF error messages.

### 4.4 — HTTP client (`cloudflare.go` refactor)

Consolidate the existing `doGET`/`doPOST` helpers into a single internal client:

```go
type cfClient struct {
    token      string
    httpClient *http.Client
}
func (c *cfClient) get(path string) ([]byte, error)
func (c *cfClient) post(path string, body []byte) ([]byte, error)
func (c *cfClient) put(path string, body []byte) ([]byte, error)
func (c *cfClient) putMultipart(path string, body io.Reader, contentType string) ([]byte, error)
```

`cfClient` is constructed with the token after `Auth` completes.

### 4.5 — Tests (`tests/auth_test.go`)

Uses mock HTTP server and `MemoryStore`.

- `TestAuth_TokenAlreadyInStore` — store has token, no prompt read, no HTTP call made
- `TestAuth_ValidatesAndStores` — store empty, `io.Reader` provides token, mock server returns valid, token saved in store
- `TestAuth_InvalidToken` — mock server returns invalid response, error returned, token not saved
- `TestAuth_KeyFormat` — token stored under key `goflare/<ProjectName>`
- `TestGetToken_ReturnsToken` — after Auth, GetToken returns same token
- `TestGetToken_ErrorWhenMissing` — GetToken errors if Auth was not called

---

## Files Added
- `auth.go`
- `tests/auth_test.go`

## Files Changed
- `cloudflare.go` — extract cfClient struct, remove old doGET/doPOST helpers
