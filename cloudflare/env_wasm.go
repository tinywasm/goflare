//go:build wasm

package cloudflare

import (
	"syscall/js"
)

// Env returns the value of an environment variable or secret.
func Env(key string) string {
	val, _ := Lookup(key)
	return val
}

// EnvOr returns the value of an environment variable or secret, or a fallback.
func EnvOr(key, fallback string) string {
	if val, ok := Lookup(key); ok {
		return val
	}
	return fallback
}

// Lookup returns the value of an environment variable or secret and a boolean indicating if it was found.
// Reads from runtimeContext.env via syscall/js.
func Lookup(key string) (string, bool) {
	// runtimeContext is injected into the global object by wasm_exec.js / worker.mjs
	// global.context = { env, ctx, binding }
	jsEnv := js.Global().Get("context").Get("env")
	if jsEnv.IsNull() || jsEnv.IsUndefined() {
		return "", false
	}

	val := jsEnv.Get(key)
	if val.IsNull() || val.IsUndefined() {
		return "", false
	}

	return val.String(), true
}
