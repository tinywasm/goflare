//go:build !wasm

package goflare

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// hasFunctionsArtifacts reports whether dir contains compiled Pages Function
// artifacts (.wasm or .mjs files), indicating this is a Pages Functions project
// rather than a standalone Worker deployment.
func hasFunctionsArtifacts(dir string) bool {
	if dir == "" {
		return false
	}
	found := false
	filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || found {
			return nil
		}
		ext := filepath.Ext(d.Name())
		if ext == ".wasm" || ext == ".mjs" {
			found = true
		}
		return nil
	})
	return found
}

// RunAuth runs the auth command.
func RunAuth(envPath string, out io.Writer, check bool) error {
	cfg, err := LoadConfigFromEnv(envPath)
	if err != nil {
		return err
	}
	g := New(cfg)

	if check {
		if err := g.Auth(); err != nil {
			fmt.Fprintln(out, "Token invalid:", err)
			return err
		}
		fmt.Fprintln(out, "Token OK.")
		return nil
	}

	return g.Auth()
}

// RunBuild runs the build command.
func RunBuild(envPath string, out io.Writer) error {
	cfg, err := LoadConfigFromEnv(envPath)
	if err != nil {
		return err
	}

	if err := cfg.ValidateBuild(); err != nil {
		return err
	}

	// Ensure TinyGo is installed and in PATH before creating compilers.
	// This prevents "exec: tinygo: not found in $PATH" errors when TinyGo
	// is installed at a non-PATH location (e.g. /usr/local/tinygo/bin in CI).
	if err := EnsureTinyGo(out); err != nil {
		return err
	}

	g := New(cfg)
	g.SetLog(func(msgs ...any) {
		fmt.Fprintln(out, msgs...)
	})

	if err := g.Build(); err != nil {
		fmt.Fprintln(out, "Error:", err)
		return err
	}

	fmt.Fprintln(out, "Build complete. Artifacts are in", cfg.OutputDir)
	return nil
}

// RunDeploy runs the deploy command.
func RunDeploy(envPath string, out io.Writer) error {
	cfg, err := LoadConfigFromEnv(envPath)
	if err != nil {
		return err
	}

	if err := cfg.ValidateDeploy(); err != nil {
		return err
	}

	g := New(cfg)
	g.SetLog(func(msgs ...any) {
		fmt.Fprintln(out, msgs...)
	})

	if err := g.Auth(); err != nil {
		return err
	}

	token, err := g.token()
	if err != nil {
		return err
	}
	client := &CfClient{
		Token:      token,
		BaseURL:    g.BaseURL,
		HttpClient: http.DefaultClient,
	}

	var results []DeployResult

	// Deploy as standalone Worker only when Entry is set AND no Pages Functions
	// artifacts exist. When FunctionsDir has compiled files (e.g. edge.wasm +
	// [[path]].mjs), the edge function is deployed as a Pages Function via
	// DeployPages — calling DeployWorker would look for a non-existent edge.js.
	if cfg.Entry != "" && !hasFunctionsArtifacts(cfg.FunctionsDir) {
		err := g.DeployWorker()

		subdomain := "<your-subdomain>"
		if err == nil {
			subdomain = g.getWorkerSubdomain(client)
		}

		results = append(results, DeployResult{
			Target: "Worker",
			URL:    fmt.Sprintf("https://%s.%s.workers.dev", cfg.WorkerName, subdomain),
			Err:    err,
		})
	}

	if cfg.PublicDir != "" {
		if err := g.ValidateDeployScopes(client); err != nil {
			return err
		}

		err := g.DeployPages()
		url := fmt.Sprintf("https://%s.pages.dev", cfg.ProjectName)
		if cfg.Domain != "" {
			url = "https://" + cfg.Domain
		}
		results = append(results, DeployResult{
			Target: "Pages",
			URL:    url,
			Err:    err,
		})
	}

	g.WriteSummary(out, results)

	for _, res := range results {
		if res.Err != nil {
			return fmt.Errorf("deploy failed")
		}
	}

	return nil
}

// Usage returns the usage string.
func Usage() string {
	return `Usage: goflare <command> [flags]

Commands:
  auth      Validate CLOUDFLARE_API_TOKEN from environment
  build     Build the project (compiles WASM and/or copies assets)
  deploy    Deploy the project to Cloudflare (requires CLOUDFLARE_API_TOKEN env var)

Flags:
  -env string
	path to .env file (default ".env")

Auth Flags:
  -check    Verify token from environment
`
}

// DeployResult represents the result of a deployment to a target.
type DeployResult struct {
	Target string
	URL    string
	Err    error
}

// WriteSummary formats and writes the deploy summary to out.
func (g *Goflare) WriteSummary(out io.Writer, results []DeployResult) {
	fmt.Fprintln(out, "\n--- Deployment Summary ---")
	for _, res := range results {
		if res.Err != nil {
			fmt.Fprintf(out, "[-] %s: Failed - %v\n", res.Target, res.Err)
		} else {
			fmt.Fprintf(out, "[+] %s: Success - %s\n", res.Target, res.URL)
		}
	}
}
