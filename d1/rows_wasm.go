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
		if err := jsvalue.ScanValue(v, ptr); err != nil {
			return err
		}
	}
	return nil
}

func (r *d1Rows) Close() error { return nil }
func (r *d1Rows) Err() error   { return nil }

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
		if err := jsvalue.ScanValue(v, ptr); err != nil {
			return err
		}
	}
	return nil
}
