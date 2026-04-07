package goflare

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

var cfBaseURL = "https://api.cloudflare.com/client/v4"

// SetCFBaseURL is for testing only.
func SetCFBaseURL(url string) string {
	old := cfBaseURL
	cfBaseURL = url
	return old
}

// Auth implementation.
func (g *Goflare) Auth(store Store, prompt io.Reader) error {
	token, err := store.Get("token")
	if err != nil || token == "" {
		scanner := bufio.NewScanner(prompt)
		fmt.Print("Cloudflare API Token: ")
		if !scanner.Scan() {
			return fmt.Errorf("failed to read token")
		}
		token = strings.TrimSpace(scanner.Text())

		if err := validateToken(token); err != nil {
			return fmt.Errorf("invalid token: %w", err)
		}

		if err := store.Set("token", token); err != nil {
			return fmt.Errorf("failed to save token: %w", err)
		}
	}
	return nil
}

// GetToken returns the token from the store.
func (g *Goflare) GetToken(store Store) (string, error) {
	token, err := store.Get("token")
	if err != nil || token == "" {
		return "", fmt.Errorf("token not found — call Auth first")
	}
	return token, nil
}

type cfVerifyResponse struct {
	Success bool     `json:"success"`
	Errors  []cfError `json:"errors"`
}

type cfError struct {
	Message string `json:"message"`
}

func validateToken(token string) error {
	req, err := http.NewRequest("GET", cfBaseURL+"/user/tokens/verify", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var verifyResp cfVerifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&verifyResp); err != nil {
		return err
	}

	if !verifyResp.Success {
		var msgs []string
		for _, e := range verifyResp.Errors {
			msgs = append(msgs, e.Message)
		}
		if len(msgs) == 0 {
			return fmt.Errorf("verification failed")
		}
		return fmt.Errorf("%s", strings.Join(msgs, ", "))
	}

	return nil
}
