# Stage 07 — CLI Wiring

## Goal
`cmd/goflare/main.go` must be a thin shell: parse flags, call library, set exit code.
All logic (orchestration, output formatting, URL construction, error summary) lives in the library.

---

## Target `main.go`

```go
package main

import (
    "flag"
    "fmt"
    "os"

    "github.com/tinywasm/goflare"
)

func main() {
    if len(os.Args) < 2 {
        fmt.Fprintln(os.Stderr, goflare.Usage())
        os.Exit(1)
    }

    env := flag.String("env", ".env", "path to .env file")
    flag.CommandLine.Parse(os.Args[2:])

    var err error
    switch os.Args[1] {
    case "init":
        err = goflare.RunInit(*env, os.Stdin, os.Stdout)
    case "build":
        err = goflare.RunBuild(*env, os.Stdout)
    case "deploy":
        err = goflare.RunDeploy(*env, os.Stdin, os.Stdout)
    default:
        fmt.Fprintln(os.Stderr, goflare.Usage())
        os.Exit(1)
    }

    if err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}
```

That is the entire file. No other logic in `main.go`.

---

## Tasks

### 7.1 — `RunInit(envPath string, in io.Reader, out io.Writer) error` (`run.go`)

New file in the library package. Contains the three runner functions.

```
1. Call Init(in) → cfg
2. Call WriteEnvFile(cfg, envPath)
3. Call UpdateGitignore(".")
4. Write to out: "Init complete. Edit .env if needed, then run: goflare build"
```

### 7.2 — `RunBuild(envPath string, out io.Writer) error` (`run.go`)

```
1. LoadConfigFromEnv(envPath)
2. cfg.Validate()
3. g := New(cfg)
4. g.Build()
5. Write to out: artifact paths on success
```

### 7.3 — `RunDeploy(envPath string, in io.Reader, out io.Writer) error` (`run.go`)

```
1. LoadConfigFromEnv(envPath)
2. cfg.Validate()
3. g := New(cfg)
4. store := NewKeyringStore()
5. g.Auth(store, in)
6. if cfg.Entry != ""     → g.DeployWorker(store) → record result
7. if cfg.PublicDir != "" → g.DeployPages(store)  → record result
8. g.WriteSummary(out, results)  ← formats URLs and errors
9. return combined error if any target failed
```

### 7.4 — `Usage() string` (`run.go`)

Returns the usage string. Keeps the help text in the library, not in main.

### 7.5 — `(g *Goflare) WriteSummary(out io.Writer, results []DeployResult)` (`run.go`)

```go
type DeployResult struct {
    Target string // "Worker" or "Pages"
    URL    string // live URL on success
    Err    error
}
```

Formats and writes the deploy summary to `out`. Called by `RunDeploy`.

---

## Files Added
- `run.go` — RunInit, RunBuild, RunDeploy, Usage, WriteSummary, DeployResult

## Files Changed
- `cmd/goflare/main.go` — full rewrite to thin shell (as shown above)
- `cmd/goflare/README.md` — deleted (see Stage 09)
