package goflare

import (
	"fmt"
	"io"
)

type DeployResult struct {
	Target string // "Worker" or "Pages"
	URL    string // live URL on success
	Err    error
}

func RunInit(envPath string, in io.Reader, out io.Writer) error {
	cfg, err := Init(in)
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

	fmt.Fprintf(out, "Build complete. Artifacts in %s\n", cfg.OutputDir)
	return nil
}

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

	store := &KeyringStore{ProjectName: cfg.ProjectName}
	if err := g.Auth(store, in); err != nil {
		return err
	}

	var results []DeployResult
	if cfg.Entry != "" {
		err := g.DeployWorker(store)
		results = append(results, DeployResult{Target: "Worker", Err: err})
	}
	if cfg.PublicDir != "" {
		err := g.DeployPages(store)
		results = append(results, DeployResult{Target: "Pages", Err: err})
	}

	g.WriteSummary(out, results)

	for _, r := range results {
		if r.Err != nil {
			return fmt.Errorf("deploy failed")
		}
	}

	return nil
}

func (g *Goflare) WriteSummary(out io.Writer, results []DeployResult) {
	fmt.Fprintln(out, "\n--- Deploy Summary ---")
	for _, r := range results {
		status := "SUCCESS"
		if r.Err != nil {
			status = "FAILED: " + r.Err.Error()
		}
		fmt.Fprintf(out, "%-10s %s\n", r.Target+":", status)
	}
}

func Usage() string {
	return `Goflare — Deploy Go WASM projects to Cloudflare

Usage:
  goflare <command> [flags]

Commands:
  init      Initialize project (.env and .gitignore)
  build     Build Worker and/or Pages artifacts
  deploy    Deploy artifacts to Cloudflare

Flags:
  -env string    Path to .env file (default ".env")
`
}
