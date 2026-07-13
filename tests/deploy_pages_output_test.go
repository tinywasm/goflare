//go:build !wasm

package goflare_test

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinywasm/goflare"
)

// TestDeployPages_ReadsFromPublicDir verifies that DeployPages uploads files
// found in cfg.PublicDir, not from .build/dist/.
func TestDeployPages_ReadsFromPublicDir(t *testing.T) {
	env := newTestEnv(t)

	os.Setenv("CLOUDFLARE_API_TOKEN", "token")
	defer os.Unsetenv("CLOUDFLARE_API_TOKEN")

	uploadedFiles := make(map[string]bool)
	server := MockHTTPServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/pages/assets/upload") {
			uploadedFiles["uploaded"] = true
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success":true,"result":{"jwt":"fake","url":"fake"}}`))
	})
	defer server.Close()

	cfg := &goflare.Config{
		ProjectName: "test",
		AccountID:   "acc",
		PublicDir:   env.PublicDir,
		OutputDir:   env.OutputDir,
	}
	g := goflare.New(cfg)
	g.BaseURL = server.URL

	if err := g.DeployPages(); err != nil {
		t.Fatalf("DeployPages failed: %v", err)
	}

	if !uploadedFiles["uploaded"] {
		t.Error("Files should have been uploaded from PublicDir")
	}
}

// TestDeployPages_NoDistDirRequired verifies that DeployPages succeeds
// even when .build/dist/ does not exist, as long as PublicDir has files.
func TestDeployPages_NoDistDirRequired(t *testing.T) {
	env := newTestEnv(t)

	os.Setenv("CLOUDFLARE_API_TOKEN", "token")
	defer os.Unsetenv("CLOUDFLARE_API_TOKEN")

	server := MockHTTPServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success":true,"result":{"jwt":"fake","url":"fake"}}`))
	})
	defer server.Close()

	cfg := &goflare.Config{
		ProjectName: "test",
		AccountID:   "acc",
		PublicDir:   env.PublicDir,
		OutputDir:   env.OutputDir,
	}
	g := goflare.New(cfg)
	g.BaseURL = server.URL

	if err := g.DeployPages(); err != nil {
		t.Fatalf("DeployPages failed: %v", err)
	}
}

// TestDeployPages_FailsWhenPublicDirEmpty verifies that DeployPages returns
// a "no files found" error when PublicDir exists but is empty.
func TestDeployPages_FailsWhenPublicDirEmpty(t *testing.T) {
	env := newTestEnv(t)

	os.Setenv("CLOUDFLARE_API_TOKEN", "token")
	defer os.Unsetenv("CLOUDFLARE_API_TOKEN")

	// Empty the PublicDir
	os.Remove(filepath.Join(env.PublicDir, "index.html"))

	server := MockHTTPServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success":true,"result":{"jwt":"fake","url":"fake"}}`))
	})
	defer server.Close()

	cfg := &goflare.Config{
		ProjectName: "test",
		AccountID:   "acc",
		PublicDir:   env.PublicDir,
		OutputDir:   env.OutputDir,
	}
	g := goflare.New(cfg)
	g.BaseURL = server.URL

	err := g.DeployPages()
	if err == nil {
		t.Fatal("Expected error when PublicDir is empty")
	}
	if !strings.Contains(err.Error(), "no files found") {
		t.Errorf("Unexpected error: %v", err)
	}
}