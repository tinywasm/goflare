//go:build wasm

package d1

import (
	"syscall/js"

	"github.com/tinywasm/jsvalue"
	"github.com/tinywasm/orm"
	"github.com/tinywasm/sqlt"
)

type adapter struct{ dbObj js.Value }

// New opens the named D1 binding and returns an *orm.DB.
func New(bindingName string) (*orm.DB, error) {
	v := js.Global().Get("context").Get("env").Get(bindingName)
	if v.IsUndefined() || v.IsNull() {
		return nil, ErrDatabaseNotFound
	}
	a := &adapter{dbObj: v}
	return orm.New(a, sqlt.NewCompiler()), nil
}

// Exec implements orm.Executor — runs INSERT, UPDATE, DELETE.
func (a *adapter) Exec(query string, args ...any) error {
	stmt := a.dbObj.Call("prepare", query)
	bound := bindArgs(stmt, args)
	_, err := jsvalue.AwaitPromise(bound.Call("run"))
	return err
}

// QueryRow implements orm.Executor — fetches a single row.
func (a *adapter) QueryRow(query string, args ...any) orm.Scanner {
	stmt := a.dbObj.Call("prepare", query)
	bound := bindArgs(stmt, args)
	opts := js.Global().Get("Object").New()
	opts.Set("columnNames", true)
	p := bound.Call("raw", opts)
	arr, err := jsvalue.AwaitPromise(p)
	if err != nil {
		return &errScanner{err}
	}
	if arr.Length() == 0 {
		return &errScanner{orm.ErrNotFound}
	}
	// first element is column names array, second is the first row
	if arr.Length() < 2 {
		return &errScanner{orm.ErrNotFound}
	}
	return &rowScanner{arr.Index(1)}
}

// Query implements orm.Executor — fetches multiple rows.
func (a *adapter) Query(query string, args ...any) (orm.Rows, error) {
	stmt := a.dbObj.Call("prepare", query)
	bound := bindArgs(stmt, args)
	opts := js.Global().Get("Object").New()
	opts.Set("columnNames", true)
	p := bound.Call("raw", opts)
	arr, err := jsvalue.AwaitPromise(p)
	if err != nil {
		return nil, err
	}
	if arr.Length() == 0 {
		return &d1Rows{}, nil
	}
	// first element is column names array
	colsJS := arr.Call("shift")
	cols := make([]string, colsJS.Length())
	for i := range cols {
		cols[i] = colsJS.Index(i).String()
	}
	return &d1Rows{arr: arr, cols: cols}, nil
}

func (a *adapter) Close() error { return nil }

// bindArgs calls .bind(...args) on a prepared statement, converting []byte to Uint8Array.
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
