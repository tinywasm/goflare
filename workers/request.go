//go:build wasm

package workers

import (
	"syscall/js"

	"github.com/tinywasm/fmt"
)

// Request represents an incoming HTTP request to the Worker.
type Request struct {
	Method  string
	URL     string
	Headers map[string]string
	jsReq   js.Value
	body    []byte
	hasBody bool
}

// Body returns the raw request body bytes.
// It reads the body lazily on the first call.
func (r *Request) Body() []byte {
	if r.hasBody {
		return r.body
	}

	ch := make(chan []byte, 1)
	errCh := make(chan string, 1)

	var thenFn, catchFn js.Func

	thenFn = js.FuncOf(func(this js.Value, args []js.Value) any {
		defer thenFn.Release()
		defer catchFn.Release()

		arrayBuffer := args[0]
		byteLength := arrayBuffer.Get("byteLength").Int()
		buf := make([]byte, byteLength)
		ua := js.Global().Get("Uint8Array").New(arrayBuffer)
		js.CopyBytesToGo(buf, ua)
		ch <- buf
		return nil
	})
	catchFn = js.FuncOf(func(this js.Value, args []js.Value) any {
		defer thenFn.Release()
		defer catchFn.Release()
		errCh <- args[0].String()
		return nil
	})

	r.jsReq.Call("arrayBuffer").Call("then", thenFn).Call("catch", catchFn)

	select {
	case b := <-ch:
		r.body = b
		r.hasBody = true
	case msg := <-errCh:
		// We could panic or just return nil, but workers usually don't expect
		// an error from Body() since it's not in the signature.
		// For now we write to stderr and return empty.
		js.Global().Get("console").Call("error", fmt.Errf("workers: failed to read body: %s", msg).Error())
		r.body = []byte{}
		r.hasBody = true
	}

	return r.body
}

// newRequest reads a JS Fetch Request into a Go Request.
func newRequest(jsReq js.Value) (*Request, error) {
	r := &Request{
		Method:  jsReq.Get("method").String(),
		URL:     jsReq.Get("url").String(),
		Headers: map[string]string{},
		jsReq:   jsReq,
	}

	// Read headers
	jsHeaders := jsReq.Get("headers")
	if !jsHeaders.IsNull() && !jsHeaders.IsUndefined() {
		entries := jsHeaders.Call("entries")
		for {
			next := entries.Call("next")
			if next.Get("done").Bool() {
				break
			}
			val := next.Get("value")
			r.Headers[val.Index(0).String()] = val.Index(1).String()
		}
	}

	return r, nil
}
