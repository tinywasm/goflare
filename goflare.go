package goflare

import (
	"errors"
	"fmt"

	"github.com/tinywasm/client"
)

type Config struct {
	// Project identity
	ProjectName string // PROJECT_NAME
	AccountID   string // CLOUDFLARE_ACCOUNT_ID
	WorkerName  string // WORKER_NAME  (default: ProjectName + "-worker")

	// Routing
	Domain string // DOMAIN (optional — custom domain for Pages)

	// Build inputs
	Entry     string // ENTRY      (path to main Go file, empty = Pages only)
	PublicDir string // PUBLIC_DIR (path to static assets, empty = Worker only)

	// Build output (not in .env — always .goflare/)
	OutputDir string // default: ".goflare/"

	// Compiler
	CompilerMode string // "S" | "M" | "L"  default: "S"
}

type Goflare struct {
	tw      *client.WasmClient
	Config  *Config // exported so CLI can read it after LoadConfigFromEnv
	log     func(message ...any)
	BaseURL string
}

// New creates a new Goflare instance with the provided configuration
func New(cfg *Config) *Goflare {
	if cfg == nil {
		cfg = &Config{}
	}
	cfg.applyDefaults()

	tw := client.New(&client.Config{
		SourceDir: func() string {
			if cfg.Entry != "" {
				return cfg.Entry
			}
			return cfg.PublicDir
		},
		OutputDir: func() string { return cfg.OutputDir },
	})

	tw.SetBuildOnDisk(true, false)
	tw.Change(cfg.CompilerMode)

	return &Goflare{
		tw:      tw,
		Config:  cfg,
		BaseURL: cfAPIBase,
	}
}

func (g *Goflare) SetLog(f func(message ...any)) {
	g.log = f
	if g.tw != nil {
		g.tw.SetLog(f)
	}
}

func (g *Goflare) Logger(messages ...any) {
	if g.log != nil {
		g.log(messages...)
	}
}

// SetCompilerMode changes the compiler mode
// mode: "L" (Large fast/Go), "M" (Medium TinyGo debug), "S" (Small TinyGo production)
func (g *Goflare) SetCompilerMode(newValue string) {
	g.Config.CompilerMode = newValue
	if g.tw != nil {
		g.tw.Change(newValue)
	}
}

func (g *Goflare) Deploy(store Store) error {
	var buildErrors []error

	if g.Config.Entry != "" {
		if err := g.DeployWorker(store); err != nil {
			buildErrors = append(buildErrors, fmt.Errorf("worker deploy failed: %w", err))
		}
	}

	if g.Config.PublicDir != "" {
		if err := g.DeployPages(store); err != nil {
			buildErrors = append(buildErrors, fmt.Errorf("pages deploy failed: %w", err))
		}
	}

	if len(buildErrors) > 0 {
		return errors.Join(buildErrors...)
	}

	return nil
}
