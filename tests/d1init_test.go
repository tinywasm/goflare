//go:build !wasm

package goflare_test

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinywasm/goflare"
)

// Test fixtures — all hardcoded values in one place for easy change.
const (
	testProject   = "myapp"
	testAccountID = "acc123"
	testToken     = "tok"
	testTokenGH   = "mytoken"     // token usado en el test que verifica GitHub
	testDBUUID    = "db-uuid-1"   // UUID de DB preexistente
	testDBUUIDNew = "new-uuid"    // UUID de DB recién creada
	testDBUUIDGH  = "uuid-gh"     // UUID en test de integración GitHub
	testDBUUIDMan = "uuid-manual" // UUID en test de instrucciones manuales
	testRepoURL   = "https://github.com/org/repo"
	testKeyring   = "goflare/" + testProject
)

// writeTestEnv escribe un .env en un directorio temporal y retorna el path.
func writeTestEnv(t *testing.T, lines string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".env")
	os.WriteFile(path, []byte(lines), 0644)
	return path
}

// testSetup retorna el envPath con PROJECT_NAME + CLOUDFLARE_ACCOUNT_ID y un store
// con el token ya cargado. Es el setup estándar para la mayoría de los tests.
func testSetup(t *testing.T, token string) (envPath string, store *goflare.MemoryStore) {
	t.Helper()
	envPath = writeTestEnv(t, "PROJECT_NAME="+testProject+"\nCLOUDFLARE_ACCOUNT_ID="+testAccountID+"\n")
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
func (m *mockD1Manager) CreateD1Database(a, n string) (goflare.D1Database, error) {
	return m.createFn(a, n)
}

// mockGHRunner
type mockGHRunner struct {
	available bool
	secrets   map[string]string
	variables map[string]string
	remoteURL string
}

func (m *mockGHRunner) Available() bool                         { return m.available }
func (m *mockGHRunner) RemoteURL() (string, error)              { return m.remoteURL, nil }
func (m *mockGHRunner) SetSecret(_, name, value string) error   { m.secrets[name] = value; return nil }
func (m *mockGHRunner) SetVariable(_, name, value string) error { m.variables[name] = value; return nil }

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
	envPath := writeTestEnv(t, "PROJECT_NAME="+testProject+"\n") // sin CLOUDFLARE_ACCOUNT_ID
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if createCalled {
		t.Fatal("CreateD1Database must not be called when DB already exists")
	}
	if w.written[goflare.EnvKeyD1DatabaseID] != testDBUUID {
		t.Fatalf("expected D1_DATABASE_ID=%s, got %q", testDBUUID, w.written[goflare.EnvKeyD1DatabaseID])
	}
}

// Node: DB no existe → crea, escribe ID en .env
func TestD1Init_DBNotExists_Creates(t *testing.T) {
	envPath, store := testSetup(t, testToken)

	d1 := &mockD1Manager{
		listFn: func(_ string) ([]goflare.D1Database, error) { return nil, nil },
		createFn: func(_, name string) (goflare.D1Database, error) {
			return goflare.D1Database{UUID: testDBUUIDNew, Name: name}, nil
		},
	}
	w := &mockEnvWriter{written: map[string]string{}}
	gh := &mockGHRunner{available: false}

	err := goflare.RunD1Init(envPath, "", store, d1, gh, w, io.Discard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w.written[goflare.EnvKeyD1DatabaseID] != testDBUUIDNew {
		t.Fatalf("expected D1_DATABASE_ID=%s, got %q", testDBUUIDNew, w.written[goflare.EnvKeyD1DatabaseID])
	}
}

// Node: gh disponible → SetSecret para token, SetVariable para accountID y dbID
func TestD1Init_GHAvailable_SetsCorrectly(t *testing.T) {
	envPath, store := testSetup(t, testTokenGH)

	d1 := &mockD1Manager{
		listFn: func(_ string) ([]goflare.D1Database, error) { return nil, nil },
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

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
	apiErr := errors.New("api error")
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
	apiErr := errors.New("create error")
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
		listFn: func(_ string) ([]goflare.D1Database, error) { return nil, nil },
		createFn: func(_, _ string) (goflare.D1Database, error) {
			return goflare.D1Database{UUID: testDBUUIDMan, Name: testProject}, nil
		},
	}
	w := &mockEnvWriter{written: map[string]string{}}
	gh := &mockGHRunner{available: false}

	var buf strings.Builder
	err := goflare.RunD1Init(envPath, "", store, d1, gh, w, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

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
