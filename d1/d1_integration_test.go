//go:build integration && !wasm

package d1_test

import (
	"os"
	"testing"

	"github.com/tinywasm/goflare/d1"
	"github.com/tinywasm/orm"
	keyring "github.com/zalando/go-keyring"
)

const (
	envKeyToken      = "CLOUDFLARE_API_TOKEN"
	envKeyAccountID  = "CLOUDFLARE_ACCOUNT_ID"
	envKeyDatabaseID = "D1_DATABASE_ID"
	keyringService   = "goflare"
	testTable        = "_goflare_test"
)

// testItem is a minimal model for the integration test.
// ormc is not used for test-only structs — methods are written inline.
type testItem struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func (m *testItem) ModelName() string { return testTable }
func (m *testItem) Schema() []orm.Field {
	return []orm.Field{
		{Name: "id", PK: true},
		{Name: "name"},
	}
}
func (m *testItem) Pointers() []any { return []any{&m.ID, &m.Name} }

func resolveToken(t *testing.T) string {
	t.Helper()
	token, err := keyring.Get(keyringService, envKeyToken)
	if err == nil && token != "" {
		return token
	}
	token = os.Getenv(envKeyToken)
	if token != "" {
		return token
	}
	t.Skip("no token: run 'goflare auth' locally or set CLOUDFLARE_API_TOKEN in CI")
	return ""
}

func resolveEnv(t *testing.T, key string) string {
	t.Helper()
	v := os.Getenv(key)
	if v == "" {
		t.Skipf("env var %s not set", key)
	}
	return v
}

func TestD1Integration(t *testing.T) {
	token     := resolveToken(t)
	accountID := resolveEnv(t, envKeyAccountID)
	dbID      := resolveEnv(t, envKeyDatabaseID)

	db, err := d1.NewDirect(token, accountID, dbID)
	if err != nil {
		t.Fatalf("NewDirect: %v", err)
	}
	defer db.Close()

	// Setup table — same call as in the edge Worker
	if err := db.CreateTable(&testItem{}); err != nil {
		t.Fatalf("CreateTable: %v", err)
	}
	t.Cleanup(func() {
		db.DropTable(&testItem{}) //nolint
	})

	// Create
	item := &testItem{ID: 1, Name: "hello"}
	if err := db.Create(item); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Read one
	got := &testItem{}
	if err := db.First(got, orm.Where("id", 1)); err != nil {
		t.Fatalf("First: %v", err)
	}
	if got.Name != "hello" {
		t.Fatalf("expected name=hello, got %q", got.Name)
	}

	// Update
	item.Name = "world"
	if err := db.Save(item); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify update
	got2 := &testItem{}
	if err := db.First(got2, orm.Where("id", 1)); err != nil {
		t.Fatalf("First after Save: %v", err)
	}
	if got2.Name != "world" {
		t.Fatalf("expected name=world after update, got %q", got2.Name)
	}

	// Delete
	if err := db.Delete(item); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Verify gone
	got3 := &testItem{}
	err = db.First(got3, orm.Where("id", 1))
	if err != orm.ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got: %v", err)
	}
}
