//go:build wasm

package pages

import (
	"syscall/js"

	"github.com/tinywasm/goflare/router"
	"github.com/tinywasm/goflare/workers"
)

type wasmContext struct {
	req  *workers.Request
	res  *workers.Response
	path string
}

func (c *wasmContext) Method() string { return c.req.Method }
func (c *wasmContext) Path() string   { return c.path }
func (c *wasmContext) Body() []byte   { return c.req.Body() }
func (c *wasmContext) GetHeader(key string) string {
	return c.req.Headers[key]
}
func (c *wasmContext) SetHeader(key, value string) {
	c.res.Header()[key] = value
}
func (c *wasmContext) WriteStatus(code int) {
	c.res.WriteHeader(code)
}
func (c *wasmContext) Write(b []byte) (int, error) {
	return c.res.Write(b)
}

type route struct {
	method string
	path   string
	h      router.HandlerFunc
}

type wasmRouter struct {
	routes []route
}

func NewRouter() router.Router {
	return &wasmRouter{}
}

func (r *wasmRouter) Get(path string, h router.HandlerFunc) {
	r.Handle("GET", path, h)
}
func (r *wasmRouter) Post(path string, h router.HandlerFunc) {
	r.Handle("POST", path, h)
}
func (r *wasmRouter) Put(path string, h router.HandlerFunc) {
	r.Handle("PUT", path, h)
}
func (r *wasmRouter) Delete(path string, h router.HandlerFunc) {
	r.Handle("DELETE", path, h)
}
func (r *wasmRouter) Options(path string, h router.HandlerFunc) {
	r.Handle("OPTIONS", path, h)
}
func (r *wasmRouter) Handle(method, path string, h router.HandlerFunc) {
	r.routes = append(r.routes, route{method, path, h})
}

func Serve(r router.Router) {
	wr := r.(*wasmRouter)
	workers.Handle(func(res *workers.Response, req *workers.Request) {
		// Extract pathname from full URL using JS URL API
		pathname := js.Global().Get("URL").New(req.URL).Get("pathname").String()

		ctx := &wasmContext{req: req, res: res, path: pathname}

		// Very simple matching for MVP
		for _, rt := range wr.routes {
			if (rt.method == "" || rt.method == req.Method) && rt.path == pathname {
				rt.h(ctx)
				return
			}
		}

		res.WriteHeader(404)
		res.Write([]byte("Not Found"))
	})
}
