//go:build !wasm

package goflare

func (g *Goflare) GenerateWorkerFiles() error {
	return g.buildWorker()
}
