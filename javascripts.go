package goflare

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tdewolff/minify/v2"
	minjs "github.com/tdewolff/minify/v2/js"
)

//go:embed assets/wasm_exec_worker.js
var embeddedWasmExec []byte

//go:embed assets/runtime.mjs
var embeddedRuntime []byte

//go:embed assets/worker.mjs
var embeddedWorker []byte

// generateWorkerFile bundles and minifies the three JS assets into a single edge.js.
//
// Bundle order:
//  1. Static imports (top-level — required by Cloudflare module format)
//  2. wasm_exec.js  — TinyGo runtime IIFE (no imports)
//  3. runtime.mjs   — loadModule + createRuntimeContext (imports stripped, already at top)
//  4. worker.mjs    — fetch/scheduled/queue/onRequest + export default (imports stripped)
func (g *Goflare) generateWorkerFile() error {
	wasmExecBody := stripIIFEWrapper(string(embeddedWasmExec))
	runtimeBody := stripImports(string(embeddedRuntime))
	workerBody := stripImports(string(embeddedWorker))

	bundle := strings.Join([]string{
		`import mod from "./edge.wasm";`,
		`import { connect } from "cloudflare:sockets";`,
		wasmExecBody,
		runtimeBody,
		workerBody,
	}, "\n\n")

	m := minify.New()
	m.AddFunc("text/javascript", minjs.Minify)
	minified, err := m.String("text/javascript", bundle)
	if err != nil {
		return fmt.Errorf("failed to minify edge.js: %w", err)
	}

	dest := filepath.Join(g.Config.OutputDir, "edge.js")
	return os.WriteFile(dest, []byte(minified), 0644)
}

// stripImports removes ES module import lines from a JS source string.
func stripImports(src string) string {
	var lines []string
	for _, line := range strings.Split(src, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "import ") {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

// stripIIFEWrapper removes the outer (() => { ... })(); from wasm_exec.js,
// leaving the inner body for inline embedding.
func stripIIFEWrapper(src string) string {
	// wasm_exec.js wraps everything in (() => { ... });
	// Find first { and last }); and extract the inner body.
	start := strings.Index(src, "{")
	end := strings.LastIndex(src, "});")
	if start == -1 || end == -1 || end <= start {
		return src // fallback: return as-is
	}
	return src[start+1 : end]
}
