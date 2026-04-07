package goflare_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
)

// TempDir creates a temporary directory for testing and returns its path and a cleanup function.
func TempDir() (string, func(), error) {
	tmpDir, err := os.MkdirTemp("", "goflare-test-*")
	if err != nil {
		return "", nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	cleanup := func() {
		os.RemoveAll(tmpDir)
	}
	return tmpDir, cleanup, nil
}

// MockHTTPServer creates a mock HTTP server for testing.
func MockHTTPServer(handler http.HandlerFunc) *httptest.Server {
	return httptest.NewServer(handler)
}
