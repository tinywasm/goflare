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
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
)

type cfClient struct {
	token      string
	httpClient *http.Client
}

func newCFClient(token string) *cfClient {
	return &cfClient{
		token:      token,
		httpClient: http.DefaultClient,
	}
}

func (c *cfClient) get(path string) (json.RawMessage, error) {
	return c.do(http.MethodGet, path, nil, "")
}

func (c *cfClient) post(path string, body []byte) (json.RawMessage, error) {
	return c.do(http.MethodPost, path, bytes.NewReader(body), "application/json")
}

func (c *cfClient) put(path string, body []byte) (json.RawMessage, error) {
	return c.do(http.MethodPut, path, bytes.NewReader(body), "application/json")
}

func (c *cfClient) putMultipart(path string, body io.Reader, contentType string) (json.RawMessage, error) {
	return c.do(http.MethodPut, path, body, contentType)
}

func (c *cfClient) do(method, path string, body io.Reader, contentType string) (json.RawMessage, error) {
	url := cfBaseURL + path
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return parseCFResponse(resp)
}

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
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("CF API error %d: %s", resp.StatusCode, string(data))
		}
		return nil, fmt.Errorf("parse CF envelope: %w", err)
	}

	if !env.Success {
		if len(env.Errors) > 0 {
			return nil, fmt.Errorf("CF API error %d: %s", env.Errors[0].Code, env.Errors[0].Message)
		}
		return nil, fmt.Errorf("CF API returned success=false")
	}

	return env.Result, nil
}

// DeployWorker uploads the Worker script via Cloudflare API.
func (g *Goflare) DeployWorker(store Store) error {
	if g.Config.Entry == "" {
		return nil // Nothing to deploy
	}

	token, err := g.GetToken(store)
	if err != nil {
		return err
	}

	client := newCFClient(token)

	artifacts := []string{"worker.js", "worker.wasm", "wasm_exec.js"}
	for _, art := range artifacts {
		if _, err := os.Stat(filepath.Join(g.Config.OutputDir, art)); os.IsNotExist(err) {
			return fmt.Errorf("missing artifact: %s", art)
		}
	}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)

	// Field metadata
	metadata := map[string]string{"main_module": "worker.js"}
	metadataJSON, _ := json.Marshal(metadata)
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="metadata"`)
	h.Set("Content-Type", "application/json")
	part, _ := mw.CreatePart(h)
	part.Write(metadataJSON)

	// Files
	fileConfigs := []struct {
		name string
		path string
		ct   string
	}{
		{"worker.js", "worker.js", "application/javascript+module"},
		{"worker.wasm", "worker.wasm", "application/wasm"},
		{"wasm_exec.js", "wasm_exec.js", "application/javascript"},
	}

	for _, fc := range fileConfigs {
		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, fc.name, fc.name))
		h.Set("Content-Type", fc.ct)
		part, err := mw.CreatePart(h)
		if err != nil {
			return err
		}
		f, err := os.Open(filepath.Join(g.Config.OutputDir, fc.path))
		if err != nil {
			return err
		}
		defer f.Close()
		io.Copy(part, f)
	}

	mw.Close()

	path := fmt.Sprintf("/accounts/%s/workers/scripts/%s", g.Config.AccountID, g.Config.WorkerName)
	_, err = client.putMultipart(path, &body, mw.FormDataContentType())
	if err != nil {
		return fmt.Errorf("worker upload failed: %w", err)
	}

	return nil
}

type uploadFile struct {
	key         string // sha256 hex
	path        string // relative path from dist/ root, with leading /
	contentType string
	value       string // base64-encoded content
}

// DeployPages implementation.
func (g *Goflare) DeployPages(store Store) error {
	if g.Config.PublicDir == "" {
		return nil
	}

	token, err := g.GetToken(store)
	if err != nil {
		return err
	}

	client := newCFClient(token)

	// 1. Ensure project exists
	projectPath := fmt.Sprintf("/accounts/%s/pages/projects/%s", g.Config.AccountID, g.Config.ProjectName)
	_, err = client.get(projectPath)
	if err != nil {
		// Try to create project if not found
		createBody := map[string]string{
			"name":              g.Config.ProjectName,
			"production_branch": "main",
		}
		createJSON, _ := json.Marshal(createBody)
		_, err = client.post(fmt.Sprintf("/accounts/%s/pages/projects", g.Config.AccountID), createJSON)
		if err != nil {
			return fmt.Errorf("failed to create pages project: %w", err)
		}
	}

	// 2. Get upload JWT
	jwtPath := fmt.Sprintf("/accounts/%s/pages/projects/%s/upload-token", g.Config.AccountID, g.Config.ProjectName)
	jwtRes, err := client.post(jwtPath, nil)
	if err != nil {
		return fmt.Errorf("failed to get upload token: %w", err)
	}
	var jwtData struct {
		JWT string `json:"jwt"`
	}
	json.Unmarshal(jwtRes, &jwtData)

	// 3. Walk dist/ and collect files
	distDir := filepath.Join(g.Config.OutputDir, "dist")
	var files []uploadFile
	manifest := make(map[string]string)

	err = filepath.Walk(distDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(distDir, path)
		rel = "/" + filepath.ToSlash(rel)

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		hash := sha256.Sum256(content)
		hexHash := fmt.Sprintf("%x", hash)

		files = append(files, uploadFile{
			key:         hexHash,
			path:        rel,
			contentType: detectContentType(path),
			value:       base64.StdEncoding.EncodeToString(content),
		})
		manifest[rel] = hexHash
		return nil
	})
	if err != nil {
		return err
	}

	if len(files) == 0 {
		return fmt.Errorf("no files to upload in %s", distDir)
	}

	// 4. Upload in batches
	jwtClient := newCFClient(jwtData.JWT)
	for i := 0; i < len(files); i += 50 {
		end := i + 50
		if end > len(files) {
			end = len(files)
		}
		batch := files[i:end]
		batchJSON, _ := json.Marshal(batch)
		_, err = jwtClient.post("/pages/assets/upload", batchJSON)
		if err != nil {
			return fmt.Errorf("file upload batch failed: %w", err)
		}
	}

	// 5. Create deployment
	deployBody := map[string]any{
		"files":  manifest,
		"branch": "main",
	}
	deployJSON, _ := json.Marshal(deployBody)
	deployRes, err := client.post(fmt.Sprintf("/accounts/%s/pages/projects/%s/deployments", g.Config.AccountID, g.Config.ProjectName), deployJSON)
	if err != nil {
		return fmt.Errorf("failed to create pages deployment: %w", err)
	}

	if g.Config.Domain != "" {
		g.configurePagesDomain(client)
	}

	var deployData struct {
		URL string `json:"url"`
	}
	json.Unmarshal(deployRes, &deployData)
	if deployData.URL != "" {
		g.Logger("Pages deployed to:", deployData.URL)
	}

	return nil
}

func detectContentType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	m := map[string]string{
		".html": "text/html",
		".css":  "text/css",
		".js":   "application/javascript",
		".json": "application/json",
		".png":  "image/png",
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".svg":  "image/svg+xml",
		".ico":  "image/x-icon",
		".wasm": "application/wasm",
		".txt":  "text/plain",
	}
	if ct, ok := m[ext]; ok {
		return ct
	}
	return "application/octet-stream"
}

func (g *Goflare) configurePagesDomain(client *cfClient) error {
	path := fmt.Sprintf("/accounts/%s/pages/projects/%s/domains", g.Config.AccountID, g.Config.ProjectName)
	body, _ := json.Marshal(map[string]string{"name": g.Config.Domain})
	_, err := client.post(path, body)
	if err != nil {
		g.Logger("Warning: failed to configure custom domain:", err)
		return err
	}
	return nil
}
