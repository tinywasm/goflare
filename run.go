package goflare

import (
	"fmt"
	"io"
)

// RunInit runs the init command.
func RunInit(envPath string, in io.Reader, out io.Writer) error {
	cfg, err := Init(in, out)
	if err != nil {
		return err
	}

	if err := WriteEnvFile(cfg, envPath); err != nil {
		return err
	}

	if err := UpdateGitignore("."); err != nil {
		return err
	}

	fmt.Fprintln(out, "Init complete. Edit .env if needed, then run: goflare build")
	return nil
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
func RunDeploy(envPath string, in io.Reader, out io.Writer) error {
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

	store := NewKeyringStore()
	if err := g.Auth(store, in); err != nil {
		return err
	}

	var results []DeployResult

	if cfg.Entry != "" {
		err := g.DeployWorker(store)
		results = append(results, DeployResult{
			Target: "Worker",
			URL:    fmt.Sprintf("https://%s.<your-subdomain>.workers.dev", cfg.WorkerName), // We don't easily know the subdomain without another API call
			Err:    err,
		})
	}

	if cfg.PublicDir != "" {
		err := g.DeployPages(store)
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
  init      Initialize a new project (creates .env)
  build     Build the project (compiles WASM and/or copies assets)
  deploy    Deploy the project to Cloudflare

Flags:
  -env string
	path to .env file (default ".env")
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
