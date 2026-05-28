//go:build !wasm

package goflare

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/tinywasm/tinygo"
)

func (g *Goflare) generateWasmFile() error {
	return g.edgeCompiler.RecompileMainWasm()
}

// EnsureTinyGo installs TinyGo if absent and guarantees its bin dir is in PATH
// before any compilation attempt. Safe to call multiple times (idempotent).
func EnsureTinyGo(out io.Writer) error {
	installedPath, err := tinygo.EnsureInstalled()
	if err != nil {
		return fmt.Errorf("tinygo setup: %w", err)
	}
	if _, lookErr := exec.LookPath("tinygo"); lookErr != nil {
		binDir := filepath.Dir(installedPath)
		current := os.Getenv("PATH")
		if current != "" {
			os.Setenv("PATH", current+string(os.PathListSeparator)+binDir)
		} else {
			os.Setenv("PATH", binDir)
		}
		fmt.Fprintln(out, "TinyGo ready:", installedPath)
	}
	return nil
}
