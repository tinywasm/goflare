package router

// Context es la abstracción mínima que ve un handler.
// Misma firma en wasm (edge) y en dev local (nativo).
type Context interface {
	Method() string
	Path() string
	Body() []byte
	GetHeader(key string) string
	SetHeader(key, value string)
	WriteStatus(code int)
	Write(b []byte) (int, error)
}

type HandlerFunc func(Context)

type Router interface {
	Get(path string, h HandlerFunc)
	Post(path string, h HandlerFunc)
	Put(path string, h HandlerFunc)
	Delete(path string, h HandlerFunc)
	Options(path string, h HandlerFunc)
	Handle(method, path string, h HandlerFunc) // catch-all
}
