//go:build !wasm

package goflare

import (
	"errors"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
)

// Mode represents the inferred project build mode.
type Mode string

const (
	ModeUnknown        Mode = ""
	ModeWorkers        Mode = "workers"
	ModePagesFunctions Mode = "pages-functions"
	ModePagesStatic    Mode = "pages"

	ImportEdge    = "github.com/tinywasm/goflare/edge"
	ImportWorkers = "github.com/tinywasm/goflare/workers"

	ErrNoKnownImport = "cannot infer mode: edge/main.go imports neither " + ImportEdge + " (pages-functions) nor " + ImportWorkers + " (workers)"
	ErrAmbiguous     = "cannot infer mode: edge/main.go imports both " + ImportEdge + " and " + ImportWorkers + " — import exactly one"
)

// inferMode determines the project mode by inspecting edge/main.go imports.
//
// Returns:
//   - ModePagesFunctions if edge/main.go imports github.com/tinywasm/goflare/edge
//   - ModeWorkers if edge/main.go imports github.com/tinywasm/goflare/workers
//   - ModePagesStatic if edge/main.go does not exist but PublicDir does
//   - ModeUnknown + error otherwise
//
// The code is the source of truth; .env never carries MODE (D11).
func inferMode(entry, publicDir string) (Mode, error) {
	mainGo := filepath.Join(entry, "main.go")
	if _, err := os.Stat(mainGo); errors.Is(err, os.ErrNotExist) {
		if publicDir != "" {
			if _, perr := os.Stat(publicDir); perr == nil {
				return ModePagesStatic, nil
			}
		}
		return ModeUnknown, errors.New("cannot infer mode: " + mainGo + " does not exist and no PublicDir")
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, mainGo, nil, parser.ImportsOnly)
	if err != nil {
		return ModeUnknown, err
	}

	hasEdge := false
	hasWorkers := false

	for _, spec := range f.Imports {
		path, _ := strconv.Unquote(spec.Path.Value)
		if path == ImportEdge {
			hasEdge = true
		}
		if path == ImportWorkers {
			hasWorkers = true
		}
	}

	if hasEdge && hasWorkers {
		return ModeUnknown, errors.New(ErrAmbiguous)
	}
	if hasEdge {
		return ModePagesFunctions, nil
	}
	if hasWorkers {
		return ModeWorkers, nil
	}

	return ModeUnknown, errors.New(ErrNoKnownImport)
}
