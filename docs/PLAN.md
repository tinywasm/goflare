> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.

# Plan: goflare d1 init — Comando + tests del flujo completo

## Context

Implementar `goflare d1 init` que automatiza el setup de D1 + GitHub en un solo comando.
El flujo completo está documentado en `docs/diagrams/CLOUDFLARE_GH_ENV_FLOW.md`.

Los tests de este plan son **mocks del diagrama** — cada nodo de decisión del diagrama
tiene un test correspondiente. El objetivo es capturar el comportamiento para detectar
regresiones si el flujo cambia.

## Code quality rules

- No string literals — todos los env keys, paths, mensajes de error: constantes exportadas.
- Thin `cmd/` — toda la lógica en el paquete `goflare`, no en `main.go`.
- Interfaces para todo lo inyectable (CF API, gh CLI, env file) — testables sin red.
- `RunD1Init` es la función de librería; `main.go` solo parsea flags y llama.
- **No stdlib `fmt` ni `errors`** — usar `. "github.com/tinywasm/fmt"` (dot-import).
  - `Err("msg")` reemplaza `errors.New("msg")` y `fmt.Errorf("msg")`
  - `Errf("format", args...)` reemplaza `fmt.Errorf("format", args...)`
  - `Sprintf` / `Fprintf` / `Fprintln` de `tinywasm/fmt` reemplazan los de stdlib.
- **Build tag `//go:build !wasm`** en todos los archivos nuevos y en los archivos existentes
  modificados que contengan código host-only (keyring, `os/exec`, `net/http`, CF API).
  Esto evita contaminar el binario WASM del worker/pages con dependencias innecesarias.
  Patrón establecido en el codebase: ver `d1/client.go`.

## Interfaces nuevas

### `D1Manager` — abstrae las llamadas CF API para D1

```go
// D1Manager abstracts the Cloudflare D1 REST API for listing and creating databases.
type D1Manager interface {
    ListD1Databases(accountID string) ([]D1Database, error)
    CreateD1Database(accountID, name string) (D1Database, error)
}

type D1Database struct {
    UUID string `json:"uuid"`
    Name string `json:"name"`
}
```

Implementación real: `cfD1Manager` (usa `cfClient` ya existente).
Implementación test: `mockD1Manager` (struct con campos funcionales).

### `GHRunner` — abstrae `gh` CLI

```go
// GHRunner abstracts the gh CLI for setting GitHub secrets and variables.
type GHRunner interface {
    SetSecret(repo, name, value string) error
    SetVariable(repo, name, value string) error
    RemoteURL() (string, error) // git remote get-url origin
    Available() bool            // gh CLI present in PATH
}
```

Implementación real: `execGHRunner` (usa `os/exec`).
Implementación test: `mockGHRunner`.

### `EnvWriter` — abstrae escritura de `.env`

```go
// EnvWriter abstracts writing a key=value pair to the .env file.
type EnvWriter interface {
    WriteKey(path, key, value string) error
}
```

Implementación real: `fileEnvWriter` (lee el archivo, actualiza o añade la línea).
Implementación test: `mockEnvWriter`.

## Constantes nuevas — ubicación exacta

### `goflare/config.go` — env keys (junto a las existentes PROJECT_NAME, CLOUDFLARE_ACCOUNT_ID)

Los env keys ya existentes (`PROJECT_NAME`, `CLOUDFLARE_ACCOUNT_ID`) están como string literals
en `LoadConfigFromEnv`. **Extraerlos como constantes exportadas** en `config.go` para que
`d1init.go` y los tests los referencien sin duplicar strings:

```go
// goflare/config.go — añadir al inicio del archivo, antes de LoadConfigFromEnv
const (
    EnvKeyProjectName    = "PROJECT_NAME"
    EnvKeyAccountID      = "CLOUDFLARE_ACCOUNT_ID"
    EnvKeyWorkerName     = "WORKER_NAME"
    EnvKeyDomain         = "DOMAIN"
    EnvKeyEntry          = "ENTRY"
    EnvKeyPublicDir      = "PUBLIC_DIR"
    EnvKeyCompilerMode   = "COMPILER_MODE"
    EnvKeyD1DatabaseID   = "D1_DATABASE_ID"    // nuevo — usado por d1init y test de integración
    EnvKeyD1DatabaseName = "D1_DATABASE_NAME"  // nuevo — nombre lógico de la DB (default: PROJECT_NAME)
)
```

Actualizar el `switch key` en `LoadConfigFromEnv` y el fallback OS env para usar estas constantes
en lugar de los strings literales actuales.

**NO modificar `WriteEnvFile`** — la escritura de `D1_DATABASE_ID` la maneja exclusivamente
`fileEnvWriter.WriteKey` en el flujo `d1 init`. `WriteEnvFile` solo se usa en `goflare init`.

Añadir a `Config` struct:
```go
D1DatabaseID   string // D1_DATABASE_ID   — set by `goflare d1 init`
D1DatabaseName string // D1_DATABASE_NAME — optional, default: ProjectName
```

Añadir lectura de `EnvKeyD1DatabaseID` y `EnvKeyD1DatabaseName` al `switch key` en
`LoadConfigFromEnv`.

### `goflare/d1init.go` — errores y constantes GitHub (solo usadas en el flujo d1 init)

```go
// goflare/d1init.go
import . "github.com/tinywasm/fmt"

var (
    ErrNoToken     = Err("not authenticated: run 'goflare auth' first")
    ErrNoAccountID = Err("CLOUDFLARE_ACCOUNT_ID missing: run 'goflare init' first")
    ErrNoDBName    = Err("database name is required (set PROJECT_NAME in .env)")
)

const (
    GHSecretToken   = "CLOUDFLARE_API_TOKEN"  // va como Secret en GitHub
    GHVarAccountID  = "CLOUDFLARE_ACCOUNT_ID" // va como Variable en GitHub
    GHVarDatabaseID = "D1_DATABASE_ID"        // va como Variable en GitHub
)
```

Los errores son `var` de tipo `error` (no `const string`) para permitir `errors.Is` en los tests.

## `RunD1Init` — función de librería

**Orden de pasos:** cargar config ANTES de leer el token, para construir la keyring key
`"goflare/" + cfg.ProjectName`. Esto evita pasar la key como parámetro extra.

```go
// RunD1Init implements `goflare d1 init`.
// Flow matches docs/diagrams/CLOUDFLARE_GH_ENV_FLOW.md exactly.
func RunD1Init(envPath, dbName string, store Store, d1 D1Manager, gh GHRunner, w EnvWriter, out io.Writer) error {
    // 1. Load config from .env (needed to build the keyring key)
    cfg, _ := LoadConfigFromEnv(envPath)

    // 2. Token from keyring — key: "goflare/<project>"
    token, err := store.Get("goflare/" + cfg.ProjectName)
    if err != nil || token == "" {
        return ErrNoToken
    }

    // 3. AccountID from .env
    if cfg.AccountID == "" {
        return ErrNoAccountID
    }

    // 4. DB name: flag > PROJECT_NAME
    if dbName == "" {
        dbName = cfg.ProjectName
    }
    if dbName == "" {
        return ErrNoDBName
    }

    // 5. List existing D1 databases
    dbs, err := d1.ListD1Databases(cfg.AccountID)
    if err != nil {
        return err
    }

    // 6. Find or create
    var dbID string
    for _, db := range dbs {
        if db.Name == dbName {
            dbID = db.UUID
            Fprintf(out, "D1: reusing existing database %q (%s)\n", dbName, dbID)
            break
        }
    }
    if dbID == "" {
        created, err := d1.CreateD1Database(cfg.AccountID, dbName)
        if err != nil {
            return err
        }
        dbID = created.UUID
        Fprintf(out, "D1: created database %q (%s)\n", dbName, dbID)
    }

    // 7. Write D1_DATABASE_ID to .env
    if err := w.WriteKey(envPath, EnvKeyD1DatabaseID, dbID); err != nil {
        return err
    }

    // 8. GitHub via gh CLI
    if !gh.Available() {
        Fprintf(out, "gh CLI not found. Set manually:\n")
        Fprintf(out, "  gh secret set %s --body %q\n", GHSecretToken, token)
        Fprintf(out, "  gh variable set %s --body %q\n", GHVarAccountID, cfg.AccountID)
        Fprintf(out, "  gh variable set %s --body %q\n", GHVarDatabaseID, dbID)
        return nil
    }

    repo, err := gh.RemoteURL()
    if err != nil {
        return err
    }
    gh.SetSecret(repo, GHSecretToken, token)
    gh.SetVariable(repo, GHVarAccountID, cfg.AccountID)
    gh.SetVariable(repo, GHVarDatabaseID, dbID)
    Fprintln(out, "GitHub: secrets and variables configured.")
    return nil
}
```

## Changes

### Stage 0 — añadir `//go:build !wasm` a archivos host-only existentes

**Deuda técnica existente.** Ninguno de los archivos del paquete raíz `goflare` tiene el build
tag, pero todos usan stdlib pesada o dependencias incompatibles con WASM. Añadir `//go:build !wasm`
como primera línea en cada uno de ellos antes de cualquier otro cambio.

Archivos a etiquetar — confirmados por imports:

| Archivo | Razón |
|---|---|
| `goflare/auth.go` | `os/exec`, `go-keyring`, `net/http` |
| `goflare/build.go` | `tinywasm/client`, `tinywasm/assetmin`, `tinywasm/gobuild` |
| `goflare/cloudflare.go` | `net/http`, `os`, `bufio`, `mime/multipart` |
| `goflare/config.go` | `os`, `bufio` (lee `.env` del filesystem) |
| `goflare/goflare.go` | `tinywasm/client`, `tinywasm/assetmin`, `tinywasm/js` |
| `goflare/init.go` | `os`, `bufio` (wizard interactivo, escribe filesystem) |
| `goflare/javascripts.go` | `os`, `embed`, `tdewolff/minify` |
| `goflare/mode.go` | `os`, `bufio` |
| `goflare/run.go` | `net/http`, `go-keyring` |
| `goflare/store.go` | `go-keyring` |

Archivos que **no** llevan el tag (compartidos o ya etiquetados):

| Archivo | Razón |
|---|---|
| `goflare/devtui.go` | solo métodos sobre `Goflare` sin imports stdlib pesada |
| `goflare/events.go` | solo `path/filepath` — revisar; si solo lo usa el host, añadir tag |
| `goflare/pages.go` | revisar imports antes de decidir |
| `goflare/workers.go` | revisar imports antes de decidir |
| `goflare/wasm.go` | genera el archivo WASM — es host-only; añadir `//go:build !wasm` |

Para cada archivo de la lista "a etiquetar", insertar como primera línea:
```go
//go:build !wasm
```
seguido de una línea en blanco antes del `package goflare`.

### Stage 1 — `goflare/config.go` — extraer env keys como constantes

**Archivo existente — solo modificaciones.**

1. Añadir el bloque `const` con `EnvKeyProjectName`, `EnvKeyAccountID`, ..., `EnvKeyD1DatabaseID`,
   `EnvKeyD1DatabaseName` al inicio del archivo (antes de `LoadConfigFromEnv`).
2. Reemplazar todos los string literals dentro del `switch key` en `LoadConfigFromEnv`
   y en el bloque de fallback OS env por las constantes correspondientes.
3. Añadir lectura de `EnvKeyD1DatabaseID` y `EnvKeyD1DatabaseName` al `switch key`
   y guardarlos en `Config.D1DatabaseID` y `Config.D1DatabaseName` (campos nuevos en `Config`).
4. Añadir a `Config` struct:
   ```go
   D1DatabaseID   string // D1_DATABASE_ID   — set by `goflare d1 init`
   D1DatabaseName string // D1_DATABASE_NAME — optional, default: ProjectName
   ```
5. **No tocar `WriteEnvFile`** — no escribe campos D1.

### Stage 2 — `goflare/d1init.go` (new file)

**Nuevo archivo en el paquete `goflare`** (no en `d1/` — opera a nivel de proyecto, no de DB).

Primera línea obligatoria:
```go
//go:build !wasm
```

Contiene en este orden:
1. Dot-import: `. "github.com/tinywasm/fmt"`
2. Errores sentinela: `var ErrNoToken`, `ErrNoAccountID`, `ErrNoDBName`
3. Constantes GitHub: `GHSecretToken`, `GHVarAccountID`, `GHVarDatabaseID`
4. Interfaces: `D1Manager`, `GHRunner`, `EnvWriter`
5. Tipo: `D1Database`
6. Función: `RunD1Init` (lógica del diagrama — ver pseudocódigo arriba)
7. Implementaciones reales: `cfD1Manager`, `execGHRunner`, `fileEnvWriter`

`cfD1Manager` usa el `cfClient` unexported existente en `cloudflare.go`:
```go
type cfD1Manager struct{ client *cfClient }

func (m *cfD1Manager) ListD1Databases(accountID string) ([]D1Database, error) {
    path := Sprintf("/accounts/%s/d1/database", accountID)
    data, err := m.client.get(path)
    if err != nil {
        return nil, err
    }
    // unmarshal []D1Database from data["result"]
    var result []D1Database
    // json.Unmarshal into result
    return result, nil
}

func (m *cfD1Manager) CreateD1Database(accountID, name string) (D1Database, error) {
    path := Sprintf("/accounts/%s/d1/database", accountID)
    body, _ := json.Marshal(map[string]string{"name": name})
    data, err := m.client.post(path, body)
    if err != nil {
        return D1Database{}, err
    }
    // unmarshal D1Database from data["result"]
    var result D1Database
    // json.Unmarshal into result
    return result, nil
}
```

`fileEnvWriter.WriteKey` reads `.env`, replaces the line if key exists, appends if not.

`execGHRunner`:
- `Available()` → `exec.LookPath("gh") == nil`
- `RemoteURL()` → `exec.Command("git", "remote", "get-url", "origin")`
- `SetSecret()` → `exec.Command("gh", "secret", "set", name, "--body", value, "--repo", repo)`
- `SetVariable()` → `exec.Command("gh", "variable", "set", name, "--body", value, "--repo", repo)`

### Stage 3 — `goflare/run.go` — `RunD1InitCmd` wrapper

Add to `run.go`:
```go
func RunD1InitCmd(envPath, dbName string) error {
    store := NewKeyringStore()
    client := &cfClient{baseURL: cfAPIBase, httpClient: http.DefaultClient}
    // token se obtiene dentro de RunD1Init via store — no se lee aquí para evitar doble lectura
    return RunD1Init(envPath, dbName, store, &cfD1Manager{client}, &execGHRunner{}, &fileEnvWriter{}, os.Stdout)
}
```

**Nota:** `cfClient` se construye sin token aquí porque `RunD1Init` valida el token primero
y el `cfD1Manager` solo necesita el client cuando llega al paso 5 (list/create). El token
se inyecta en las llamadas HTTP via el `cfClient` que recibe el token en su campo — actualizar
`cfD1Manager` para recibir el token en `ListD1Databases`/`CreateD1Database`, o alternativamente
pasar el token al `cfClient` dentro de `RunD1InitCmd` tras que `RunD1Init` lo haya validado.

La forma más simple: `cfD1Manager` recibe el token en construcción igual que los otros clients:
```go
func RunD1InitCmd(envPath, dbName string) error {
    store := NewKeyringStore()
    cfg, _ := LoadConfigFromEnv(envPath)
    token, _ := store.Get("goflare/" + cfg.ProjectName)
    // token puede estar vacío — RunD1Init retornará ErrNoToken antes de usar d1
    client := &cfClient{token: token, baseURL: cfAPIBase, httpClient: http.DefaultClient}
    return RunD1Init(envPath, dbName, store, &cfD1Manager{client}, &execGHRunner{}, &fileEnvWriter{}, os.Stdout)
}
```

### Stage 4 — `cmd/goflare/main.go` — subcomando `d1 init`

Agregar al `switch cmd`. **El subcomando se extrae ANTES de llamar a `fs.Parse`** para evitar
que `flag` intente parsear "init" como un flag y falle:

```go
case "d1":
    sub := ""
    if len(os.Args) >= 3 {
        sub = os.Args[2]
    }
    var dbName string
    fs.StringVar(&dbName, "db-name", "", "D1 database name (default: PROJECT_NAME)")
    fs.Parse(os.Args[3:])
    switch sub {
    case "init":
        err = goflare.RunD1InitCmd(env, dbName)
    default:
        fmt.Fprintf(os.Stderr, "unknown d1 subcommand: %s\n", sub)
        os.Exit(1)
    }
```

Actualizar `goflare.Usage()` para incluir el nuevo subcomando.

### Stage 5 — `goflare/tests/d1init_test.go` (new file) — tests del diagrama

**Nuevo archivo en `tests/`. Package `goflare_test` (caja negra — solo exportados).**
Va en `tests/` porque no necesita variables internas del paquete.

Primera línea obligatoria:
```go
//go:build !wasm
```

Los mocks se declaran en este mismo archivo. **Usar `goflare.NewMemoryStore()` en lugar de
un `mockStore` propio** — `MemoryStore` ya existe y es exportado en `store.go`.

```go
package goflare_test

import (
    "io"
    "os"
    "path/filepath"
    "strings"
    "testing"

    . "github.com/tinywasm/fmt"
    "github.com/tinywasm/goflare"
)

// Test fixtures — all hardcoded values in one place for easy change.
const (
    testProject   = "myapp"
    testAccountID = "acc123"
    testToken     = "tok"
    testTokenGH   = "mytoken"      // token usado en el test que verifica GitHub
    testDBUUID    = "db-uuid-1"    // UUID de DB preexistente
    testDBUUIDNew = "new-uuid"     // UUID de DB recién creada
    testDBUUIDGH  = "uuid-gh"      // UUID en test de integración GitHub
    testDBUUIDMan = "uuid-manual"  // UUID en test de instrucciones manuales
    testRepoURL   = "https://github.com/org/repo"
    testKeyring   = "goflare/" + testProject
)

// testEnv escribe un .env en un directorio temporal y retorna el path.
func testEnv(t *testing.T, lines string) string {
    t.Helper()
    path := filepath.Join(t.TempDir(), ".env")
    os.WriteFile(path, []byte(lines), 0644)
    return path
}

// testSetup retorna el envPath con PROJECT_NAME + CLOUDFLARE_ACCOUNT_ID y un store
// con el token ya cargado. Es el setup estándar para la mayoría de los tests.
func testSetup(t *testing.T, token string) (envPath string, store *goflare.MemoryStore) {
    t.Helper()
    envPath = testEnv(t, "PROJECT_NAME="+testProject+"\nCLOUDFLARE_ACCOUNT_ID="+testAccountID+"\n")
    store = goflare.NewMemoryStore()
    store.Set(testKeyring, token)
    return
}

// mockD1Manager
type mockD1Manager struct {
    listFn   func(accountID string) ([]goflare.D1Database, error)
    createFn func(accountID, name string) (goflare.D1Database, error)
}
func (m *mockD1Manager) ListD1Databases(a string) ([]goflare.D1Database, error) { return m.listFn(a) }
func (m *mockD1Manager) CreateD1Database(a, n string) (goflare.D1Database, error) { return m.createFn(a, n) }

// mockGHRunner
type mockGHRunner struct {
    available bool
    secrets   map[string]string
    variables map[string]string
    remoteURL string
}
func (m *mockGHRunner) Available() bool                               { return m.available }
func (m *mockGHRunner) RemoteURL() (string, error)                    { return m.remoteURL, nil }
func (m *mockGHRunner) SetSecret(_, name, value string) error         { m.secrets[name] = value; return nil }
func (m *mockGHRunner) SetVariable(_, name, value string) error       { m.variables[name] = value; return nil }

// mockEnvWriter
type mockEnvWriter struct{ written map[string]string }
func (m *mockEnvWriter) WriteKey(_, key, value string) error { m.written[key] = value; return nil }

// --- Tests matching CLOUDFLARE_GH_ENV_FLOW.md nodes ---

// Node: Token no encontrado → error (ProjectName vacío → key "goflare/" → ErrNoToken)
func TestD1Init_NoToken(t *testing.T) {
    store := goflare.NewMemoryStore() // vacío — Get retorna ErrNotFound
    err := goflare.RunD1Init("", "", store, nil, nil, nil, io.Discard)
    if !errors.Is(err, goflare.ErrNoToken) {
        t.Fatalf("expected ErrNoToken, got: %v", err)
    }
}

// Node: AccountID no en .env → error
func TestD1Init_NoAccountID(t *testing.T) {
    envPath := testEnv(t, "PROJECT_NAME="+testProject+"\n") // sin CLOUDFLARE_ACCOUNT_ID
    store := goflare.NewMemoryStore()
    store.Set(testKeyring, testToken)

    err := goflare.RunD1Init(envPath, "", store, nil, nil, nil, io.Discard)
    if !errors.Is(err, goflare.ErrNoAccountID) {
        t.Fatalf("expected ErrNoAccountID, got: %v", err)
    }
}

// Node: DB ya existe → reutiliza, NO llama CreateD1Database
func TestD1Init_DBAlreadyExists(t *testing.T) {
    envPath, store := testSetup(t, testToken)

    createCalled := false
    d1 := &mockD1Manager{
        listFn: func(_ string) ([]goflare.D1Database, error) {
            return []goflare.D1Database{{UUID: testDBUUID, Name: testProject}}, nil
        },
        createFn: func(_, _ string) (goflare.D1Database, error) {
            createCalled = true
            return goflare.D1Database{}, nil
        },
    }
    w := &mockEnvWriter{written: map[string]string{}}
    gh := &mockGHRunner{available: false}

    err := goflare.RunD1Init(envPath, "", store, d1, gh, w, io.Discard)
    if err != nil { t.Fatalf("unexpected error: %v", err) }
    if createCalled { t.Fatal("CreateD1Database must not be called when DB already exists") }
    if w.written[goflare.EnvKeyD1DatabaseID] != testDBUUID {
        t.Fatalf("expected D1_DATABASE_ID=%s, got %q", testDBUUID, w.written[goflare.EnvKeyD1DatabaseID])
    }
}

// Node: DB no existe → crea, escribe ID en .env
func TestD1Init_DBNotExists_Creates(t *testing.T) {
    envPath, store := testSetup(t, testToken)

    d1 := &mockD1Manager{
        listFn:   func(_ string) ([]goflare.D1Database, error) { return nil, nil },
        createFn: func(_, name string) (goflare.D1Database, error) {
            return goflare.D1Database{UUID: testDBUUIDNew, Name: name}, nil
        },
    }
    w := &mockEnvWriter{written: map[string]string{}}
    gh := &mockGHRunner{available: false}

    err := goflare.RunD1Init(envPath, "", store, d1, gh, w, io.Discard)
    if err != nil { t.Fatalf("unexpected error: %v", err) }
    if w.written[goflare.EnvKeyD1DatabaseID] != testDBUUIDNew {
        t.Fatalf("expected D1_DATABASE_ID=%s, got %q", testDBUUIDNew, w.written[goflare.EnvKeyD1DatabaseID])
    }
}

// Node: gh disponible → SetSecret para token, SetVariable para accountID y dbID
func TestD1Init_GHAvailable_SetsCorrectly(t *testing.T) {
    envPath, store := testSetup(t, testTokenGH)

    d1 := &mockD1Manager{
        listFn:   func(_ string) ([]goflare.D1Database, error) { return nil, nil },
        createFn: func(_, _ string) (goflare.D1Database, error) {
            return goflare.D1Database{UUID: testDBUUIDGH, Name: testProject}, nil
        },
    }
    w := &mockEnvWriter{written: map[string]string{}}
    gh := &mockGHRunner{
        available: true,
        remoteURL: testRepoURL,
        secrets:   map[string]string{},
        variables: map[string]string{},
    }

    err := goflare.RunD1Init(envPath, "", store, d1, gh, w, io.Discard)
    if err != nil { t.Fatalf("unexpected error: %v", err) }

    // Token → Secret
    if gh.secrets[goflare.GHSecretToken] != testTokenGH {
        t.Fatalf("CLOUDFLARE_API_TOKEN must be set as Secret, got: %v", gh.secrets)
    }
    // AccountID → Variable (not Secret)
    if gh.variables[goflare.GHVarAccountID] != testAccountID {
        t.Fatalf("CLOUDFLARE_ACCOUNT_ID must be set as Variable, got: %v", gh.variables)
    }
    if _, isSecret := gh.secrets[goflare.GHVarAccountID]; isSecret {
        t.Fatal("CLOUDFLARE_ACCOUNT_ID must NOT be a Secret")
    }
    // D1 ID → Variable (not Secret)
    if gh.variables[goflare.GHVarDatabaseID] != testDBUUIDGH {
        t.Fatalf("D1_DATABASE_ID must be set as Variable, got: %v", gh.variables)
    }
    if _, isSecret := gh.secrets[goflare.GHVarDatabaseID]; isSecret {
        t.Fatal("D1_DATABASE_ID must NOT be a Secret")
    }
}

// Node: ListD1Databases falla → propaga error
func TestD1Init_ListFails(t *testing.T) {
    envPath, store := testSetup(t, testToken)
    apiErr := Err("api error")
    d1 := &mockD1Manager{
        listFn: func(_ string) ([]goflare.D1Database, error) { return nil, apiErr },
    }
    err := goflare.RunD1Init(envPath, "", store, d1, &mockGHRunner{}, &mockEnvWriter{written: map[string]string{}}, io.Discard)
    if !errors.Is(err, apiErr) {
        t.Fatalf("expected api error to propagate, got: %v", err)
    }
}

// Node: CreateD1Database falla → propaga error
func TestD1Init_CreateFails(t *testing.T) {
    envPath, store := testSetup(t, testToken)
    apiErr := Err("create error")
    d1 := &mockD1Manager{
        listFn:   func(_ string) ([]goflare.D1Database, error) { return nil, nil },
        createFn: func(_, _ string) (goflare.D1Database, error) { return goflare.D1Database{}, apiErr },
    }
    err := goflare.RunD1Init(envPath, "", store, d1, &mockGHRunner{}, &mockEnvWriter{written: map[string]string{}}, io.Discard)
    if !errors.Is(err, apiErr) {
        t.Fatalf("expected create error to propagate, got: %v", err)
    }
}

// Node: gh no disponible → imprime instrucciones manuales con los valores
func TestD1Init_GHNotAvailable_PrintsInstructions(t *testing.T) {
    envPath, store := testSetup(t, testToken)

    d1 := &mockD1Manager{
        listFn:   func(_ string) ([]goflare.D1Database, error) { return nil, nil },
        createFn: func(_, _ string) (goflare.D1Database, error) {
            return goflare.D1Database{UUID: testDBUUIDMan, Name: testProject}, nil
        },
    }
    w := &mockEnvWriter{written: map[string]string{}}
    gh := &mockGHRunner{available: false}

    var buf strings.Builder
    err := goflare.RunD1Init(envPath, "", store, d1, gh, w, &buf)
    if err != nil { t.Fatalf("unexpected error: %v", err) }

    out := buf.String()
    if !strings.Contains(out, goflare.GHSecretToken) {
        t.Fatal("output must mention CLOUDFLARE_API_TOKEN")
    }
    if !strings.Contains(out, goflare.GHVarAccountID) {
        t.Fatal("output must mention CLOUDFLARE_ACCOUNT_ID")
    }
    if !strings.Contains(out, testDBUUIDMan) {
        t.Fatalf("output must include the database ID (%s)", testDBUUIDMan)
    }
}
```

### Stage 6 — `goflare/go.mod`

No new dependencies. `os/exec` is stdlib. `tinywasm/fmt` ya está en `go.mod`.

Run:
```bash
go mod tidy
```

## Stages Summary

| # | Archivo | Tipo | Acción |
|---|---|---|---|
| 0 | `goflare/*.go` (10 archivos) | Existente | Añadir `//go:build !wasm` — eliminar deuda técnica antes de nuevos cambios |
| 1 | `goflare/config.go` | Existente | Constantes `EnvKey*`; campos `D1DatabaseID`/`D1DatabaseName` en `Config`; actualizar `LoadConfigFromEnv` con constantes; NO tocar `WriteEnvFile` |
| 2 | `goflare/d1init.go` | Nuevo | `//go:build !wasm`; `var` errores sentinela; constantes GH; interfaces `D1Manager`/`GHRunner`/`EnvWriter`; `D1Database`; `RunD1Init`; impls reales `cfD1Manager`/`execGHRunner`/`fileEnvWriter`; dot-import `tinywasm/fmt` |
| 3 | `goflare/run.go` | Existente | Agregar `RunD1InitCmd` wrapper (tag ya añadido en Stage 0) |
| 4 | `cmd/goflare/main.go` | Existente | Agregar subcomando `d1 init [--db-name]`; subcomando extraído ANTES de `fs.Parse` |
| 5 | `goflare/tests/d1init_test.go` | Nuevo | `//go:build !wasm`; 8 tests mockeados — nodos del diagrama + propagación de errores API; usa `MemoryStore`; `errors.Is` para comparar errores |
| 6 | `goflare/go.mod` | Existente | `go mod tidy` |

## Verification

```bash
go install github.com/tinywasm/devflow/cmd/gotest@latest
gotest
```

Todos los tests pasan incluyendo los 8 nuevos de `tests/d1init_test.go`. Sin regresiones.

Uso manual tras implementación:
```bash
goflare d1 init              # usa PROJECT_NAME como nombre de DB
goflare d1 init --db-name=contacts-db
```
