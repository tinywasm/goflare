//go:build wasm

package goflare_test

import (
	"syscall/js"
	"testing"
)

// promise wraps a value in a resolved JS Promise — every Cloudflare binding is async.
func promise(v js.Value) js.Value {
	return js.Global().Get("Promise").Call("resolve", v)
}

// fakeBucket implements the shape of an R2 binding: put/get/delete returning Promises.
func fakeBucket(store map[string][]byte, contentTypes map[string]string) js.Value {
	b := js.Global().Get("Object").New()

	b.Set("put", js.FuncOf(func(_ js.Value, args []js.Value) any {
		key := args[0].String()
		ua := args[1]
		buf := make([]byte, ua.Get("byteLength").Int())
		js.CopyBytesToGo(buf, ua) // bytes in, verbatim
		store[key] = buf

		if len(args) > 2 {
			opts := args[2]
			httpMetadata := opts.Get("httpMetadata")
			if !httpMetadata.IsUndefined() && !httpMetadata.IsNull() {
				ct := httpMetadata.Get("contentType")
				if !ct.IsUndefined() && !ct.IsNull() {
					contentTypes[key] = ct.String()
				}
			}
		}
		return promise(js.Undefined())
	}))

	b.Set("get", js.FuncOf(func(_ js.Value, args []js.Value) any {
		key := args[0].String()
		data, ok := store[key]
		if !ok {
			return promise(js.Null()) // R2 returns null for a missing key
		}

		// R2 get returns an R2Object. For simplicity in tests, we'll return
		// an object that has a .body which is a ReadableStream (or something that Response can handle)
		// and .httpMetadata.

		obj := js.Global().Get("Object").New()

		ua := js.Global().Get("Uint8Array").New(len(data))
		js.CopyBytesToJS(ua, data)

		// In our R2 implementation, we use new Response(obj.body).arrayBuffer()
		// So obj.body can be a Uint8Array and Response will handle it.
		obj.Set("body", ua)

		httpMetadata := js.Global().Get("Object").New()
		if ct, ok := contentTypes[key]; ok {
			httpMetadata.Set("contentType", ct)
		}
		obj.Set("httpMetadata", httpMetadata)

		return promise(obj)
	}))

	b.Set("delete", js.FuncOf(func(_ js.Value, args []js.Value) any {
		key := args[0].String()
		delete(store, key)
		delete(contentTypes, key)
		return promise(js.Undefined())
	}))

	return b
}

func setupEnv(t *testing.T) map[string][]byte {
	store := map[string][]byte{}
	contentTypes := map[string]string{}
	env := js.Global().Get("Object").New()
	env.Set("FILES", fakeBucket(store, contentTypes))

	ctx := js.Global().Get("Object").New()
	ctx.Set("env", env)
	js.Global().Set("context", ctx) // exactly what Cloudflare injects

	t.Cleanup(func() { js.Global().Delete("context") })
	return store
}
