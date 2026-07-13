//go:build wasm

package r2

import (
	"syscall/js"

	"github.com/tinywasm/fmt"
	"github.com/tinywasm/jsvalue"
)

type Bucket struct {
	obj js.Value
}

// NewEdge obtains the bucket from the binding declared in wrangler (e.g. "FILES").
func NewEdge(binding string) (*Bucket, error) {
	v := js.Global().Get("context").Get("env").Get(binding)
	if v.IsUndefined() || v.IsNull() {
		return nil, fmt.Errf("r2: bucket %s not found", binding)
	}
	return &Bucket{obj: v}, nil
}

func (b *Bucket) Put(key string, data []byte, contentType string) error {
	ua := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(ua, data)

	opts := js.Global().Get("Object").New()
	if contentType != "" {
		httpMetadata := js.Global().Get("Object").New()
		httpMetadata.Set("contentType", contentType)
		opts.Set("httpMetadata", httpMetadata)
	}

	_, err := jsvalue.AwaitPromise(b.obj.Call("put", key, ua, opts))
	return err
}

func (b *Bucket) Get(key string) ([]byte, string, error) {
	obj, err := jsvalue.AwaitPromise(b.obj.Call("get", key))
	if err != nil {
		return nil, "", err
	}
	if obj.IsNull() || obj.IsUndefined() {
		return nil, "", fmt.Err("r2: object not found")
	}

	// obj is an R2Object. To get the data, we need to read from the body stream.
	// However, usually in Workers R2 get() returns an R2Object with a .body (ReadableStream).
	// But sometimes it returns R2ObjectBody which HAS the body.

	jsBody := obj.Get("body")
	if jsBody.IsNull() || jsBody.IsUndefined() {
		// Maybe it was just a metadata head() call, but we called get()
		return nil, "", fmt.Err("r2: object has no body")
	}

	// We need to read the entire stream. Easiest way in this environment:
	// new Response(body).arrayBuffer()
	resp := js.Global().Get("Response").New(jsBody)
	arrayBuffer, err := jsvalue.AwaitPromise(resp.Call("arrayBuffer"))
	if err != nil {
		return nil, "", fmt.Errf("r2: read body: %s", err.Error())
	}

	byteLength := arrayBuffer.Get("byteLength").Int()
	buf := make([]byte, byteLength)
	ua := js.Global().Get("Uint8Array").New(arrayBuffer)
	js.CopyBytesToGo(buf, ua)

	contentType := ""
	httpMetadata := obj.Get("httpMetadata")
	if !httpMetadata.IsUndefined() && !httpMetadata.IsNull() {
		ct := httpMetadata.Get("contentType")
		if !ct.IsUndefined() && !ct.IsNull() {
			contentType = ct.String()
		}
	}

	return buf, contentType, nil
}

func (b *Bucket) Delete(key string) error {
	_, err := jsvalue.AwaitPromise(b.obj.Call("delete", key))
	return err
}

type ObjectInfo struct {
	Key         string
	Size        int64
	ContentType string
}

func (b *Bucket) List(prefix string) ([]ObjectInfo, error) {
	opts := js.Global().Get("Object").New()
	if prefix != "" {
		opts.Set("prefix", prefix)
	}

	res, err := jsvalue.AwaitPromise(b.obj.Call("list", opts))
	if err != nil {
		return nil, err
	}

	jsObjects := res.Get("objects")
	n := jsObjects.Length()
	out := make([]ObjectInfo, n)
	for i := 0; i < n; i++ {
		o := jsObjects.Index(i)
		info := ObjectInfo{
			Key:  o.Get("key").String(),
			Size: int64(o.Get("size").Float()),
		}
		httpMetadata := o.Get("httpMetadata")
		if !httpMetadata.IsUndefined() && !httpMetadata.IsNull() {
			ct := httpMetadata.Get("contentType")
			if !ct.IsUndefined() && !ct.IsNull() {
				info.ContentType = ct.String()
			}
		}
		out[i] = info
	}

	return out, nil
}
