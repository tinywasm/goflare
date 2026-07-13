//go:build wasm

package edge

import (
	"syscall/js"

	"github.com/tinywasm/fmt"
	"github.com/tinywasm/goflare/workers"
	"github.com/tinywasm/router"
)

type wasmContext struct {
	req  *workers.Request
	res  *workers.Response
	path string
	vals map[string]any
	uid  string
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

func (c *wasmContext) SetValue(key string, v any) {
	if c.vals == nil {
		c.vals = make(map[string]any)
	}
	c.vals[key] = v
}

func (c *wasmContext) Value(key string) any {
	if c.vals == nil {
		return nil
	}
	return c.vals[key]
}

func (c *wasmContext) SetUserID(id string) {
	c.uid = id
}

func (c *wasmContext) UserID() string {
	return c.uid
}

func (c *wasmContext) SetCookie(cookie router.Cookie) {
	var s string
	s = fmt.Sprintf("%s=%s; Path=%s", cookie.Name, cookie.Value, cookie.Path)
	if cookie.Domain != "" {
		s += "; Domain=" + cookie.Domain
	}
	if cookie.MaxAge > 0 {
		s += fmt.Sprintf("; Max-Age=%d", cookie.MaxAge)
	}
	if cookie.Secure {
		s += "; Secure"
	}
	if cookie.HttpOnly {
		s += "; HttpOnly"
	}
	switch cookie.SameSite {
	case router.SameSiteLax:
		s += "; SameSite=Lax"
	case router.SameSiteStrict:
		s += "; SameSite=Strict"
	case router.SameSiteNone:
		s += "; SameSite=None"
	}

	existing := c.res.Header()["Set-Cookie"]
	if existing == "" {
		c.res.Header()["Set-Cookie"] = s
	} else {
		c.res.Header()["Set-Cookie"] = existing + ", " + s
	}
}

func (c *wasmContext) Cookie(name string) (router.Cookie, bool) {
	h := c.req.Headers["Cookie"]
	if h == "" {
		return router.Cookie{}, false
	}

	// Manual parsing of "Cookie" header: name1=val1; name2=val2
	// Minimal implementation: search for "name="
	// Note: we can't use strings.Split (stdlib prohibited)
	// We'll do a simple scan.
	b := []byte(h)
	nb := []byte(name + "=")
	for i := 0; i <= len(b)-len(nb); i++ {
		match := true
		for j := 0; j < len(nb); j++ {
			if b[i+j] != nb[j] {
				match = false
				break
			}
		}
		if match && (i == 0 || b[i-1] == ' ' || b[i-1] == ';') {
			start := i + len(nb)
			end := start
			for end < len(b) && b[end] != ';' {
				end++
			}
			val := string(b[start:end])
			return router.Cookie{Name: name, Value: val}, true
		}
	}

	return router.Cookie{}, false
}

type wasmRoute struct {
	info router.RouteInfo
	h    router.HandlerFunc
}

func (r *wasmRoute) Requires(resource, action string) router.Route {
	r.info.Resource = resource
	r.info.Action = action
	return r
}

func (r *wasmRoute) Public() router.Route {
	r.info.Public = true
	return r
}

type wasmRouter struct {
	routes      []*wasmRoute
	middlewares []router.Middleware
}

func NewRouter() router.Router {
	return &wasmRouter{}
}

func (r *wasmRouter) Get(path string, h router.HandlerFunc) router.Route {
	return r.Handle("GET", path, h)
}
func (r *wasmRouter) Post(path string, h router.HandlerFunc) router.Route {
	return r.Handle("POST", path, h)
}
func (r *wasmRouter) Put(path string, h router.HandlerFunc) router.Route {
	return r.Handle("PUT", path, h)
}
func (r *wasmRouter) Delete(path string, h router.HandlerFunc) router.Route {
	return r.Handle("DELETE", path, h)
}
func (r *wasmRouter) Options(path string, h router.HandlerFunc) router.Route {
	return r.Handle("OPTIONS", path, h)
}
func (r *wasmRouter) Handle(method, path string, h router.HandlerFunc) router.Route {
	rt := &wasmRoute{
		info: router.RouteInfo{Method: method, Path: path},
		h:    h,
	}
	r.routes = append(r.routes, rt)
	return rt
}

// PublicAsset registra UNA ruta que sirve UN archivo al navegador.
func (r *wasmRouter) PublicAsset(path string, h router.HandlerFunc) {
	route := &wasmRoute{
		info: router.RouteInfo{Method: "GET", Path: path, Public: true},
		h:    h,
	}
	r.routes = append(r.routes, route)
}

// PublicDir sirve un directorio bajo un prefijo. Mismo contrato.
func (r *wasmRouter) PublicDir(prefix string, dir string) {
	route := &wasmRoute{
		info: router.RouteInfo{Method: "GET", Path: prefix, Public: true, Dir: dir},
	}
	r.routes = append(r.routes, route)
}

func (r *wasmRouter) Use(m ...router.Middleware) {
	r.middlewares = append(r.middlewares, m...)
}

func (r *wasmRouter) Routes() []router.RouteInfo {
	infos := make([]router.RouteInfo, len(r.routes))
	for i, rt := range r.routes {
		infos[i] = rt.info
	}
	return infos
}

func (r *wasmRouter) Stream(path string, h router.StreamFunc) router.Route {
	panic("Stream not supported in this runtime")
}

func (r *wasmRouter) Socket(path string, h router.SocketFunc) router.Route {
	panic("Socket not supported in this runtime")
}

func Serve(r router.Router) {
	wr := r.(*wasmRouter)
	workers.Handle(func(res *workers.Response, req *workers.Request) {
		pathname := js.Global().Get("URL").New(req.URL).Get("pathname").String()

		ctx := &wasmContext{req: req, res: res, path: pathname}

		var bestMatch *wasmRoute
		for _, rt := range wr.routes {
			if rt.info.Method != "" && rt.info.Method != req.Method {
				continue
			}

			matched := false
			pattern := rt.info.Path
			if pattern != "" && pattern[len(pattern)-1] == '/' {
				// Prefix match
				if len(pathname) >= len(pattern) {
					match := true
					for i := 0; i < len(pattern); i++ {
						if pathname[i] != pattern[i] {
							match = false
							break
						}
					}
					if match {
						matched = true
					}
				}
			} else {
				// Exact match
				if pathname == pattern {
					matched = true
				}
			}

			if matched {
				if bestMatch == nil || len(rt.info.Path) > len(bestMatch.info.Path) {
					bestMatch = rt
				}
			}
		}

		if bestMatch != nil {
			// RBAC check
			if !bestMatch.info.Public && bestMatch.info.Resource == "" && ctx.UserID() == "" {
				res.WriteHeader(403)
				res.Write([]byte("Forbidden: Private by default"))
				return
			}
			// Note: if Resource is set, an actual authorizer would be needed.
			// The plan says: "without marker and without identity -> 403"
			// If Requires() was called, it should also probably check UserID.
			if bestMatch.info.Resource != "" && ctx.UserID() == "" {
				res.WriteHeader(403)
				res.Write([]byte("Forbidden: Identity required"))
				return
			}

			h := bestMatch.h
			// Apply middlewares in reverse order
			for i := len(wr.middlewares) - 1; i >= 0; i-- {
				h = wr.middlewares[i](h)
			}
			h(ctx)
			return
		}

		res.WriteHeader(404)
		res.Write([]byte("Not Found"))
	})
}

var _ router.Router = (*wasmRouter)(nil)
var _ router.Context = (*wasmContext)(nil)
var _ router.Route = (*wasmRoute)(nil)
