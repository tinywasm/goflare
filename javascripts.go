package goflare

import (
	"fmt"
	"os"
	"path/filepath"
)

func (g *Goflare) generateWorkerFile() error {
	workerJsPath := filepath.Join(g.Config.OutputDir, "worker.js")
	wasmExecPath := filepath.Join(g.Config.OutputDir, "wasm_exec.js")

	// Read wasm_exec.js content
	wasmExecContent, err := g.tw.GetSSRClientInitJS("", "")
	if err != nil {
		return fmt.Errorf("failed to get wasm_exec content: %w", err)
	}

	if err := os.WriteFile(wasmExecPath, []byte(wasmExecContent), 0644); err != nil {
		return fmt.Errorf("failed to write wasm_exec.js: %w", err)
	}

	// Read worker template
	workerTemplate := g.getWorkerMjs()

	if err := os.WriteFile(workerJsPath, []byte(workerTemplate), 0644); err != nil {
		return fmt.Errorf("failed to write worker.js: %w", err)
	}

	return nil
}

func (g *Goflare) getWorkerMjs() string {
	return `// Worker logic
import "./wasm_exec.js";
import mod from "./worker.wasm";

let instance;
let go;

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

async function init() {
  if (!instance) {
    go = new Go();
    let ready;
    const readyPromise = new Promise((resolve) => {
      ready = resolve;
    });
    instance = await WebAssembly.instantiate(mod, {
      ...go.importObject,
      workers: {
        ready: () => {
          ready();
        },
      },
    });
    // Start the Go runtime. It will call workers.ready() when initialized.
    go.run(instance);
    await readyPromise;
  }
}

async function fetch(req, env, ctx) {
  await init();

  const binding = {};
  // The Go side is expected to have registered a handler that we can call.
  // We pass the request-specific context (req, env, ctx, binding) to it.
  if (globalThis.goflare && globalThis.goflare.handleRequest) {
      return globalThis.goflare.handleRequest(req, env, ctx, binding);
  }

  // Fallback for older/different implementations
  if (binding.handleRequest) {
      return binding.handleRequest(req);
  }

  return new Response("Go WASM handler not found", { status: 500 });
}

export default {
  fetch,
};`
}
