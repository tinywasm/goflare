# Testing Strategy — `goflare`

How to test code that runs **inside a Cloudflare Worker** without deploying to Cloudflare on
every change.

> **The one rule.** Ask: *can this test fail because of a bug in **our** Go code?* → **Tier 2**
> (browser + fake `context.env`). *Can it only fail because **our fake lies** about
> Cloudflare?* → **Tier 3** (`wrangler dev`). Around 95% of tests are Tier 2 and never touch
> Cloudflare or wrangler.

## Where tests live

**All tests go in `tests/`**, as `package goflare_test` — black-box, exercising the public API.
That is where the existing suite lives; follow it.

The **only** exception is a test that must reach **unexported** symbols. Those live next to
the code they test, named `*_internal_test.go`, in the package itself (see
`build_internal_test.go`). Use this only when the symbol genuinely cannot be exercised through
the public API — e.g. `inferMode`, which is unexported by design.

| Test | Where | Package |
|---|---|---|
| Anything reachable through the public API | `tests/` | `goflare_test` |
| Unexported symbol (`inferMode`, …) | next to the code, `*_internal_test.go` | the package itself |

A WASM test is a normal file in `tests/` carrying `//go:build wasm`.

## The key insight: the seam is `js.Global()`, not Cloudflare

Our edge code never talks to Cloudflare. It talks to a **JS global object** that the
Cloudflare runtime happens to inject. See `d1/adapter.go`:

```go
v := js.Global().Get("context").Get("env").Get(bindingName)
```

That is the entire boundary. In a browser — which `gotest` already launches for WASM — **we
can inject that object ourselves**. The Go code under test cannot tell the difference: the
real code path runs end to end (`syscall/js` interop, promise blocking, `js.CopyBytesToGo`),
only the binding on the far side is ours.

This is a **seam**, not a mock of our own code. We never fake `edge`, `workers`, `d1` or
`r2` — we fake only what Cloudflare would have injected.

## The three tiers

| Tier | Tool | What it covers | When |
|---|---|---|---|
| **1 — Native** | `gotest` (`!wasm`) | Build pipeline, `inferMode`, config, Cloudflare API client (against `httptest`) | Always |
| **2 — Browser** | `gotest` (WASM) + fake `context.env` | **All edge code**: route matching, cookies, RBAC, binary body, D1, R2 | Always — this is the fast loop |
| **3 — Smoke** | `wrangler dev` (miniflare, **local**) | That the generated config boots, and that the **real** bindings behave like our fake | CI and before publishing |

**Node is not a tier.** It is only the runtime wrangler needs internally. We never write tests
in node, vitest or jest.

**Deploying is not a test.** Deploying to Cloudflare to see if something works belongs to
release, never to the development loop. `wrangler dev --local` boots miniflare in seconds
with emulated D1 and R2 — **no network, no account, no deploy**.

## Tier 2: how to fake `context.env`

The fake is written **in Go**, with `syscall/js` — no JS files, no fixtures. Set the global
before the code under test reads it.

Put it in `tests/` as a shared helper (e.g. `tests/edge_helpers_test.go`), so every WASM test
reuses the same fake instead of inventing its own:

```go
//go:build wasm

package goflare_test

// promise wraps a value in a resolved JS Promise — every Cloudflare binding is async.
func promise(v js.Value) js.Value {
	return js.Global().Get("Promise").Call("resolve", v)
}

// fakeBucket implements the shape of an R2 binding: put/get/delete returning Promises.
func fakeBucket(store map[string][]byte) js.Value {
	b := js.Global().Get("Object").New()

	b.Set("put", js.FuncOf(func(_ js.Value, args []js.Value) any {
		key := args[0].String()
		buf := make([]byte, args[1].Get("byteLength").Int())
		js.CopyBytesToGo(buf, args[1])   // bytes in, verbatim
		store[key] = buf
		return promise(js.Undefined())
	}))

	b.Set("get", js.FuncOf(func(_ js.Value, args []js.Value) any {
		data, ok := store[args[0].String()]
		if !ok {
			return promise(js.Null())     // R2 returns null for a missing key
		}
		arr := js.Global().Get("Uint8Array").New(len(data))
		js.CopyBytesToJS(arr, data)
		return promise(arr)
	}))

	return b
}

func setupEnv(t *testing.T) map[string][]byte {
	store := map[string][]byte{}
	env := js.Global().Get("Object").New()
	env.Set("FILES", fakeBucket(store))

	ctx := js.Global().Get("Object").New()
	ctx.Set("env", env)
	js.Global().Set("context", ctx)   // exactly what Cloudflare injects

	t.Cleanup(func() { js.Global().Delete("context") })
	return store
}
```

Rules for the fake:

- **It stores bytes, never strings.** A fake that round-trips through `string` would hide the
  very corruption bug we are testing for.
- **It returns Promises**, because the real bindings do. A fake that returns values
  synchronously would let broken promise-handling code pass.
- **It mimics failure too**: a missing key returns `null`, an absent binding is `undefined`.
  Tests must prove we fail loudly on both.

## Tier 3: what belongs in the smoke test (and nothing more)

A fake nobody checks **drifts silently** — green tests, broken production. Tier 3 exists to
catch exactly that, and it is deliberately tiny. Run the **same scenarios** as Tier 2, but
against real miniflare bindings:

- Upload a binary blob to R2 and read it back byte-for-byte.
- One D1 query.
- The generated worker actually boots with the generated config.

If a Tier 3 test fails while its Tier 2 twin passes, **the fake lied** — fix the fake, then
the code.

Do **not** grow Tier 3 into a second test suite. Route matching, RBAC, cookies and body
handling are our logic: they belong in Tier 2.

## Commands

```bash
gotest                                  # never `go test` — dual WASM/stdlib, browser-driven
```

`gotest` compiles for both targets and drives a real browser for the WASM side, so a Tier 2
test needs no extra setup beyond the fake above.
