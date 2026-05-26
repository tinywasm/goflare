package goflare_test

import (
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/tinywasm/goflare"
)

func TestDeployPages_FullFlow(t *testing.T) {
	env := newTestEnv(t)
	defer env.Close()

	os.Setenv("CLOUDFLARE_API_TOKEN", "valid-token")
	defer os.Unsetenv("CLOUDFLARE_API_TOKEN")

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
	defer env.Close()

	os.Setenv("CLOUDFLARE_API_TOKEN", "token")
	defer os.Unsetenv("CLOUDFLARE_API_TOKEN")

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
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/uploadToken") {
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

	if err := g.DeployPages(); err != nil {
		t.Errorf("DeployPages failed: %v", err)
	}
	if !projectCreated {
		t.Error("Project should have been created after 404")
	}
}
