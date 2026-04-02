# GoFlare
<img src="docs/img/badges.svg">

**GoFlare** is a lightweight and flexible handler designed to help you build and deploy **Go-based WebAssembly (WASM)** and **JavaScript** bundles for **Cloudflare Workers**, **Pages**, or **Functions**.

It’s not a full framework — GoFlare is meant to **fit into your existing toolchain**.
You can use it as a standalone build step or attach it to any build or deploy workflow you already have.

---

## 🚀 Features

* **Go → WASM/JS** conversion made simple
* **Compatible with Cloudflare’s edge platforms** (Workers, Pages, and Functions)
* **Integrates easily** with CI/CD pipelines or other Go tools
* **Minimal dependencies** and **lightweight footprint**
* Provides a **unified handler interface** to hook into other builders or deployment systems

---

## 🧩 Example Usage

### Basic Usage

```go
import "github.com/tinywasm/goflare"

func main() {
	// Create a new Goflare instance with default configuration
	g := goflare.New(nil)
	
	// Generate Cloudflare Pages files
	err := g.GeneratePagesFiles()
	if err != nil {
		panic(err)
	}
}
```

### Custom Configuration

```go
config := &goflare.Config{
	AppRootDir:              ".",
	RelativeInputDirectory:  "web",
	RelativeOutputDirectory: "deploy/cloudflare",
	MainInputFile:           "main.worker.go",
	OutputWasmFileName:      "worker.wasm",
	Logger:                  func(msg ...any) { fmt.Println(msg...) },
}

g := goflare.New(config)
```

### Using with DevTUI (HandlerExecution Interface)

GoFlare provides handlers that implement the `HandlerExecution` interface for integration with interactive TUI applications:

```go
import (
	"github.com/tinywasm/goflare"
	"github.com/tinywasm/devtui"
)

func main() {
	g := goflare.New(nil)
	
	// Create execution handlers
	buildPagesHandler := g.NewBuildPagesHandler()
	buildWorkersHandler := g.NewBuildWorkersHandler()
	fastModeHandler := g.NewSetCompilerModeHandler("f")  // Fast/Go
	debugModeHandler := g.NewSetCompilerModeHandler("b") // Debug/TinyGo
	prodModeHandler := g.NewSetCompilerModeHandler("m")  // Production/TinyGo
	
	// Register handlers with DevTUI
	tui := devtui.New()
	tui.AddField(buildPagesHandler)
	tui.AddField(buildWorkersHandler)
	tui.AddField(fastModeHandler)
	tui.AddField(debugModeHandler)
	tui.AddField(prodModeHandler)
	
	// Run the TUI
	if err := tui.Run(); err != nil {
		panic(err)
	}
}
```

#### Available Handlers

- **`NewBuildPagesHandler()`** - Builds Cloudflare Pages files (_worker.js + WASM)
- **`NewBuildWorkersHandler()`** - Builds Cloudflare Workers files
- **`NewSetCompilerModeHandler(mode)`** - Changes compiler mode:
  - `"f"` - Fast mode (Go compiler)
  - `"b"` - Debug mode (TinyGo with debug symbols)
  - `"m"` - Production mode (TinyGo optimized)

All handlers implement the `HandlerExecution` interface:
```go
type HandlerExecution interface {
    Name() string                       // Identifier for logging
    Label() string                      // Button label
    Execute(progress func(msgs ...any)) // Execute operation with progress callback
}
```

---

## [Contributing](https://github.com/tinywasm/cdvelop/blob/main/CONTRIBUTING.md)