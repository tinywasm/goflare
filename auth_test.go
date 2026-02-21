package goflare_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tinywasm/goflare"
)

// mockKeyManager is an in-memory KeyManager for tests.
type mockKeyManager struct {
	store map[string]string
}

func newMockKeyManager() *mockKeyManager {
	return &mockKeyManager{store: make(map[string]string)}
}

func (m *mockKeyManager) Get(service, user string) (string, error) {
	v, ok := m.store[service+":"+user]
	if !ok {
		return "", &mockNotFound{service + ":" + user}
	}
	return v, nil
}

func (m *mockKeyManager) Set(service, user, password string) error {
	m.store[service+":"+user] = password
	return nil
}

type mockNotFound struct{ key string }

func (e *mockNotFound) Error() string { return "not found: " + e.key }

// mockCFServer creates a test HTTP server simulating the Cloudflare API.
func mockCFServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	// GET /client/v4/user/tokens/permission_groups
	mux.HandleFunc("/client/v4/user/tokens/permission_groups", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		groups := []map[string]string{
			{"id": "perm-read-id", "name": "Cloudflare Pages:Read"},
			{"id": "perm-edit-id", "name": "Cloudflare Pages:Edit"},
		}
		raw, _ := json.Marshal(groups)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"errors":  []any{},
			"result":  json.RawMessage(raw),
		})
	})

	// POST /client/v4/user/tokens
	mux.HandleFunc("/client/v4/user/tokens", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		result := map[string]string{"id": "tok-id-123", "value": "scoped-token-abc"}
		raw, _ := json.Marshal(result)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"errors":  []any{},
			"result":  json.RawMessage(raw),
		})
	})

	return httptest.NewServer(mux)
}

func TestAuth_IsConfigured_False(t *testing.T) {
	keys := newMockKeyManager()
	a := goflare.NewAuth(keys)
	if a.IsConfigured() {
		t.Error("expected IsConfigured to return false when no token stored")
	}
}

func TestAuth_IsConfigured_True(t *testing.T) {
	keys := newMockKeyManager()
	keys.Set("cloudflare", "pages_token", "some-token")
	a := goflare.NewAuth(keys)
	if !a.IsConfigured() {
		t.Error("expected IsConfigured to return true when token stored")
	}
}

func TestAuth_Setup_StoresCredentials(t *testing.T) {
	srv := mockCFServer(t)
	defer srv.Close()

	keys := newMockKeyManager()
	a := goflare.NewAuthWithBaseURL(keys, srv.URL+"/client/v4")

	if err := a.Setup("acct-123", "bootstrap-token", "my-pages-project"); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	tok, err := keys.Get("cloudflare", "pages_token")
	if err != nil || tok != "scoped-token-abc" {
		t.Errorf("expected pages_token='scoped-token-abc', got %q (err=%v)", tok, err)
	}

	acct, err := keys.Get("cloudflare", "account_id")
	if err != nil || acct != "acct-123" {
		t.Errorf("expected account_id='acct-123', got %q (err=%v)", acct, err)
	}

	proj, err := keys.Get("cloudflare", "pages_project")
	if err != nil || proj != "my-pages-project" {
		t.Errorf("expected pages_project='my-pages-project', got %q (err=%v)", proj, err)
	}
}

func TestAuth_Setup_NoPagesEditPermission(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/client/v4/user/tokens/permission_groups", func(w http.ResponseWriter, r *http.Request) {
		// Return groups without Pages:Edit
		groups := []map[string]string{
			{"id": "other-id", "name": "DNS:Edit"},
		}
		raw, _ := json.Marshal(groups)
		json.NewEncoder(w).Encode(map[string]any{"success": true, "errors": []any{}, "result": json.RawMessage(raw)})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	keys := newMockKeyManager()
	a := goflare.NewAuthWithBaseURL(keys, srv.URL+"/client/v4")

	err := a.Setup("acct-123", "bootstrap-token", "project")
	if err == nil {
		t.Error("expected error when Pages:Edit permission not found")
	}
}
