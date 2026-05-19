package goflare

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Mode represents the inferred project build mode.
type Mode string

const (
	ModeUnknown        Mode = ""
	ModeWorkers        Mode = "workers"
	ModePagesFunctions Mode = "pages-functions"
	ModePagesStatic    Mode = "pages"
)

// inferMode determines the project mode by inspecting edge/main.go imports.
//
// Returns:
//   - ModePagesFunctions if edge/main.go imports github.com/tinywasm/goflare/pages
//   - ModeWorkers if edge/main.go imports github.com/tinywasm/goflare/workers (or exists but has no goflare import)
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

	f, err := os.Open(mainGo)
	if err != nil {
		return ModeUnknown, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.Contains(line, `"github.com/tinywasm/goflare/pages"`) {
			return ModePagesFunctions, nil
		}
		if strings.Contains(line, `"github.com/tinywasm/goflare/workers"`) {
			return ModeWorkers, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return ModeUnknown, err
	}

	// edge/main.go exists but no goflare import — treat as workers (legacy default)
	return ModeWorkers, nil
}
