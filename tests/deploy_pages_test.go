package goflare_test

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/tinywasm/goflare"
)

func TestDeployPages_Upload(t *testing.T) {
	tmpDir, cleanup, err := TempDir()
	if err != nil {
		t.Fatalf("TempDir failed: %v", err)
	}
	defer cleanup()

	distDir := filepath.Join(tmpDir, ".goflare", "dist")
	os.MkdirAll(distDir, 0755)
	os.WriteFile(filepath.Join(distDir, "index.html"), []byte("index"), 0644)

	server := MockHTTPServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/accounts/acc123/pages/projects/myproject":
			fmt.Fprintln(w, `{"success": true, "result": {}}`)
		case "/accounts/acc123/pages/projects/myproject/upload-token":
			fmt.Fprintln(w, `{"success": true, "result": {"jwt": "dummy-jwt"}}`)
		case "/pages/assets/upload":
			fmt.Fprintln(w, `{"success": true, "result": {}}`)
		case "/accounts/acc123/pages/projects/myproject/deployments":
			fmt.Fprintln(w, `{"success": true, "result": {"url": "https://myproject.pages.dev"}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer server.Close()

	origURL := goflare.SetCFBaseURL(server.URL)
	defer goflare.SetCFBaseURL(origURL)

	store := goflare.NewMemoryStore()
	store.Set("token", "valid-token")

	cfg := &goflare.Config{
		ProjectName: "myproject",
		AccountID:   "acc123",
		PublicDir:   "public",
		OutputDir:   filepath.Join(tmpDir, ".goflare"),
	}
	g := goflare.New(cfg)

	if err := g.DeployPages(store); err != nil {
		t.Fatalf("DeployPages failed: %v", err)
	}
}

func TestDeployPages_EmptyDist(t *testing.T) {
	tmpDir, cleanup, _ := TempDir()
	defer cleanup()

	store := goflare.NewMemoryStore()
	store.Set("token", "valid-token")

	cfg := &goflare.Config{
		ProjectName: "myproject",
		AccountID:   "acc123",
		PublicDir:   "public",
		OutputDir:   tmpDir,
	}
	g := goflare.New(cfg)

	err := g.DeployPages(store)
	if err == nil {
		t.Error("expected error for empty dist, got nil")
	}
}
