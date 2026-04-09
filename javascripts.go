package goflare

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed assets/worker.mjs
var embeddedWorkerMjs []byte

//go:embed assets/runtime.mjs
var embeddedRuntimeMjs []byte

//go:embed assets/wasm_exec_worker.js
var embeddedWasmExecWorker []byte

// generateWorkerFile copies the three pre-built JS assets for a Cloudflare Worker
// into OutputDir. Files are embedded at compile time — no generation needed.
//
//   - worker.mjs         — ES module entry, calls binding.handleRequest(req)
//   - runtime.mjs        — loads worker.wasm, exposes createRuntimeContext
//   - wasm_exec.js    — TinyGo runtime with context Proxy patch
func (g *Goflare) generateWorkerFile() error {
	files := []struct {
		name string
		data []byte
	}{
		{"worker.mjs", embeddedWorkerMjs},
		{"runtime.mjs", embeddedRuntimeMjs},
		{"wasm_exec.js", embeddedWasmExecWorker},
	}
	for _, f := range files {
		dest := filepath.Join(g.Config.OutputDir, f.name)
		if err := os.WriteFile(dest, f.data, 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", f.name, err)
		}
	}
	return nil
}
