package goflare_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinywasm/goflare"
)

func TestInit_PromptsAndReturnsConfig(t *testing.T) {
	input := "my-project\nmy-account\nexample.com\nweb/main.go\nweb/public\nmy-worker\n"
	in := strings.NewReader(input)
	out := &bytes.Buffer{}

	cfg, err := goflare.Init(in, out)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	if cfg.ProjectName != "my-project" {
		t.Errorf("Expected ProjectName my-project, got %s", cfg.ProjectName)
	}
	if cfg.AccountID != "my-account" {
		t.Errorf("Expected AccountID my-account, got %s", cfg.AccountID)
	}
	if cfg.Domain != "example.com" {
		t.Errorf("Expected Domain example.com, got %s", cfg.Domain)
	}
	if cfg.Entry != "web/main.go" {
		t.Errorf("Expected Entry web/main.go, got %s", cfg.Entry)
	}
	if cfg.PublicDir != "web/public" {
		t.Errorf("Expected PublicDir web/public, got %s", cfg.PublicDir)
	}
	if cfg.WorkerName != "my-worker" {
		t.Errorf("Expected WorkerName my-worker, got %s", cfg.WorkerName)
	}
}

func TestInit_AutoDetect(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "goflare-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	os.MkdirAll("edge", 0755)
	os.WriteFile(filepath.Join("edge", "main.go"), []byte("package main"), 0644)

	// In this case, it should skip the Entry prompt.
	// Input: ProjectName, AccountID, Domain, PublicDir, WorkerName
	input := "my-project\nmy-account\nexample.com\nweb/public\nmy-worker\n"
	in := strings.NewReader(input)
	out := &bytes.Buffer{}

	cfg, err := goflare.Init(in, out)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	if cfg.Entry != "edge" {
		t.Errorf("Expected Entry to be 'edge' (auto-detected), got %s", cfg.Entry)
	}
	if !strings.Contains(out.String(), "edge/main.go detected") {
		t.Error("Expected output to mention auto-detection")
	}
}

func TestInit_ErrorWhenBothEmpty(t *testing.T) {
	input := "my-project\nmy-account\n\n\n\n\n"
	in := strings.NewReader(input)
	out := &bytes.Buffer{}

	_, err := goflare.Init(in, out)
	if err == nil {
		t.Fatal("Expected error when both Entry and PublicDir are empty, got nil")
	}
	if !strings.Contains(err.Error(), "at least one of Entry or PublicDir is required") {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestWriteEnvFile(t *testing.T) {
	tmpDir, cleanup, err := TempDir()
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer cleanup()

	cfg := &goflare.Config{
		ProjectName: "test-project",
		AccountID:   "account-123",
		Domain:      "test.com",
		WorkerName:  "worker-123",
		Entry:       "main.go",
		PublicDir:   "public",
	}

	envPath := filepath.Join(tmpDir, ".env")
	err = goflare.WriteEnvFile(cfg, envPath)
	if err != nil {
		t.Fatalf("WriteEnvFile failed: %v", err)
	}

	content, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("Failed to read .env file: %v", err)
	}

	expectedFields := []string{
		"PROJECT_NAME=test-project",
		"CLOUDFLARE_ACCOUNT_ID=account-123",
		"DOMAIN=test.com",
		"WORKER_NAME=worker-123",
		"ENTRY=main.go",
		"PUBLIC_DIR=public",
	}

	for _, field := range expectedFields {
		if !strings.Contains(string(content), field) {
			t.Errorf("Expected field %s not found in .env", field)
		}
	}
}

func TestWriteEnvFile_OmitsEmptyFields(t *testing.T) {
	tmpDir, cleanup, err := TempDir()
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer cleanup()

	cfg := &goflare.Config{
		ProjectName: "test-project",
		AccountID:   "account-123",
		Entry:       "main.go",
	}

	envPath := filepath.Join(tmpDir, ".env")
	err = goflare.WriteEnvFile(cfg, envPath)
	if err != nil {
		t.Fatalf("WriteEnvFile failed: %v", err)
	}

	content, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("Failed to read .env file: %v", err)
	}

	if strings.Contains(string(content), "DOMAIN=") {
		t.Error("DOMAIN should be omitted when empty")
	}
	if strings.Contains(string(content), "PUBLIC_DIR=") {
		t.Error("PUBLIC_DIR should be omitted when empty")
	}
}

func TestUpdateGitignore_Creates(t *testing.T) {
	tmpDir, cleanup, err := TempDir()
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer cleanup()

	err = goflare.UpdateGitignore(tmpDir)
	if err != nil {
		t.Fatalf("UpdateGitignore failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(tmpDir, ".gitignore"))
	if err != nil {
		t.Fatalf("Failed to read .gitignore: %v", err)
	}

	if !strings.Contains(string(content), ".env") {
		t.Error(".gitignore should contain .env")
	}
	if !strings.Contains(string(content), ".build/") {
		t.Error(".gitignore should contain .build/")
	}
}

func TestUpdateGitignore_Appends(t *testing.T) {
	tmpDir, cleanup, err := TempDir()
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer cleanup()

	gitignorePath := filepath.Join(tmpDir, ".gitignore")
	initialContent := "node_modules\n"
	os.WriteFile(gitignorePath, []byte(initialContent), 0644)

	err = goflare.UpdateGitignore(tmpDir)
	if err != nil {
		t.Fatalf("UpdateGitignore failed: %v", err)
	}

	content, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("Failed to read .gitignore: %v", err)
	}

	if !strings.HasPrefix(string(content), "node_modules") {
		t.Error(".gitignore should preserve existing content")
	}
	if !strings.Contains(string(content), ".env") {
		t.Error(".gitignore should contain .env")
	}
}

func TestUpdateGitignore_Idempotent(t *testing.T) {
	tmpDir, cleanup, err := TempDir()
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer cleanup()

	err = goflare.UpdateGitignore(tmpDir)
	if err != nil {
		t.Fatalf("First UpdateGitignore failed: %v", err)
	}

	err = goflare.UpdateGitignore(tmpDir)
	if err != nil {
		t.Fatalf("Second UpdateGitignore failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(tmpDir, ".gitignore"))
	if err != nil {
		t.Fatalf("Failed to read .gitignore: %v", err)
	}

	if strings.Count(string(content), ".env") != 1 {
		t.Errorf(".env should only appear once in .gitignore, found %d times", strings.Count(string(content), ".env"))
	}
}
