package goflare_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinywasm/goflare"
)

func TestInit_PromptsAndReturnsConfig(t *testing.T) {
	input := "myapp\nabc123\nmyapp.example.com\nweb/server.go\nweb/public\nmyapp-worker\n"
	in := strings.NewReader(input)

	cfg, err := goflare.Init(in)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	if cfg.ProjectName != "myapp" {
		t.Errorf("expected ProjectName myapp, got %s", cfg.ProjectName)
	}
	if cfg.AccountID != "abc123" {
		t.Errorf("expected AccountID abc123, got %s", cfg.AccountID)
	}
	if cfg.Domain != "myapp.example.com" {
		t.Errorf("expected Domain myapp.example.com, got %s", cfg.Domain)
	}
	if cfg.Entry != "web/server.go" {
		t.Errorf("expected Entry web/server.go, got %s", cfg.Entry)
	}
	if cfg.PublicDir != "web/public" {
		t.Errorf("expected PublicDir web/public, got %s", cfg.PublicDir)
	}
	if cfg.WorkerName != "myapp-worker" {
		t.Errorf("expected WorkerName myapp-worker, got %s", cfg.WorkerName)
	}
}

func TestInit_ErrorWhenBothEmpty(t *testing.T) {
	input := "myapp\nabc123\n\n-\n-\n\n"
	in := strings.NewReader(input)

	_, err := goflare.Init(in)
	if err == nil {
		t.Fatal("expected error when Entry and PublicDir are both empty, got nil")
	}
	if !strings.Contains(err.Error(), "at least one of Entry or PublicDir is required") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestWriteEnvFile(t *testing.T) {
	tmpDir, cleanup, err := TempDir()
	if err != nil {
		t.Fatalf("TempDir failed: %v", err)
	}
	defer cleanup()

	cfg := &goflare.Config{
		ProjectName: "myapp",
		AccountID:   "abc123",
		Domain:      "myapp.example.com",
		WorkerName:  "myapp-worker",
		Entry:       "web/server.go",
		PublicDir:   "web/public",
	}

	envPath := filepath.Join(tmpDir, ".env")
	if err := goflare.WriteEnvFile(cfg, envPath); err != nil {
		t.Fatalf("WriteEnvFile failed: %v", err)
	}

	content, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	expected := []string{
		"PROJECT_NAME=myapp",
		"CLOUDFLARE_ACCOUNT_ID=abc123",
		"DOMAIN=myapp.example.com",
		"WORKER_NAME=myapp-worker",
		"ENTRY=web/server.go",
		"PUBLIC_DIR=web/public",
	}

	for _, s := range expected {
		if !strings.Contains(string(content), s) {
			t.Errorf("expected .env to contain %s", s)
		}
	}
}

func TestWriteEnvFile_OmitsEmptyFields(t *testing.T) {
	tmpDir, cleanup, err := TempDir()
	if err != nil {
		t.Fatalf("TempDir failed: %v", err)
	}
	defer cleanup()

	cfg := &goflare.Config{
		ProjectName: "myapp",
		AccountID:   "abc123",
	}

	envPath := filepath.Join(tmpDir, ".env")
	if err := goflare.WriteEnvFile(cfg, envPath); err != nil {
		t.Fatalf("WriteEnvFile failed: %v", err)
	}

	content, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	if strings.Contains(string(content), "DOMAIN=") {
		t.Error("expected .env to omit DOMAIN")
	}
}

func TestUpdateGitignore_Creates(t *testing.T) {
	tmpDir, cleanup, err := TempDir()
	if err != nil {
		t.Fatalf("TempDir failed: %v", err)
	}
	defer cleanup()

	if err := goflare.UpdateGitignore(tmpDir); err != nil {
		t.Fatalf("UpdateGitignore failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(tmpDir, ".gitignore"))
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	if !strings.Contains(string(content), ".env") {
		t.Error("expected .gitignore to contain .env")
	}
	if !strings.Contains(string(content), ".goflare/") {
		t.Error("expected .gitignore to contain .goflare/")
	}
}

func TestUpdateGitignore_Appends(t *testing.T) {
	tmpDir, cleanup, err := TempDir()
	if err != nil {
		t.Fatalf("TempDir failed: %v", err)
	}
	defer cleanup()

	path := filepath.Join(tmpDir, ".gitignore")
	if err := os.WriteFile(path, []byte("node_modules\n"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	if err := goflare.UpdateGitignore(tmpDir); err != nil {
		t.Fatalf("UpdateGitignore failed: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	if !strings.Contains(string(content), "node_modules") {
		t.Error("expected .gitignore to preserve existing content")
	}
	if !strings.Contains(string(content), ".env") {
		t.Error("expected .gitignore to contain .env")
	}
}

func TestUpdateGitignore_Idempotent(t *testing.T) {
	tmpDir, cleanup, err := TempDir()
	if err != nil {
		t.Fatalf("TempDir failed: %v", err)
	}
	defer cleanup()

	if err := goflare.UpdateGitignore(tmpDir); err != nil {
		t.Fatalf("UpdateGitignore failed: %v", err)
	}
	if err := goflare.UpdateGitignore(tmpDir); err != nil {
		t.Fatalf("UpdateGitignore failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(tmpDir, ".gitignore"))
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	if strings.Count(string(content), ".env") != 1 {
		t.Errorf("expected .env to appear only once in .gitignore, got %d", strings.Count(string(content), ".env"))
	}
}
