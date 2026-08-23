import "./wasm_exec.js";
import { createRuntimeContext, loadModule } from "./runtime.mjs";

// Cache of the compiled WebAssembly.Module. Named distinctly from the
// `mod` import injected at bundle scope (import mod from "./edge.wasm"),
// otherwise the single-module bundle redeclares `mod` (esbuild error).
let cachedModule;

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

async function run(ctx) {
  if (cachedModule === undefined) {
    cachedModule = await loadModule();
  }
  const go = new Go();

  let ready;
  const readyPromise = new Promise((resolve) => {
    ready = resolve;
  });
  // Go signals init completion via js.Global().Get("workers").ready() (see
  // workers.Ready). The handshake MUST live on globalThis: the Go binary uses
  // syscall/js, not //go:wasmimport, so a `workers` entry in the wasm importObject
  // is dead — readyPromise would never resolve and every request would hang.
  globalThis.workers = {
    ready: () => {
      ready();
    },
  };
  const instance = new WebAssembly.Instance(cachedModule, go.importObject);
  go.run(instance, ctx);
  await readyPromise;
}

async function fetch(req, env, ctx) {
  const binding = {};
  await run(createRuntimeContext({ env, ctx, binding }));
  const res = await binding.handleRequest(req);
  const out = new Response(res.body, res);
  out.headers.set("x-goflare", "__GOFLARE_VERSION__");
  return out;
}

async function scheduled(event, env, ctx) {
  const binding = {};
  await run(createRuntimeContext({ env, ctx, binding }));
  return binding.runScheduler(event);
}

async function queue(batch, env, ctx) {
  const binding = {};
  await run(createRuntimeContext({ env, ctx, binding }));
  return binding.handleQueueMessageBatch(batch);
}

export default {
  fetch,
  scheduled,
  queue,
};
