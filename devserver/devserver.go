//go:build !wasm

package devserver

import (
	"io"
	"net/http"

	"github.com/tinywasm/goflare/router"
)

type nativeContext struct {
	w http.ResponseWriter
	r *http.Request
	body []byte
}

func (c *nativeContext) Method() string { return c.r.Method }
func (c *nativeContext) Path() string   { return c.r.URL.Path }
func (c *nativeContext) Body() []byte {
	if c.body == nil {
		c.body, _ = io.ReadAll(c.r.Body)
	}
	return c.body
}
func (c *nativeContext) GetHeader(key string) string {
	return c.r.Header.Get(key)
}
func (c *nativeContext) SetHeader(key, value string) {
	c.w.Header().Set(key, value)
}
func (c *nativeContext) WriteStatus(code int) {
	c.w.WriteHeader(code)
}
func (c *nativeContext) Write(b []byte) (int, error) {
	return c.w.Write(b)
}

type nativeRouter struct {
	mux *http.ServeMux
}

func NewRouter() router.Router {
	return &nativeRouter{mux: http.NewServeMux()}
}

func (r *nativeRouter) Get(path string, h router.HandlerFunc) {
	r.Handle(http.MethodGet, path, h)
}
func (r *nativeRouter) Post(path string, h router.HandlerFunc) {
	r.Handle(http.MethodPost, path, h)
}
func (r *nativeRouter) Put(path string, h router.HandlerFunc) {
	r.Handle(http.MethodPut, path, h)
}
func (r *nativeRouter) Delete(path string, h router.HandlerFunc) {
	r.Handle(http.MethodDelete, path, h)
}
func (r *nativeRouter) Options(path string, h router.HandlerFunc) {
	r.Handle(http.MethodOptions, path, h)
}
func (r *nativeRouter) Handle(method, path string, h router.HandlerFunc) {
	pattern := method + " " + path
	if method == "" {
		pattern = path
	}
	r.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		h(&nativeContext{w: w, r: r})
	})
}

// noCache wraps a handler to disable browser caching. A dev server must never
// cache: after a rebuild the browser has to fetch the fresh .wasm, not a stale copy.
func noCache(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		h.ServeHTTP(w, r)
	})
}

func ListenAndServe(addr string, r router.Router, staticDir string) error {
	nr := r.(*nativeRouter)

	// Serve static files if staticDir is provided, with caching disabled (dev).
	if staticDir != "" {
		nr.mux.Handle("/", noCache(http.FileServer(http.Dir(staticDir))))
	}

	return http.ListenAndServe(addr, nr.mux)
}
