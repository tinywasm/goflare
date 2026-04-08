package goflare

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadConfigFromEnv reads a .env file and populates Config.
// Falls back to OS environment variables if .env path is empty or does not exist.
// Applies defaults after loading.
func LoadConfigFromEnv(path string) (*Config, error) {
	cfg := &Config{}

	if path != "" {
		file, err := os.Open(path)
		if err == nil {
			defer file.Close()
			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}

				parts := strings.SplitN(line, "=", 2)
				if len(parts) != 2 {
					continue
				}

				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])

				// Strip optional surrounding quotes
				if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
					value = value[1 : len(value)-1]
				}

				switch key {
				case "PROJECT_NAME":
					cfg.ProjectName = value
				case "CLOUDFLARE_ACCOUNT_ID":
					cfg.AccountID = value
				case "WORKER_NAME":
					cfg.WorkerName = value
				case "DOMAIN":
					cfg.Domain = value
				case "ENTRY":
					cfg.Entry = value
				case "PUBLIC_DIR":
					cfg.PublicDir = value
				case "COMPILER_MODE":
					cfg.CompilerMode = value
				}
			}
			if err := scanner.Err(); err != nil {
				return nil, fmt.Errorf("error reading .env file: %w", err)
			}
		}
	}

	// Fallback to OS environment variables if still empty
	if cfg.ProjectName == "" {
		cfg.ProjectName = os.Getenv("PROJECT_NAME")
	}
	if cfg.AccountID == "" {
		cfg.AccountID = os.Getenv("CLOUDFLARE_ACCOUNT_ID")
	}
	if cfg.WorkerName == "" {
		cfg.WorkerName = os.Getenv("WORKER_NAME")
	}
	if cfg.Domain == "" {
		cfg.Domain = os.Getenv("DOMAIN")
	}
	if cfg.Entry == "" {
		cfg.Entry = os.Getenv("ENTRY")
	}
	if cfg.PublicDir == "" {
		cfg.PublicDir = os.Getenv("PUBLIC_DIR")
	}
	if cfg.CompilerMode == "" {
		cfg.CompilerMode = os.Getenv("COMPILER_MODE")
	}

	cfg.applyDefaults()
	return cfg, nil
}

func (c *Config) Validate() error {
	if c.ProjectName == "" {
		return fmt.Errorf("ProjectName is required")
	}
	if c.AccountID == "" {
		return fmt.Errorf("AccountID is required")
	}
	if c.Entry == "" && c.PublicDir == "" {
		return fmt.Errorf("Entry and PublicDir cannot both be empty")
	}
	return nil
}

func (c *Config) applyDefaults() {
	if c.WorkerName == "" && c.ProjectName != "" {
		c.WorkerName = c.ProjectName + "-worker"
	}
	if c.OutputDir == "" {
		c.OutputDir = ".goflare/"
	}
	if c.CompilerMode == "" {
		c.CompilerMode = "S"
	}

	// Auto-detect Worker entry if worker/main.go exists and Entry is not configured.
	if c.Entry == "" {
		if _, err := os.Stat(filepath.Join("worker", "main.go")); err == nil {
			c.Entry = "worker"
		}
	}
}
