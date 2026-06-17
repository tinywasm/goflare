//go:build !wasm

package goflare

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"lukechampine.com/blake3"
)

const cfAPIBase = "https://api.cloudflare.com/client/v4"

type CfClient struct {
	Token      string
	BaseURL    string // default: cfAPIBase; overridden in tests
	HttpClient *http.Client
}

func (c *CfClient) get(path string) ([]byte, error) {
	return c.do(http.MethodGet, path, nil)
}

func (c *CfClient) post(path string, body []byte) ([]byte, error) {
	return c.do(http.MethodPost, path, bytes.NewReader(body))
}

func (c *CfClient) put(path string, body []byte) ([]byte, error) {
	return c.do(http.MethodPut, path, bytes.NewReader(body))
}

func (c *CfClient) putMultipart(path string, body io.Reader, contentType string) ([]byte, error) {
	return c.doMultipart(http.MethodPut, path, body, contentType)
}

func (c *CfClient) postMultipart(path string, body io.Reader, contentType string) ([]byte, error) {
	return c.doMultipart(http.MethodPost, path, body, contentType)
}

func (c *CfClient) doMultipart(method, path string, body io.Reader, contentType string) ([]byte, error) {
	req, err := http.NewRequest(method, c.BaseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", contentType)

	resp, err := c.HttpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return parseCFResponse(method, path, resp)
}

func (c *CfClient) do(method, path string, body io.Reader) ([]byte, error) {
	req, err := http.NewRequest(method, c.BaseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.HttpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return parseCFResponse(method, path, resp)
}

// DeployPages uploads the Pages build output (from config.OutputDir) to Cloudflare Pages.
// validateDeployScopes confirms that the token has the necessary permissions to
// manage Pages projects and deployments.
func (g *Goflare) ValidateDeployScopes(client *CfClient) error {
	path := fmt.Sprintf("/accounts/%s/pages/projects", g.Config.AccountID)
	if _, err := client.get(path); err != nil {
		return fmt.Errorf(
			"the token cannot access Pages on account %s.\n"+
				"  - Verify permission Account → Cloudflare Pages → Edit\n"+
				"  - Verify that CLOUDFLARE_ACCOUNT_ID is correct\n"+
				"Detail: %w", g.Config.AccountID, err)
	}
	return nil
}

func (g *Goflare) createPagesProject(client *CfClient) error {
	g.Logger("Pages project not found — creating", g.Config.ProjectName)
	createPath := fmt.Sprintf("/accounts/%s/pages/projects", g.Config.AccountID)
	body, _ := json.Marshal(map[string]string{
		"name":              g.Config.ProjectName,
		"production_branch": "main",
	})
	_, err := client.post(createPath, body)
	if err != nil {
		var apiErr *cfError
		if errors.As(err, &apiErr) && apiErr.alreadyExists() {
			return nil
		}
		return fmt.Errorf("failed to create Pages project: %w", err)
	}
	return nil
}

func (g *Goflare) DeployPages() error {
	token, err := g.token()
	if err != nil {
		return err
	}

	client := &CfClient{
		Token:      token,
		BaseURL:    g.BaseURL,
		HttpClient: http.DefaultClient,
	}

	// 2. Ensure Pages project exists
	projectPath := fmt.Sprintf("/accounts/%s/pages/projects/%s", g.Config.AccountID, g.Config.ProjectName)
	_, err = client.get(projectPath)
	if err != nil {
		var apiErr *cfError
		notFound := errors.As(err, &apiErr) && (apiErr.Status == http.StatusNotFound || apiErr.Code == 8000007)
		if !notFound {
			return fmt.Errorf("failed to check Pages project: %w", err)
		}
		if err := g.createPagesProject(client); err != nil {
			return err
		}
	}

	// 3. Get upload JWT — retry because a newly created project takes time to be ready.
	tokenPath := fmt.Sprintf("/accounts/%s/pages/projects/%s/upload-token", g.Config.AccountID, g.Config.ProjectName)
	var tokenResp []byte
	err = g.retry(5, g.RetryBackoff, func() error {
		var e error
		tokenResp, e = client.get(tokenPath)
		return e
	})
	if err != nil {
		return fmt.Errorf("failed to get upload Token: %w", err)
	}
	var tokenData struct {
		JWT string `json:"jwt"`
	}
	if err := json.Unmarshal(tokenResp, &tokenData); err != nil {
		return fmt.Errorf("failed to parse upload Token: %w", err)
	}

	// 4. Walk PublicDir and FunctionsDir, collect all files for the manifest.
	// FunctionsDir (e.g. "functions/") contains the compiled Pages Functions
	// (edge.wasm + [[path]].mjs) and must be uploaded alongside static assets.
	distDir := g.Config.PublicDir
	if _, err := os.Stat(distDir); os.IsNotExist(err) {
		return fmt.Errorf("public directory missing: %s", distDir)
	}

	var files []uploadFile
	manifest := make(map[string]string)

	collectDir := func(dir, prefix string) error {
		return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(dir, path)
			relPath := prefix + filepath.ToSlash(rel)
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			// Cloudflare Pages asset key: blake3(base64(content)+ext).hex()[:32]
			// (matches wrangler; a plain sha256 hash is rejected with HTTP 500 code 1101).
			b64 := base64.StdEncoding.EncodeToString(content)
			ext := strings.TrimPrefix(filepath.Ext(path), ".")
			sum := blake3.Sum256([]byte(b64 + ext))
			hashHex := hex.EncodeToString(sum[:])[:32]
			files = append(files, uploadFile{
				Key:      hashHex,
				Value:    b64,
				Metadata: map[string]string{"contentType": detectContentType(path)},
				Base64:   true,
			})
			manifest[relPath] = hashHex
			return nil
		})
	}

	if err := collectDir(distDir, "/"); err != nil {
		return err
	}

	// Include Pages Functions artifacts if present.
	if g.Config.FunctionsDir != "" {
		if _, statErr := os.Stat(g.Config.FunctionsDir); statErr == nil {
			if err := collectDir(g.Config.FunctionsDir, "/functions/"); err != nil {
				return err
			}
		}
	}

	if len(files) == 0 {
		return fmt.Errorf("no files found to upload in %s", distDir)
	}

	// 5. Upload files in batches of 50
	uploadClient := &CfClient{
		Token:      tokenData.JWT,
		BaseURL:    g.BaseURL,
		HttpClient: http.DefaultClient,
	}
	for i := 0; i < len(files); i += 50 {
		end := i + 50
		if end > len(files) {
			end = len(files)
		}
		batch := files[i:end]
		batchJSON, _ := json.Marshal(batch)
		_, err = uploadClient.post("/pages/assets/upload", batchJSON)
		if err != nil {
			return fmt.Errorf("failed to upload assets batch: %w", err)
		}
	}

	// 6. Create deployment — Cloudflare expects multipart/form-data with a
	// "manifest" field (JSON map of path->hash), not a JSON body (else HTTP 400 code 8000096).
	deployPath := fmt.Sprintf("/accounts/%s/pages/projects/%s/deployments", g.Config.AccountID, g.Config.ProjectName)
	manifestJSON, _ := json.Marshal(manifest)
	var deployForm bytes.Buffer
	mw := multipart.NewWriter(&deployForm)
	mw.WriteField("manifest", string(manifestJSON))
	mw.WriteField("branch", "main")
	mw.Close()
	_, err = client.postMultipart(deployPath, &deployForm, mw.FormDataContentType())
	if err != nil {
		return fmt.Errorf("failed to create deployment: %w", err)
	}

	// 7. Configure domain
	if g.Config.Domain != "" {
		if err := g.configurePagesDomain(client); err != nil {
			g.Logger("Warning: failed to configure domain:", err)
		}
	}

	return nil
}

type uploadFile struct {
	Key      string            `json:"key"`
	Value    string            `json:"value"`
	Metadata map[string]string `json:"metadata"`
	Base64   bool              `json:"base64"`
}

func detectContentType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".html":
		return "text/html"
	case ".css":
		return "text/css"
	case ".js":
		return "application/javascript"
	case ".json":
		return "application/json"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".svg":
		return "image/svg+xml"
	case ".ico":
		return "image/x-icon"
	case ".wasm":
		return "application/wasm"
	case ".txt":
		return "text/plain"
	default:
		return "application/octet-stream"
	}
}

func (g *Goflare) configurePagesDomain(client *CfClient) error {
	path := fmt.Sprintf("/accounts/%s/pages/projects/%s/domains", g.Config.AccountID, g.Config.ProjectName)
	body := map[string]string{"name": g.Config.Domain}
	bodyJSON, _ := json.Marshal(body)
	_, err := client.post(path, bodyJSON)
	if err != nil {
		var apiErr *cfError
		if errors.As(err, &apiErr) && apiErr.alreadyExists() {
			return nil
		}
		return err
	}
	return nil
}

// DeployWorker uploads the Worker build output to Cloudflare Workers.
func (g *Goflare) getWorkerSubdomain(client *CfClient) string {
	path := fmt.Sprintf("/accounts/%s/workers/subdomain", g.Config.AccountID)
	data, err := client.get(path)
	if err != nil {
		return "<your-subdomain>"
	}
	var result struct {
		Result struct {
			Subdomain string `json:"subdomain"`
		} `json:"result"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "<your-subdomain>"
	}
	return result.Result.Subdomain
}

func (g *Goflare) DeployWorker() error {
	token, err := g.token()
	if err != nil {
		return err
	}

	edgeJs := filepath.Join(g.Config.OutputDir, "edge.js")
	edgeWasm := filepath.Join(g.Config.OutputDir, "edge.wasm")

	files := []string{edgeJs, edgeWasm}
	for _, f := range files {
		if _, err := os.Stat(f); os.IsNotExist(err) {
			return fmt.Errorf("missing artifact: %s", filepath.Base(f))
		}
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	// metadata
	metadata := map[string]string{"main_module": "edge.js"}
	metadataJSON, _ := json.Marshal(metadata)
	if err := mw.WriteField("metadata", string(metadataJSON)); err != nil {
		return err
	}

	// edge.js
	if err := addFilePart(mw, "edge.js", edgeJs); err != nil {
		return err
	}

	// edge.wasm
	if err := addFilePart(mw, "edge.wasm", edgeWasm); err != nil {
		return err
	}

	mw.Close()

	client := &CfClient{
		Token:      token,
		BaseURL:    g.BaseURL,
		HttpClient: http.DefaultClient,
	}

	path := fmt.Sprintf("/accounts/%s/workers/scripts/%s", g.Config.AccountID, g.Config.WorkerName)
	_, err = client.putMultipart(path, &buf, mw.FormDataContentType())
	return err
}

// ── internal helpers ──────────────────────────────────────────────────────────

type cfEnvelope struct {
	Success bool            `json:"success"`
	Errors  []cfAPIError    `json:"errors"`
	Result  json.RawMessage `json:"result"`
}

type cfAPIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type cfError struct {
	Status  int          // status HTTP
	Code    int          // primer errors[].code, si hay
	Message string       // resumen legible
	Errors  []cfAPIError // todos los errores del envelope
	Body    string       // cuerpo crudo truncado (fallback)
	Path    string       // método + ruta que falló
}

func (e *cfError) Error() string {
	if len(e.Errors) > 0 {
		return fmt.Sprintf("CF API %s → HTTP %d: %s", e.Path, e.Status, e.Message)
	}
	return fmt.Sprintf("CF API %s → HTTP %d, success=false, body: %s", e.Path, e.Status, e.Body)
}

func (e *cfError) alreadyExists() bool {
	for _, x := range e.Errors {
		if x.Code == 8000009 || x.Code == 8000045 || strings.Contains(x.Message, "already exists") {
			return true
		}
	}
	return false
}

func parseCFResponse(method, path string, resp *http.Response) (json.RawMessage, error) {
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read CF response: %w", err)
	}
	var env cfEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, &cfError{
			Status: resp.StatusCode,
			Path:   method + " " + path,
			Body:   truncate(string(data), 500),
		}
	}
	if !env.Success || resp.StatusCode >= 400 {
		ce := &cfError{
			Status: resp.StatusCode,
			Errors: env.Errors,
			Path:   method + " " + path,
			Body:   truncate(string(data), 500),
		}
		if len(env.Errors) > 0 {
			ce.Code = env.Errors[0].Code
			var msgs []string
			for _, e := range env.Errors {
				msgs = append(msgs, fmt.Sprintf("%s (code: %d)", e.Message, e.Code))
			}
			ce.Message = strings.Join(msgs, ", ")
		}
		return nil, ce
	}
	return env.Result, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// retry executes fn up to n times with exponential backoff.
func (g *Goflare) retry(n int, base time.Duration, fn func() error) error {
	var err error
	for i := 0; i < n; i++ {
		if err = fn(); err == nil {
			return nil
		}
		if i < n-1 {
			time.Sleep(base << i)
		}
	}
	return err
}

func addFilePart(mw *multipart.Writer, fieldName, filePath string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open %s: %w", filePath, err)
	}
	defer f.Close()
	part, err := mw.CreateFormFile(fieldName, filepath.Base(filePath))
	if err != nil {
		return err
	}
	_, err = io.Copy(part, f)
	return err
}
