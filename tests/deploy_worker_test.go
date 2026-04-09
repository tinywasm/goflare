package goflare_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinywasm/goflare"
)

func TestDeployWorker_UploadScript(t *testing.T) {
	tmpDir, cleanup, err := TempDir()
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer cleanup()

	outputDir := filepath.Join(tmpDir, ".build")
	os.MkdirAll(outputDir, 0755)
	os.WriteFile(filepath.Join(outputDir, "edge.js"), []byte("console.log('edge')"), 0644)
	os.WriteFile(filepath.Join(outputDir, "edge.wasm"), []byte("wasm"), 0644)

	server := MockHTTPServer(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/workers/scripts/my-worker") {
			// Verify multipart
			err := r.ParseMultipartForm(10 * 1024 * 1024)
			if err != nil {
				t.Errorf("Failed to parse multipart form: %v", err)
			}
			if r.FormValue("metadata") == "" {
				t.Error("Missing metadata field")
			}
			var metadata map[string]string
			json.Unmarshal([]byte(r.FormValue("metadata")), &metadata)
			if metadata["main_module"] != "edge.js" {
				t.Errorf("Expected main_module edge.js, got %s", metadata["main_module"])
			}

			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":{}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	defer server.Close()

	store := goflare.NewMemoryStore()
	store.Set("goflare/test-project", "valid-token")

	cfg := &goflare.Config{
		ProjectName: "test-project",
		AccountID:   "account-id",
		WorkerName:  "my-worker",
		OutputDir:   outputDir,
	}
	g := goflare.New(cfg)
	g.BaseURL = server.URL

	err = g.DeployWorker(store)
	if err != nil {
		t.Fatalf("DeployWorker failed: %v", err)
	}
}

func TestDeployWorker_MissingArtifact(t *testing.T) {
	tmpDir, cleanup, err := TempDir()
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer cleanup()

	outputDir := filepath.Join(tmpDir, ".build")
	os.MkdirAll(outputDir, 0755)
	// Only one file
	os.WriteFile(filepath.Join(outputDir, "edge.js"), []byte("console.log('edge')"), 0644)

	store := goflare.NewMemoryStore()
	store.Set("goflare/test-project", "valid-token")

	cfg := &goflare.Config{
		ProjectName: "test-project",
		AccountID:   "account-id",
		WorkerName:  "my-worker",
		OutputDir:   outputDir,
	}
	g := goflare.New(cfg)

	err = g.DeployWorker(store)
	if err == nil {
		t.Fatal("Expected error due to missing artifacts")
	}
	if !strings.Contains(err.Error(), "missing artifact") {
		t.Errorf("Unexpected error message: %v", err)
	}
}
