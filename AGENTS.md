# Agent Guide — `tinywasm/goflare`

Constraints for agents working on this library. Read this before any change.

---

## Scope — single responsibility

`goflare` is a **deployment runtime for Cloudflare**: it builds a Go project into the edge
artifacts Cloudflare expects, deploys them, and provides the **edge runtime** the deployed
code runs on.

- It **implements** the routing contract `github.com/tinywasm/router` — it does **not** define
  it. Anything shaped like a `Context`, `Router` or `HandlerFunc` belongs in `tinywasm/router`,
  never here.
- The native (`!wasm`) dev server is **not** ours: it is `github.com/tinywasm/server/httpd`,
  the ecosystem's only native implementor. Do not write a second one.
- Business logic never lives here.

## Two build targets — know which one you are in

This is the single most important thing to get right in this repo.

| Target | Packages | Runs where | Rules |
|---|---|---|---|
| **`wasm`** | `edge/`, `workers/`, `d1/`, `r2/`, `cloudflare/env_wasm.go` | **Inside the Cloudflare Worker** | **No standard library.** Use `tinywasm/fmt` instead of `errors`/`fmt`/`strconv`/`strings`. Talks to the runtime through `syscall/js`. |
| **`!wasm`** | `build.go`, `mode.go`, `config.go`, `cloudflare.go`, `devserver/`, `cmd/` | The developer's machine / CI | **The standard library is correct and expected here** — `net/http`, `go/parser`, `os`, `strings`. |

> ⚠️ **Anti-footgun.** The ecosystem rule "no stdlib in WASM code" applies **only to the `wasm`
> column**. Do not "fix" stdlib imports in host tooling — that code never reaches a browser or
> a Worker, and purging it breaks the build for no reason.

## Testing

**Read [docs/TESTING.md](docs/TESTING.md) before writing a single test.** It defines the three
tiers and the rule for choosing one.

**Tests live in `tests/`**, as `package goflare_test` — black-box, through the public API. The
only exception is a test that must reach an **unexported** symbol: it goes next to the code as
`*_internal_test.go` (see `build_internal_test.go`).

The short version: our edge code talks to `js.Global()`, not to Cloudflare — so we inject a
fake `context.env` and test the real code path in a browser. **Deploying is not a test**, and
`wrangler` is reserved for a tiny smoke tier that proves the fake does not lie.

```bash
go install github.com/tinywasm/devflow/cmd/gotest@latest
gotest        # never `go test` — dual WASM/stdlib, browser-driven
```

Stdlib assertions only. Publish with `gopush 'message'`.

## Fail loudly, never silently

A misconfiguration must break the build or the request — never degrade into a wrong result.
This repo has been bitten by exactly that: a body decoded as text corrupted binaries with no
error, and an unrecognized import silently produced the wrong deploy artifact. When in doubt,
return an error that **names what was missing**.

## Documentation First

Update docs **before** code and before `gopush`: keep `docs/PLAN.md` (the master index) and
`docs/ARCHITECTURE.md` in sync, and re-index `README.md` so every `docs/` file is linked.
Diagrams use `flowchart TD`, no `subgraph`, `<br/>` for line breaks.
