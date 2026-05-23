//go:build !wasm

package goflare

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"strings"

	. "github.com/tinywasm/fmt"
)

var (
	ErrNoToken     = Err("not authenticated: run 'goflare auth' first")
	ErrNoAccountID = Err("CLOUDFLARE_ACCOUNT_ID missing: run 'goflare init' first")
	ErrNoDBName    = Err("database name is required (set PROJECT_NAME in .env)")
)

const (
	GHSecretToken   = "CLOUDFLARE_API_TOKEN"
	GHVarAccountID  = "CLOUDFLARE_ACCOUNT_ID"
	GHVarDatabaseID = "D1_DATABASE_ID"
)

// D1Manager abstracts the Cloudflare D1 REST API for listing and creating databases.
type D1Manager interface {
	ListD1Databases(accountID string) ([]D1Database, error)
	CreateD1Database(accountID, name string) (D1Database, error)
}

type D1Database struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
}

// GHRunner abstracts the gh CLI for setting GitHub secrets and variables.
type GHRunner interface {
	SetSecret(repo, name, value string) error
	SetVariable(repo, name, value string) error
	RemoteURL() (string, error)
	Available() bool
}

// EnvWriter abstracts writing a key=value pair to the .env file.
type EnvWriter interface {
	WriteKey(path, key, value string) error
}

// RunD1Init implements `goflare d1 init`.
func RunD1Init(envPath, dbName string, store Store, d1 D1Manager, gh GHRunner, w EnvWriter, out io.Writer) error {
	// 1. Load config from .env (needed to build the keyring key)
	cfg, _ := LoadConfigFromEnv(envPath)

	// 2. Token from keyring — key: "goflare/" + cfg.ProjectName
	token, err := store.Get("goflare/" + cfg.ProjectName)
	if err != nil || token == "" {
		return ErrNoToken
	}

	// 3. AccountID from .env
	if cfg.AccountID == "" {
		return ErrNoAccountID
	}

	// 4. DB name: flag > PROJECT_NAME
	if dbName == "" {
		dbName = cfg.ProjectName
	}
	if dbName == "" {
		return ErrNoDBName
	}

	// 5. List existing D1 databases
	dbs, err := d1.ListD1Databases(cfg.AccountID)
	if err != nil {
		return err
	}

	// 6. Find or create
	var dbID string
	for _, db := range dbs {
		if db.Name == dbName {
			dbID = db.UUID
			Fprintf(out, "D1: reusing existing database %q (%s)\n", dbName, dbID)
			break
		}
	}
	if dbID == "" {
		created, err := d1.CreateD1Database(cfg.AccountID, dbName)
		if err != nil {
			return err
		}
		dbID = created.UUID
		Fprintf(out, "D1: created database %q (%s)\n", dbName, dbID)
	}

	// 7. Write D1_DATABASE_ID to .env
	if err := w.WriteKey(envPath, EnvKeyD1DatabaseID, dbID); err != nil {
		return err
	}

	// 8. GitHub via gh CLI
	if !gh.Available() {
		Fprintf(out, "gh CLI not found. Set manually:\n")
		Fprintf(out, "  gh secret set %s --body %q\n", GHSecretToken, token)
		Fprintf(out, "  gh variable set %s --body %q\n", GHVarAccountID, cfg.AccountID)
		Fprintf(out, "  gh variable set %s --body %q\n", GHVarDatabaseID, dbID)
		return nil
	}

	repo, err := gh.RemoteURL()
	if err != nil {
		return err
	}
	gh.SetSecret(repo, GHSecretToken, token)
	gh.SetVariable(repo, GHVarAccountID, cfg.AccountID)
	gh.SetVariable(repo, GHVarDatabaseID, dbID)
	Fprintf(out, "GitHub: secrets and variables configured.\n")
	return nil
}

// Implementations

type cfD1Manager struct{ client *cfClient }

func (m *cfD1Manager) ListD1Databases(accountID string) ([]D1Database, error) {
	path := Sprintf("/accounts/%s/d1/database", accountID)
	data, err := m.client.get(path)
	if err != nil {
		return nil, err
	}
	var result []D1Database
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (m *cfD1Manager) CreateD1Database(accountID, name string) (D1Database, error) {
	path := Sprintf("/accounts/%s/d1/database", accountID)
	body, _ := json.Marshal(map[string]string{"name": name})
	data, err := m.client.post(path, body)
	if err != nil {
		return D1Database{}, err
	}
	var result D1Database
	if err := json.Unmarshal(data, &result); err != nil {
		return D1Database{}, err
	}
	return result, nil
}

type execGHRunner struct{}

func (g *execGHRunner) Available() bool {
	_, err := exec.LookPath("gh")
	return err == nil
}

func (g *execGHRunner) RemoteURL() (string, error) {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (g *execGHRunner) SetSecret(repo, name, value string) error {
	cmd := exec.Command("gh", "secret", "set", name, "--body", value, "--repo", repo)
	return cmd.Run()
}

func (g *execGHRunner) SetVariable(repo, name, value string) error {
	cmd := exec.Command("gh", "variable", "set", name, "--body", value, "--repo", repo)
	return cmd.Run()
}

type fileEnvWriter struct{}

func (e *fileEnvWriter) WriteKey(path, key, value string) error {
	if path == "" {
		path = ".env"
	}

	content, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	lines := strings.Split(string(content), "\n")
	found := false
	for i, line := range lines {
		if strings.HasPrefix(line, key+"=") {
			lines[i] = Sprintf("%s=%s", key, value)
			found = true
			break
		}
	}

	if !found {
		// Remove empty lines at the end to avoid multiple empty lines
		for len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
		lines = append(lines, Sprintf("%s=%s", key, value))
	}

	// Join with \n and ensure trailing newline
	output := strings.Join(lines, "\n")
	if !strings.HasSuffix(output, "\n") {
		output += "\n"
	}

	return os.WriteFile(path, []byte(output), 0644)
}
