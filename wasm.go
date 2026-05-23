//go:build !wasm

package goflare

func (g *Goflare) generateWasmFile() error {
	return g.edgeCompiler.RecompileMainWasm()
}
