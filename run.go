//go:build !wasm

package goflare

import (
	"fmt"
	"io"
	"net/http"
)

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

	if err := cfg.Validate(); err != nil {
		return err
	}

	g := New(cfg)
	g.SetLog(func(msgs ...any) {
		fmt.Fprintln(out, msgs...)
	})

	if err := g.Build(); err != nil {
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

	if err := cfg.Validate(); err != nil {
		return err
	}

	g := New(cfg)
	g.SetLog(func(msgs ...any) {
		fmt.Fprintln(out, msgs...)
	})

	if err := g.Auth(); err != nil {
		return err
	}

	var results []DeployResult

	if cfg.Entry != "" {
		err := g.DeployWorker()

		subdomain := "<your-subdomain>"
		if err == nil {
			if token, tokenErr := g.token(); tokenErr == nil {
				client := &cfClient{
					token:      token,
					baseURL:    g.BaseURL,
					httpClient: http.DefaultClient,
				}
				subdomain = g.getWorkerSubdomain(client)
			}
		}

		results = append(results, DeployResult{
			Target: "Worker",
			URL:    fmt.Sprintf("https://%s.%s.workers.dev", cfg.WorkerName, subdomain),
			Err:    err,
		})
	}

	if cfg.PublicDir != "" {
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
