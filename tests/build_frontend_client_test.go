//go:build !wasm

package goflare_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tinywasm/goflare"
)

// El compilador de frontend resuelve web/client.go desde la raiz del modulo.
// Pasarle web/ hacia que buscara web/web/client.go: el build moria, o peor,
// compilaba una copia duplicada si alguien la creaba para "arreglarlo" —y
// entonces los cambios en web/client.go dejaban de llegar al binario servido.
//
// Los demas tests de build usan rutas absolutas de t.TempDir(), que enmascaran
// el problema porque nunca llegan a entrar en esta rama.
func TestBuild_FrontendWasmFromModuleRoot(t *testing.T) {
	t.Chdir(t.TempDir())

	publicDir := filepath.Join("web", "public")
	if err := os.MkdirAll(publicDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("web", "client.go"), []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &goflare.Config{
		ProjectName:  "test",
		AccountID:    "123",
		PublicDir:    publicDir,
		OutputDir:    ".build",
		CompilerMode: "L", // stdlib: el test no necesita TinyGo instalado
	}

	if err := goflare.New(cfg).Build(); err != nil {
		t.Fatalf("Build fallo: %v", err)
	}

	if _, err := os.Stat(filepath.Join(publicDir, "client.wasm")); err != nil {
		t.Errorf("client.wasm no se genero en PublicDir: %v", err)
	}
	if _, err := os.Stat(filepath.Join("web", "web")); err == nil {
		t.Error("se creo web/web: el entry se sigue resolviendo desde web/")
	}
}
