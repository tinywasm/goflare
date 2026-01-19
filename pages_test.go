package goflare

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGeneratePagesFiles(t *testing.T) {
	// Create a temporary directory for the test
	tempDir := t.TempDir()

	// Use default config but set AppRootDir to temp dir
	config := DefaultConfig()
	config.AppRootDir = tempDir

	// Define paths
	inputDir := filepath.Join(tempDir, config.RelativeInputDirectory())
	outputDir := filepath.Join(tempDir, config.RelativeOutputDirectory())

	// Create directories
	err := os.MkdirAll(inputDir, 0755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.MkdirAll(outputDir, 0755)
	if err != nil {
		t.Fatal(err)
	}

	mainFile := filepath.Join(inputDir, config.MainInputFile)
	dummyCode := `package main

import (
	"syscall/js"
)

func main() {
	js.Global().Set("hello", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		return "Hello from WASM test"
	}))
	select {}
}
`
	err = os.WriteFile(mainFile, []byte(dummyCode), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create Goflare instance
	g := New(config)

	// Change to temp dir for relative paths
	originalDir, _ := os.Getwd()
	defer os.Chdir(originalDir)
	os.Chdir(tempDir)

	// Call GeneratePagesFiles
	err = g.GeneratePagesFiles()
	if err != nil {
		t.Fatalf("GeneratePagesFiles failed: %v", err)
	}

	// Check if output files exist
	workerFile := filepath.Join(outputDir, "_worker.js")
	wasmFile := filepath.Join(outputDir, config.OutputWasmFileName)

	if _, err := os.Stat(workerFile); os.IsNotExist(err) {
		t.Error("_worker.js file was not created")
	}

	if _, err := os.Stat(wasmFile); os.IsNotExist(err) {
		t.Error("worker.wasm file was not created")
	}

	// Verify that ONLY these 2 files were created in the output directory
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		t.Fatalf("Failed to read output directory: %v", err)
	}

	if len(entries) != 2 {
		t.Errorf("Expected exactly 2 files in output directory, but found %d", len(entries))
		t.Log("Files found:")
		for _, entry := range entries {
			t.Logf("  - %s", entry.Name())
		}
	}

	// Verify the exact file names
	expectedFiles := map[string]bool{
		"_worker.js":              false,
		config.OutputWasmFileName: false,
	}

	for _, entry := range entries {
		if _, exists := expectedFiles[entry.Name()]; exists {
			expectedFiles[entry.Name()] = true
		} else {
			t.Errorf("Unexpected file created: %s", entry.Name())
		}
	}

	for filename, found := range expectedFiles {
		if !found {
			t.Errorf("Expected file not found: %s", filename)
		}
	}
}
