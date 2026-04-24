package goflare

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/tinywasm/assetmin"
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

	// Build output (not in .env — always .build/)
	OutputDir string // default: ".build/"

	// Compiler
	CompilerMode string // "S" | "M" | "L"  default: "S"
}

type Goflare struct {
	edgeCompiler    *client.WasmClient // Worker compiler (Entry)
	browserCompiler *client.WasmClient // Frontend compiler (web/client.go) — nil if it doesn't apply
	assetMin        *assetmin.AssetMin // generates script.js + style.css — nil if no PublicDir
	Config          *Config            // exported so CLI can read it after LoadConfigFromEnv
	log             func(message ...any)
	BaseURL string
}

// New creates a new Goflare instance with the provided configuration
func New(cfg *Config) *Goflare {
	if cfg == nil {
		cfg = &Config{}
	}
	cfg.applyDefaults()

	edgeCompiler := client.New(&client.Config{
		SourceDir: func() string {
			if cfg.Entry != "" {
				return cfg.Entry
			}
			return cfg.PublicDir
		},
		OutputDir: func() string { return cfg.OutputDir },
	})

	edgeCompiler.SetBuildOnDisk(true, false)
	edgeCompiler.SetMode(cfg.CompilerMode)

	// Edge functions use main.go (not client.go, which is the frontend default).
	// OutputName "edge" makes TinyGo produce edge.wasm directly — no rename needed.
	edgeCompiler.SetMainInputFile("main.go")
	edgeCompiler.SetOutputName("edge")

	g := &Goflare{
		edgeCompiler: edgeCompiler,
		Config:       cfg,
		BaseURL:      cfAPIBase,
	}

	// If PublicDir is present, create a client to compile web/client.go.
	// SourceDir is derived from the parent of PublicDir (e.g., "web/public" -> "web").
	// Do not call Change() here — it triggers immediate compilation.
	// Use SetMode() which only updates the internal state.
	if cfg.PublicDir != "" {
		frontSourceDir := filepath.Dir(cfg.PublicDir)
		browserCompiler := client.New(&client.Config{
			SourceDir: func() string { return frontSourceDir },
			OutputDir: func() string { return cfg.PublicDir },
		})
		browserCompiler.SetBuildOnDisk(true, false)
		browserCompiler.SetMode(cfg.CompilerMode)
		g.browserCompiler = browserCompiler

		g.assetMin = assetmin.NewAssetMin(&assetmin.Config{
			OutputDir: cfg.PublicDir,
			GetSSRClientInitJS: func() (string, error) {
				return browserCompiler.GetSSRClientInitJS()
			},
		})
	}

	return g
}

func (g *Goflare) SetLog(f func(message ...any)) {
	g.log = f
	if g.edgeCompiler != nil {
		g.edgeCompiler.SetLog(f)
	}
	if g.browserCompiler != nil {
		g.browserCompiler.SetLog(f)
	}
	if g.assetMin != nil {
		g.assetMin.SetLog(f)
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
	if g.edgeCompiler != nil {
		g.edgeCompiler.Change(newValue)
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
