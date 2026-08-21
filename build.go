//go:build !wasm

package goflare

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tinywasm/sitec"
)

// Build orchestrates the build pipeline as a method.
//
// Mode is inferred from edge/main.go imports (D11):
//   - pages-functions: edge/main.go imports github.com/tinywasm/goflare/edge
//     → output functions/[[path]].mjs + functions/edge.wasm
//   - workers:         edge/main.go imports github.com/tinywasm/goflare/workers
//     → output .build/edge.js + .build/edge.wasm (legacy)
//   - pages (static):  no edge/main.go but PublicDir exists
//     → only static + optional frontend wasm
func (g *Goflare) Build() error {
	if g.stagingDir != g.Config.OutputDir {
		defer os.RemoveAll(g.stagingDir)
	}

	if g.Config.Entry == "" && g.Config.PublicDir == "" {
		return errors.New("nothing to build: both Entry and PublicDir are empty")
	}

	mode := ModeUnknown
	if g.Config.Entry != "" {
		m, err := inferMode(g.Config.Entry, g.Config.PublicDir)
		if err != nil {
			return fmt.Errorf("mode detection failed: %w", err)
		}
		mode = m
	} else if g.Config.PublicDir != "" {
		mode = ModePagesStatic
	}

	var buildErrors []error

	switch mode {
	case ModePagesFunctions:
		if err := g.buildPagesFunctions(); err != nil {
			buildErrors = append(buildErrors, fmt.Errorf("pages-functions build failed: %w", err))
		}
		if g.Config.PublicDir != "" {
			if err := g.buildPages(); err != nil {
				buildErrors = append(buildErrors, fmt.Errorf("pages build failed: %w", err))
			}
		}
	case ModeWorkers:
		if err := g.buildWorker(); err != nil {
			buildErrors = append(buildErrors, fmt.Errorf("worker build failed: %w", err))
		}
		if g.Config.PublicDir != "" {
			if err := g.buildPages(); err != nil {
				buildErrors = append(buildErrors, fmt.Errorf("pages build failed: %w", err))
			}
		}
	case ModePagesStatic:
		if err := g.buildPages(); err != nil {
			buildErrors = append(buildErrors, fmt.Errorf("pages build failed: %w", err))
		}
	default:
		return errors.New("could not determine build mode from edge/main.go imports")
	}

	if len(buildErrors) > 0 {
		return errors.Join(buildErrors...)
	}

	return nil
}

// maxWasmSize is the Cloudflare Workers/Pages Free limit for the WASM binary.
// https://developers.cloudflare.com/workers/platform/limits/#worker-size
const maxWasmSize = 1 * 1024 * 1024 // 1 MiB

// CompilerModeStdlib compila el frontend con el Go estandar en vez de TinyGo:
// binario grande, compilacion rapida, sin minificar. Es el modo de desarrollo.
const CompilerModeStdlib = "L"

// siteMode traduce el modo de compilador de goflare al de sitec.
func siteMode(compilerMode string) sitec.Mode {
	if compilerMode == CompilerModeStdlib {
		return sitec.ModeDev
	}
	return sitec.ModeRelease
}

const (
	// moduleRoot es el directorio desde el que corre goflare. Toda ruta de
	// fuente del proyecto —edge/main.go, web/client.go— se resuelve desde aqui.
	moduleRoot = "."
)

func checkWasmSize(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("wasm size check: %w", err)
	}
	size := info.Size()
	if size > maxWasmSize {
		return fmt.Errorf(
			"edge.wasm exceeds Cloudflare Free limit: %d bytes (%.1f KiB) > 1 MiB — "+
				"reduce binary size or upgrade to a paid plan",
			size, float64(size)/1024,
		)
	}
	return nil
}

// buildPagesFunctions compiles edge/main.go to functions/edge.wasm and writes the
// glue bundle functions/[[path]].mjs (catch-all, exports onRequest only).
//
// Both outputs end up in the project tree (no .build/ staging visible to the dev)
// so the dev commits them and CF Git Integration deploys them as-is (D8).
//
// Implementation note: sitec.WasmBuilder compiles to memory; we write edge.wasm into staging
// and then move edge.wasm into functions/.
func (g *Goflare) buildPagesFunctions() error {
	if _, err := os.Stat(g.Config.Entry); os.IsNotExist(err) {
		return fmt.Errorf("entry path does not exist: %s", g.Config.Entry)
	}

	functionsDir := g.functionsDir()
	if err := os.MkdirAll(functionsDir, 0755); err != nil {
		return fmt.Errorf("failed to create functions dir: %w", err)
	}

	if err := g.generateWasmFile(); err != nil {
		return err
	}

	srcWasm := filepath.Join(g.stagingDir, "edge.wasm")
	dstWasm := filepath.Join(functionsDir, "edge.wasm")
	if err := moveFile(srcWasm, dstWasm); err != nil {
		return fmt.Errorf("failed to move edge.wasm to %s: %w", functionsDir, err)
	}

	if err := checkWasmSize(dstWasm); err != nil {
		return err
	}

	if err := g.generatePagesFunctionFile(); err != nil {
		return err
	}

	return nil
}

// moveFile renames src to dst, falling back to copy+delete across filesystems.
func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dst, data, 0644); err != nil {
		return err
	}
	return os.Remove(src)
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

	// 5. Move files from staging to OutputDir
	for _, name := range []string{"edge.wasm", "edge.js"} {
		src := filepath.Join(g.stagingDir, name)
		dst := filepath.Join(g.Config.OutputDir, name)
		if err := moveFile(src, dst); err != nil && !os.IsNotExist(err) {
			return err
		}
	}

	// 6. Check WASM size
	wasmPath := filepath.Join(g.Config.OutputDir, "edge.wasm")
	if err := checkWasmSize(wasmPath); err != nil {
		return err
	}

	return nil
}

func (g *Goflare) buildPages() error {
	// 1. Verify that PUBLIC_DIR exists
	if _, err := os.Stat(g.Config.PublicDir); os.IsNotExist(err) {
		return fmt.Errorf("public dir does not exist: %s", g.Config.PublicDir)
	}

	// 2. sitec recorre el modulo, recoge lo que el proyecto declara, compila
	//    el WASM del frontend y arma el sitio completo en memoria.
	g.Logger("building site →", g.Config.PublicDir)
	site, err := g.siteBuilder(sitec.BuildConfig{
		RootDir:   moduleRoot,
		Mode:      siteMode(g.Config.CompilerMode),
		OutputDir: g.Config.PublicDir,
		AppName:   g.Config.ProjectName,
		Log:       g.Logger,
	})
	if err != nil {
		return fmt.Errorf("site build failed: %w", err)
	}

	// 3. Nada existe hasta que se vuelca: Build() trabaja en memoria.
	if err := site.WriteTo(sitec.NewOsFS()); err != nil {
		return fmt.Errorf("failed to write site artifacts: %w", err)
	}

	return nil
}
