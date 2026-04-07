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
)

const cfAPIBase = "https://api.cloudflare.com/client/v4"

// DeployPages uploads the Pages build output (from config.OutputDir) to Cloudflare Pages.
func (g *Goflare) DeployPages(store Store) error {
	token, err := store.Get("CF_PAGES_TOKEN")
	if err != nil || token == "" {
		return fmt.Errorf("cloudflare: pages token not configured")
	}
	accountID := g.Config.AccountID
	if accountID == "" {
		return fmt.Errorf("cloudflare: account_id not configured")
	}
	project := g.Config.ProjectName
	if project == "" {
		return fmt.Errorf("cloudflare: project not configured")
	}

	outputDir := g.Config.OutputDir
	jsFileName := "_worker.js"
	wasmFileName := "worker.wasm"

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
	// Note: For Workers, we usually need to upload the script + wasm.
	// For now, let's implement the basic script upload if it's a single file,
	// or return an error if it's not yet fully supported (requires module bundler).
	return fmt.Errorf("cloudflare: DeployWorker not fully implemented yet (requires module bundling)")
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
