//go:build !wasm

package goflare

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	EnvKeyProjectName    = "PROJECT_NAME"
	EnvKeyAccountID      = "CLOUDFLARE_ACCOUNT_ID"
	EnvKeyWorkerName     = "WORKER_NAME"
	EnvKeyDomain         = "DOMAIN"
	EnvKeyCompilerMode   = "COMPILER_MODE"
	EnvKeyD1DatabaseID   = "D1_DATABASE_ID"
	EnvKeyD1DatabaseName = "D1_DATABASE_NAME"
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
				case EnvKeyProjectName:
					cfg.ProjectName = value
				case EnvKeyAccountID:
					cfg.AccountID = value
				case EnvKeyWorkerName:
					cfg.WorkerName = value
				case EnvKeyDomain:
					cfg.Domain = value
				case EnvKeyCompilerMode:
					cfg.CompilerMode = value
				case EnvKeyD1DatabaseID:
					cfg.D1DatabaseID = value
				case EnvKeyD1DatabaseName:
					cfg.D1DatabaseName = value
				}
			}
			if err := scanner.Err(); err != nil {
				return nil, fmt.Errorf("error reading .env file: %w", err)
			}
		}
	}

	// Fallback to OS environment variables if still empty
	if cfg.ProjectName == "" {
		cfg.ProjectName = os.Getenv(EnvKeyProjectName)
	}
	if cfg.AccountID == "" {
		cfg.AccountID = os.Getenv(EnvKeyAccountID)
	}
	if cfg.WorkerName == "" {
		cfg.WorkerName = os.Getenv(EnvKeyWorkerName)
	}
	if cfg.Domain == "" {
		cfg.Domain = os.Getenv(EnvKeyDomain)
	}
	if cfg.CompilerMode == "" {
		cfg.CompilerMode = os.Getenv(EnvKeyCompilerMode)
	}
	if cfg.D1DatabaseID == "" {
		cfg.D1DatabaseID = os.Getenv(EnvKeyD1DatabaseID)
	}
	if cfg.D1DatabaseName == "" {
		cfg.D1DatabaseName = os.Getenv(EnvKeyD1DatabaseName)
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
		c.OutputDir = ".build/"
	}
	if c.FunctionsDir == "" {
		c.FunctionsDir = "functions"
	}
	if c.CompilerMode == "" {
		c.CompilerMode = "S"
	}

	// Auto-detect edge function entry (convention).
	if c.Entry == "" {
		if _, err := os.Stat(filepath.Join("edge", "main.go")); err == nil {
			c.Entry = "edge"
		}
	}

	// Auto-detect public directory (convention).
	if c.PublicDir == "" {
		if _, err := os.Stat("web/public"); err == nil {
			c.PublicDir = "web/public"
		}
	}
}
