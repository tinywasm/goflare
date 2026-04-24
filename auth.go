package goflare

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// Auth implements token validation and keyring storage as a method.
func (g *Goflare) Auth(store Store, in io.Reader) error {
	key := "goflare/" + g.Config.ProjectName

	// 1. Keyring
	token, err := store.Get(key)
	if err == nil && token != "" {
		return nil
	}

	// 2. Env var — CI/CD, no se guarda en keyring (el orquestador gestiona el secreto)
	if t := os.Getenv("CLOUDFLARE_API_TOKEN"); t != "" {
		return g.validateToken(t)
	}

	// 3. Prompt interactivo
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Cloudflare API Token required.")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "  1. Go to: https://dash.cloudflare.com/profile/api-tokens")
	fmt.Fprintln(os.Stderr, "  2. Click \"Create Token\" → template \"Edit Cloudflare Workers\"")
	fmt.Fprintln(os.Stderr, "     Add permission: Cloudflare Pages — Edit")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "  Token is saved to your system keyring — only asked once per project.")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprint(os.Stderr, "Paste token: ")

	scanner := bufio.NewScanner(in)
	if !scanner.Scan() {
		return scanner.Err()
	}
	token = strings.TrimSpace(scanner.Text())

	if err := g.validateToken(token); err != nil {
		return err
	}

	if err := store.Set(key, token); err != nil {
		return fmt.Errorf("failed to save token: %w", err)
	}

	return nil
}

// GetToken reads the token from the store without prompting.
func (g *Goflare) GetToken(store Store) (string, error) {
	key := "goflare/" + g.Config.ProjectName
	token, err := store.Get(key)
	if err != nil || token == "" {
		return "", fmt.Errorf("not authenticated: call Auth first")
	}
	return token, nil
}

func (g *Goflare) validateToken(token string) error {
	client := &cfClient{
		token:      token,
		httpClient: http.DefaultClient,
		baseURL:    g.BaseURL,
	}

	if _, err := client.get("/user/tokens/verify"); err != nil {
		return fmt.Errorf(
			"%w\n\nToken validation failed. Check:\n"+
				"  - Token is not expired\n"+
				"  - Token has Workers Scripts (Edit) + Pages (Edit) permissions\n"+
				"  - Token is for the correct Cloudflare account\n\n"+
				"To reset saved token: goflare auth --reset",
			err,
		)
	}
	return nil
}
