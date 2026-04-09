package goflare

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const cfAPIBase = "https://api.cloudflare.com/client/v4"

type cfClient struct {
	token      string
	baseURL    string // default: cfAPIBase; overridden in tests
	httpClient *http.Client
}

func (c *cfClient) get(path string) ([]byte, error) {
	return c.do(http.MethodGet, path, nil)
}

func (c *cfClient) post(path string, body []byte) ([]byte, error) {
	return c.do(http.MethodPost, path, bytes.NewReader(body))
}

func (c *cfClient) put(path string, body []byte) ([]byte, error) {
	return c.do(http.MethodPut, path, bytes.NewReader(body))
}

func (c *cfClient) putMultipart(path string, body io.Reader, contentType string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodPut, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", contentType)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return parseCFResponse(resp)
}

func (c *cfClient) do(method, path string, body io.Reader) ([]byte, error) {
	req, err := http.NewRequest(method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return parseCFResponse(resp)
}

// DeployPages uploads the Pages build output (from config.OutputDir) to Cloudflare Pages.
func (g *Goflare) DeployPages(store Store) error {
	token, err := g.GetToken(store)
	if err != nil {
		return err
	}

	client := &cfClient{
		token:      token,
		baseURL:    g.BaseURL,
		httpClient: http.DefaultClient,
	}

	// 2. Ensure Pages project exists
	projectPath := fmt.Sprintf("/accounts/%s/pages/projects/%s", g.Config.AccountID, g.Config.ProjectName)
	_, err = client.get(projectPath)
	if err != nil {
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "8000007") {
			// Create project
			createPath := fmt.Sprintf("/accounts/%s/pages/projects", g.Config.AccountID)
			body := map[string]string{
				"name":              g.Config.ProjectName,
				"production_branch": "main",
			}
			bodyJSON, _ := json.Marshal(body)
			_, err = client.post(createPath, bodyJSON)
			if err != nil {
				return fmt.Errorf("failed to create Pages project: %w", err)
			}
		} else {
			return fmt.Errorf("failed to check Pages project: %w", err)
		}
	}

	// 3. Get upload JWT
	tokenPath := fmt.Sprintf("/accounts/%s/pages/projects/%s/uploadToken", g.Config.AccountID, g.Config.ProjectName)
	tokenResp, err := client.post(tokenPath, nil)
	if err != nil {
		return fmt.Errorf("failed to get upload token: %w", err)
	}
	var tokenData struct {
		JWT string `json:"jwt"`
	}
	if err := json.Unmarshal(tokenResp, &tokenData); err != nil {
		return fmt.Errorf("failed to parse upload token: %w", err)
	}

	// 4. Walk dist/ and collect files
	distDir := filepath.Join(g.Config.OutputDir, "dist")
	if _, err := os.Stat(distDir); os.IsNotExist(err) {
		return fmt.Errorf("dist directory missing: %s", distDir)
	}

	var files []uploadFile
	manifest := make(map[string]string)

	err = filepath.Walk(distDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		rel, _ := filepath.Rel(distDir, path)
		relPath := "/" + filepath.ToSlash(rel)

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		hash := sha256.Sum256(content)
		hashHex := fmt.Sprintf("%x", hash)

		files = append(files, uploadFile{
			Key:         hashHex,
			Value:       base64.StdEncoding.EncodeToString(content),
			Metadata:    map[string]string{"contentType": detectContentType(path)},
			Base64:      true,
		})
		manifest[relPath] = hashHex
		return nil
	})
	if err != nil {
		return err
	}

	if len(files) == 0 {
		return fmt.Errorf("no files found to upload in %s", distDir)
	}

	// 5. Upload files in batches of 50
	uploadClient := &cfClient{
		token:      tokenData.JWT,
		baseURL:    g.BaseURL,
		httpClient: http.DefaultClient,
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

	// 6. Create deployment
	deployPath := fmt.Sprintf("/accounts/%s/pages/projects/%s/deployments", g.Config.AccountID, g.Config.ProjectName)
	deployBody := map[string]any{
		"files":  manifest,
		"branch": "main",
	}
	deployBodyJSON, _ := json.Marshal(deployBody)
	_, err = client.post(deployPath, deployBodyJSON)
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

func (g *Goflare) configurePagesDomain(client *cfClient) error {
	path := fmt.Sprintf("/accounts/%s/pages/projects/%s/domains", g.Config.AccountID, g.Config.ProjectName)
	body := map[string]string{"name": g.Config.Domain}
	bodyJSON, _ := json.Marshal(body)
	_, err := client.post(path, bodyJSON)
	if err != nil {
		// If already exists, ignore
		if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "8000045") {
			return nil
		}
		return err
	}
	return nil
}

// DeployWorker uploads the Worker build output to Cloudflare Workers.
func (g *Goflare) DeployWorker(store Store) error {
	token, err := g.GetToken(store)
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

	client := &cfClient{
		token:      token,
		baseURL:    g.BaseURL,
		httpClient: http.DefaultClient,
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

func parseCFResponse(resp *http.Response) (json.RawMessage, error) {
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read CF response: %w", err)
	}
	var env cfEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("parse CF envelope: %w", err)
	}
	if !env.Success {
		if len(env.Errors) > 0 {
			var msgs []string
			for _, e := range env.Errors {
				msgs = append(msgs, fmt.Sprintf("%s (code: %d)", e.Message, e.Code))
			}
			return nil, fmt.Errorf("CF API error: %s", strings.Join(msgs, ", "))
		}
		return nil, fmt.Errorf("CF API returned success=false")
	}
	return env.Result, nil
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
