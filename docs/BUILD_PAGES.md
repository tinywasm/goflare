# Cloudflare Pages Build Guide

GoFlare handles building and deploying Go WASM projects to Cloudflare Pages without requiring any Node.js dependencies.

## Pages Overview

Cloudflare Pages allows hosting static sites. When using Go WASM, your application consists of a set of static files (HTML, JS, and WASM) served from a directory.

## Build Process

GoFlare executes the following steps during the Pages build:

1. **Verify Source Directory:** Checks if the `PublicDir` configured in `.env` exists.
2. **Copy Files:** Recursively copies all files and subdirectories from `PublicDir` to `.goflare/dist/`.
3. **Preserve Structure:** Maintains the exact directory hierarchy from your source.

## Configuration

Relevant `.env` keys for Pages builds:

- `PROJECT_NAME`: The name of your Cloudflare Pages project.
- `PUBLIC_DIR`: Path to your static assets (e.g., `web/public`).
- `DOMAIN`: (Optional) Custom domain to attach to your Pages project.

## Deployment

GoFlare uses the Cloudflare Pages Direct Upload v2 API. The deployment flow is:

1. **Project Verification:** Checks if the project exists on Cloudflare. If not, it is automatically created.
2. **Upload Token:** Requests a short-lived upload JWT for the project.
3. **Hashing & Batching:** Computes the SHA-256 hex hash of each file in `.goflare/dist/`.
4. **Asset Upload:** Uploads file contents (base64-encoded) in batches of 50.
5. **Deployment Creation:** Submits a manifest mapping file paths to their SHA-256 hashes to finalize the deployment.
6. **Domain Binding:** (Optional) Attaches the configured custom domain to the project.

## Content Type Detection

GoFlare includes a built-in content type detector for common web extensions:
- `.html`, `.css`, `.js`, `.json`
- `.png`, `.jpg`, `.svg`, `.ico`
- `.wasm`, `.txt`

Files with unknown extensions default to `application/octet-stream`.
