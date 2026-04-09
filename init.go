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
func Init(in io.Reader, out io.Writer) (*Config, error) {
	scanner := bufio.NewScanner(in)
	cfg := &Config{}

	ask := func(prompt string, required bool) (string, error) {
		fmt.Fprintf(out, "%s ", prompt)
		if !scanner.Scan() {
			return "", scanner.Err()
		}
		val := strings.TrimSpace(scanner.Text())
		if required && val == "" {
			return "", fmt.Errorf("field is required")
		}
		return val, nil
	}

	var err error
	cfg.ProjectName, err = ask("Project name:", true)
	if err != nil {
		return nil, err
	}

	cfg.AccountID, err = ask("Cloudflare Account ID (see dash.cloudflare.com -> right sidebar):", true)
	if err != nil {
		return nil, err
	}

	cfg.Domain, err = ask("Custom domain (leave empty for *.pages.dev):", false)
	if err != nil {
		return nil, err
	}

	// Only ask for Entry if edge/main.go does not exist
	if _, err := os.Stat(filepath.Join("edge", "main.go")); err == nil {
		fmt.Fprintln(out, "  → edge/main.go detected, Entry set to \"edge\" automatically")
		cfg.Entry = "edge"
	} else {
		cfg.Entry, err = ask("Entry point (edge function dir, leave empty for Pages-only) [edge]:", false)
		if err != nil {
			return nil, err
		}
	}

	cfg.PublicDir, err = ask("Public dir (leave empty for Worker-only) [web/public]:", false)
	if err != nil {
		return nil, err
	}

	defaultWorkerName := cfg.ProjectName + "-worker"
	cfg.WorkerName, err = ask(fmt.Sprintf("Worker name [%s]:", defaultWorkerName), false)
	if err != nil {
		return nil, err
	}
	if cfg.WorkerName == "" {
		cfg.WorkerName = defaultWorkerName
	}

	if cfg.Entry == "" && cfg.PublicDir == "" {
		return nil, fmt.Errorf("at least one of Entry or PublicDir is required")
	}

	return cfg, nil
}

// WriteEnvFile writes a .env file with all non-empty fields.
func WriteEnvFile(cfg *Config, path string) error {
	var lines []string

	if cfg.ProjectName != "" {
		lines = append(lines, fmt.Sprintf("PROJECT_NAME=%s", cfg.ProjectName))
	}
	if cfg.AccountID != "" {
		lines = append(lines, fmt.Sprintf("CLOUDFLARE_ACCOUNT_ID=%s", cfg.AccountID))
	}
	if cfg.Domain != "" {
		lines = append(lines, fmt.Sprintf("DOMAIN=%s", cfg.Domain))
	}
	if cfg.WorkerName != "" {
		lines = append(lines, fmt.Sprintf("WORKER_NAME=%s", cfg.WorkerName))
	}
	if cfg.Entry != "" {
		lines = append(lines, fmt.Sprintf("ENTRY=%s", cfg.Entry))
	}
	if cfg.PublicDir != "" {
		lines = append(lines, fmt.Sprintf("PUBLIC_DIR=%s", cfg.PublicDir))
	}
	if cfg.CompilerMode != "" {
		lines = append(lines, fmt.Sprintf("COMPILER_MODE=%s", cfg.CompilerMode))
	}

	content := strings.Join(lines, "\n") + "\n"
	return os.WriteFile(path, []byte(content), 0644)
}

// UpdateGitignore reads .gitignore in dir. Appends .env and .goflare/ if not already present.
// Creates .gitignore if it does not exist.
func UpdateGitignore(dir string) error {
	path := filepath.Join(dir, ".gitignore")
	var content string
	if _, err := os.Stat(path); err == nil {
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		content = string(b)
	}

	entries := []string{".env", ".build/"}
	modified := false

	lines := strings.Split(content, "\n")
	for _, entry := range entries {
		found := false
		for _, line := range lines {
			if strings.TrimSpace(line) == entry {
				found = true
				break
			}
		}
		if !found {
			if content != "" && !strings.HasSuffix(content, "\n") {
				content += "\n"
			}
			content += entry + "\n"
			modified = true
		}
	}

	if modified {
		return os.WriteFile(path, []byte(content), 0644)
	}
	return nil
}
