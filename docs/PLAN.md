---
PLAN: "fix!: Header() replaces Request.Headers (case-insensitive, lazy); precompute the middleware chain once per route"
EXECUTOR: jules
REVIEWER: none
STATUS: review
SESSION: 11783888586625483558
PR: https://github.com/tinywasm/goflare/pull/25
---

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.

# Plan — `tinywasm/goflare`: per-request waste in the edge runtime, and a hidden correctness bug in the same code

## Lead finding: `Cookie()` has never actually read a cookie

This is not primarily a performance plan. While investigating allocations on
the request hot path, the map that causes them turned out to also be silently
broken. Verified against the real Fetch API under `workerd` (not assumed):

```
entries() key: cookie
get("Cookie"): sid=abc123
get("cookie"): sid=abc123
```

The Fetch `Headers` object normalizes names to **lowercase** for
`entries()`/iteration, but `Headers.get(name)` matches **case-insensitively**.
[`workers/request.go`](../workers/request.go)'s `newRequest` builds its map
from `entries()` — so every key it stores is lowercase — while
[`edge/edge.go`](../edge/edge.go) reads it back with the literal case
`"Cookie"`:

```go
h := c.req.Headers["Cookie"]   // map key is "cookie" — this is ALWAYS ""
```

`Cookie()` has therefore returned `false` for every cookie, on every request,
in every goflare-deployed Worker to date — `veltylabs/iam` included, whose
entire purpose is reading an SSO session cookie. This was invisible because no
test exercises a real `Headers` object:
[`tests/edge_conformance_test.go`](../tests/edge_conformance_test.go) drives
`router.Context` directly with a hand-built fake, never through
`workers.Request`.

## The waste this plan also removes

Three things happen on **every single HTTP request**, unconditionally, for
work that is either constant across requests or usually never read:

1. **`newRequest`** iterates the full `Headers` object via
   `entries()`/`.next()` — a JS↔Go round trip per header, two string
   allocations per header, and a map allocation — to populate a field that
   `c.req.Header(key)`/`Cookie()` (via `GetHeader`) usually reads **at most
   one or two keys** from. Cloudflare Workers attach on the order of ten
   headers per request in production (`cf-connecting-ip`, `cf-ray`,
   `cf-visitor`, `accept-encoding`, `user-agent`, `host`, …).
2. **`gateAndServe`** ([`edge/edge.go`](../edge/edge.go)) rebuilds the
   middleware-wrapped handler — one closure allocation per middleware — on
   every request, even though neither the route's handler nor
   `r.middlewares` ever changes after startup.
3. **`Cookie()`** copies the entire `Cookie` header into a fresh `[]byte`
   (`b := []byte(h)`) before scanning it, on every call, discarding the copy
   immediately after.

All three matter more than they used to: since
`fix!: start the Go instance once per isolate` (already merged), one Go
instance now serves every request in the isolate, so none of this is
amortized by a fresh instance anymore — it is pure repeated cost.

## Stage 1 — `Header(key)` replaces `Headers` (fixes the bug, removes the map)

[`workers/request.go`](../workers/request.go):

```go
//go:build wasm

package workers

import (
	"syscall/js"

	"github.com/tinywasm/fmt"
)

// Request represents an incoming HTTP request to the Worker.
type Request struct {
	Method  string
	URL     string
	jsReq   js.Value
	headers js.Value
	body    []byte
	hasBody bool
}

// Header returns the value of the named header, or "" if absent. Lookup is
// case-insensitive, per the Fetch Headers.get() contract — do not "fix" this
// by lowercasing key yourself; Headers.get() already does the right thing,
// and double-normalizing invites the exact bug this method replaces (see
// docs/PLAN.md at the time of this change: Headers.entries() lowercases
// names for iteration, but .get() matches case-insensitively — reading one
// as if it behaved like the other silently returned "" for every request).
func (r *Request) Header(key string) string {
	if r.headers.IsNull() || r.headers.IsUndefined() {
		return ""
	}
	v := r.headers.Call("get", key)
	if v.IsNull() || v.IsUndefined() {
		return ""
	}
	return v.String()
}

// Body returns the raw request body bytes.
// It reads the body lazily on the first call.
func (r *Request) Body() []byte {
	// unchanged — keep the existing implementation below this line
	...
}

// newRequest reads a JS Fetch Request into a Go Request.
func newRequest(jsReq js.Value) (*Request, error) {
	return &Request{
		Method:  jsReq.Get("method").String(),
		URL:     jsReq.Get("url").String(),
		jsReq:   jsReq,
		headers: jsReq.Get("headers"),
	}, nil
}
```

Delete the `entries()`/`.next()` loop entirely — there is nothing left for it
to populate.

[`edge/edge.go`](../edge/edge.go) — update the two call sites:

```go
func (c *wasmContext) GetHeader(key string) string {
	return c.req.Header(key)
}
```

```go
func (c *wasmContext) Cookie(name string) (router.Cookie, bool) {
	h := c.req.Header("Cookie")
	if h == "" {
		return router.Cookie{}, false
	}
	...
```

Grep the whole repo for `\.Headers\b` first — the plan's own investigation
found exactly these two call sites, but confirm no other consumer of this
package reads the field before deleting it.

## Stage 2 — `Cookie()` scans the string directly, no `[]byte` copy

Same function, only the local variable types change — the matching logic is
unchanged, it now indexes `h` (a `string`) instead of `b` (a copied
`[]byte`):

```go
func (c *wasmContext) Cookie(name string) (router.Cookie, bool) {
	h := c.req.Header("Cookie")
	if h == "" {
		return router.Cookie{}, false
	}

	// Manual parsing of "Cookie" header: name1=val1; name2=val2
	// Minimal implementation: search for "name="
	// Note: we can't use strings.Split (stdlib prohibited)
	// We'll do a simple scan directly over the string — Go strings already
	// index by byte, so copying to []byte first only allocated a throwaway
	// copy for no benefit.
	needle := name + "="
	for i := 0; i <= len(h)-len(needle); i++ {
		if h[i:i+len(needle)] != needle {
			continue
		}
		if i != 0 && h[i-1] != ' ' && h[i-1] != ';' {
			continue
		}
		start := i + len(needle)
		end := start
		for end < len(h) && h[end] != ';' {
			end++
		}
		return router.Cookie{Name: name, Value: h[start:end]}, true
	}

	return router.Cookie{}, false
}
```

`h[i:i+len(needle)] != needle` is a byte-for-byte string comparison — the Go
compiler does not allocate for it. `h[start:end]` returned directly as
`Value` is fine: `router.Cookie.Value` takes a `string`, and a Go substring
slice shares the backing array by design — the tiny header string being kept
alive slightly longer costs nothing next to the allocation removed.

## Stage 3 — precompute each route's middleware chain once

[`edge/edge.go`](../edge/edge.go), `wasmRoute` gains a memoized field:

```go
type wasmRoute struct {
	info    router.RouteInfo
	h       router.HandlerFunc
	wrapped router.HandlerFunc // set once by compile(), never per request
}
```

Add `compile()`, called from `Serve()` right after `Validate()` — routes and
middlewares are only ever registered before `Serve()` runs, so this is the
one correct place to freeze them:

```go
// compile wraps every route's handler with the middleware chain exactly
// once. gateAndServe used to do this on every request — a closure
// allocation per middleware, per request — even though neither a route's
// handler nor r.middlewares ever changes after Serve() starts. Now there is
// nothing left to rebuild: dispatch calls the frozen wrapped handler
// directly.
func (r *wasmRouter) compile() {
	for _, rt := range r.routes {
		h := rt.h
		for i := len(r.middlewares) - 1; i >= 0; i-- {
			h = r.middlewares[i](h)
		}
		rt.wrapped = h
	}
}
```

```go
func Serve(r router.Router) {
	wr := r.(*wasmRouter)

	// Loudly, at startup — never a silent 403 in production.
	Validate(wr)
	wr.compile()

	workers.Handle(func(res *workers.Response, req *workers.Request) {
		pathname := js.Global().Get("URL").New(req.URL).Get("pathname").String()
		Dispatch(wr, &wasmContext{req: req, res: res, path: pathname})
	})
}
```

`gateAndServe` calls the frozen handler instead of rebuilding it:

```go
func (r *wasmRouter) gateAndServe(ctx router.Context) {
	method, pathname := ctx.Method(), ctx.Path()

	route, status := r.match(method, pathname)
	if route == nil {
		reason := "no route matches"
		if status == 405 {
			reason = "the path exists but not for this method"
		}
		log.Reject(status, method, pathname, reason)
		ctx.WriteStatus(status)
		ctx.Write([]byte(fmt.Convert(status).String()))
		return
	}

	if ok, why := r.allows(route.info, ctx.UserID()); !ok {
		log.Reject(403, method, pathname, why)
		ctx.WriteStatus(403)
		ctx.Write([]byte("Forbidden"))
		return
	}

	route.wrapped(ctx)
}
```

`route.wrapped` must equal the loop that used to run inline — same
composition order (last-registered middleware wraps outermost), same
"middleware runs behind the gate" ordering (match → allow → middleware →
handler). Do not change that order while moving the wrapping earlier; only
*when* it runs changes, not *what* it produces.

If `PublicDir`/`PublicAsset` routes have no handler (`rt.h == nil` — check
`PublicDir`'s definition, which sets no `h`), `compile()` must not panic on a
nil `HandlerFunc` — wrapping `nil` through zero middlewares is harmless, but
guard it explicitly if any middleware would be invoked on it:

```go
	for _, rt := range r.routes {
		if rt.h == nil {
			continue // static file / directory routes are served elsewhere
		}
		...
```

## Stage 4 — tests

**`workers/request_internal_test.go`** (new, `package workers`, `//go:build
wasm` — same internal-test pattern as
[`workers/response_internal_test.go`](../workers/response_internal_test.go),
which already accesses unexported fields this way):

```go
//go:build wasm

package workers

import (
	"syscall/js"
	"testing"
)

// TestRequest_HeaderIsCaseInsensitive is the regression test for the bug this
// plan's own investigation found: newRequest used to build a map from
// Headers.entries(), which the Fetch spec lowercases, while callers looked
// keys up with their original case — so Cookie() returned false on every
// single request, in every deployed goflare Worker. Verified against the
// real Fetch API under workerd before this fix:
//
//	entries() key: cookie
//	get("Cookie"): sid=abc123
//	get("cookie"): sid=abc123
func TestRequest_HeaderIsCaseInsensitive(t *testing.T) {
	h := js.Global().Get("Headers").New()
	h.Call("set", "Cookie", "sid=abc123")

	jsReq := js.Global().Get("Object").New()
	jsReq.Set("method", "GET")
	jsReq.Set("url", "https://example.test/")
	jsReq.Set("headers", h)

	r, err := newRequest(jsReq)
	if err != nil {
		t.Fatalf("newRequest: %v", err)
	}

	for _, key := range []string{"Cookie", "cookie", "COOKIE"} {
		if got := r.Header(key); got != "sid=abc123" {
			t.Errorf("Header(%q) = %q, want %q", key, got, "sid=abc123")
		}
	}
	if got := r.Header("X-Missing"); got != "" {
		t.Errorf(`Header("X-Missing") = %q, want ""`, got)
	}
}
```

**`tests/edge_middleware_compile_test.go`** (new, `package goflare_test`,
`//go:build wasm` — same package as
[`tests/edge_conformance_test.go`](../tests/edge_conformance_test.go), using
its existing `conformanceCtx` fake):

```go
//go:build wasm

package goflare_test

import (
	"testing"

	"github.com/tinywasm/goflare/edge"
	"github.com/tinywasm/router"
)

// TestMiddleware_WrappedOnceNotPerRequest is the regression test for
// gateAndServe rebuilding the middleware chain on every request. wraps counts
// how many times THIS middleware's own (HandlerFunc) HandlerFunc conversion
// runs — the composition step edge.go used to repeat per request — not how
// many times the resulting handler serves a request.
func TestMiddleware_WrappedOnceNotPerRequest(t *testing.T) {
	wraps := 0
	counting := func(next router.HandlerFunc) router.HandlerFunc {
		wraps++
		return next
	}

	r := edge.NewRouter(edge.Config{})
	r.Use(counting)
	r.Get("/ping", func(ctx router.Context) {
		ctx.Write([]byte("pong"))
	}).Public()

	edge.Validate(r)
	// compile() is unexported and reached only through Serve(), which blocks
	// forever (workers.Handle's select{}) — outside a real Worker there is no
	// way to call it directly. Dispatch three requests through the same
	// router and assert wraps stayed at the count Serve()'s compile() would
	// have produced: exactly once, regardless of how many requests follow.
	for i := 0; i < 3; i++ {
		edge.Dispatch(r, &conformanceCtx{method: "GET", path: "/ping"})
	}

	if wraps != 1 {
		t.Errorf("middleware composed %d times across 3 requests, want 1 — "+
			"gateAndServe is rebuilding the chain per request instead of reusing "+
			"a handler compiled once", wraps)
	}
}
```

If `Dispatch` does not itself trigger the same compilation path `Serve()`
uses (check `edge.Dispatch`'s current body against Stage 3's design before
assuming it does) — expose a minimal test seam instead, e.g. an unexported
`compileForTest()` alias called from an exported-for-test wrapper following
the `ExportX` convention already used in
[`assets.go`](../assets.go)
(`ExportAssetHash`, `ExportBuildAssetManifest`, `ExportUploadAssets`). Whatever
the seam, the test must exercise the real `compile()`/dispatch path — not a
hand-rolled reimplementation of it, which would prove nothing about the
actual code.

## Stage 5 — verification

- `grep -n "Headers " workers/request.go` → empty (no map field remains).
- `grep -rn "\.Headers\[" .` → empty.
- `grep -n "\[\]byte(h)" edge/edge.go` → empty.

## Acceptance criteria

- [ ] `go build ./...` and `go vet ./...` clean.
- [ ] `go test ./tests/...` — full non-WASM suite green.
- [ ] `GOOS=js GOARCH=wasm go test -exec wasmbrowsertest ./...` — green,
      including `TestRequest_HeaderIsCaseInsensitive` and
      `TestMiddleware_WrappedOnceNotPerRequest`.
- [ ] Stage 5's three greps all pass.
- [ ] `router.Cookie` returned by `Cookie()` carries the correct name/value
      for a cookie set anywhere in the header (first, middle, last position)
      — existing behavior, must not regress.

| Stage | File(s) | Done when |
|---|---|---|
| 1 | `workers/request.go`, `edge/edge.go` | `Request.Headers` map gone; `Header(key)` reads via `Headers.get()`, case-insensitively |
| 2 | `edge/edge.go` | `Cookie()` scans the header string directly, no `[]byte` copy |
| 3 | `edge/edge.go` | `wasmRoute.wrapped` computed once in `compile()`, called from `Serve()`; `gateAndServe` calls it directly |
| 4 | `workers/request_internal_test.go`, `tests/edge_middleware_compile_test.go` | Both new tests pass |
| 5 | (verification only) | Stage 5's greps pass |
