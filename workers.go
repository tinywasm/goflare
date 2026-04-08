package goflare

func (g *Goflare) GenerateWorkerFiles() error {
	return g.buildWorker()
}
