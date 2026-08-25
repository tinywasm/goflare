//go:build !wasm

package goflare_test

import (
	"github.com/tinywasm/goflare"
	"io"
	"os"
	"os/exec"
	"testing"
)

// TestEdgeSize verifica el límite duro de Cloudflare (<1 MB) para .build/edge.wasm.
// SRP: migrado desde veltylabs/misitio/tests/edge_size_test.go — es límite de
// plataforma (goflare/Cloudflare), no de dominio misitio.
func TestEdgeSize(t *testing.T) {
	if err := goflare.EnsureTinyGo(io.Discard); err != nil && os.Getenv("CI") != "" {
		t.Fatalf("no se pudo instalar tinygo en CI: %v", err)
	}
	tinygoPath, err := exec.LookPath("tinygo")
	if err != nil {
		if os.Getenv("CI") != "" {
			t.Fatalf("tinygo no esta instalado en el PATH en CI: %v", err)
		}
		t.Skip("tinygo no esta instalado en el PATH")
	}
	tmpFile, err := os.CreateTemp("", "edge-*.wasm")
	if err != nil {
		t.Fatalf("falla al crear archivo temporal: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	// Crea un main mínimo temporal que importa cloudflare/edge para medir el
	// costo base de la plataforma sin lógica de dominio.
	tmpDir, err := os.MkdirTemp("", "edge-main-*")
	if err != nil {
		t.Fatalf("falla al crear dir temporal: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	mainPath := tmpDir + "/main.go"
	if err := os.WriteFile(mainPath, []byte("package main\nimport _ \"github.com/tinywasm/cloudflare/edge\"\nfunc main(){}\n"), 0644); err != nil {
		t.Fatalf("falla al escribir main temporal: %v", err)
	}
	cmd := exec.Command(tinygoPath, "build", "-target", "wasm", "-no-debug", "-o", tmpPath, mainPath)
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("falla al compilar edge con tinygo: %v\n%s", err, string(out))
	}
	info, err := os.Stat(tmpPath)
	if err != nil {
		t.Fatalf("falla al obtener tamano del binario: %v", err)
	}
	const maxBytes = 1048576
	if info.Size() > maxBytes {
		t.Fatalf("edge.wasm supera el limite de Cloudflare: %d bytes (maximo 1048576)", info.Size())
	}
}
