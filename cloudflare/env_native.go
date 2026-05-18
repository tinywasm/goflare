//go:build !wasm

package cloudflare

import (
	"os"
)

// Env returns the value of an environment variable or secret.
func Env(key string) string {
	return os.Getenv(key)
}

// EnvOr returns the value of an environment variable or secret, or a fallback.
func EnvOr(key, fallback string) string {
	if val, ok := Lookup(key); ok {
		return val
	}
	return fallback
}

// Lookup returns the value of an environment variable or secret and a boolean indicating if it was found.
func Lookup(key string) (string, bool) {
	return os.LookupEnv(key)
}
