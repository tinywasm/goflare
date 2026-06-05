//go:build !wasm

package goflare

import (
	"fmt"
	"net/http"
	"os"
)

var errNoToken = fmt.Errorf("CLOUDFLARE_API_TOKEN is not defined.\n" +
	"Deployment is executed in CI. Register the token in:\n" +
	"  GitHub → Settings → Secrets and variables → Actions → New repository secret\n" +
	"  Name: CLOUDFLARE_API_TOKEN\n\n" +
	"To validate a token locally before pasting it:\n" +
	"  CLOUDFLARE_API_TOKEN=cfat_... goflare auth --check")

func (g *Goflare) token() (string, error) {
	t := os.Getenv("CLOUDFLARE_API_TOKEN")
	if t == "" {
		return "", errNoToken
	}
	return t, nil
}

// Auth implements token validation.
func (g *Goflare) Auth() error {
	t, err := g.token()
	if err != nil {
		return err
	}
	return g.validateToken(t)
}

func (g *Goflare) validateToken(token string) error {
	client := &CfClient{
		Token:      token,
		HttpClient: http.DefaultClient,
		BaseURL:    g.BaseURL,
	}

	if _, err := client.get("/user/tokens/verify"); err != nil {
		return fmt.Errorf(
			"%w\n\nToken validation failed. Check:\n"+
				"  - Token is not expired\n"+
				"  - Token has Workers Scripts (Edit) + Pages (Edit) permissions\n"+
				"  - Token is for the correct Cloudflare account\n\n"+
				"Check instructions in GitHub → Settings → Secrets and variables → Actions",
			err,
		)
	}
	return nil
}
