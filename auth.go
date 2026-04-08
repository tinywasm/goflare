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
	token, err := store.Get(key)
	if err != nil || token == "" {
		fmt.Fprint(os.Stderr, "Cloudflare API Token: ")
		scanner := bufio.NewScanner(in)
		if !scanner.Scan() {
			return scanner.Err()
		}
		token = strings.TrimSpace(scanner.Text())

		if err := g.validateToken(token); err != nil {
			return fmt.Errorf("invalid token: %w", err)
		}

		if err := store.Set(key, token); err != nil {
			return fmt.Errorf("failed to save token: %w", err)
		}
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

	_, err := client.get("/user/tokens/verify")
	return err
}
