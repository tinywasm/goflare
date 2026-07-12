//go:build !wasm

package goflare

func (g *Goflare) GeneratePagesFiles() error {
	return g.buildPages()
}
