//go:build !wasm

package devserver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tinywasm/goflare/router"
)

func TestNoCacheSetsHeaders(t *testing.T) {
	h := noCache(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/app.wasm", nil))

	if got := rec.Header().Get("Cache-Control"); got != "no-store, no-cache, must-revalidate, max-age=0" {
		t.Errorf("Cache-Control = %q, want no-store...", got)
	}
	if got := rec.Header().Get("Pragma"); got != "no-cache" {
		t.Errorf("Pragma = %q, want no-cache", got)
	}
}

func TestRouterDispatchesRegisteredRoute(t *testing.T) {
	r := NewRouter()
	r.Post("/api/contacto", func(ctx router.Context) {
		ctx.WriteStatus(200)
		ctx.Write([]byte(`{"ok":true}`))
	})

	mux := r.(*nativeRouter).mux
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/contacto", nil))

	if rec.Code != 200 {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != `{"ok":true}` {
		t.Errorf("body = %q", rec.Body.String())
	}
}
