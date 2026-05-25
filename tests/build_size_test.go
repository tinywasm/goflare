package goflare_test

import (
	"os"
	"testing"

	"github.com/tinywasm/goflare"
)

func TestBuild_NoDotBuildInRepo(t *testing.T) {
	// If OutputDir is ".build/" (default), New() creates a staging dir.
	// Build() should use it and then remove it.

	// We use a separate directory to avoid conflicts with existing .build
	tmpDir, err := os.MkdirTemp("", "goflare-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	// Create minimal project
	os.MkdirAll("edge", 0755)
	os.WriteFile("edge/main.go", []byte(`package main
import _ "github.com/tinywasm/goflare/workers"
func main() {}`), 0644)

	cfg := &goflare.Config{
		ProjectName: "test-staging",
		AccountID:   "123",
		Entry:       "edge/main.go",
		// OutputDir defaults to .build/
	}

	_ = goflare.New(cfg)

	// Before build, .build/ should not exist
	if _, err := os.Stat(".build"); err == nil {
		t.Error(".build/ should not exist yet")
	}
}
