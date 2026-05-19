//go:build wasm

package main

import (
	"github.com/tinywasm/goflare/pages"
	"github.com/tinywasm/goflare/router"
)

func main() {
	r := pages.NewRouter()

	r.Get("/api/hello", func(ctx router.Context) {
		ctx.Write([]byte("Hello from Go Pages Functions!"))
	})

	pages.Serve(r)
}
