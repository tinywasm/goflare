package goflare

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Build orchestrates the build pipeline.
func (g *Goflare) Build() error {
	if g.Config.Entry == "" && g.Config.PublicDir == "" {
		return errors.New("nothing to build: both Entry and PublicDir are empty")
	}

	var errs []error

	if g.Config.Entry != "" {
		if err := g.buildWorker(); err != nil {
			errs = append(errs, fmt.Errorf("worker build failed: %w", err))
		}
	}

	if g.Config.PublicDir != "" {
		if err := g.buildPages(); err != nil {
			errs = append(errs, fmt.Errorf("pages build failed: %w", err))
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

func (g *Goflare) buildWorker() error {
	if _, err := os.Stat(g.Config.Entry); os.IsNotExist(err) {
		return fmt.Errorf("entry file not found: %s", g.Config.Entry)
	}

	if err := os.MkdirAll(g.Config.OutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	if err := g.generateWasmFile(); err != nil {
		return err
	}

	if err := g.generateWorkerFile(); err != nil {
		return err
	}

	return nil
}

func (g *Goflare) buildPages() error {
	if _, err := os.Stat(g.Config.PublicDir); os.IsNotExist(err) {
		return fmt.Errorf("public directory not found: %s", g.Config.PublicDir)
	}

	distDir := filepath.Join(g.Config.OutputDir, "dist")
	if err := os.MkdirAll(distDir, 0755); err != nil {
		return fmt.Errorf("failed to create dist directory: %w", err)
	}

	return filepath.Walk(g.Config.PublicDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(g.Config.PublicDir, path)
		if err != nil {
			return err
		}

		destPath := filepath.Join(distDir, relPath)

		if info.IsDir() {
			return os.MkdirAll(destPath, info.Mode())
		}

		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		destFile, err := os.Create(destPath)
		if err != nil {
			return err
		}
		defer destFile.Close()

		_, err = io.Copy(destFile, srcFile)
		return err
	})
}
