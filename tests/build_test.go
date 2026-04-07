package goflare_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tinywasm/goflare"
)

func TestBuild_PagesOnly(t *testing.T) {
	tmpDir, cleanup, err := TempDir()
	if err != nil {
		t.Fatalf("TempDir failed: %v", err)
	}
	defer cleanup()

	publicDir := filepath.Join(tmpDir, "public")
	if err := os.MkdirAll(publicDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(publicDir, "index.html"), []byte("<h1>Hello</h1>"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	cfg := &goflare.Config{
		ProjectName: "test-pages",
		AccountID:   "abc123",
		PublicDir:   publicDir,
		OutputDir:   filepath.Join(tmpDir, ".goflare"),
	}

	g := goflare.New(cfg)
	if err := g.Build(); err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	distIndex := filepath.Join(cfg.OutputDir, "dist", "index.html")
	if _, err := os.Stat(distIndex); os.IsNotExist(err) {
		t.Errorf("dist/index.html not found")
	}
}

func TestBuild_NothingToBuild(t *testing.T) {
	cfg := &goflare.Config{
		ProjectName: "test",
		AccountID:   "abc123",
	}
	g := goflare.New(cfg)
	if err := g.Build(); err == nil {
		t.Error("expected error when nothing to build, got nil")
	}
}

func TestBuild_MissingPublicDir(t *testing.T) {
	cfg := &goflare.Config{
		ProjectName: "test",
		AccountID:   "abc123",
		PublicDir:   "nonexistent",
	}
	g := goflare.New(cfg)
	if err := g.Build(); err == nil {
		t.Error("expected error when public dir is missing, got nil")
	}
}

func TestBuildPages_CopiesFiles(t *testing.T) {
	tmpDir, cleanup, err := TempDir()
	if err != nil {
		t.Fatalf("TempDir failed: %v", err)
	}
	defer cleanup()

	publicDir := filepath.Join(tmpDir, "public")
	os.MkdirAll(filepath.Join(publicDir, "subdir"), 0755)
	os.WriteFile(filepath.Join(publicDir, "index.html"), []byte("index"), 0644)
	os.WriteFile(filepath.Join(publicDir, "subdir", "sub.txt"), []byte("sub"), 0644)

	cfg := &goflare.Config{
		ProjectName: "test",
		AccountID:   "abc123",
		PublicDir:   publicDir,
		OutputDir:   filepath.Join(tmpDir, ".goflare"),
	}
	g := goflare.New(cfg)
	if err := g.GeneratePagesFiles(); err != nil {
		t.Fatalf("GeneratePagesFiles failed: %v", err)
	}

	for _, rel := range []string{"dist/index.html", "dist/subdir/sub.txt"} {
		if _, err := os.Stat(filepath.Join(cfg.OutputDir, rel)); os.IsNotExist(err) {
			t.Errorf("expected file %s not found", rel)
		}
	}
}
