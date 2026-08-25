---
PLAN: "fix!: start the Go instance once per isolate instead of once per request"
EXECUTOR: jules
REVIEWER: none
---

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.

# Plan — `tinywasm/goflare`: one Go instance per isolate, not one per request

## The problem, in one line

`assets/worker.mjs` builds a **fresh Go/WASM instance on every HTTP request** and
coordinates its startup through a **shared global**, so concurrent requests
resolve each other's startup promises and hang — while every request pays a full
WebAssembly instantiation plus a complete `main()` re-run.

## Production evidence

`veltylabs/iam`, deployed on `iam.velty.cl`, answered **every** request with
Cloudflare error 1101. The server-side log (Workers Observability) shows it is
not a thrown exception — the runtime **cancels a hung request**:

```
The Workers runtime canceled this request because it detected that your
Worker's code had hung and would never generate a response.
```

Two details from those logs pin the cause:

- `wallTimeMs` equals `cpuTimeMs` (1–7 ms) on the canceled requests. If the
  Worker were stuck awaiting I/O, wall time would far exceed CPU time. Equal
  values mean the isolate reached a state with **no pending I/O and an
  unresolved promise** — a deadlock, detected immediately.
- **No** `console.log` output from the consumer's own `main()` appears anywhere
  in the window (`iam`'s `edge/main.go` prints on every early-return path).
  `main()` did not fail; it ran and then the startup handshake went to the wrong
  place.

This reproduces for **any** goflare Worker under concurrent traffic. The failing
window shows ordinary requests (`/`, `/favicon.ico`, `/robots.txt`) arriving
~10 ms apart — a browser loading a page, not a contrived stress test.

## Root cause

Three files, read together.

**`assets/worker.mjs`** — `run()` is called from `fetch()` on *every* invocation:

```js
let cachedModule;

async function run(ctx) {
  if (cachedModule === undefined) {
    cachedModule = await loadModule();
  }
  const go = new Go();

  let ready;
  const readyPromise = new Promise((resolve) => { ready = resolve; });
  globalThis.workers = {                    // ← ONE slot, shared by the isolate
    ready: () => { ready(); },
  };
  const instance = new WebAssembly.Instance(cachedModule, go.importObject);
  go.run(instance, ctx);
  await readyPromise;                       // ← never resolves if overwritten
}

async function fetch(req, env, ctx) {
  const binding = {};                       // ← fresh binding ⇒ fresh instance
  await run(createRuntimeContext({ env, ctx, binding }));
  const res = await binding.handleRequest(req);
  ...
}
```

Only `cachedModule` (the compiled `WebAssembly.Module`) is reused. The
**instance**, the `Go` shim, and the whole of `main()` are rebuilt per request.

**`assets/wasm_exec_worker.js`** — each instance gets a Proxy that isolates
exactly one property name:

```js
const globalProxy = new Proxy(global, {
	get(target, prop) {
		if (prop === 'context') {
			return context;            // per instance, closed over THIS run()'s ctx
		}
		return Reflect.get(...arguments);   // the REAL, shared global
	}
})
```

**`workers/workers.go`** — `Ready()` does not use that door:

```go
func Ready() {
	workers := js.Global().Get("workers")   // ← falls through to the shared global
	if !workers.IsNull() && !workers.IsUndefined() {
		workers.Call("ready")
	}
}
```

`Handle()`, in the same file, *does* use it correctly
(`js.Global().Get("context").Get("binding")`), which is why `handleRequest`
never collides between requests. `ready` is the one signal that never migrated.

### The race, step by step

Startup is not instantaneous: any consumer that awaits during init — `iam` runs
a D1 schema sync in `main()` — yields to the JS event loop. That yield is the
window:

1. Request **A**: `run()` sets `globalThis.workers = {ready: readyA}`, starts its Go instance.
2. A's `main()` awaits its first D1 call → the Go scheduler blocks → control returns to the JS event loop, with A parked on `await readyPromise`.
3. Request **B** arrives and calls `run()`: **overwrites** `globalThis.workers = {ready: readyB}`.
4. A's D1 call resolves, A's `main()` continues, reaches `Handle()` → `Ready()` → reads the *current* shared slot → invokes **readyB**.
5. A's own `readyPromise` never resolves. Nothing is pending. The runtime cancels A with the message above.

## The fix, and why it is one change and not two

Starting **once per isolate** removes the race *by construction* — with a single
instance there is exactly one handshake, so there is nothing to overwrite — and
simultaneously removes the per-request cost. Routing `ready` through the
instance-scoped `binding` (Stage 2) closes the door on the shared global so the
bug cannot silently return.

What this buys per request, after the first: no `WebAssembly.Instance`
construction, no fresh linear memory, and no `main()` re-run — which for `iam`
means its D1 schema sync stops firing on **every single HTTP request**.

`env` is safe to capture once: in Cloudflare Workers the bindings object is
identical for every request in an isolate. The per-request `ExecutionContext`
(`ctx`) is **not**, which is why Stage 3 removes it from the runtime context
rather than capturing a value that would be permanently stale.

## Stage 1 — start once per isolate (`assets/worker.mjs`)

Replace the whole file with:

```js
import "./wasm_exec.js";
import { createRuntimeContext, loadModule } from "./runtime.mjs";

// The startup promise for this isolate's single Go instance. This variable IS the
// concurrency contract: ensureStarted assigns it synchronously, before any await,
// so N concurrent requests all await the SAME startup instead of each building
// their own instance and racing to signal it.
let started;

// The one object the Go instance registers its handlers on (handleRequest, and
// the ready signal below). Module scope, not per request: the instance that
// registers handleRequest outlives every individual request.
const binding = {};

globalThis.tryCatch = (fn) => {
  try {
    return {
      result: fn(),
    };
  } catch (e) {
    return {
      error: e,
    };
  }
};

// start boots the Go instance exactly once. `env` is captured here because in
// Workers the bindings object is identical for every request in an isolate.
// Anything that genuinely varies per request must travel through
// binding.handleRequest instead — never through this context.
async function start(env) {
  const wasmModule = await loadModule();
  const go = new Go();

  let ready;
  const readyPromise = new Promise((resolve) => {
    ready = resolve;
  });
  // Go signals init completion via
  // js.Global().Get("context").Get("binding").Get("ready") — see workers.Ready.
  // This MUST ride ctx.binding and never globalThis: wasm_exec_worker.js gives
  // each instance a Proxy that resolves ONLY the "context" property per instance,
  // so any other global name is the single object shared by the whole isolate.
  binding.ready = () => {
    ready();
  };

  const instance = new WebAssembly.Instance(wasmModule, go.importObject);
  go.run(instance, createRuntimeContext({ env, binding }));
  await readyPromise;
}

// ensureStarted is deliberately NOT async: the check-and-assign must complete
// without yielding to the event loop, or two concurrent requests could both see
// `started === undefined` and start two instances.
function ensureStarted(env) {
  if (started === undefined) {
    started = start(env);
  }
  return started;
}

async function fetch(req, env, ctx) {
  await ensureStarted(env);
  const res = await binding.handleRequest(req);
  const out = new Response(res.body, res);
  out.headers.set("x-goflare", "__GOFLARE_VERSION__");
  return out;
}

async function scheduled(event, env, ctx) {
  await ensureStarted(env);
  return binding.runScheduler(event);
}

async function queue(batch, env, ctx) {
  await ensureStarted(env);
  return binding.handleQueueMessageBatch(batch);
}

export default {
  fetch,
  scheduled,
  queue,
};
```

Notes for the executor:

- `cachedModule` is gone: `start` runs once, so `loadModule()` is called once.
- Name the local `wasmModule`, not `mod`. `bundleJS` (in `javascripts.go`)
  prepends `import mod from "./edge.wasm";` at bundle scope, and reusing that
  name invites confusion even where shadowing would be legal.
- Do **not** keep a `globalThis.workers` fallback "for compatibility". A
  handshake that sometimes takes the safe path and sometimes the racy one would
  pass casual testing and still hang under real concurrency — strictly worse
  than either alone.
- `runScheduler` / `handleQueueMessageBatch` are left exactly as they are. No Go
  code in this repo registers them today; that is pre-existing and out of scope.

## Stage 2 — read `ready` from the instance binding (`workers/workers.go`)

Replace `Ready()`:

```go
// Ready signals the Workers runtime that Go initialization is complete.
// Called automatically by Handle(). Call manually only if not using Handle().
//
// It reads the callback from context.binding — the same instance-scoped door
// Handle() uses — never from a bare global. js.Global() is a Proxy
// (assets/wasm_exec_worker.js) that resolves ONLY the "context" property per wasm
// instance; every other name resolves to the single object shared by the whole
// isolate, where a second request would silently clobber the signal. See
// assets/worker.mjs's start() for the JS side of this contract.
func Ready() {
	ready := js.Global().Get("context").Get("binding").Get("ready")
	if !ready.IsNull() && !ready.IsUndefined() {
		ready.Invoke()
	}
}
```

`Handle()` in the same file needs no change — it already reads `binding` this
way. Update only its doc comment where it says the instance is per request, if
such wording exists.

## Stage 3 — drop the per-request `ctx` from the runtime context (`assets/runtime.mjs`)

```js
import { connect } from "cloudflare:sockets";
import mod from "./worker.wasm";

export async function loadModule() {
  return mod;
}

// The context handed to the isolate's single Go instance. It carries only
// isolate-stable values. The per-request ExecutionContext is deliberately absent:
// with one instance serving many requests, a captured ExecutionContext would
// belong to whichever request happened to start the isolate and would be stale —
// and therefore unsafe — for every request after it.
export function createRuntimeContext({ env, binding }) {
  return {
    env,
    connect,
    binding,
  };
}
```

No Go code in this repo reads `context.ctx` — verify with
`grep -rn 'Get("context")' --include="*.go" .`, which must show only `env` and
`binding` accesses (`d1/adapter.go`, `r2/bucket.go`, `cloudflare/env_wasm.go`,
`workers/workers.go`).

## Stage 4 — tests (ALREADY WRITTEN, currently RED — do not modify)

Two tests are already committed and fail against the current code. Stage 1–3
must make both pass. **Do not edit them to fit the implementation.**

`tests/ready_handshake_test.go` (`//go:build wasm`) —
`TestReady_SignalsItsOwnInstanceNotASharedGlobal` builds an instance-scoped
`context.binding.ready` and a decoy `globalThis.workers.ready`, calls
`workers.Ready()`, and asserts the decoy is never invoked. Current output:

```
ready_handshake_test.go:73: Ready() signalled through the shared globalThis.workers slot: it resolved a concurrent request's promise instead of its own, leaving this instance's request hung until the Workers runtime cancels it
ready_handshake_test.go:78: Ready() did not signal its own instance's binding (context.binding.ready): this instance's request never resolves
```

`tests/worker_runtime_test.go` (`//go:build !wasm`) —
`TestWorkerRuntime_StartsOncePerIsolate` reads `assets/worker.mjs` and asserts
the shipped source neither routes the handshake through `globalThis.workers` nor
mints a `binding` inside `fetch()`. Current output:

```
worker_runtime_test.go:58: worker.mjs still routes the readiness handshake through globalThis.workers: ...
worker_runtime_test.go:75: fetch() mints a new binding per request: ...
```

That second test asserts on JS source text rather than behaviour, on purpose:
this repo ships no Node and no Wrangler (see "Anti-footguns"), so there is no
harness able to drive two overlapping `worker.mjs` invocations. Do **not** add
one, and do **not** fabricate a "concurrency test" that cannot actually
interleave two real instances — `tests/edge_conformance_test.go`'s own header
documents how a fake that cannot fail already burned this codebase once.

## Stage 5 — documentation

`docs/BUILD_WORKER_ASSETS.md` — add a section after "Verificación posterior al
despliegue":

```markdown
## Ciclo de vida del Worker

La instancia de Go se crea **una vez por isolate**, no una vez por petición: la
primera petición paga la instanciación del WASM y el `main()` completo, y todas
las siguientes reutilizan esa instancia a través de `binding.handleRequest`.

> *`main()` se ejecuta una sola vez. Todo lo que haga — sincronizar el esquema,
> leer secretos, construir el router — es coste de arranque del isolate, no de
> cada petición.*

El handshake de arranque viaja por `context.binding.ready`, nunca por una global
compartida: `wasm_exec_worker.js` sólo aísla la propiedad `context` por
instancia, así que cualquier otro nombre global es un único objeto compartido por
todo el isolate.
```

`docs/ARCHITECTURE.md` — the "Core Modules" list has no entry for the JS runtime
glue. Add one describing `assets/worker.mjs`, `assets/runtime.mjs` and
`workers/workers.go` as the edge runtime, stating the once-per-isolate lifecycle.

## Anti-footguns

- **This repo is Go backend tooling.** It legitimately uses the standard library
  (`os`, `net/http`, `strings`, …). Do **not** "fix" those imports — the
  no-stdlib rule applies to WASM-compiled packages, and the only WASM-tagged
  code here is `edge/`, `workers/`, `d1/`, `r2/`, `cloudflare/env_wasm.go`.
- **No Node, no Wrangler, no npm, no bundler.** `assets/*.js` and `assets/*.mjs`
  are shipped verbatim: `javascripts.go` embeds them with `//go:embed`, strips
  imports/exports, concatenates and minifies in Go. Do not add a build step, a
  `package.json`, or a dependency.
- **`assets/wasm_exec_worker.js` is vendored from the Go/TinyGo toolchain.** Do
  not reformat or "clean" it. This plan does not change it.

## Code quality rules

- No hardcoded repeated strings in Go logic — named constants only.
- `cmd/` stays thin: argument parsing, dependency injection, print/exit. All
  logic lives in the library. This plan touches neither.

## Acceptance criteria

- [ ] `go build ./...` and `go vet ./...` clean.
- [ ] `go test ./tests/...` — full non-WASM suite green, including
      `TestWorkerRuntime_StartsOncePerIsolate`.
- [ ] `GOOS=js GOARCH=wasm go test -exec wasmbrowsertest ./tests/...` — green,
      including `TestReady_SignalsItsOwnInstanceNotASharedGlobal`.
- [ ] `grep -rn "globalThis.workers" .` → empty.
- [ ] `grep -n 'Get("workers")' workers/workers.go` → empty.
- [ ] `grep -n "const binding" assets/worker.mjs` → exactly one match, at module
      scope, outside every function.
- [ ] `grep -n "cachedModule" assets/worker.mjs` → empty.
- [ ] `docs/BUILD_WORKER_ASSETS.md` documents the once-per-isolate lifecycle.

| Stage | File(s) | Done when |
|---|---|---|
| 1 | `assets/worker.mjs` | One instance per isolate via a module-scope `started` memo; module-scope `binding`; `ready` on `binding` |
| 2 | `workers/workers.go` | `Ready()` reads `context.binding.ready`, not `js.Global().Get("workers")` |
| 3 | `assets/runtime.mjs` | `createRuntimeContext({env, binding})` — no per-request `ctx` |
| 4 | `tests/` (already written) | Both red tests pass, unmodified |
| 5 | `docs/BUILD_WORKER_ASSETS.md`, `docs/ARCHITECTURE.md` | Once-per-isolate lifecycle documented |
