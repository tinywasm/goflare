package goflare

import (
	"fmt"
)

// GeneratePagesFiles generates all necessary files for Cloudflare Pages Functions using Advanced Mode
// generates the _worker.js file for Pages (Advanced Mode)
// https://developers.cloudflare.com/pages/functions/advanced-mode/
// This creates a single combined file with wasm_exec.js and runtime.mjs inline
func (g *Goflare) GeneratePagesFiles() error {

	err := g.generateWorkerFile()
	if err != nil {
		return fmt.Errorf("failed to generate _worker.js: %w", err)
	}

	err = g.generateWasmFile()
	if err != nil {
		return fmt.Errorf("failed to generate wasm: %w", err)
	}

	return nil
}
