package goflare_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tinywasm/goflare"
)

func TestBuild_PagesOnly(t *testing.T) {
	env := newTestEnv(t)
	defer env.Close()

	env.writePublic("assets/style.css", "body {}")

	cfg := &goflare.Config{
		ProjectName: "test",
		AccountID:   "123",
		PublicDir:   env.PublicDir,
		OutputDir:   env.OutputDir,
	}

	g := goflare.New(cfg)
	err := g.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(env.PublicDir, "index.html")); err != nil {
		t.Errorf("index.html missing in PublicDir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(env.PublicDir, "assets", "style.css")); err != nil {
		t.Errorf("style.css missing in PublicDir: %v", err)
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
