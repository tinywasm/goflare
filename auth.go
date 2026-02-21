package goflare

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	cfAPIBase        = "https://api.cloudflare.com/client/v4"
	keyringService   = "cloudflare"
	keyringPagesTok  = "pages_token"
	keyringAccountID = "account_id"
	keyringProject   = "pages_project"
)

// KeyManager manages secrets in the system keyring.
// Compatible with deploy.KeyManager — redeclared here to avoid coupling.
type KeyManager interface {
	Get(service, user string) (string, error)
	Set(service, user, password string) error
}

// Auth handles Cloudflare API authentication and scoped token creation.
type Auth struct {
	Keys    KeyManager
	log     func(...any)
	baseURL string // default: cfAPIBase; overridable for tests
}

// NewAuth creates an Auth instance backed by the provided KeyManager.
func NewAuth(keys KeyManager) *Auth {
	return &Auth{Keys: keys, log: func(...any) {}, baseURL: cfAPIBase}
}

// NewAuthWithBaseURL creates an Auth instance with a custom API base URL (for testing).
func NewAuthWithBaseURL(keys KeyManager, baseURL string) *Auth {
	return &Auth{Keys: keys, log: func(...any) {}, baseURL: baseURL}
}

func (a *Auth) SetLog(f func(...any)) { a.log = f }

// IsConfigured returns true if a scoped Pages token exists in the keyring.
func (a *Auth) IsConfigured() bool {
	tok, err := a.Keys.Get(keyringService, keyringPagesTok)
	return err == nil && tok != ""
}

// PagesToken retrieves the stored scoped Pages token.
func (a *Auth) PagesToken() (string, error) {
	return a.Keys.Get(keyringService, keyringPagesTok)
}

// AccountID retrieves the stored Cloudflare account ID.
func (a *Auth) AccountID() (string, error) {
	return a.Keys.Get(keyringService, keyringAccountID)
}

// ProjectName retrieves the stored Cloudflare Pages project name.
func (a *Auth) ProjectName() (string, error) {
	return a.Keys.Get(keyringService, keyringProject)
}

// Setup uses a bootstrap token to create a scoped Pages:Edit token,
// stores it in the keyring, and discards the bootstrap token.
// accountID and bootstrapToken come from the wizard steps.
func (a *Auth) Setup(accountID, bootstrapToken, projectName string) error {
	a.log("Cloudflare: fetching permission groups...")

	permID, err := a.findPagesEditPermission(bootstrapToken)
	if err != nil {
		return fmt.Errorf("cloudflare: find Pages:Edit permission: %w", err)
	}

	a.log("Cloudflare: creating scoped Pages token...")

	scopedToken, err := a.createScopedToken(bootstrapToken, accountID, permID)
	if err != nil {
		return fmt.Errorf("cloudflare: create scoped token: %w", err)
	}

	if err := a.Keys.Set(keyringService, keyringAccountID, accountID); err != nil {
		return fmt.Errorf("cloudflare: store account_id: %w", err)
	}
	if err := a.Keys.Set(keyringService, keyringPagesTok, scopedToken); err != nil {
		return fmt.Errorf("cloudflare: store pages_token: %w", err)
	}
	if err := a.Keys.Set(keyringService, keyringProject, projectName); err != nil {
		return fmt.Errorf("cloudflare: store pages_project: %w", err)
	}

	a.log("Cloudflare: setup complete — scoped token stored in keyring.")
	return nil
}

// cfResponse is the generic Cloudflare API envelope.
type cfResponse struct {
	Success bool              `json:"success"`
	Errors  []cfError         `json:"errors"`
	Result  json.RawMessage   `json:"result"`
}

type cfError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// findPagesEditPermission calls GET /user/tokens/permission_groups and returns
// the ID of the first permission group whose name contains "Pages" and "Edit".
func (a *Auth) findPagesEditPermission(bootstrapToken string) (string, error) {
	body, err := a.cfGET("/user/tokens/permission_groups", bootstrapToken)
	if err != nil {
		return "", err
	}

	var groups []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &groups); err != nil {
		return "", fmt.Errorf("parse permission groups: %w", err)
	}

	for _, g := range groups {
		if strings.Contains(g.Name, "Pages") && strings.Contains(g.Name, "Edit") {
			return g.ID, nil
		}
	}
	return "", fmt.Errorf("Pages:Edit permission group not found")
}

// createScopedToken calls POST /user/tokens to create a Pages:Edit scoped token.
func (a *Auth) createScopedToken(bootstrapToken, accountID, permID string) (string, error) {
	payload := map[string]any{
		"name": "tinywasm-pages-deploy",
		"policies": []map[string]any{
			{
				"effect": "allow",
				"permission_groups": []map[string]string{
					{"id": permID},
				},
				"resources": map[string]string{
					"com.cloudflare.api.account": accountID,
				},
			},
		},
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	result, err := a.cfPOST("/user/tokens", bootstrapToken, raw)
	if err != nil {
		return "", err
	}

	var tok struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(result, &tok); err != nil {
		return "", fmt.Errorf("parse token response: %w", err)
	}
	if tok.Value == "" {
		return "", fmt.Errorf("token value empty in response")
	}
	return tok.Value, nil
}

func (a *Auth) cfGET(path, token string) (json.RawMessage, error) {
	req, _ := http.NewRequest(http.MethodGet, a.baseURL+path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()

	return a.parseResponse(resp)
}

func (a *Auth) cfPOST(path, token string, body []byte) (json.RawMessage, error) {
	req, _ := http.NewRequest(http.MethodPost, a.baseURL+path, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("POST %s: %w", path, err)
	}
	defer resp.Body.Close()

	return a.parseResponse(resp)
}

func (a *Auth) parseResponse(resp *http.Response) (json.RawMessage, error) {
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var envelope cfResponse
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("parse envelope: %w", err)
	}

	if !envelope.Success {
		if len(envelope.Errors) > 0 {
			return nil, fmt.Errorf("CF API error %d: %s", envelope.Errors[0].Code, envelope.Errors[0].Message)
		}
		return nil, fmt.Errorf("CF API returned success=false")
	}
	return envelope.Result, nil
}
