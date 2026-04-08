//go:build wasm

package workers

import (
    "fmt"
    "syscall/js"
)

// Request represents an incoming HTTP request to the Worker.
type Request struct {
    Method  string
    URL     string
    Headers map[string]string
    body    []byte
}

// Body returns the raw request body bytes.
func (r *Request) Body() []byte { return r.body }

// newRequest reads a JS Fetch Request into a Go Request.
// Blocks until the body promise resolves.
func newRequest(jsReq js.Value) (*Request, error) {
    r := &Request{
        Method:  jsReq.Get("method").String(),
        URL:     jsReq.Get("url").String(),
        Headers: map[string]string{},
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

    // Read body — blocks via channel + promise chaining
    body, err := readBodyText(jsReq)
    if err != nil {
        return nil, fmt.Errorf("workers: read body: %w", err)
    }
    r.body = []byte(body)

    return r, nil
}

// readBodyText resolves req.text() via a blocking channel.
// js.FuncOf callbacks are released after the promise settles to avoid leaks.
func readBodyText(jsReq js.Value) (string, error) {
    ch := make(chan string, 1)
    errCh := make(chan string, 1)

    var thenFn, catchFn js.Func

    thenFn = js.FuncOf(func(this js.Value, args []js.Value) any {
        ch <- args[0].String()
        thenFn.Release()
        catchFn.Release()
        return nil
    })
    catchFn = js.FuncOf(func(this js.Value, args []js.Value) any {
        errCh <- args[0].String()
        thenFn.Release()
        catchFn.Release()
        return nil
    })

    jsReq.Call("text").Call("then", thenFn).Call("catch", catchFn)

    select {
    case text := <-ch:
        return text, nil
    case msg := <-errCh:
        return "", fmt.Errorf("%s", msg)
    }
}
