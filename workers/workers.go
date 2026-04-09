//go:build wasm

package workers

import "syscall/js"

// Handle registers fn as the single request handler and blocks forever.
// fn is called for every incoming HTTP request to the Worker.
// This must be called from main(); it never returns.
//
// Uses the binding pattern from goflare/assets/worker.mjs:
//   binding.handleRequest is called per request with the JS Request object.
//   binding is accessed via js.Global().Get("context") — injected by wasm_exec.js Proxy.
func Handle(fn func(*Response, *Request)) {
    // Access the runtime context injected by worker.mjs into go.run(instance, ctx).
    // wasm_exec.js patches global with a Proxy: global.context → ctx = {env, ctx, binding}.
    binding := js.Global().Get("context").Get("binding")

    binding.Set("handleRequest", js.FuncOf(func(this js.Value, args []js.Value) any {
        req := args[0]
        return newPromise(func() (js.Value, error) {
            r, err := newRequest(req)
            if err != nil {
                return errorResponse(500, "failed to parse request"), nil
            }
            w := newResponse()
            fn(w, r)
            return w.build(), nil
        })
    }))

    Ready()
    select {}
}

// Ready signals the Workers runtime that Go initialization is complete.
// Called automatically by Handle(). Call manually only if not using Handle().
func Ready() {
    workers := js.Global().Get("workers")
    if !workers.IsNull() && !workers.IsUndefined() {
        workers.Call("ready")
    }
}

// newPromise wraps a blocking Go func in a JS Promise.
func newPromise(fn func() (js.Value, error)) js.Value {
    executor := js.FuncOf(func(this js.Value, args []js.Value) any {
        resolve, reject := args[0], args[1]
        go func() {
            result, err := fn()
            if err != nil {
                reject.Invoke(js.ValueOf(err.Error()))
                return
            }
            resolve.Invoke(result)
        }()
        return nil
    })
    return js.Global().Get("Promise").New(executor)
}

// errorResponse builds a minimal JS Response for internal errors.
func errorResponse(status int, msg string) js.Value {
    h := js.Global().Get("Headers").New()
    h.Call("set", "Content-Type", "text/plain")
    init := js.Global().Get("Object").New()
    init.Set("status", status)
    init.Set("headers", h)
    return js.Global().Get("Response").New(js.ValueOf(msg), init)
}
