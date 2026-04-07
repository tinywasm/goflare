# Stage 08 — Tests

## Goal
Ensure all flows described in the diagram are covered by tests.
Unit tests use mocks. Integration tests (tinygo required) are gated behind a build tag.

---

## Test Structure

```
tests/
├── helpers_test.go         shared: mock HTTP server, temp dir utils
├── pages_test.go           GeneratePagesFiles (//go:build integration)
├── init_test.go            Init, WriteEnvFile, UpdateGitignore
├── build_test.go           Build flows (unit + integration)
├── auth_test.go            Auth, GetToken, validateToken
├── deploy_worker_test.go   DeployWorker
└── deploy_pages_test.go    DeployPages
```

Note: `MemoryStore` lives in the main package (`store.go`), not in helpers — no duplication needed.

---

## Tasks

### 8.1 — `tests/helpers_test.go`

**Mock HTTP server:**
```go
// MockCFServer wraps httptest.Server with route registration helpers.
func NewMockCFServer(t *testing.T) *MockCFServer
func (m *MockCFServer) Handle(method, path string, handler http.HandlerFunc)
func (m *MockCFServer) URL() string
```

Standard response fixtures (package-level helpers):
```go
func TokenVerifyOK() http.HandlerFunc
func TokenVerifyFail() http.HandlerFunc
func PagesProjectNotFound() http.HandlerFunc
func PagesProjectExists() http.HandlerFunc
func PagesUploadTokenOK(jwt string) http.HandlerFunc
func PagesAssetsUploadOK() http.HandlerFunc
func PagesDeploymentOK() http.HandlerFunc
```

**Temp dir utilities:**
```go
func TempDirWithFiles(t *testing.T, files map[string]string) string  // path → content
func AssertFileExists(t *testing.T, path string)
func AssertFileNotExists(t *testing.T, path string)
func AssertFileContent(t *testing.T, path, expected string)
```

### 8.2 — Build tags convention

```go
//go:build integration
```

Files with this tag require `tinygo` in PATH.
Run with: `go test ./tests/ -tags=integration`
Default `go test ./...` skips integration tests.

### 8.3 — Coverage targets (per flow in diagram)

| Flow | Test file | Tag |
|------|-----------|-----|
| Init — prompts return correct Config | init_test.go | unit |
| Init — error when both Entry and PublicDir empty | init_test.go | unit |
| Init — writes .env | init_test.go | unit |
| Init — omits empty fields from .env | init_test.go | unit |
| Init — updates .gitignore | init_test.go | unit |
| Init — gitignore idempotent | init_test.go | unit |
| Build — Worker only | build_test.go | integration |
| Build — Pages only | build_test.go | unit |
| Build — Both | build_test.go | integration |
| Build — nothing to build | build_test.go | unit |
| Build — Worker fail does not stop Pages | build_test.go | unit |
| Auth — token already in store | auth_test.go | unit |
| Auth — prompts, validates, stores token | auth_test.go | unit |
| Auth — invalid token not stored | auth_test.go | unit |
| Auth — key format goflare/ProjectName | auth_test.go | unit |
| GetToken — returns token after Auth | auth_test.go | unit |
| GetToken — errors if not authenticated | auth_test.go | unit |
| Deploy Worker — PUT to correct URL | deploy_worker_test.go | unit |
| Deploy Worker — correct multipart fields | deploy_worker_test.go | unit |
| Deploy Worker — missing artifact error | deploy_worker_test.go | unit |
| Deploy Worker — CF API error propagated | deploy_worker_test.go | unit |
| Deploy Pages — creates project if missing | deploy_pages_test.go | unit |
| Deploy Pages — skips create if exists | deploy_pages_test.go | unit |
| Deploy Pages — gets upload JWT | deploy_pages_test.go | unit |
| Deploy Pages — uploads files in batches | deploy_pages_test.go | unit |
| Deploy Pages — batches >50 files correctly | deploy_pages_test.go | unit |
| Deploy Pages — creates deployment with manifest | deploy_pages_test.go | unit |
| Deploy Pages — domain warning on failure | deploy_pages_test.go | unit |
| Deploy Pages — skips domain if Domain empty | deploy_pages_test.go | unit |
| Deploy Pages — error when dist/ empty | deploy_pages_test.go | unit |

---

## Files Added
- `tests/helpers_test.go`
- `tests/auth_test.go`
- `tests/build_test.go`
- `tests/deploy_worker_test.go`
- `tests/deploy_pages_test.go`

## Files Changed
- `tests/pages_test.go` — add `//go:build integration`
- `tests/init_test.go` — created in Stage 02, no changes needed
