# Quick Reference Guide

A concise guide for using GoFlare for Go WASM deployment to Cloudflare.

## Commands

| Command | Description |
|---------|-------------|
| `goflare init` | Interactive setup to create `.env` and update `.gitignore`. |
| `goflare build` | Build Worker (WASM/JS) and/or prepare Pages static assets. |
| `goflare deploy` | Authenticate with Cloudflare and push artifacts. |

## Configuration Cheat Sheet (`.env` keys)

| Key | Description | Default |
|-----|-------------|---------|
| `PROJECT_NAME` | Project identifier for Cloudflare. | (Required) |
| `CLOUDFLARE_ACCOUNT_ID` | Your Cloudflare Account ID. | (Required) |
| `WORKER_NAME` | Name of the Worker script on Cloudflare. | `<ProjectName>-worker` |
| `DOMAIN` | Custom domain for Pages projects. | - |
| `ENTRY` | Path to Go main file (for Worker builds). | - |
| `PUBLIC_DIR` | Path to static assets (for Pages builds). | - |
| `COMPILER_MODE` | TinyGo optimization mode (`S`, `M`, or `L`). | `S` |

## Common Workflows

### Worker-Only Project
Set `ENTRY` and leave `PUBLIC_DIR` empty in `.env`.
```bash
goflare init   # provide Entry point
goflare build  # builds worker.wasm, worker.js
goflare deploy # pushes to Workers
```

### Pages-Only Project
Set `PUBLIC_DIR` and leave `ENTRY` empty in `.env`.
```bash
goflare init   # provide Public dir
goflare build  # copies static assets to .goflare/dist/
goflare deploy # pushes to Pages
```

### Full-Stack Project (Both)
Set both `ENTRY` and `PUBLIC_DIR` in `.env`.
```bash
goflare build  # builds both artifacts
goflare deploy # deploys to both Workers and Pages
```

## Troubleshooting

- **TinyGo Not Found:** Ensure `tinygo` is in your system `PATH`.
- **Invalid API Token:** Tokens must have `Workers Scripts` and `Pages` permissions.
- **Missing Account ID:** Find your Account ID in the Cloudflare Dashboard right sidebar.
- **Port Conflicts:** Ensure no other tools are locking `.goflare/` or its subdirectories.
