package goflare

import (
	"os"
	"path/filepath"
)

func (g *Goflare) generateWasmFile() error {
	if err := g.edgeCompiler.RecompileMainWasm(); err != nil {
		return err
	}

	producedName := filepath.Base(g.Config.Entry) + ".wasm"
	destName := "edge.wasm"

	if producedName != destName {
		src := filepath.Join(g.Config.OutputDir, producedName)
		dst := filepath.Join(g.Config.OutputDir, destName)
		if _, err := os.Stat(src); err == nil {
			// Rename it to edge.wasm so deployment can find it
			return os.Rename(src, dst)
		}
	}

	return nil
}
