package goflare_test

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/tinywasm/goflare"
)

func TestDeployWorker_UploadScript(t *testing.T) {
	tmpDir, cleanup, err := TempDir()
	if err != nil {
		t.Fatalf("TempDir failed: %v", err)
	}
	defer cleanup()

	outputDir := filepath.Join(tmpDir, ".goflare")
	os.MkdirAll(outputDir, 0755)
	os.WriteFile(filepath.Join(outputDir, "worker.js"), []byte("js"), 0644)
	os.WriteFile(filepath.Join(outputDir, "worker.wasm"), []byte("wasm"), 0644)
	os.WriteFile(filepath.Join(outputDir, "wasm_exec.js"), []byte("exec"), 0644)

	server := MockHTTPServer(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/accounts/acc123/workers/scripts/myworker" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"success": true, "result": {}}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	defer server.Close()

	origURL := goflare.SetCFBaseURL(server.URL)
	defer goflare.SetCFBaseURL(origURL)

	store := goflare.NewMemoryStore()
	store.Set("token", "valid-token")

	cfg := &goflare.Config{
		ProjectName: "test",
		AccountID:   "acc123",
		WorkerName:  "myworker",
		Entry:       "main.go",
		OutputDir:   outputDir,
	}
	g := goflare.New(cfg)

	if err := g.DeployWorker(store); err != nil {
		t.Fatalf("DeployWorker failed: %v", err)
	}
}

func TestDeployWorker_MissingArtifact(t *testing.T) {
	tmpDir, cleanup, _ := TempDir()
	defer cleanup()

	store := goflare.NewMemoryStore()
	store.Set("token", "valid-token")

	cfg := &goflare.Config{
		ProjectName: "test",
		AccountID:   "acc123",
		Entry:       "main.go",
		OutputDir:   tmpDir,
	}
	g := goflare.New(cfg)

	err := g.DeployWorker(store)
	if err == nil {
		t.Error("expected error for missing artifact, got nil")
	}
}
