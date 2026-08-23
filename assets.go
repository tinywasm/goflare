//go:build !wasm

package goflare

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
)

// assetEntry describes a file in the Worker asset manifest.
type assetEntry struct {
	Hash string `json:"hash"`
	Size int    `json:"size"`
}

// assetHash computes sha256(base64(content) + ext), hex-encoded, truncated to 32 chars.
// ext is expected without a leading dot (e.g. "html", not ".html").
func assetHash(content []byte, ext string) string {
	b64 := base64.StdEncoding.EncodeToString(content)
	sum := sha256.Sum256([]byte(b64 + ext))
	return hex.EncodeToString(sum[:])[:32]
}

// ExportAssetHash exports assetHash for testing.
func ExportAssetHash(content []byte, ext string) string {
	return assetHash(content, ext)
}

// buildAssetManifest walks dir and generates the asset manifest map and a hash -> file path index.
func (g *Goflare) buildAssetManifest(dir string) (map[string]assetEntry, map[string]string, error) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil, nil, fmt.Errorf("assets directory missing: %s", dir)
	}

	manifest := make(map[string]assetEntry)
	byHash := make(map[string]string)

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		ext := strings.TrimPrefix(filepath.Ext(path), ".")
		hash := assetHash(content, ext)

		key := "/" + filepath.ToSlash(rel)
		manifest[key] = assetEntry{
			Hash: hash,
			Size: len(content),
		}
		byHash[hash] = path
		return nil
	})

	if err != nil {
		return nil, nil, err
	}

	if len(manifest) == 0 {
		return nil, nil, fmt.Errorf("no assets found in %s", dir)
	}

	return manifest, byHash, nil
}

// ExportBuildAssetManifest exports buildAssetManifest for testing.
func (g *Goflare) ExportBuildAssetManifest(dir string) (map[string]assetEntry, map[string]string, error) {
	return g.buildAssetManifest(dir)
}

// uploadAssets executes Phase 1 (assets-upload-session) and Phase 2 (upload buckets)
// and returns the asset completion token.
func (g *Goflare) uploadAssets(client *CfClient, manifest map[string]assetEntry, byHash map[string]string) (string, error) {
	// Phase 1 — Upload Session
	sessionPath := fmt.Sprintf("/accounts/%s/workers/scripts/%s/assets-upload-session", g.Config.AccountID, g.Config.WorkerName)
	payload, err := json.Marshal(map[string]any{
		"manifest": manifest,
	})
	if err != nil {
		return "", fmt.Errorf("failed to marshal asset manifest: %w", err)
	}

	respData, err := client.post(sessionPath, payload)
	if err != nil {
		return "", fmt.Errorf("failed to create asset upload session: %w", err)
	}

	var session struct {
		JWT     string     `json:"jwt"`
		Buckets [][]string `json:"buckets"`
	}
	if err := json.Unmarshal(respData, &session); err != nil {
		return "", fmt.Errorf("failed to parse asset upload session response: %w", err)
	}

	if len(session.Buckets) == 0 {
		return session.JWT, nil
	}

	// Phase 2 — Asset Upload per bucket
	uploadClient := &CfClient{
		Token:      session.JWT,
		BaseURL:    client.BaseURL,
		HttpClient: client.HttpClient,
	}

	var completionJWT string

	for _, bucket := range session.Buckets {
		if len(bucket) == 0 {
			continue
		}

		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)

		for _, hash := range bucket {
			filePath, ok := byHash[hash]
			if !ok {
				return "", fmt.Errorf("asset hash %s not found in file index", hash)
			}

			content, err := os.ReadFile(filePath)
			if err != nil {
				return "", fmt.Errorf("failed to read asset %s: %w", filePath, err)
			}

			b64 := base64.StdEncoding.EncodeToString(content)
			contentType := detectContentType(filePath)

			h := make(textproto.MIMEHeader)
			h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"`, hash))
			h.Set("Content-Type", contentType)

			part, err := mw.CreatePart(h)
			if err != nil {
				return "", fmt.Errorf("failed to create form part for hash %s: %w", hash, err)
			}

			if _, err := part.Write([]byte(b64)); err != nil {
				return "", fmt.Errorf("failed to write form part for hash %s: %w", hash, err)
			}
		}

		if err := mw.Close(); err != nil {
			return "", fmt.Errorf("failed to close multipart writer: %w", err)
		}

		uploadResp, err := uploadClient.postMultipart("/workers/assets/upload?base64=true", &buf, mw.FormDataContentType())
		if err != nil {
			return "", fmt.Errorf("failed to upload asset bucket: %w", err)
		}

		var uploadResult struct {
			JWT string `json:"jwt"`
		}
		if err := json.Unmarshal(uploadResp, &uploadResult); err == nil && uploadResult.JWT != "" {
			completionJWT = uploadResult.JWT
		}
	}

	if completionJWT == "" {
		return "", fmt.Errorf("assets uploaded but no completion token received")
	}

	return completionJWT, nil
}

// ExportUploadAssets exports uploadAssets for testing.
func (g *Goflare) ExportUploadAssets(client *CfClient, manifest map[string]assetEntry, byHash map[string]string) (string, error) {
	return g.uploadAssets(client, manifest, byHash)
}
