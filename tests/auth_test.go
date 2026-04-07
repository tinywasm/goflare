package goflare_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/tinywasm/goflare"
)

func TestAuth_TokenAlreadyInStore(t *testing.T) {
	store := goflare.NewMemoryStore()
	store.Set("token", "valid-token")

	cfg := &goflare.Config{ProjectName: "test"}
	g := goflare.New(cfg)

	// Auth should not read from prompt because token is in store
	err := g.Auth(store, strings.NewReader("unused-token\n"))
	if err != nil {
		t.Fatalf("Auth failed: %v", err)
	}

	token, _ := g.GetToken(store)
	if token != "valid-token" {
		t.Errorf("expected valid-token, got %s", token)
	}
}

func TestAuth_ValidatesAndStores(t *testing.T) {
	origURL := goflare.SetCFBaseURL("http://localhost") // Mocking via global variable change
	defer goflare.SetCFBaseURL(origURL)

	server := MockHTTPServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/user/tokens/verify" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"success": true, "result": {"status": "active"}}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	defer server.Close()
	goflare.SetCFBaseURL(server.URL)

	store := goflare.NewMemoryStore()
	cfg := &goflare.Config{ProjectName: "test"}
	g := goflare.New(cfg)

	err := g.Auth(store, strings.NewReader("new-token\n"))
	if err != nil {
		t.Fatalf("Auth failed: %v", err)
	}

	token, _ := g.GetToken(store)
	if token != "new-token" {
		t.Errorf("expected new-token, got %s", token)
	}
}

func TestAuth_InvalidToken(t *testing.T) {
	server := MockHTTPServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"success": false, "errors": [{"message": "invalid token"}]}`)
	})
	defer server.Close()

	origURL := goflare.SetCFBaseURL(server.URL)
	defer goflare.SetCFBaseURL(origURL)

	store := goflare.NewMemoryStore()
	cfg := &goflare.Config{ProjectName: "test"}
	g := goflare.New(cfg)

	err := g.Auth(store, strings.NewReader("bad-token\n"))
	if err == nil {
		t.Fatal("expected error for invalid token, got nil")
	}
	if !strings.Contains(err.Error(), "invalid token") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestGetToken_ErrorWhenMissing(t *testing.T) {
	store := goflare.NewMemoryStore()
	cfg := &goflare.Config{ProjectName: "test"}
	g := goflare.New(cfg)

	_, err := g.GetToken(store)
	if err == nil {
		t.Error("expected error when token missing, got nil")
	}
}
