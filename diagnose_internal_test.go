package goflare

import (
	"slices"
	"testing"
)

func TestSizeBreakdownArgs(t *testing.T) {
	tmpPath := "/tmp/test.wasm"
	args := sizeBreakdownArgs(tmpPath)

	if !slices.Contains(args, "-size=full") {
		t.Errorf("expected sizeBreakdownArgs to contain -size=full, got: %v", args)
	}

	if slices.Contains(args, "-no-debug") {
		t.Errorf("expected sizeBreakdownArgs to NOT contain -no-debug, got: %v", args)
	}
}
