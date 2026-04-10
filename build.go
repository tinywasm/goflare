package goflare

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Build orchestrates the build pipeline as a method.
func (g *Goflare) Build() error {
	if g.Config.Entry == "" && g.Config.PublicDir == "" {
		return errors.New("nothing to build: both Entry and PublicDir are empty")
	}

	var buildErrors []error

	if g.Config.Entry != "" {
		if err := g.buildWorker(); err != nil {
			buildErrors = append(buildErrors, fmt.Errorf("worker build failed: %w", err))
		}
	}

	if g.Config.PublicDir != "" {
		if err := g.buildPages(); err != nil {
			buildErrors = append(buildErrors, fmt.Errorf("pages build failed: %w", err))
		}
	}

	if len(buildErrors) > 0 {
		return errors.Join(buildErrors...)
	}

	return nil
}

func (g *Goflare) buildWorker() error {
	// 1. Verify Entry file exists
	if _, err := os.Stat(g.Config.Entry); os.IsNotExist(err) {
		return fmt.Errorf("entry path does not exist: %s", g.Config.Entry)
	}

	// 2. Ensure OutputDir exists
	if err := os.MkdirAll(g.Config.OutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// 3. Call generateWasmFile()
	if err := g.generateWasmFile(); err != nil {
		return err
	}

	// 4. Call generateWorkerFile()
	if err := g.generateWorkerFile(); err != nil {
		return err
	}

	return nil
}

func (g *Goflare) buildPages() error {
	// 1. Verify that PUBLIC_DIR exists
	if _, err := os.Stat(g.Config.PublicDir); os.IsNotExist(err) {
		return fmt.Errorf("public dir does not exist: %s", g.Config.PublicDir)
	}

	// 2. Compile frontend WASM if web/client.go exists
	frontEntry := filepath.Join("web", "client.go")
	if _, err := os.Stat(frontEntry); err == nil {
		if g.browserCompiler == nil {
			return fmt.Errorf("frontend compiler not initialized (browserCompiler is nil)")
		}
		g.Logger("compiling frontend WASM: web/client.go →", g.Config.PublicDir)
		if err := g.browserCompiler.Compile(); err != nil {
			return fmt.Errorf("frontend WASM compilation failed: %w", err)
		}
	}

	// 3. Generate script.js + style.css via assetmin
	if g.assetMin != nil {
		g.Logger("generating assets: script.js, style.css →", g.Config.PublicDir)
		g.assetMin.SetBuildOnDisk(true)
	}

	return nil
}
