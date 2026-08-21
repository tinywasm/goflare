//go:build !wasm

package goflare

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/tinywasm/js"
	"github.com/tinywasm/sitec"
)

type Config struct {
	// Project identity
	ProjectName string // PROJECT_NAME
	AccountID   string // CLOUDFLARE_ACCOUNT_ID
	WorkerName  string // WORKER_NAME  (default: ProjectName + "-worker")

	// Routing
	Domain string // DOMAIN (optional — custom domain for Pages)

	// Build inputs (conventions, not configurable via .env)
	Entry     string // ENTRY      (path to main Go file, empty = Pages only)
	PublicDir string // PUBLIC_DIR (path to static assets, empty = Worker only)

	// Build output (not in .env — always .build/)
	OutputDir string // default: ".build/"

	// Pages Functions output (sibling to web/public/, committed to git)
	FunctionsDir string // default: "functions"

	// Compiler
	CompilerMode string // "S" | "M" | "L"  default: "S"

	D1DatabaseID   string // D1_DATABASE_ID
	D1DatabaseName string // D1_DATABASE_NAME — optional, default: ProjectName
	R2BucketID     string // R2_BUCKET_ID
	R2BucketName   string // R2_BUCKET_NAME
}

type Goflare struct {
	edgeBuilder  sitec.WasmBuilder // Worker compiler (Entry)
	assetMin     *sitec.AssetMin   // generates script.js + style.css — nil if no PublicDir
	Config       *Config           // exported so CLI can read it after LoadConfigFromEnv
	log          func(message ...any)
	BaseURL      string
	stagingDir   string        // temporary directory for build artifacts
	RetryBackoff time.Duration // base duration for retries (defaults to 1s)
}

func syncJSRuntime(mode string) {
	if mode == "L" {
		js.SetRuntime(js.RuntimeGo)
	} else {
		js.SetRuntime(js.RuntimeTinyGo)
	}
}

// New creates a new Goflare instance with the provided configuration
func New(cfg *Config) *Goflare {
	if cfg == nil {
		cfg = &Config{}
	}
	cfg.applyDefaults()

	staging, err := os.MkdirTemp("", "goflare-*")
	if err != nil {
		// Fallback to configured OutputDir if MkdirTemp fails
		staging = cfg.OutputDir
	}

	// goflare ALWAYS compiles the edge with TinyGo in production mode.
	// Cloudflare Workers/Pages enforce a 1 MiB wasm limit on Free plans,
	// which standard Go (2-10 MB) cannot meet. The typed wrapper insulates
	// goflare from any future change to the internal mode letter and
	// persists the choice to disk storage so a stale previous mode does
	// not silently override it.
	edgeBuilder := sitec.NewWasmBuilder(false, sitec.WasmBuildOptions{
		Entry:      "main.go",
		OutputName: "edge",
	})

	g := &Goflare{
		edgeBuilder:  edgeBuilder,
		Config:       cfg,
		BaseURL:      cfAPIBase,
		stagingDir:   staging,
		RetryBackoff: time.Second,
	}

	if cfg.PublicDir != "" {
		syncJSRuntime(cfg.CompilerMode)

		g.assetMin = sitec.NewAssetMin(&sitec.Config{
			OutputDir: cfg.PublicDir,
		})
		g.assetMin.SetFS(sitec.NewOsFS())
		g.assetMin.UpdateSSRModule("tinywasm/js/bootstrap", "", []*js.Script{js.PageBootstrap()}, "", nil)
	}

	return g
}

// StagingDir returns the temporary directory used for intermediate build artifacts.
// Exposed for testing — verifies that staging is outside the project tree.
func (g *Goflare) StagingDir() string { return g.stagingDir }

func (g *Goflare) SetLog(f func(message ...any)) {
	g.log = f
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
}

func (g *Goflare) Deploy() error {
	var buildErrors []error

	if g.Config.Entry != "" {
		if err := g.DeployWorker(); err != nil {
			buildErrors = append(buildErrors, fmt.Errorf("worker deploy failed: %w", err))
		}
	}

	if g.Config.PublicDir != "" {
		if err := g.DeployPages(); err != nil {
			buildErrors = append(buildErrors, fmt.Errorf("pages deploy failed: %w", err))
		}
	}

	if len(buildErrors) > 0 {
		return errors.Join(buildErrors...)
	}

	return nil
}
