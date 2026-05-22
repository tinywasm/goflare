//go:build !wasm

package d1

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/tinywasm/orm"
	"github.com/tinywasm/sqlt"
)

const (
	cfD1APIBase   = "https://api.cloudflare.com/client/v4"
	cfD1QueryPath = "/accounts/%s/d1/database/%s/query"
)

// directAdapter implements orm.Executor over the Cloudflare D1 REST API.
// Used only on the host (integration tests). Production uses adapter (JS binding).
type directAdapter struct {
	client *d1RestClient
}

type d1RestClient struct {
	token      string
	accountID  string
	databaseID string
	httpClient *http.Client
	baseURL    string
}

type d1QueryRequest struct {
	SQL    string `json:"sql"`
	Params []any  `json:"params"`
}

type d1QueryResult struct {
	Results []map[string]any `json:"results"`
	Success bool             `json:"success"`
}

type d1RestEnvelope struct {
	Success bool            `json:"success"`
	Errors  []d1RestError   `json:"errors"`
	Result  []d1QueryResult `json:"result"`
}

type d1RestError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// NewDirect opens a D1 database via the REST API and returns an *orm.DB.
// Uses the same sqlt.NewCompiler() as the WASM adapter — identical SQL generation path.
// token, accountID, databaseID come from keyring or env vars (never hardcoded).
func NewDirect(token, accountID, databaseID string) (*orm.DB, error) {
	if token == "" || accountID == "" || databaseID == "" {
		return nil, fmt.Errorf(errPrefix + "token, accountID and databaseID are required")
	}
	a := &directAdapter{
		client: &d1RestClient{
			token:      token,
			accountID:  accountID,
			databaseID: databaseID,
			httpClient: http.DefaultClient,
			baseURL:    cfD1APIBase,
		},
	}
	return orm.New(a, sqlt.NewCompiler()), nil
}

func (c *d1RestClient) do(sql string, params []any) ([]map[string]any, error) {
	if params == nil {
		params = []any{}
	}
	body, err := json.Marshal(d1QueryRequest{SQL: sql, Params: params})
	if err != nil {
		return nil, fmt.Errorf(errPrefix+"marshal: %w", err)
	}
	path := fmt.Sprintf(cfD1QueryPath, c.accountID, c.databaseID)
	req, err := http.NewRequest(http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf(errPrefix+"request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf(errPrefix+"http: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf(errPrefix+"read: %w", err)
	}
	var env d1RestEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf(errPrefix+"parse: %w", err)
	}
	if !env.Success {
		if len(env.Errors) > 0 {
			return nil, fmt.Errorf(errPrefix+"%s (code: %d)", env.Errors[0].Message, env.Errors[0].Code)
		}
		return nil, fmt.Errorf(errPrefix + "success=false")
	}
	if len(env.Result) == 0 {
		return nil, nil
	}
	return env.Result[0].Results, nil
}

// Exec implements orm.Executor for INSERT / UPDATE / DELETE.
func (a *directAdapter) Exec(query string, args ...any) error {
	_, err := a.client.do(query, args)
	return err
}

// QueryRow implements orm.Executor for single-row SELECT.
func (a *directAdapter) QueryRow(query string, args ...any) orm.Scanner {
	rows, err := a.client.do(query, args)
	if err != nil {
		return &errScanner{err}
	}
	if len(rows) == 0 {
		return &errScanner{orm.ErrNotFound}
	}
	return &directRowScanner{row: rows[0]}
}

// Query implements orm.Executor for multi-row SELECT.
func (a *directAdapter) Query(query string, args ...any) (orm.Rows, error) {
	rows, err := a.client.do(query, args)
	if err != nil {
		return nil, err
	}
	return &directRows{rows: rows}, nil
}

func (a *directAdapter) Close() error { return nil }

// errScanner is a Scanner that always returns an error.
type errScanner struct{ err error }

func (e *errScanner) Scan(...any) error { return e.err }

// directRowScanner scans a single row from the REST response (map[string]any).
type directRowScanner struct{ row map[string]any }

func (s *directRowScanner) Scan(dest ...any) error {
	i := 0
	for _, v := range s.row {
		if i >= len(dest) {
			break
		}
		if err := orm.ScanAny(v, dest[i]); err != nil {
			return err
		}
		i++
	}
	return nil
}

// directRows iterates over REST response rows.
type directRows struct {
	rows []map[string]any
	cur  int
}

func (r *directRows) Next() bool {
	if r.cur < len(r.rows) {
		r.cur++
		return true
	}
	return false
}

func (r *directRows) Scan(dest ...any) error {
	row := r.rows[r.cur-1]
	i := 0
	for _, v := range row {
		if i >= len(dest) {
			break
		}
		if err := orm.ScanAny(v, dest[i]); err != nil {
			return err
		}
		i++
	}
	return nil
}

func (r *directRows) Close() error { return nil }
func (r *directRows) Err() error   { return nil }
