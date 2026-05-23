//go:build !wasm

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

// generateWorkerFile bundles and minifies the three JS assets into a single edge.js
// for the Workers mode (default OutputDir, exports default { fetch, onRequest, ... }).
func (g *Goflare) generateWorkerFile() error {
	dest := filepath.Join(g.Config.OutputDir, "edge.js")
	return g.bundleJS(dest, "./edge.wasm", false)
}

// generatePagesFunctionFile writes the bundle for Pages Functions mode:
// destination is functions/[[path]].mjs and the bundle re-exports only `onRequest`
// (Cloudflare Pages does not consume the `default { fetch }` shape).
func (g *Goflare) generatePagesFunctionFile() error {
	functionsDir := g.functionsDir()
	if err := os.MkdirAll(functionsDir, 0755); err != nil {
		return fmt.Errorf("failed to create functions dir: %w", err)
	}
	dest := filepath.Join(functionsDir, "[[path]].mjs")
	return g.bundleJS(dest, "./edge.wasm", true)
}

// bundleJS produces the JS glue. If pagesOnly is true, the embedded worker.mjs
// `export default { fetch, scheduled, queue, onRequest }` block is replaced by
// `export const onRequest = ...`.
//
// Bundle order:
//  1. Static imports (top-level — required by Cloudflare module format)
//  2. wasm_exec.js  — TinyGo runtime IIFE (no imports)
//  3. runtime.mjs   — loadModule + createRuntimeContext (imports stripped, already at top)
//  4. worker.mjs    — fetch/scheduled/queue/onRequest + export (default OR onRequest only)
func (g *Goflare) bundleJS(dest, wasmImport string, pagesOnly bool) error {
	wasmExecBody := stripIIFEWrapper(string(embeddedWasmExec))
	runtimeBody := stripExports(stripImports(string(embeddedRuntime)))
	workerBody := stripImports(string(embeddedWorker))
	if pagesOnly {
		workerBody = pagesOnlyExport(workerBody)
	}

	bundle := strings.Join([]string{
		`import mod from "` + wasmImport + `";`,
		`import { connect } from "cloudflare:sockets";`,
		wasmExecBody,
		runtimeBody,
		workerBody,
	}, "\n\n")

	m := minify.New()
	m.AddFunc("text/javascript", minjs.Minify)
	minified, err := m.String("text/javascript", bundle)
	if err != nil {
		return fmt.Errorf("failed to minify %s: %w", filepath.Base(dest), err)
	}

	return os.WriteFile(dest, []byte(minified), 0644)
}

// pagesOnlyExport replaces the embedded `export default { fetch, scheduled, queue, onRequest }`
// block with `export { onRequest };` (named re-export) so the file is a valid Pages Functions
// module. The `onRequest` function itself stays defined upstream in the bundle.
//
// Note: `export const onRequest = onRequest;` would self-reference the identifier on the same
// line and the minifier rejects it. Named re-export of the existing top-level function is the
// idiomatic ES form.
func pagesOnlyExport(src string) string {
	const marker = "export default {"
	idx := strings.Index(src, marker)
	if idx == -1 {
		// Already a pages-only bundle or unexpected shape — leave untouched.
		return src
	}
	end := strings.Index(src[idx:], "};")
	if end == -1 {
		return src
	}
	return src[:idx] + "export { onRequest };" + src[idx+end+2:]
}

// functionsDir returns the configured functions output dir, defaulting to "functions".
func (g *Goflare) functionsDir() string {
	if g.Config != nil && g.Config.FunctionsDir != "" {
		return g.Config.FunctionsDir
	}
	return "functions"
}

// stripImports removes ES module import lines from a JS source string.
func stripImports(src string) string {
	var lines []string
	for _, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "import ") && !strings.HasPrefix(trimmed, "import{") {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

// stripExports removes "export " from the beginning of lines,
// but keeps "export default".
func stripExports(src string) string {
	var lines []string
	for _, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "export ") && !strings.HasPrefix(trimmed, "export default") {
			line = strings.Replace(line, "export ", "", 1)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// stripIIFEWrapper removes the outer (() => { ... })(); from wasm_exec.js,
// leaving the inner body for inline embedding.
func stripIIFEWrapper(src string) string {
	start := strings.Index(src, "(() => {")
	if start == -1 {
		return src
	}
	end := strings.LastIndex(src, "})();")
	if end == -1 || end <= start {
		return src
	}
	// Extract content between (() => { and })();
	return src[start+8 : end]
}
