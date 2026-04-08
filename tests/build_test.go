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
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer cleanup()

	publicDir := filepath.Join(tmpDir, "public")
	os.MkdirAll(publicDir, 0755)
	os.WriteFile(filepath.Join(publicDir, "index.html"), []byte("<h1>Hello</h1>"), 0644)
	os.MkdirAll(filepath.Join(publicDir, "assets"), 0755)
	os.WriteFile(filepath.Join(publicDir, "assets", "style.css"), []byte("body {}"), 0644)

	outputDir := filepath.Join(tmpDir, ".goflare")

	cfg := &goflare.Config{
		ProjectName: "test",
		AccountID:   "123",
		PublicDir:   publicDir,
		OutputDir:   outputDir,
	}

	g := goflare.New(cfg)
	err = g.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	distDir := filepath.Join(outputDir, "dist")
	if _, err := os.Stat(filepath.Join(distDir, "index.html")); err != nil {
		t.Errorf("index.html missing in dist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(distDir, "assets", "style.css")); err != nil {
		t.Errorf("style.css missing in dist: %v", err)
	}
}

func TestBuild_NothingToBuild(t *testing.T) {
	cfg := &goflare.Config{
		ProjectName: "test",
		AccountID:   "123",
		Entry:       "",
		PublicDir:   "",
	}
	g := goflare.New(cfg)
	err := g.Build()
	if err == nil {
		t.Fatal("Expected error when nothing to build")
	}
}

func TestBuild_MissingEntry(t *testing.T) {
	cfg := &goflare.Config{
		ProjectName: "test",
		AccountID:   "123",
		Entry:       "non-existent.go",
	}
	g := goflare.New(cfg)
	err := g.Build()
	if err == nil {
		t.Fatal("Expected error when Entry does not exist")
	}
}

func TestBuild_MissingPublicDir(t *testing.T) {
	cfg := &goflare.Config{
		ProjectName: "test",
		AccountID:   "123",
		PublicDir:   "non-existent-dir",
	}
	g := goflare.New(cfg)
	err := g.Build()
	if err == nil {
		t.Fatal("Expected error when PublicDir does not exist")
	}
}
