//go:build wasm

package d1

import (
	"syscall/js"
	. "github.com/tinywasm/fmt"
	"github.com/tinywasm/jsvalue"
)

// d1Rows implements orm.Rows over a raw JS array of row arrays.
type d1Rows struct {
	arr  js.Value
	cols []string
	cur  int
	len  int
	once bool
}

func (r *d1Rows) rowsLen() int {
	if !r.once {
		if r.arr.Truthy() {
			r.len = r.arr.Length()
		}
		r.once = true
	}
	return r.len
}

func (r *d1Rows) Next() bool {
	if r.cur < r.rowsLen() {
		r.cur++
		return true
	}
	return false
}

func (r *d1Rows) Scan(dest ...any) error {
	if r.cur == 0 || r.cur > r.rowsLen() {
		return Err(errPrefix + "invalid row cursor")
	}
	row := r.arr.Index(r.cur - 1)
	if len(dest) != len(r.cols) {
		return Err(errPrefix + "scan destination count mismatch")
	}
	for i, ptr := range dest {
		v := row.Index(i)
		if err := scanValue(v, ptr); err != nil {
			return err
		}
	}
	return nil
}

func (r *d1Rows) Close() error { return nil }
func (r *d1Rows) Err() error   { return nil }

// scanValue copies a JS value into a Go pointer.
func scanValue(v js.Value, dest any) error {
	switch p := dest.(type) {
	case *string:
		*p = v.String()
	case *int:
		*p = v.Int()
	case *int64:
		*p = int64(v.Float())
	case *float64:
		*p = v.Float()
	case *bool:
		*p = v.Bool()
	case *[]byte:
		src := jsvalue.Uint8ArrayClass.New(v)
		b := make([]byte, src.Length())
		js.CopyBytesToGo(b, src)
		*p = b
	case *any:
		*p = jsValueToAny(v)
	default:
		return Errf(errPrefix+"unsupported scan type: %T", dest)
	}
	return nil
}

func jsValueToAny(v js.Value) any {
	switch v.Type() {
	case js.TypeNull, js.TypeUndefined:
		return nil
	case js.TypeBoolean:
		return v.Bool()
	case js.TypeNumber:
		f := v.Float()
		if f == float64(int64(f)) {
			return int64(f)
		}
		return f
	case js.TypeString:
		return v.String()
	default:
		return v.String()
	}
}

// errScanner is a Scanner that always returns an error.
type errScanner struct{ err error }

func (e *errScanner) Scan(...any) error { return e.err }

// rowScanner scans a single D1 row (JS array).
type rowScanner struct{ row js.Value }

func (s *rowScanner) Scan(dest ...any) error {
	if s.row.IsUndefined() || s.row.IsNull() {
		return Err(errPrefix + "no results to scan")
	}
	if destLen := len(dest); destLen > s.row.Length() {
		return Err(errPrefix + "scan destination count mismatch")
	}
	for i, ptr := range dest {
		v := s.row.Index(i)
		if err := scanValue(v, ptr); err != nil {
			return err
		}
	}
	return nil
}
