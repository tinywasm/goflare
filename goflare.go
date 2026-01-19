package goflare

import (
	"github.com/tinywasm/client"
)

type Goflare struct {
	tw               *client.WasmClient
	config           *Config
	outputJsFileName string // e.g., "_worker.js"
	log              func(message ...any)
}

type Config struct {
	AppRootDir                string        // default: "."
	RelativeInputDirectory    func() string // input relative directory for source code server app.go to deploy app.wasm (relative) default: "web"
	RelativeOutputDirectory   func() string // output relative directory for worker.js and app.wasm file (relative) default: "deploy/cloudflare"
	MainInputFile             string        // eg: "main.go"
	CompilingArguments        func() []string
	OutputWasmFileName        string // WASM file name (default: "worker.wasm")
	BuildPageFunctionShortcut string // build assets wasm,js, json files to pages functions (default: "f")
	BuildWorkerShortcut       string // build assets wasm,js, json files to workers (default: "w")
}

// DefaultConfig returns a Config with all default values set
// AppRootDir=".", RelativeInputDirectory="web", RelativeOutputDirectory="deploy/cloudflare", MainInputFile="main.worker.go", OutputWasmFileName="worker.wasm"
func DefaultConfig() *Config {
	return &Config{
		AppRootDir:              ".",
		RelativeInputDirectory:  func() string { return "web" },
		RelativeOutputDirectory: func() string { return "deploy/cloudflare" },
		MainInputFile:           "main.go",
		CompilingArguments:      nil,
		OutputWasmFileName:      "worker.wasm",

		BuildPageFunctionShortcut: "f",
		BuildWorkerShortcut:       "w",
	}
}

// New creates a new Goflare instance with the provided configuration
// Timeout is set to 40 seconds maximum as TinyGo compilation can be slow
// Default values: AppRootDir=".", RelativeOutputDirectory="deploy/cloudflare", MainInputFile="main.worker.go", OutputWasmFileName="app.wasm"
func New(c *Config) *Goflare {

	dc := DefaultConfig()

	if c == nil {
		c = dc
	} else {
		// Set defaults for empty fields
		if c.AppRootDir == "" {
			c.AppRootDir = dc.AppRootDir
		}

		if c.RelativeInputDirectory == nil {
			c.RelativeInputDirectory = dc.RelativeInputDirectory
		}
		if c.RelativeOutputDirectory == nil {
			c.RelativeOutputDirectory = dc.RelativeOutputDirectory
		}
		if c.MainInputFile == "" {
			c.MainInputFile = dc.MainInputFile
		}
		if c.OutputWasmFileName == "" {
			c.OutputWasmFileName = dc.OutputWasmFileName
		}

		if c.BuildPageFunctionShortcut == "" {
			c.BuildPageFunctionShortcut = dc.BuildPageFunctionShortcut
		}
		if c.BuildWorkerShortcut == "" {
			c.BuildWorkerShortcut = dc.BuildWorkerShortcut
		}
	}

	// Extract output name from OutputWasmFileName (remove .wasm extension)
	outputName := c.OutputWasmFileName
	if len(outputName) > 5 && outputName[len(outputName)-5:] == ".wasm" {
		outputName = outputName[:len(outputName)-5]
	}

	tw := client.New(&client.Config{
		SourceDir:          c.RelativeInputDirectory,
		OutputDir:          c.RelativeOutputDirectory,
		CompilingArguments: c.CompilingArguments,
	})
	tw.SetAppRootDir(c.AppRootDir)
	tw.SetMainInputFile(c.MainInputFile)
	tw.SetOutputName(outputName)
	// tw.SetDisableWasmExecJsOutput(true) // Defaults to disabled now
	tw.SetWasmExecJsOutputDir(c.RelativeOutputDirectory()) // Call it as it expects string
	tw.SetBuildOnDisk(true, false)

	g := &Goflare{
		tw:               tw,
		config:           c,
		outputJsFileName: "_worker.js",
	}

	return g
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
	// Execute mode change
	g.tw.Change(newValue)
}
