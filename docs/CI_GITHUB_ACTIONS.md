# CI con GitHub Actions

GoFlare está diseñado para que el despliegue ocurra exclusivamente en CI.

## Variables requeridas

| Variable | Tipo GitHub | Descripción |
|---|---|---|
| `CLOUDFLARE_API_TOKEN` | Secret | Token con permiso **Cloudflare Pages: Edit** |
| `CLOUDFLARE_ACCOUNT_ID` | Secret | ID de tu cuenta Cloudflare (no es secreto, pero se trata como tal por convención) |
| `PROJECT_NAME` | En el workflow | Nombre del proyecto de Cloudflare Pages (valor público, hardcodear en el workflow) |

## Cómo crear el API Token

`goflare deploy` solo usa la API de Cloudflare Pages
(`/accounts/{id}/pages/projects/{name}/deployments`).
El token debe tener **exactamente** este permiso:

| Tipo de token | Dónde crearlo | Permiso requerido |
|---|---|---|
| **User API Token** | Dashboard → My Profile → API Tokens → Create Token | **Cloudflare Pages: Edit** |

> **Account API Tokens** (Dashboard → Manage Account → API Tokens) también funcionan,
> pero Cloudflare los recomienda para credenciales no asociadas a un usuario específico.
> Para uso personal en CI, el User API Token es más simple.

**Pasos exactos:**
1. Ir a [dash.cloudflare.com/profile/api-tokens](https://dash.cloudflare.com/profile/api-tokens)
2. **Create Token** → **Use template** → busca **"Edit Cloudflare Pages"** (o crea custom)
3. Permisos mínimos:
   - `Account` → `Cloudflare Pages` → `Edit`
4. Seleccionar la cuenta correcta en "Account Resources"
5. Crear el token y copiarlo (solo se muestra una vez)

**Configurar en GitHub:**
```bash
gh secret set CLOUDFLARE_API_TOKEN --body "tu-token"
gh secret set CLOUDFLARE_ACCOUNT_ID --body "tu-account-id"
```

Para obtener tu Account ID:
```bash
# Via goflare MCP o dashboard: dash.cloudflare.com → sidebar inferior izquierdo
```

## Error frecuente: Account API Token con permisos de Page Shield

El token `TINYWASM_TOKEN` o similares creados desde **Manage Account → API Tokens**
con permisos de "Domain Page Shield" / "Page Shield" NO funcionan para deploy —
son permisos de seguridad web, no de despliegue.

Síntoma: `CF API error: Invalid API Token (code: 1000)`
Solución: Crear un nuevo **User API Token** con `Cloudflare Pages: Edit`.

## Workflow de ejemplo

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

      - name: Install goflare
        run: |
          curl -fsSL https://github.com/tinywasm/goflare/releases/download/v0.2.23/goflare-linux-amd64 \
            -o /usr/local/bin/goflare
          chmod +x /usr/local/bin/goflare

      - name: Build
        run: goflare build

      - name: Deploy
        env:
          CLOUDFLARE_API_TOKEN: ${{ secrets.CLOUDFLARE_API_TOKEN }}
          CLOUDFLARE_ACCOUNT_ID: ${{ secrets.CLOUDFLARE_ACCOUNT_ID }}
          PROJECT_NAME: my-project-name
        run: goflare deploy
```
