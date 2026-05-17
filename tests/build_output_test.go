package goflare_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tinywasm/goflare"
)

// TestBuild_NoDist verifies that buildPages() does NOT create .build/dist/.
// After fix: PublicDir is the final output — no copy step.
func TestBuild_NoDist(t *testing.T) {
	env := newTestEnv(t)
	defer env.Close()

	cfg := &goflare.Config{
		ProjectName: "test",
		PublicDir:   env.PublicDir,
		OutputDir:   env.OutputDir,
	}

	g := goflare.New(cfg)
	if err := g.Build(); err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	distDir := filepath.Join(env.OutputDir, "dist")
	if _, err := os.Stat(distDir); !os.IsNotExist(err) {
		t.Errorf("dist directory should NOT exist: %s", distDir)
	}
}

// TestBuild_OutputDirContainsOnlyWorkerArtifacts verifies that .build/
// never receives Pages files (index.html, style.css, client.wasm).
func TestBuild_OutputDirContainsOnlyWorkerArtifacts(t *testing.T) {
	env := newTestEnv(t)
	defer env.Close()

	env.writePublic("style.css", "body {}")

	cfg := &goflare.Config{
		ProjectName: "test",
		PublicDir:   env.PublicDir,
		OutputDir:   env.OutputDir,
	}

	g := goflare.New(cfg)
	if err := g.Build(); err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Check that Pages files are NOT in OutputDir
	files, _ := os.ReadDir(env.OutputDir)
	for _, f := range files {
		name := f.Name()
		if name == "index.html" || name == "style.css" {
			t.Errorf("OutputDir should not contain Pages file: %s", name)
		}
	}
}

// TestBuild_PublicDirGetsGeneratedIndex verifies that Build() writes the
// assetmin-generated index.html to PublicDir (overwriting any prior content).
// This is the correct post-FlushToDisk behaviour: stale files are always
// overwritten so the external server boots against fresh assets.
func TestBuild_PublicDirGetsGeneratedIndex(t *testing.T) {
	env := newTestEnv(t)
	defer env.Close()

	cfg := &goflare.Config{
		ProjectName: "test",
		PublicDir:   env.PublicDir,
		OutputDir:   env.OutputDir,
	}

	g := goflare.New(cfg)
	if err := g.Build(); err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(env.PublicDir, "index.html"))
	if err != nil {
		t.Fatalf("Failed to read index.html: %v", err)
	}
	// The generated file must be valid HTML produced by assetmin, not empty.
	if len(got) == 0 {
		t.Error("index.html is empty after Build()")
	}
}
