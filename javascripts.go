package goflare

import (
	"fmt"
	"os"
	"path/filepath"
)

func (g *Goflare) generateWorkerFile() error {
	destPath := filepath.Join(g.config.RelativeOutputDirectory(), g.outputJsFileName)

	// Create output directory if it doesn't exist
	outputDir := filepath.Dir(destPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Read wasm_exec.js content
	wasmExecContent, err := g.tw.GetSSRClientInitJS("", "")
	if err != nil {
		return fmt.Errorf("failed to get wasm_exec content: %w", err)
	}

	// Read and modify runtime.mjs content
	runtimeContent := g.runtimeMjs()

	// Read worker template
	workerTemplate := g.getWorkerMjs()

	// Combine all content: wasm_exec.js + runtime.mjs + worker logic
	combinedContent := string(wasmExecContent) + "\n\n" + string(runtimeContent) + "\n\n" + string(workerTemplate)

	// Write the combined file
	err = os.WriteFile(destPath, []byte(combinedContent), 0644)
	if err != nil {
		return fmt.Errorf("failed to write combined worker file: %w", err)
	}

	return nil
}

// runtimeMjs returns the runtime code,load wasm file for inline use (no ES6 imports)
// This version is for combining into a single _worker.js file
func (g *Goflare) runtimeMjs() string {
	return fmt.Sprintf(`// Runtime functions - inline version
import { connect } from "cloudflare:sockets";
import mod from "%v";

async function loadModule() {
  return mod;
}

function createRuntimeContext({ env, ctx, binding }) {
  return {
    env,
    ctx,
    connect,
    binding,
  };
}`, g.config.OutputWasmFileName)
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
