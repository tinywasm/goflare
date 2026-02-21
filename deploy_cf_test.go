package goflare_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/tinywasm/goflare"
)

func TestDeployPages_Success(t *testing.T) {
	// Create fake output files
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "_worker.js"), []byte("// js"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "worker.wasm"), []byte("wasm"), 0644); err != nil {
		t.Fatal(err)
	}

	var deployCalled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			deployCalled = true
		}
		result := map[string]string{"id": "deploy-123", "url": "https://my-project.pages.dev"}
		raw, _ := json.Marshal(result)
		json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"errors":  []any{},
			"result":  json.RawMessage(raw),
		})
	}))
	defer srv.Close()

	keys := newMockKeyManager()
	keys.Set("cloudflare", "pages_token", "test-token")
	keys.Set("cloudflare", "account_id", "acct-123")
	keys.Set("cloudflare", "pages_project", "my-project")

	g := goflare.NewForTest(&goflare.Config{
		AppRootDir:              dir,
		RelativeOutputDirectory: func() string { return "." },
		OutputWasmFileName:      "worker.wasm",
	}, keys, srv.URL)

	if err := g.DeployPages(); err != nil {
		t.Fatalf("DeployPages failed: %v", err)
	}
	if !deployCalled {
		t.Error("expected POST to deployments endpoint")
	}
}

func TestDeployPages_MissingToken(t *testing.T) {
	keys := newMockKeyManager() // empty keyring
	g := goflare.NewForTest(nil, keys, "http://unused")

	err := g.DeployPages()
	if err == nil {
		t.Error("expected error when pages token not configured")
	}
}
