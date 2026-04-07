package goflare

import (
	"fmt"
	"os"
	"path/filepath"
)

func (g *Goflare) generateWorkerFile() error {
	// Worker files are: worker.js, wasm_exec.js, worker.wasm
	// wasm.go handles worker.wasm

	jsPath := filepath.Join(g.Config.OutputDir, "worker.js")
	execPath := filepath.Join(g.Config.OutputDir, "wasm_exec.js")

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(g.Config.OutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Read wasm_exec.js content
	wasmExecContent, err := g.tw.GetSSRClientInitJS("", "")
	if err != nil {
		return fmt.Errorf("failed to get wasm_exec content: %w", err)
	}
	if err := os.WriteFile(execPath, []byte(wasmExecContent), 0644); err != nil {
		return fmt.Errorf("failed to write wasm_exec.js: %w", err)
	}

	// Read worker template
	workerTemplate := g.getWorkerMjs()

	// Modify template to import wasm_exec.js and worker.wasm
	// In Stage 05, we'll see that for Workers, we might need a specific structure
	// but for now, we follow Stage 03 requirements.

	header := `import "./wasm_exec.js";
import mod from "./worker.wasm";

async function loadModule() {
  return mod;
}

function createRuntimeContext({ env, ctx, binding }) {
  return {
    env,
    ctx,
    binding,
  };
}
`

	combinedContent := header + "\n\n" + string(workerTemplate)

	// Write the combined file
	err = os.WriteFile(jsPath, []byte(combinedContent), 0644)
	if err != nil {
		return fmt.Errorf("failed to write worker.js file: %w", err)
	}

	return nil
}

func (g *Goflare) getWorkerMjs() string {
	return `// Worker logic
let mod;

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
  if (mod === undefined) {
    mod = await loadModule();
  }
  const go = new Go();

  let ready;
  const readyPromise = new Promise((resolve) => {
    ready = resolve;
  });
  const instance = new WebAssembly.Instance(mod, {
    ...go.importObject,
    workers: {
      ready: () => {
        ready();
      },
    },
  });
  go.run(instance, ctx);
  await readyPromise;
}

async function fetch(req, env, ctx) {
  const binding = {};
  await run(createRuntimeContext({ env, ctx, binding }));
  return binding.handleRequest(req);
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

// onRequest handles request to Cloudflare Pages
async function onRequest(ctx) {
  const binding = {};
  const { request, env } = ctx;
  await run(createRuntimeContext({ env, ctx, binding }));
  return binding.handleRequest(request);
}

export default {
  fetch,
  scheduled,
  queue,
  onRequest,
};`
}
