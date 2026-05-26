//go:build !wasm

package d1

import (
	"github.com/tinywasm/orm"
	"github.com/tinywasm/sqlite"
)

// NewLocal opens a local SQLite database for host use (dev + tests) — no Cloudflare
// credentials, no network. D1 is SQLite under the hood and orm uses the same sqlt
// compiler in every context, so behavior matches the edge (NewEdge). Only the data
// location differs.
//
// path is a SQLite DSN: a file ("goflare-local.db") to persist across restarts, or
// ":memory:" for an ephemeral per-process database (tests).
func NewLocal(path string) (*orm.DB, error) {
	return sqlite.Open(path)
}
