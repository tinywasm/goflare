//go:build !wasm

package goflare_test

import (
	"net/http"
	"net/http/httptest"
)

// MockHTTPServer creates a mock HTTP server for testing.
func MockHTTPServer(handler http.HandlerFunc) *httptest.Server {
	return httptest.NewServer(handler)
}