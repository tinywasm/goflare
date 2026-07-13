//go:build !wasm

package goflare_test

import (
	"os"
	"path/filepath"
	"testing"
)

// testEnv is a pre-wired temporary project layout:
//
//	<root>/
//	  web/public/       ← PublicDir
//	    index.html
//	  .build/           ← OutputDir
type testEnv struct {
	Root      string
	PublicDir string
	OutputDir string
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	// t.TempDir() creates a temp dir and registers t.Cleanup(os.RemoveAll)
	// automatically — runs even on test failure or interrupt.
	dir := t.TempDir()
	pub := filepath.Join(dir, "web", "public")
	out := filepath.Join(dir, ".build")
	os.MkdirAll(pub, 0755)
	os.MkdirAll(out, 0755)
	os.WriteFile(filepath.Join(pub, "index.html"), []byte("<h1>test</h1>"), 0644)
	return &testEnv{Root: dir, PublicDir: pub, OutputDir: out}
}

// writePublic writes a file into PublicDir.
func (e *testEnv) writePublic(name, content string) {
	fullPath := filepath.Join(e.PublicDir, name)
	os.MkdirAll(filepath.Dir(fullPath), 0755)
	os.WriteFile(fullPath, []byte(content), 0644)
}

// writeOutput writes a file into OutputDir (simulates pre-built artifacts).
func (e *testEnv) writeOutput(name, content string) {
	fullPath := filepath.Join(e.OutputDir, name)
	os.MkdirAll(filepath.Dir(fullPath), 0755)
	os.WriteFile(fullPath, []byte(content), 0644)
}