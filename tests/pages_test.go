//go:build integration

package goflare_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tinywasm/goflare"
)

func TestGeneratePagesFiles_Integration(t *testing.T) {
	tmpDir, cleanup, err := TempDir()
	if err != nil {
		t.Fatalf("TempDir failed: %v", err)
	}
	defer cleanup()

	publicDir := filepath.Join(tmpDir, "public")
	os.MkdirAll(publicDir, 0755)
	os.WriteFile(filepath.Join(publicDir, "index.html"), []byte("index"), 0644)

	cfg := &goflare.Config{
		ProjectName: "test-pages",
		AccountID:   "abc123",
		PublicDir:   publicDir,
		OutputDir:   filepath.Join(tmpDir, ".goflare"),
	}

	g := goflare.New(cfg)
	if err := g.GeneratePagesFiles(); err != nil {
		t.Fatalf("GeneratePagesFiles failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(cfg.OutputDir, "dist", "index.html")); os.IsNotExist(err) {
		t.Errorf("dist/index.html not found")
	}
}
