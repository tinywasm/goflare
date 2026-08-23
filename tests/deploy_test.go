//go:build !wasm

package goflare_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinywasm/goflare"
)

// capturedMetadata parses the "metadata" multipart field of the PUT to
// /workers/scripts/{name}, and reports whether the "edge.js" file part was
// attached.
type capturedMetadata struct {
	metadata      map[string]any
	hasScriptFile bool
}

func captureDeployPUT(t *testing.T, r *http.Request) capturedMetadata {
	t.Helper()
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		t.Fatalf("failed to parse multipart PUT body: %v", err)
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(r.FormValue("metadata")), &meta); err != nil {
		t.Fatalf("failed to unmarshal metadata field: %v", err)
	}
	_, hasScript := r.MultipartForm.File["edge.js"]
	return capturedMetadata{metadata: meta, hasScriptFile: hasScript}
}

func withCloudflareToken(t *testing.T) {
	t.Helper()
	os.Setenv("CLOUDFLARE_API_TOKEN", "valid-token")
	t.Cleanup(func() { os.Unsetenv("CLOUDFLARE_API_TOKEN") })
}

func TestDeploy_AssetsOnly_NoMainModuleNoScriptPart(t *testing.T) {
	withCloudflareToken(t)
	env := newTestEnv(t) // PublicDir already has index.html; OutputDir has no edge.js

	sessionCalled := false
	var captured capturedMetadata

	server := MockHTTPServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/assets-upload-session"):
			sessionCalled = true
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":{"jwt":"session-jwt","buckets":[]}}`))
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/workers/scripts/"):
			captured = captureDeployPUT(t, r)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":{}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer server.Close()

	g := goflare.New(&goflare.Config{
		AccountID: "acc-123", WorkerName: "my-worker",
		PublicDir: env.PublicDir, OutputDir: env.OutputDir,
	})
	g.BaseURL = server.URL

	if err := g.Deploy(); err != nil {
		t.Fatalf("Deploy failed: %v", err)
	}

	if !sessionCalled {
		t.Error("expected assets-upload-session to be called since PublicDir has files")
	}
	if _, ok := captured.metadata["main_module"]; ok {
		t.Errorf("expected metadata to omit main_module, got: %v", captured.metadata)
	}
	if captured.hasScriptFile {
		t.Error("expected the multipart PUT not to attach an edge.js part")
	}
	if _, ok := captured.metadata["assets"]; !ok {
		t.Errorf("expected metadata to include assets, got: %v", captured.metadata)
	}
}

func TestDeploy_ScriptOnly_NoAssetsKeyNoUploadSession(t *testing.T) {
	withCloudflareToken(t)
	env := newTestEnv(t)
	os.Remove(filepath.Join(env.PublicDir, "index.html")) // PublicDir now empty
	env.writeOutput("edge.js", "console.log('edge')")
	env.writeOutput("edge.wasm", "wasm-bytes")

	sessionCalled := false
	var captured capturedMetadata

	server := MockHTTPServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/assets-upload-session"):
			sessionCalled = true
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":{"jwt":"session-jwt","buckets":[]}}`))
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/workers/scripts/"):
			captured = captureDeployPUT(t, r)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":{}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer server.Close()

	g := goflare.New(&goflare.Config{
		AccountID: "acc-123", WorkerName: "my-worker",
		PublicDir: env.PublicDir, OutputDir: env.OutputDir,
	})
	g.BaseURL = server.URL

	if err := g.Deploy(); err != nil {
		t.Fatalf("Deploy failed: %v", err)
	}

	if sessionCalled {
		t.Error("expected assets-upload-session NOT to be called when PublicDir has no files")
	}
	if _, ok := captured.metadata["assets"]; ok {
		t.Errorf("expected metadata to omit assets, got: %v", captured.metadata)
	}
	if got := captured.metadata["main_module"]; got != "edge.js" {
		t.Errorf("expected main_module = edge.js, got: %v", got)
	}
	if !captured.hasScriptFile {
		t.Error("expected the multipart PUT to attach an edge.js part")
	}
}

func TestDeploy_Neither_ReturnsNothingToDeployError(t *testing.T) {
	withCloudflareToken(t)
	env := newTestEnv(t)
	os.Remove(filepath.Join(env.PublicDir, "index.html")) // PublicDir now empty, OutputDir has no edge.js

	server := MockHTTPServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	defer server.Close()

	g := goflare.New(&goflare.Config{
		AccountID: "acc-123", WorkerName: "my-worker",
		PublicDir: env.PublicDir, OutputDir: env.OutputDir,
	})
	g.BaseURL = server.URL

	err := g.Deploy()
	if err == nil {
		t.Fatal("expected Deploy to fail when neither assets nor script exist")
	}
	if !strings.Contains(err.Error(), "nothing to deploy") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDeploy_Both_ThreePhasesInOrderWithFullMetadataAndBindings(t *testing.T) {
	withCloudflareToken(t)
	env := newTestEnv(t)
	env.writeOutput("edge.js", "console.log('edge')")
	env.writeOutput("edge.wasm", "wasm-bytes")

	// index.html comes from newTestEnv; compute its real asset hash so the
	// mocked phase-1 response can put it in a bucket that uploadAssets can
	// actually resolve through byHash.
	indexContent, err := os.ReadFile(filepath.Join(env.PublicDir, "index.html"))
	if err != nil {
		t.Fatalf("failed to read seeded index.html: %v", err)
	}
	indexHash := goflare.ExportAssetHash(indexContent, "html")

	var callOrder []string
	var captured capturedMetadata
	var phase2Auth string

	server := MockHTTPServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/assets-upload-session"):
			callOrder = append(callOrder, "assets-upload-session")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(fmt.Sprintf(`{"success":true,"result":{"jwt":"session-jwt","buckets":[["%s"]]}}`, indexHash)))
		case strings.Contains(r.URL.Path, "/workers/assets/upload"):
			callOrder = append(callOrder, "workers/assets/upload")
			phase2Auth = r.Header.Get("Authorization")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":{"jwt":"completion-jwt"}}`))
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/workers/scripts/"):
			callOrder = append(callOrder, "workers/scripts")
			captured = captureDeployPUT(t, r)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":{}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer server.Close()

	g := goflare.New(&goflare.Config{
		AccountID: "acc-123", WorkerName: "my-worker",
		PublicDir: env.PublicDir, OutputDir: env.OutputDir,
		D1DatabaseID: "d1-id-123",
		R2BucketID:   "r2-bucket-123",
	})
	g.BaseURL = server.URL

	if err := g.Deploy(); err != nil {
		t.Fatalf("Deploy failed: %v", err)
	}

	wantOrder := []string{"assets-upload-session", "workers/assets/upload", "workers/scripts"}
	if len(callOrder) != len(wantOrder) {
		t.Fatalf("expected call order %v, got %v", wantOrder, callOrder)
	}
	for i := range wantOrder {
		if callOrder[i] != wantOrder[i] {
			t.Fatalf("expected call order %v, got %v", wantOrder, callOrder)
		}
	}

	if got := captured.metadata["main_module"]; got != "edge.js" {
		t.Errorf("expected main_module = edge.js, got: %v", got)
	}
	if got := captured.metadata["compatibility_date"]; got != goflare.DefaultCompatibilityDate {
		t.Errorf("expected compatibility_date = %s, got: %v", goflare.DefaultCompatibilityDate, got)
	}

	assets, ok := captured.metadata["assets"].(map[string]any)
	if !ok {
		t.Fatalf("expected metadata.assets to be an object, got: %v", captured.metadata["assets"])
	}
	if assets["jwt"] != "completion-jwt" {
		t.Errorf("expected assets.jwt = completion-jwt, got: %v", assets["jwt"])
	}
	config, ok := assets["config"].(map[string]any)
	if !ok {
		t.Fatalf("expected assets.config to be an object, got: %v", assets["config"])
	}
	runWorkerFirst, ok := config["run_worker_first"].([]any)
	if !ok || len(runWorkerFirst) != 2 || runWorkerFirst[0] != "/api/*" || runWorkerFirst[1] != "/oauth/*" {
		t.Errorf("expected run_worker_first = [/api/* /oauth/*], got: %v", config["run_worker_first"])
	}

	bindings, ok := captured.metadata["bindings"].([]any)
	if !ok {
		t.Fatalf("expected metadata.bindings to be present, got: %v", captured.metadata["bindings"])
	}
	var hasD1, hasR2 bool
	for _, b := range bindings {
		bm, _ := b.(map[string]any)
		switch bm["type"] {
		case "d1":
			hasD1 = bm["id"] == "d1-id-123"
		case "r2_bucket":
			hasR2 = bm["bucket_name"] == "r2-bucket-123"
		}
	}
	if !hasD1 {
		t.Errorf("expected a d1 binding with id d1-id-123, got: %v", bindings)
	}
	if !hasR2 {
		t.Errorf("expected an r2_bucket binding with bucket_name r2-bucket-123, got: %v", bindings)
	}

	if phase2Auth != "Bearer session-jwt" {
		t.Errorf("expected phase 2 to authenticate with the phase-1 jwt, got %q", phase2Auth)
	}
}
