//go:build wasm

package d1

import (
	"syscall/js"

	"github.com/tinywasm/jsvalue"
	"github.com/tinywasm/orm"
	"github.com/tinywasm/sqlt"
)

type adapter struct{ dbObj js.Value }

// NewEdge opens the named D1 binding (Cloudflare edge runtime) and returns an *orm.DB.
func NewEdge(bindingName string) (*orm.DB, error) {
	v := js.Global().Get("context").Get("env").Get(bindingName)
	if v.IsUndefined() || v.IsNull() {
		return nil, ErrDatabaseNotFound
	}
	return orm.New(&adapter{dbObj: v}, sqlt.NewCompiler()), nil
}

func (a *adapter) Exec(query string, args ...any) error {
	stmt := a.dbObj.Call("prepare", query)
	_, err := jsvalue.AwaitPromise(bindArgs(stmt, args).Call("run"))
	return err
}

func (a *adapter) QueryRow(query string, args ...any) orm.Scanner {
	stmt := a.dbObj.Call("prepare", query)
	opts := js.Global().Get("Object").New()
	opts.Set("columnNames", true)
	arr, err := jsvalue.AwaitPromise(bindArgs(stmt, args).Call("raw", opts))
	if err != nil {
		return &errScanner{err}
	}
	if arr.Length() < 2 {
		return &errScanner{orm.ErrNotFound}
	}
	return &rowScanner{arr.Index(1)}
}

func (a *adapter) Query(query string, args ...any) (orm.Rows, error) {
	stmt := a.dbObj.Call("prepare", query)
	opts := js.Global().Get("Object").New()
	opts.Set("columnNames", true)
	arr, err := jsvalue.AwaitPromise(bindArgs(stmt, args).Call("raw", opts))
	if err != nil {
		return nil, err
	}
	if arr.Length() == 0 {
		return &d1Rows{}, nil
	}
	colsJS := arr.Call("shift")
	cols := make([]string, colsJS.Length())
	for i := range cols {
		cols[i] = colsJS.Index(i).String()
	}
	return &d1Rows{arr: arr, cols: cols}, nil
}

func (a *adapter) Close() error { return nil }

func bindArgs(stmt js.Value, args []any) js.Value {
	if len(args) == 0 {
		return stmt
	}
	jsArgs := make([]any, len(args))
	for i, arg := range args {
		if b, ok := arg.([]byte); ok {
			ua := jsvalue.Uint8ArrayClass.New(len(b))
			js.CopyBytesToJS(ua, b)
			jsArgs[i] = ua
		} else {
			jsArgs[i] = arg
		}
	}
	return stmt.Call("bind", jsArgs...)
}
