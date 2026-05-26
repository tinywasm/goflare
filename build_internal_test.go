package goflare

import (
	"os"
	"testing"
)

func TestCheckWasmSize(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-*.wasm")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	// Test under limit
	if err := checkWasmSize(tmpFile.Name()); err != nil {
		t.Errorf("Expected no error for empty file, got %v", err)
	}

	// Test over limit
	if err := tmpFile.Truncate(maxWasmSize + 1); err != nil {
		t.Fatal(err)
	}
	if err := checkWasmSize(tmpFile.Name()); err == nil {
		t.Error("Expected error for file over limit, got nil")
	}
}
