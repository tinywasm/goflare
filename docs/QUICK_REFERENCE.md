# Quick Reference

## Commands

| Command | Description |
|---|---|
| `goflare auth --check` | Validate `CLOUDFLARE_API_TOKEN` from environment. |
| `goflare build` | Build the project (wasm + assets). Inferred mode. |
| `goflare deploy` | Deploy to Cloudflare. Requires env vars. CI only. |

## Configuration (.env)

| Key | Description |
|---|---|
| `PROJECT_NAME` | Identity of the project in Cloudflare. |
| `DOMAIN` | Custom domain for Pages (optional). |
| `COMPILER_MODE` | `S` (Small), `M` (Medium), `L` (Large). Default: `S`. |

## GitHub Secrets / Variables

| Name | Type | Description |
|---|---|---|
| `CLOUDFLARE_API_TOKEN` | Secret | dash.cloudflare.com → Profile → API Tokens → Create Token (Edit Workers) |
| `CLOUDFLARE_ACCOUNT_ID` | Secret | Cloudflare Dashboard → Right sidebar |
| `D1_DATABASE_ID` | Variable | Workers & Pages → D1 → Database ID |

## Conventions (Not configurable)

| Item | Path |
|---|---|
| Go Entry | `edge/main.go` |
| Public Assets | `web/public/` |
| Pages Functions | `functions/` |
| Worker Artifacts | `.build/` |
