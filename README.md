# GoFlare

GoFlare is a self-contained Go tool (library + CLI) for deploying Go WASM projects to Cloudflare Workers and Pages. No Node.js, no Wrangler, no GitHub Actions. Pure Go, direct Cloudflare API.

## Requirements

- Go 1.25.2 or later
- TinyGo in PATH (for Worker builds)

## Installation

```bash
go install github.com/tinywasm/goflare/cmd/goflare@latest
```

## CLI Usage

### 1. Initialize
Set up your project interactively. This creates a `.env` file and updates `.gitignore`.

```bash
goflare init
```

### 2. Build
Compile your Go code into WASM and/or prepare static assets in `.goflare/`.

```bash
goflare build
```

### 3. Deploy
Authenticate and push your artifacts to Cloudflare.

```bash
goflare deploy
```

## Library Usage

GoFlare can also be used as a Go library in your own automation tools.

```go
package main

import (
	"os"
	"github.com/tinywasm/goflare"
)

func main() {
	cfg := &goflare.Config{
		ProjectName: "myapp",
		AccountID:   "abc123456",
		Entry:       "cmd/wasm/main.go",
		PublicDir:   "web/public",
	}
	
	g := goflare.New(cfg)

	// Build artifacts
	if err := g.Build(); err != nil {
		panic(err)
	}

	// Deploy to Cloudflare
	store := &goflare.KeyringStore{ProjectName: cfg.ProjectName}
	if err := g.Auth(store, os.Stdin); err != nil {
		panic(err)
	}
	
	if err := g.DeployWorker(store); err != nil {
		panic(err)
	}
	
	if err := g.DeployPages(store); err != nil {
		panic(err)
	}
}
```

## Configuration

Settings are loaded from `.env` or system environment variables.

| Field | .env Key | Default | Required |
|-------|----------|---------|----------|
| `ProjectName` | `PROJECT_NAME` | - | Yes |
| `AccountID` | `CLOUDFLARE_ACCOUNT_ID` | - | Yes |
| `WorkerName` | `WORKER_NAME` | `<ProjectName>-worker` | No |
| `Domain` | `DOMAIN` | - | No |
| `Entry` | `ENTRY` | - | No* |
| `PublicDir` | `PUBLIC_DIR` | - | No* |
| `CompilerMode` | `COMPILER_MODE` | `S` | No |

*\*At least one of `ENTRY` or `PUBLIC_DIR` must be provided.*

## Testing

Run unit tests:
```bash
go test ./...
```

Run integration tests (requires TinyGo):
```bash
go test ./... -tags=integration
```

## License

MIT
