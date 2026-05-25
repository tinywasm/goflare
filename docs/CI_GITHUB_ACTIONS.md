# CI/CD con GitHub Actions (alternativa avanzada)

> ⚠️ **No es el flujo recomendado.** El flujo principal de goflare es **Cloudflare Git Integration** + artefactos commiteados (ver [PLAN.md §6](./PLAN.md#6-despliegue-cloudflare-git-integration), D7-D8). No requiere tokens en ningún lado.
>
> Este documento existe solo para casos que necesiten explícitamente Actions: gates de tests bloqueantes en cada PR, preview deploys complejos, auditoría exhaustiva por SHA, o políticas corporativas que prohíban la GitHub App de Cloudflare.

## Por qué arrancar con Actions desde día 1

| Razón | Detalle |
|---|---|
| Token solo en GH Secrets | Nunca en `.env`, nunca en shell history, nunca en backups de devs. |
| Sumar colaboradores es trivial | El nuevo dev clona, pushea, deploy automático — sin distribuir credenciales. |
| Migrar después es caro | Si arrancás local y luego sumás devs, hay que rotar el token (estuvo en `.env`), educar al equipo, ajustar el flujo. Empezar con Actions evita esa deuda. |
| Preview deploys por PR | URL única por branch para QA — solo posible orquestando desde Actions. |
| Auditoría / SHA del commit desplegado | Log inmutable de "quién desplegó qué cuándo". |

`goflare deploy` desde local sigue funcionando — útil para hotfix o spike inicial — pero no es el flujo recomendado para proyectos persistentes.

## Setup

### 1. Scaffolding con goflare

```bash
goflare init --mode=pages-functions --ci=github
```

Esto agrega:

```
.github/workflows/
├── deploy.yml          # despliegue a producción en push a main
└── preview.yml         # preview deploy por PR (opcional)
```

Para proyectos ya scaffoldeados sin `--ci`, regenerar solo el workflow:

```bash
goflare ci github
```

### 2. Configurar GitHub Secrets

En el repo: **Settings → Secrets and variables → Actions → New repository secret**.

| Secret | Valor | De dónde sacarlo |
|---|---|---|
| `CLOUDFLARE_API_TOKEN` | Token con permisos `Account: Cloudflare Pages: Edit` | `dash.cloudflare.com` → My Profile → API Tokens → Create Token |
| `CLOUDFLARE_ACCOUNT_ID` | ID de tu cuenta CF | Sidebar derecho de cualquier vista de Cloudflare |

**No** copies estos valores al `.env` del repo. Quedan solo en GH Secrets.

### 3. (Opcional) Configurar variables de entorno por entorno

Si tu app necesita secrets en runtime (ej. `RESEND_API_KEY`), esos van **dentro de Cloudflare Pages** (Dashboard → tu proyecto → Settings → Environment variables), NO en GH Secrets. GH Secrets son solo para el build/deploy; el runtime lee de `context.env`.

## Workflows generados

### `.github/workflows/deploy.yml` (producción)

```yaml
name: Deploy to Cloudflare Pages
on:
  push:
    branches: [main]
  workflow_dispatch:

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.25.2'

      - name: Setup TinyGo
        # Única forma válida de compilar WASM para el edge — no usar go build estándar
        uses: acifani/setup-tinygo@v2
        with:
          tinygo-version: '0.41.1'

      - name: Install goflare
        # go install funciona porque el módulo está publicado en pkg.go.dev.
        # Cuando haya GitHub Releases con binarios, reemplazar por: gh release download
        run: go install github.com/tinywasm/goflare/cmd/goflare@latest

      - name: Build
        run: goflare build

      - name: Deploy
        env:
          CLOUDFLARE_API_TOKEN:  ${{ secrets.CLOUDFLARE_API_TOKEN }}
          CLOUDFLARE_ACCOUNT_ID: ${{ secrets.CLOUDFLARE_ACCOUNT_ID }}
        run: goflare deploy
```

### `.github/workflows/preview.yml` (preview por PR)

```yaml
name: Preview Deploy
on:
  pull_request:
    branches: [main]

jobs:
  preview:
    runs-on: ubuntu-latest
    permissions:
      pull-requests: write
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.25.2' }
      - uses: acifani/setup-tinygo@v2
        with: { tinygo-version: '0.41.1' }
      - run: go install github.com/tinywasm/goflare/cmd/goflare@latest
      - run: goflare build

      - name: Deploy preview
        id: deploy
        env:
          CLOUDFLARE_API_TOKEN:  ${{ secrets.CLOUDFLARE_API_TOKEN }}
          CLOUDFLARE_ACCOUNT_ID: ${{ secrets.CLOUDFLARE_ACCOUNT_ID }}
        run: goflare deploy --branch="$GITHUB_HEAD_REF" --output-url

      - name: Comment PR with preview URL
        uses: actions/github-script@v7
        with:
          script: |
            const url = process.env.PREVIEW_URL;
            github.rest.issues.createComment({
              issue_number: context.issue.number,
              owner: context.repo.owner,
              repo: context.repo.repo,
              body: `🚀 Preview: ${url}`
            });
        env:
          PREVIEW_URL: ${{ steps.deploy.outputs.url }}
```

> **Pendiente de implementación en goflare**: el flag `--branch=<name>` y `--output-url` aún no existen. Documentados aquí como dependencia del workflow de preview.

## Test gate (opcional)

Para bloquear el deploy si los tests fallan, agregá un step antes de `goflare build`:

```yaml
- name: Test
  run: go test ./...
```

Si `go test` falla, el job entero falla y no se despliega.

## Token mínimo de Cloudflare

Cuando generes el API token en CF, usá **Custom Token** con estos permisos:

- **Account → Cloudflare Pages: Edit**
- (Si usás dominio custom) **Zone → DNS: Edit** sobre la zona específica

No uses el "Global API Key" — demasiado amplio.

## Troubleshooting

| Síntoma | Causa probable |
|---|---|
| `Error: unauthorized` en deploy | Token sin permiso `Pages: Edit` o account ID incorrecto. |
| `tinygo: command not found` | El step `setup-tinygo` falló o la versión no está disponible. Probá una versión más vieja. |
| Build OK pero el endpoint devuelve 500 | Secrets de runtime (`RESEND_API_KEY` etc.) no están en CF Pages → Environment variables. Recordá: GH Secrets ≠ CF env. |
| El preview por PR no comenta la URL | `permissions: pull-requests: write` falta o `GITHUB_TOKEN` está restringido en Settings → Actions → General. |

## Migrar de Actions a local-only

Si arrancaste con Actions y luego querés volver al flujo local:

```bash
rm -rf .github/workflows
# y remové los GH Secrets desde el dashboard
```

`goflare deploy` desde local sigue funcionando idéntico — Actions no toca nada del runtime ni de la configuración de Cloudflare.
