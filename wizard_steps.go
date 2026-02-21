package goflare

import (
	"fmt"

	"github.com/tinywasm/context"
	"github.com/tinywasm/wizard"
)

const (
	ctxKeyAccountID      = "cf_account_id"
	ctxKeyBootstrapToken = "cf_bootstrap_token"
	ctxKeyProjectName    = "cf_project_name"
)

// GetSteps implements the interface expected by tinywasm/wizard.New().
// The wizard orchestrates three steps to configure Cloudflare Pages auth.
func (a *Auth) GetSteps() []*wizard.Step {
	return []*wizard.Step{
		{
			LabelText: "Cloudflare Account ID (dashboard.cloudflare.com → right sidebar)",
			OnInputFn: func(input string, ctx *context.Context) (bool, error) {
				if input == "" {
					return false, fmt.Errorf("account ID cannot be empty")
				}
				ctx.Set(ctxKeyAccountID, input)
				return true, nil
			},
		},
		{
			LabelText: "Bootstrap API Token (Cloudflare dashboard → My Profile → API Tokens → Create Token with 'Edit user API tokens' permission)",
			OnInputFn: func(input string, ctx *context.Context) (bool, error) {
				if len(input) < 20 {
					return false, fmt.Errorf("token looks too short — paste the full token")
				}
				ctx.Set(ctxKeyBootstrapToken, input)
				return true, nil
			},
		},
		{
			LabelText: "Cloudflare Pages project name (create it first at pages.cloudflare.com)",
			OnInputFn: func(input string, ctx *context.Context) (bool, error) {
				if input == "" {
					return false, fmt.Errorf("project name cannot be empty")
				}
				ctx.Set(ctxKeyProjectName, input)

				accountID := ctx.Value(ctxKeyAccountID)
				bootstrapToken := ctx.Value(ctxKeyBootstrapToken)

				if err := a.Setup(accountID, bootstrapToken, input); err != nil {
					return false, fmt.Errorf("Cloudflare setup failed: %w", err)
				}
				return true, nil
			},
		},
	}
}
