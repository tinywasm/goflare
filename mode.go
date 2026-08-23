//go:build !wasm

package goflare

import (
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
)

const (
	ImportEdge    = "github.com/tinywasm/goflare/edge"
	ImportWorkers = "github.com/tinywasm/goflare/workers"

	ErrNoKnownImport = "cannot infer mode: edge/main.go imports neither " + ImportEdge + " (pages-functions) nor " + ImportWorkers + " (workers)"
)

// validateEntry confirms that edge/main.go imports at least one of the known runtime packages.
func validateEntry(entry string) error {
	mainGo := filepath.Join(entry, "main.go")
	if _, err := os.Stat(mainGo); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("entry main.go does not exist: %s", mainGo)
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, mainGo, nil, parser.ImportsOnly)
	if err != nil {
		return err
	}

	for _, spec := range f.Imports {
		path, _ := strconv.Unquote(spec.Path.Value)
		if path == ImportEdge || path == ImportWorkers {
			return nil
		}
	}

	return errors.New(ErrNoKnownImport)
}
