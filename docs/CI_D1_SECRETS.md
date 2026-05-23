# CI — Configurar secrets para tests de integración D1

Los tests de integración (`//go:build integration`) se conectan a la API REST de Cloudflare D1.
En CI (GitHub Actions) las credenciales se inyectan como **Repository secrets**.

## Forma automática (recomendada)

```bash
goflare d1 init
```

Crea la DB D1, actualiza `.env`, y configura GitHub automáticamente via `gh` CLI.
Ver flujo completo en [CLOUDFLARE_GH_ENV_FLOW.md](diagrams/CLOUDFLARE_GH_ENV_FLOW.md).

## Forma manual

### Secret requerido (cifrado)

| Secret | Dónde obtenerlo |
|--------|----------------|
| `CLOUDFLARE_API_TOKEN` | [dash.cloudflare.com/profile/api-tokens](https://dash.cloudflare.com/profile/api-tokens) → Create Token → Edit Cloudflare Workers |

### Variables requeridas (públicas — no son secretos)

| Variable | Dónde obtenerlo |
|----------|----------------|
| `CLOUDFLARE_ACCOUNT_ID` | Dashboard CF → barra lateral derecha → Account ID |
| `D1_DATABASE_ID` | Workers & Pages → D1 → tu base de datos → Database ID |

`CLOUDFLARE_ACCOUNT_ID` es un identificador público — también está en [.env.example](../.env.example).

### Cómo agregar en GitHub

Ir a: `github.com/tinywasm/goflare` → **Settings** → **Secrets and variables** → **Actions**

![GitHub Actions secrets panel](img/github_env_managment.png)

- **Secret**: pestaña **Secrets** → **New repository secret** → `CLOUDFLARE_API_TOKEN`
- **Variables**: pestaña **Variables** → **New repository variable** → `CLOUDFLARE_ACCOUNT_ID` y `D1_DATABASE_ID`

## Comportamiento por entorno

| Entorno | Token disponible | Resultado del test |
|---------|-----------------|-------------------|
| Local con `goflare auth` | Keyring del SO | ✅ Corre |
| CI con secrets configurados | Env var inyectada | ✅ Corre |
| CI sin secrets / PR de fork | Env var vacía | ⏭ `t.Skip` — nunca falla |

## Por qué no se usa `if: secrets.X != ''` en el workflow

GitHub Actions no permite acceder al contexto `secrets` en condiciones `if:` por seguridad.
El test se auto-omite con `t.Skip()` cuando las credenciales no están disponibles — el step siempre termina con exit code 0.

## Local — autenticación

```bash
goflare auth   # guarda el token en el keyring del SO (solo se pide una vez)
```

Los tests leen el token directamente del keyring sin necesidad de variables de entorno:

```bash
CLOUDFLARE_ACCOUNT_ID=xxx D1_DATABASE_ID=yyy \
  go test -tags=integration -run TestD1Integration ./d1/ -v
```
