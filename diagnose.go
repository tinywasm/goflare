//go:build !wasm

package goflare

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tinywasm/gobuild"
)

const (
	tinygoCmdName      = "tinygo"
	tinygoBuildSubcmd  = "build"
	tinygoFlagTarget   = "-target"
	tinygoTargetWasm   = "wasm"
	tinygoFlagSize     = "-size=full"
	tinygoFlagOut      = "-o"
	tinygoEntryMain    = "main.go"
	envGOOS            = "GOOS"
	envGOARCH          = "GOARCH"
	goosJS             = "js"
	goarchWASM         = "wasm"
)

func sizeBreakdownArgs(tmpPath string) []string {
	return []string{
		tinygoBuildSubcmd,
		tinygoFlagTarget,
		tinygoTargetWasm,
		tinygoFlagSize,
		tinygoFlagOut,
		tmpPath,
		tinygoEntryMain,
	}
}

// SizeBreakdown compila entryDir con simbolos y devuelve la tabla por paquete
// que emite TinyGo. El artefacto se escribe en un directorio temporal y se
// borra: solo interesa el reporte. NUNCA sustituye a la compilacion que se
// despliega, que sigue saliendo de sitec sin simbolos.
func SizeBreakdown(entryDir string) (string, error) {
	if err := EnsureTinyGo(os.Stdout); err != nil {
		return "", fmt.Errorf("size breakdown ensure tinygo: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "goflare-size-*")
	if err != nil {
		return "", fmt.Errorf("size breakdown mktemp: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	tmpPath := filepath.Join(tmpDir, "diagnose.wasm")
	args := sizeBreakdownArgs(tmpPath)

	cmd := exec.Command(tinygoCmdName, args...)
	cmd.Dir = entryDir
	cmd.Env = append(os.Environ(), envGOOS+"="+goosJS, envGOARCH+"="+goarchWASM)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("tinygo size breakdown failed (%w): %s", err, string(out))
	}

	return string(out), nil
}

// IsStdlib reporta si un import path pertenece a la biblioteca estandar de Go.
func IsStdlib(importPath string) bool {
	if importPath == "" {
		return false
	}
	firstSegment := strings.SplitN(importPath, "/", 2)[0]
	return !strings.Contains(firstSegment, ".")
}

// FilterActionable descarta las cadenas donde el paquete prohibido se alcanza a
// traves de otro paquete de stdlib. Esas no las causa nuestro codigo y no las
// puede arreglar nuestro codigo; reportarlas solo entrena al usuario a ignorar
// la guarda.
func FilterActionable(chains []gobuild.ImportChain) []gobuild.ImportChain {
	var actionable []gobuild.ImportChain
	for _, chain := range chains {
		if len(chain.Path) < 2 {
			continue
		}
		// Finding the first stdlib package in chain.Path
		var firstStdlibIdx = -1
		for i, pkg := range chain.Path {
			if IsStdlib(pkg) {
				firstStdlibIdx = i
				break
			}
		}

		// A chain is actionable iff the FIRST stdlib package reached IS the forbidden package
		if firstStdlibIdx != -1 && chain.Path[firstStdlibIdx] == chain.Forbidden {
			actionable = append(actionable, chain)
		}
	}
	return actionable
}

// ForbiddenImports devuelve solo las cadenas ACCIONABLES hacia stdlib
// prohibida en el grafo del edge.
func ForbiddenImports(moduleRoot, entryPkg string) ([]gobuild.ImportChain, error) {
	chains, err := gobuild.CheckForbiddenImports(moduleRoot, entryPkg, goosJS, goarchWASM, gobuild.ForbiddenWASMImports)
	if err != nil {
		return nil, err
	}
	return FilterActionable(chains), nil
}
