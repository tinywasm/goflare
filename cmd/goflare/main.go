package main

import (
	"fmt"
	"os"

	"github.com/tinywasm/goflare"
)

func main() {
	config, err := goflare.LoadConfigFromEnv(".env")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// For Stage 01, we just want it to compile.
	// CLI wiring will be done in Stage 07.
	// Let's just make it a minimal placeholder that uses the new API.

	if config.ProjectName == "" {
		fmt.Println("Goflare CLI - Stage 01")
		fmt.Println("Usage: PROJECT_NAME=my-project CLOUDFLARE_ACCOUNT_ID=... goflare")
		return
	}

	g := goflare.New(config)
	g.SetLog(func(messages ...any) {
		for _, msg := range messages {
			fmt.Println(msg)
		}
	})

	fmt.Println("Goflare: Foundation active.")
}
