//go:build !wasm

package goflare_test

import (
	"os"
	"strings"
	"testing"
)

// workerMJS is the source that javascripts.go embeds with //go:embed and bundles
// verbatim into every deployed edge.js. Asserting on the source rather than on
// the built bundle is deliberate: bundleJS minifies, and a minified body cannot
// be read for the structural properties below.
const workerMJS = "../assets/worker.mjs"

// TestWorkerRuntime_StartsOncePerIsolate pins the two properties that keep the
// Worker correct AND cheap under concurrent traffic. Both were violated by the
// code that hung veltylabs/iam in production (Cloudflare error 1101, log:
// "the Workers runtime canceled this request because it detected that your
// Worker's code had hung and would never generate a response").
//
// The old shape started a fresh Go instance per request:
//
//	async function run(ctx) {
//	  const go = new Go();
//	  let ready;
//	  const readyPromise = new Promise((resolve) => { ready = resolve; });
//	  globalThis.workers = { ready: () => { ready(); } };   // ← shared slot
//	  const instance = new WebAssembly.Instance(cachedModule, go.importObject);
//	  go.run(instance, ctx);
//	  await readyPromise;
//	}
//	async function fetch(req, env, ctx) {
//	  const binding = {};
//	  await run(createRuntimeContext({ env, ctx, binding }));   // ← every request
//	  ...
//	}
//
// That costs a full WebAssembly instantiation plus a complete main() — schema
// sync, config parse, the lot — on every single request, and it reassigns one
// shared globalThis slot each time, so overlapping requests resolve each other's
// startup promises. Starting once per isolate removes both at once: the cost is
// paid on the first request only, and with a single instance there is only ever
// one handshake to get wrong.
func TestWorkerRuntime_StartsOncePerIsolate(t *testing.T) {
	src, err := os.ReadFile(workerMJS)
	if err != nil {
		t.Fatalf("read %s: %v", workerMJS, err)
	}
	js := string(src)

	// 1. The readiness handshake must not travel through a shared global.
	//    wasm_exec_worker.js's Proxy resolves only "context" per instance; every
	//    other global name — globalThis.workers included — is the one real object
	//    shared by every request in the isolate.
	if strings.Contains(js, "globalThis.workers") {
		t.Error("worker.mjs still routes the readiness handshake through globalThis.workers: " +
			"that slot is shared by every request in the isolate, so one request's Ready() " +
			"resolves another's startup promise and the first hangs until the runtime cancels it")
	}

	// 2. Startup must be memoized at module scope, so N concurrent requests await
	//    ONE instance instead of each building their own.
	fetchBody, ok := functionBody(js, "async function fetch(")
	if !ok {
		t.Fatal("could not locate the fetch entry point in worker.mjs")
	}

	// A binding minted inside fetch is the signature of the per-request design:
	// binding is what a Go instance registers handleRequest on, so a fresh one per
	// request means a fresh instance per request. Reusing one instance means the
	// binding it registered on outlives the request that created it.
	if strings.Contains(fetchBody, "const binding") {
		t.Error("fetch() mints a new binding per request: that binding is what a Go instance " +
			"registers handleRequest on, so a fresh one forces a fresh instance — a full wasm " +
			"instantiation plus a complete main() re-run (schema sync included) on every request")
	}
	for _, perRequest := range []string{"new Go()", "new WebAssembly.Instance"} {
		if strings.Contains(fetchBody, perRequest) {
			t.Errorf("fetch() constructs %s per request: a Worker must instantiate once per "+
				"isolate and reuse it", perRequest)
		}
	}
}

// functionBody returns the source between the brace that opens the declaration
// starting at marker and its matching close. It is brace-counting only — enough
// for the hand-written, string-literal-free entry points in worker.mjs.
func functionBody(src, marker string) (string, bool) {
	i := strings.Index(src, marker)
	if i < 0 {
		return "", false
	}
	open := strings.Index(src[i:], "{")
	if open < 0 {
		return "", false
	}
	open += i

	depth := 0
	for j := open; j < len(src); j++ {
		switch src[j] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return src[open+1 : j], true
			}
		}
	}
	return "", false
}
