package goflare

func (g *Goflare) generateWasmFile() error {
	return g.tw.RecompileMainWasm()
}
