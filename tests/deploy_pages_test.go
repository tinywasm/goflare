package goflare_test

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinywasm/goflare"
)

func TestDeployPages_FullFlow(t *testing.T) {
	tmpDir, cleanup, err := TempDir()
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer cleanup()

	outputDir := filepath.Join(tmpDir, ".build")
	distDir := filepath.Join(outputDir, "dist")
	os.MkdirAll(distDir, 0755)
	os.WriteFile(filepath.Join(distDir, "index.html"), []byte("<h1>Hello</h1>"), 0644)

	server := MockHTTPServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// GET project
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/pages/projects/test-project") {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":{"name":"test-project"}}`))
			return
		}
		// POST uploadToken
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/uploadToken") {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":{"jwt":"fake-jwt"}}`))
			return
		}
		// POST assets/upload
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/pages/assets/upload") {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":{}}`))
			return
		}
		// POST deployments
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/deployments") {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":{"url":"https://test-project.pages.dev"}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"success":false,"errors":[{"code":1000,"message":"Not found"}]}`))
	})
	defer server.Close()

	store := goflare.NewMemoryStore()
	store.Set("goflare/test-project", "valid-token")

	cfg := &goflare.Config{
		ProjectName: "test-project",
		AccountID:   "account-id",
		OutputDir:   outputDir,
	}
	g := goflare.New(cfg)
	g.BaseURL = server.URL

	err = g.DeployPages(store)
	if err != nil {
		t.Fatalf("DeployPages failed: %v", err)
	}
}

func TestDeployPages_CreatesProjectIfMissing(t *testing.T) {
	tmpDir, cleanup, err := TempDir()
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer cleanup()

	outputDir := filepath.Join(tmpDir, ".goflare")
	distDir := filepath.Join(outputDir, "dist")
	os.MkdirAll(distDir, 0755)
	os.WriteFile(filepath.Join(distDir, "index.html"), []byte("hello"), 0644)

	projectCreated := false
	server := MockHTTPServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/pages/projects/test-project") {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"success":false,"errors":[{"code":8000007,"message":"Not found"}]}`))
			return
		}
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/pages/projects") {
			projectCreated = true
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":{"name":"test-project"}}`))
			return
		}
		// Minimal response for the rest of the flow
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success":true,"result":{"jwt":"fake","url":"fake"}}`))
	})
	defer server.Close()

	store := goflare.NewMemoryStore()
	store.Set("goflare/test-project", "token")
	cfg := &goflare.Config{ProjectName: "test-project", AccountID: "acc", OutputDir: outputDir}
	g := goflare.New(cfg)
	g.BaseURL = server.URL

	g.DeployPages(store)
	if !projectCreated {
		t.Error("Project should have been created after 404")
	}
}
