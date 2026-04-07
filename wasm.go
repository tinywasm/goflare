package goflare

import (
	"fmt"
	"os"
)

func (g *Goflare) generateWasmFile() error {
	// Create output directory if it doesn't exist
	if err := os.MkdirAll(g.Config.OutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Tinywasm client handles the recompilation
	// WasmClient already uses OutputDir from Config via Goflare.New()
	return g.tw.RecompileMainWasm()
}
