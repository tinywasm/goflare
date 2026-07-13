//go:build !wasm

package goflare_test

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tinywasm/goflare"
)

func TestDeployPages_FullFlow(t *testing.T) {
	env := newTestEnv(t)

	os.Setenv("CLOUDFLARE_API_TOKEN", "valid-token")
	defer os.Unsetenv("CLOUDFLARE_API_TOKEN")

	server := MockHTTPServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// GET upload-token
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/upload-token") {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":{"jwt":"fake-jwt"}}`))
			return
		}
		// GET project
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/pages/projects/test-project") {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":{"name":"test-project"}}`))
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

	cfg := &goflare.Config{
		ProjectName: "test-project",
		AccountID:   "account-id",
		PublicDir:   env.PublicDir,
		OutputDir:   env.OutputDir,
	}
	g := goflare.New(cfg)
	g.BaseURL = server.URL

	err := g.DeployPages()
	if err != nil {
		t.Fatalf("DeployPages failed: %v", err)
	}
}

func TestDeployPages_CreatesProjectIfMissing(t *testing.T) {
	env := newTestEnv(t)

	os.Setenv("CLOUDFLARE_API_TOKEN", "token")
	defer os.Unsetenv("CLOUDFLARE_API_TOKEN")

	projectCreated := false
	server := MockHTTPServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/pages/projects/test-project") {
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
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/upload-token") {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":{"jwt":"fake"}}`))
			return
		}
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/pages/assets/upload") {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":{}}`))
			return
		}
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/deployments") {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":{"url":"fake"}}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success":true,"result":null}`))
	})
	defer server.Close()

	cfg := &goflare.Config{
		ProjectName: "test-project",
		AccountID:   "acc",
		PublicDir:   env.PublicDir,
		OutputDir:   env.OutputDir,
	}
	g := goflare.New(cfg)
	g.BaseURL = server.URL
	g.RetryBackoff = time.Millisecond // Speed up test

	if err := g.DeployPages(); err != nil {
		t.Errorf("DeployPages failed: %v", err)
	}
	if !projectCreated {
		t.Error("Project should have been created after 404")
	}
}

func TestDeployPages_RetryUploadToken(t *testing.T) {
	env := newTestEnv(t)

	os.Setenv("CLOUDFLARE_API_TOKEN", "token")
	defer os.Unsetenv("CLOUDFLARE_API_TOKEN")

	uploadTokenCalls := 0
	server := MockHTTPServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/pages/projects/test-project") {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":{"name":"test-project"}}`))
			return
		}
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/upload-token") {
			uploadTokenCalls++
			if uploadTokenCalls == 1 {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"success":false,"errors":[],"result":null}`))
				return
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":{"jwt":"fake-jwt"}}`))
			return
		}
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/pages/assets/upload") {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":{}}`))
			return
		}
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/deployments") {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":{"url":"fake"}}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success":true,"result":null}`))
	})
	defer server.Close()

	cfg := &goflare.Config{
		ProjectName: "test-project",
		AccountID:   "acc",
		PublicDir:   env.PublicDir,
		OutputDir:   env.OutputDir,
	}
	g := goflare.New(cfg)
	g.BaseURL = server.URL
	g.RetryBackoff = time.Millisecond // Speed up test

	if err := g.DeployPages(); err != nil {
		t.Errorf("DeployPages failed: %v", err)
	}
	if uploadTokenCalls != 2 {
		t.Errorf("Expected 2 calls to uploadToken, got %d", uploadTokenCalls)
	}
}

func TestDeployPages_PermissionError(t *testing.T) {
	tempDir := t.TempDir()
	publicDir := filepath.Join(tempDir, "web/public")
	os.MkdirAll(publicDir, 0755)

	os.Setenv("CLOUDFLARE_API_TOKEN", "token")
	os.Setenv("PROJECT_NAME", "test-project")
	os.Setenv("CLOUDFLARE_ACCOUNT_ID", "acc")
	os.Setenv("PUBLIC_DIR", publicDir)
	defer os.Unsetenv("CLOUDFLARE_API_TOKEN")
	defer os.Unsetenv("PROJECT_NAME")
	defer os.Unsetenv("CLOUDFLARE_ACCOUNT_ID")
	defer os.Unsetenv("PUBLIC_DIR")

	server := MockHTTPServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/pages/projects") {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"success":false,"errors":[{"code":1000,"message":"Forbidden"}]}`))
			return
		}
	})
	defer server.Close()

	g := goflare.New(&goflare.Config{})
	g.BaseURL = server.URL
	// Note: RunDeploy will load config from env, so we set the server URL via a hack
	// or by calling the underlying methods. Since RunDeploy is a high-level runner,
	// we'll use a trick: we can't easily override BaseURL for RunDeploy without
	// modifying the global state if it was there, but it's not.
	// Actually, RunDeploy creates a NEW Goflare instance.
	// Let's modify RunDeploy to allow passing a Goflare instance or just test the
	// validateDeployScopes method directly.

	token := "token"
	client := &goflare.CfClient{
		Token:      token,
		BaseURL:    server.URL,
		HttpClient: http.DefaultClient,
	}
	err := g.ValidateDeployScopes(client)
	if err == nil {
		t.Fatal("RunDeploy should have failed due to permission error")
	}
	if !strings.Contains(err.Error(), "the token cannot access Pages on account") {
		t.Errorf("Expected permission error message, got: %v", err)
	}
}