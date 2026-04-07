package goflare

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Init runs the interactive wizard and returns a populated Config.
func Init(in io.Reader) (*Config, error) {
	scanner := bufio.NewScanner(in)
	cfg := &Config{}

	ask := func(prompt, defaultValue string) string {
		fmt.Print(prompt + " ")
		if defaultValue != "" {
			fmt.Printf("[%s] ", defaultValue)
		}
		if !scanner.Scan() {
			return defaultValue
		}
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			return defaultValue
		}
		if text == "-" {
			return ""
		}
		return text
	}

	cfg.ProjectName = ask("Project name:", "")
	if cfg.ProjectName == "" {
		return nil, fmt.Errorf("Project name is required")
	}

	cfg.AccountID = ask("Cloudflare Account ID (see dash.cloudflare.com → right sidebar):", "")
	if cfg.AccountID == "" {
		return nil, fmt.Errorf("Cloudflare Account ID is required")
	}

	cfg.Domain = ask("Custom domain (leave empty for *.pages.dev):", "")
	cfg.Entry = ask("Entry point (leave empty for Pages-only) [web/server.go]:", "web/server.go")
	cfg.PublicDir = ask("Public dir (leave empty for Worker-only) [web/public]:", "web/public")

	defaultWorkerName := cfg.ProjectName + "-worker"
	cfg.WorkerName = ask(fmt.Sprintf("Worker name [%s]:", defaultWorkerName), defaultWorkerName)

	if cfg.Entry == "" && cfg.PublicDir == "" {
		return nil, fmt.Errorf("at least one of Entry or PublicDir is required")
	}

	return cfg, nil
}

// WriteEnvFile writes a .env file with all non-empty fields.
func WriteEnvFile(cfg *Config, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if cfg.ProjectName != "" {
		fmt.Fprintf(f, "PROJECT_NAME=%s\n", cfg.ProjectName)
	}
	if cfg.AccountID != "" {
		fmt.Fprintf(f, "CLOUDFLARE_ACCOUNT_ID=%s\n", cfg.AccountID)
	}
	if cfg.Domain != "" {
		fmt.Fprintf(f, "DOMAIN=%s\n", cfg.Domain)
	}
	if cfg.WorkerName != "" {
		fmt.Fprintf(f, "WORKER_NAME=%s\n", cfg.WorkerName)
	}
	if cfg.Entry != "" {
		fmt.Fprintf(f, "ENTRY=%s\n", cfg.Entry)
	}
	if cfg.PublicDir != "" {
		fmt.Fprintf(f, "PUBLIC_DIR=%s\n", cfg.PublicDir)
	}
	if cfg.CompilerMode != "" {
		fmt.Fprintf(f, "COMPILER_MODE=%s\n", cfg.CompilerMode)
	}

	return nil
}

// UpdateGitignore reads .gitignore in dir. Appends .env and .goflare/ if not already present.
func UpdateGitignore(dir string) error {
	path := filepath.Join(dir, ".gitignore")

	content, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	lines := strings.Split(string(content), "\n")
	hasEnv := false
	hasGoflare := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == ".env" {
			hasEnv = true
		}
		if line == ".goflare/" {
			hasGoflare = true
		}
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	if !hasEnv {
		if len(content) > 0 && content[len(content)-1] != '\n' {
			fmt.Fprintln(f)
		}
		fmt.Fprintln(f, ".env")
	}
	if !hasGoflare {
		if !hasEnv && len(content) > 0 && content[len(content)-1] != '\n' {
			fmt.Fprintln(f)
		}
		fmt.Fprintln(f, ".goflare/")
	}

	return nil
}
