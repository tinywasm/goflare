package goflare_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/tinywasm/goflare"
)

func TestAuth_TokenAlreadyInStore(t *testing.T) {
	store := goflare.NewMemoryStore()
	store.Set("goflare/test-project", "valid-token")

	cfg := &goflare.Config{ProjectName: "test-project"}
	g := goflare.New(cfg)

	in := strings.NewReader("prompted-token\n")
	err := g.Auth(store, in)
	if err != nil {
		t.Fatalf("Auth failed: %v", err)
	}

	token, _ := g.GetToken(store)
	if token != "valid-token" {
		t.Errorf("Expected valid-token, got %s", token)
	}
}

func TestAuth_ValidatesAndStores(t *testing.T) {
	server := MockHTTPServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/user/tokens/verify" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":{"id":"test","status":"active"}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	defer server.Close()

	store := goflare.NewMemoryStore()
	cfg := &goflare.Config{ProjectName: "test-project"}
	g := goflare.New(cfg)
	g.BaseURL = server.URL

	in := strings.NewReader("new-token\n")
	err := g.Auth(store, in)
	if err != nil {
		t.Fatalf("Auth failed: %v", err)
	}

	token, _ := g.GetToken(store)
	if token != "new-token" {
		t.Errorf("Expected new-token, got %s", token)
	}
}

func TestAuth_InvalidToken(t *testing.T) {
	server := MockHTTPServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"success":false,"errors":[{"code":1000,"message":"Invalid token"}]}`))
	})
	defer server.Close()

	store := goflare.NewMemoryStore()
	cfg := &goflare.Config{ProjectName: "test-project"}
	g := goflare.New(cfg)
	g.BaseURL = server.URL

	in := strings.NewReader("invalid-token\n")
	err := g.Auth(store, in)
	if err == nil {
		t.Fatal("Expected error for invalid token")
	}

	_, err = g.GetToken(store)
	if err == nil {
		t.Fatal("Token should not be stored if invalid")
	}
}

func TestGetToken_ErrorWhenMissing(t *testing.T) {
	store := goflare.NewMemoryStore()
	cfg := &goflare.Config{ProjectName: "test-project"}
	g := goflare.New(cfg)

	_, err := g.GetToken(store)
	if err == nil {
		t.Fatal("Expected error when token missing from store")
	}
}
