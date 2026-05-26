# CI con GitHub Actions

GoFlare está diseñado para que el despliegue ocurra exclusivamente en CI.

## Workflow de ejemplo

Crea `.github/workflows/deploy.yml`:

```yaml
name: Deploy
on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: 'stable'

      - name: Build and Deploy
        env:
          CLOUDFLARE_API_TOKEN: ${{ secrets.CLOUDFLARE_API_TOKEN }}
          CLOUDFLARE_ACCOUNT_ID: ${{ secrets.CLOUDFLARE_ACCOUNT_ID }}
        run: |
          go run github.com/tinywasm/goflare/cmd/goflare@latest build
          go run github.com/tinywasm/goflare/cmd/goflare@latest deploy
```

## Configuración de Secretos

Asegúrate de haber configurado los secretos en GitHub como se describe en [CI_D1_SECRETS.md](CI_D1_SECRETS.md).
`CLOUDFLARE_ACCOUNT_ID` debe ser un Secret para mayor seguridad.
