package goflare

import (
	"bytes"
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

// Store is a flat key-value store for Cloudflare configuration and secrets.
type Store interface {
	Get(key string) (string, error)
	Set(key, value string) error
}

// SetupPages uses a bootstrap token to create a scoped Pages:Edit token.
// Stores CF_ACCOUNT_ID, CF_PAGES_TOKEN, CF_PROJECT in the Store.
func (g *Goflare) SetupPages(store Store, accountID, bootstrapToken, projectName string) error {
	g.Logger("Cloudflare: fetching permission groups...")
	permID, err := g.findPagesEditPermission(bootstrapToken)
	if err != nil {
		return fmt.Errorf("cloudflare: find Pages:Edit permission: %w", err)
	}

	g.Logger("Cloudflare: creating scoped Pages token...")
	scopedToken, err := g.createScopedToken(bootstrapToken, accountID, permID)
	if err != nil {
		return fmt.Errorf("cloudflare: create scoped token: %w", err)
	}

	if err := store.Set("CF_ACCOUNT_ID", accountID); err != nil {
		return fmt.Errorf("cloudflare: store account_id: %w", err)
	}
	if err := store.Set("CF_PAGES_TOKEN", scopedToken); err != nil {
		return fmt.Errorf("cloudflare: store pages_token: %w", err)
	}
	if err := store.Set("CF_PROJECT", projectName); err != nil {
		return fmt.Errorf("cloudflare: store project: %w", err)
	}

	g.Logger("Cloudflare: setup complete.")
	return nil
}

// DeployPages uploads the Pages build output (from config.RelativeOutputDirectory) to Cloudflare Pages.
func (g *Goflare) DeployPages(store Store) error {
	token, err := store.Get("CF_PAGES_TOKEN")
	if err != nil || token == "" {
		return fmt.Errorf("cloudflare: pages token not configured")
	}
	accountID, err := store.Get("CF_ACCOUNT_ID")
	if err != nil || accountID == "" {
		return fmt.Errorf("cloudflare: account_id not configured")
	}
	project, err := store.Get("CF_PROJECT")
	if err != nil || project == "" {
		return fmt.Errorf("cloudflare: project not configured")
	}

	outputDir := g.config.RelativeOutputDirectory()
	jsFileName := g.outputJsFileName
	wasmFileName := g.config.OutputWasmFileName

	jsFile := filepath.Join(outputDir, jsFileName)
	wasmFile := filepath.Join(outputDir, wasmFileName)

	g.Logger("Deploying to Cloudflare Pages project:", project)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := addFilePart(mw, jsFileName, jsFile); err != nil {
		return fmt.Errorf("cloudflare: add %s: %w", jsFileName, err)
	}
	if err := addFilePart(mw, wasmFileName, wasmFile); err != nil {
		return fmt.Errorf("cloudflare: add %s: %w", wasmFileName, err)
	}
	mw.Close()

	url := fmt.Sprintf("%s/accounts/%s/pages/projects/%s/deployments",
		cfAPIBase, accountID, project)

	req, err := http.NewRequest(http.MethodPost, url, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("cloudflare: POST deployments: %w", err)
	}
	defer resp.Body.Close()

	result, err := parseCFResponse(resp)
	if err != nil {
		return err
	}

	var deployment struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(result, &deployment); err == nil && deployment.URL != "" {
		g.Logger("Deployment URL:", deployment.URL)
	}
	return nil
}

// DeployWorker uploads the Worker build output to Cloudflare Workers.
func (g *Goflare) DeployWorker(store Store) error {
	token, err := store.Get("CF_WORKER_TOKEN")
	if err != nil || token == "" {
		return fmt.Errorf("cloudflare: worker token not configured")
	}
	accountID, err := store.Get("CF_ACCOUNT_ID")
	if err != nil || accountID == "" {
		return fmt.Errorf("cloudflare: account_id not configured")
	}
	project, err := store.Get("CF_PROJECT")
	if err != nil || project == "" {
		return fmt.Errorf("cloudflare: project not configured")
	}

	outputDir := g.config.RelativeOutputDirectory()
	wasmFileName := g.config.OutputWasmFileName
	wasmFile := filepath.Join(outputDir, wasmFileName)

	g.Logger("Deploying to Cloudflare Worker script:", project)

	// Note: For Workers, we usually need to upload the script + wasm.
	// For now, let's implement the basic script upload if it's a single file,
	// or return an error if it's not yet fully supported (requires module bundler).
	_ = wasmFile // avoid unused error for now
	return fmt.Errorf("cloudflare: DeployWorker not fully implemented yet (requires module bundling)")
}

// IsPagesConfigured returns true if a scoped Pages token exists in the store.
func (g *Goflare) IsPagesConfigured(store Store) bool {
	tok, err := store.Get("CF_PAGES_TOKEN")
	return err == nil && tok != ""
}

// IsWorkerConfigured returns true if a scoped Worker token exists in the store.
func (g *Goflare) IsWorkerConfigured(store Store) bool {
	tok, err := store.Get("CF_WORKER_TOKEN")
	return err == nil && tok != ""
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

func (g *Goflare) findPagesEditPermission(token string) (string, error) {
	result, err := g.doGET("/user/tokens/permission_groups", token)
	if err != nil {
		return "", err
	}
	var groups []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(result, &groups); err != nil {
		return "", fmt.Errorf("parse permission groups: %w", err)
	}
	for _, group := range groups {
		if strings.Contains(group.Name, "Pages") && strings.Contains(group.Name, "Edit") {
			return group.ID, nil
		}
	}
	return "", fmt.Errorf("Pages:Edit permission group not found")
}

func (g *Goflare) createScopedToken(bootstrapToken, accountID, permID string) (string, error) {
	payload := map[string]any{
		"name": "tinywasm-pages-deploy",
		"policies": []map[string]any{
			{
				"effect":            "allow",
				"permission_groups": []map[string]string{{"id": permID}},
				"resources":         map[string]string{"com.cloudflare.api.account": accountID},
			},
		},
	}
	body, _ := json.Marshal(payload)
	result, err := g.doPOST("/user/tokens", bootstrapToken, body)
	if err != nil {
		return "", err
	}
	var tok struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(result, &tok); err != nil || tok.Value == "" {
		return "", fmt.Errorf("token value missing in response")
	}
	return tok.Value, nil
}

func (g *Goflare) doGET(path, token string) (json.RawMessage, error) {
	req, _ := http.NewRequest(http.MethodGet, cfAPIBase+path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	return parseCFResponse(resp)
}

func (g *Goflare) doPOST(path, token string, body []byte) (json.RawMessage, error) {
	req, _ := http.NewRequest(http.MethodPost, cfAPIBase+path, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("POST %s: %w", path, err)
	}
	defer resp.Body.Close()
	return parseCFResponse(resp)
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
			return nil, fmt.Errorf("CF API error %d: %s", env.Errors[0].Code, env.Errors[0].Message)
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
