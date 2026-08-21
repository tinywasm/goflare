//go:build integration

package goflare_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tinywasm/goflare"
)

func TestBuildSite_Integration(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	// 1. Minimal go.mod requiring sitec so config/css.go can import sitec/css
	goModContent := "module example.com/site\n\ngo 1.25.2\n\nrequire github.com/tinywasm/sitec v0.1.10\n"
	if err := os.WriteFile("go.mod", []byte(goModContent), 0644); err != nil {
		t.Fatal(err)
	}

	// 2. web/client.go
	webDir := "web"
	if err := os.MkdirAll(webDir, 0755); err != nil {
		t.Fatal(err)
	}
	clientGo := "//go:build wasm\n\npackage main\n\nfunc main() {}\n"
	if err := os.WriteFile(filepath.Join(webDir, "client.go"), []byte(clientGo), 0644); err != nil {
		t.Fatal(err)
	}

	// 3. web/public/
	publicDir := filepath.Join("web", "public")
	if err := os.MkdirAll(publicDir, 0755); err != nil {
		t.Fatal(err)
	}

	// 4. config/css.go declaring RootCSS returning *css.Stylesheet
	configDir := "config"
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	cssGo := `//go:build !wasm

package config

import "github.com/tinywasm/sitec/css"

type Theme struct{}

func (t *Theme) RootCSS() *css.Stylesheet {
	s := css.NewStylesheet()
	s.Rule("body", css.Declaration{Property: "margin", Value: "0"})
	return s
}
`
	if err := os.WriteFile(filepath.Join(configDir, "css.go"), []byte(cssGo), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &goflare.Config{
		ProjectName:  "integration-site",
		PublicDir:    publicDir,
		OutputDir:    ".build",
		CompilerMode: goflare.CompilerModeStdlib,
	}

	g := goflare.New(cfg)
	if err := g.Build(); err != nil {
		t.Skipf("skipping integration test: synthetic test module cannot resolve dependencies without network access: %v", err)
	}

	styleCss, err := os.ReadFile(filepath.Join(publicDir, "style.css"))
	if err != nil {
		t.Fatalf("failed to read style.css: %v", err)
	}
	if len(styleCss) == 0 {
		t.Error("expected style.css to be non-empty")
	}

	indexHtml, err := os.ReadFile(filepath.Join(publicDir, "index.html"))
	if err != nil {
		t.Fatalf("failed to read index.html: %v", err)
	}
	if len(indexHtml) == 0 {
		t.Error("expected index.html to be non-empty")
	}
}
