> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.

# Plan: goflare — Simplificación de secretos + CI de tests

## Contexto y motivación

Hoy goflare gestiona el token de Cloudflare de forma **insegura y compleja**:

- `goflare auth` ([auth.go:41](../auth.go#L41)) lee el token con `bufio.NewScanner(stdin)`.
  La terminal hace **echo** del input → el token aparece en pantalla al pegarlo
  (`cfat_zM...`). También queda en el historial/scrollback.
- El token se persiste en el **keyring del SO** ([store.go](../store.go)), y el
  `SECRETS_PLAN.md` proponía aún más complejidad (`goflare secrets push`,
  `SecretManager`, sincronización keyring↔GitHub↔Cloudflare).
- Esto multiplica las superficies donde vive un secreto: terminal, keyring, posible
  `.env`. Cada una es un sitio donde se puede filtrar.

**Decisión** (este plan reemplaza y descarta `SECRETS_PLAN.md`):

1. **Cero gestión local de secretos.** Se elimina el keyring por completo y todo
   prompt interactivo de token. No hay `goflare auth` que guarde nada.
2. **El secreto solo vive en la plataforma.** El `CLOUDFLARE_API_TOKEN` se registra
   **manualmente** en GitHub → Settings → Secrets and variables → Actions. Nunca en
   `.env`, nunca en keyring, nunca en disco local.
3. **El deploy ocurre solo en CI.** `goflare deploy` requiere `CLOUDFLARE_API_TOKEN`
   en el entorno (lo provee el runner de GitHub Actions). No hay prompt ni fallback;
   sin la env var falla con un mensaje accionable. Localmente goflare solo hace `build`
   (y `auth --check` para validar un token). La CLI final es `auth` / `build` / `deploy`.
4. **`.env` local solo lleva una entrada**: `PROJECT_NAME` (identidad no sensible del
   proyecto Cloudflare). Todo lo demás es convención o vive en la plataforma:
   - `ENTRY` y `PUBLIC_DIR` **no son configurables** → convención fija: `edge/` para el
     entry del Worker y `web/public/` para los assets. Se auto-detectan; se eliminan de
     `.env` y de los prompts de `init`.
   - `CLOUDFLARE_ACCOUNT_ID` es **secreto** → GitHub Secret.
   - `CLOUDFLARE_API_TOKEN` es **secreto** → GitHub Secret.
   - `D1_DATABASE_ID` es **variable** (no secreto) → la DB se crea **manualmente** en el
     dashboard de Cloudflare y su ID se registra **manualmente** como GitHub Variable.
     goflare no tiene comando `d1 init` (se elimina).

### Por qué esto es lo correcto

Es el modelo estándar de la industria (12-factor / "secrets belong to the platform"):
el orquestador de CI/CD es el guardián del secreto, lo inyecta como env var en el
runner, y nunca toca el disco del desarrollador. GitHub Secrets ya está cifrado en
reposo y enmascarado en logs. Duplicarlo en un keyring local solo añade superficie de
fuga sin ganancia.

---

## Aclaración de las dos dudas del usuario

### "El test no compila edge.wasm ni edge.js — ¿cómo pasa?"

El test de integración ([d1/d1_integration_test.go:66](../d1/d1_integration_test.go#L66))
llama `d1.NewDirect(token, accountID, dbID)` y hace **CRUD directo contra la REST API
de D1**. No despliega un Worker ni necesita `edge.wasm`/`edge.js`. Prueba exactamente
el adaptador D1 que el Worker usa en runtime, pero por HTTP desde el runner — no a
través del binario edge. Por eso pasa sin esos archivos: **no los necesita.**

Alcance confirmado: **se mantiene este test D1-CRUD** como única prueba de
integración. Un test end-to-end real (build → deploy → HTTP al `*.workers.dev`) queda
fuera de alcance.

### "La DB ya está creada y vacía — quiero que cada corrida la deje limpia"

El test ya es **autocontenido y reentrante**: crea la tabla `_goflare_test`, hace el
ciclo CRUD, y la dropea en `t.Cleanup`
([d1_integration_test.go:76](../d1/d1_integration_test.go#L76)). No toca otras tablas.
Se mantiene este enfoque de **tabla temporal aislada** (no un wipe destructivo de toda
la DB): es seguro aunque una corrida previa muera a medias, porque `CreateTable` es
idempotente vía `CREATE TABLE IF NOT EXISTS` y el cleanup la elimina.

---

## Stages

### Stage 1 — `store.go`: eliminar `KeyringStore`

- Borrar `KeyringStore`, `NewKeyringStore`, el import `go-keyring`.
- La interfaz `Store` y los métodos de deploy que reciben `store Store` se eliminan
  (ver Stage 3). Mantener **`MemoryStore`** solo si algún test lo sigue necesitando;
  si no queda ningún consumidor tras Stage 3, borrar el archivo entero.

### Stage 2 — `auth.go`: solo validar, nunca guardar

- Eliminar el prompt interactivo y `store.Set`.
- `Auth` se reduce a: leer `os.Getenv("CLOUDFLARE_API_TOKEN")` y llamar
  `validateToken`. Sin env var → error accionable:
  ```
  CLOUDFLARE_API_TOKEN no está definido.
  El deploy se ejecuta en CI. Registra el token en:
    GitHub → Settings → Secrets and variables → Actions → New repository secret
    Nombre: CLOUDFLARE_API_TOKEN
  Para validar un token localmente antes de pegarlo:
    CLOUDFLARE_API_TOKEN=cfat_... goflare auth --check
  ```
- Eliminar `GetToken(store)`. Añadir helper sin estado:
  ```go
  func (g *Goflare) token() (string, error) {
      t := os.Getenv("CLOUDFLARE_API_TOKEN")
      if t == "" {
          return "", errNoToken // mensaje accionable de arriba
      }
      return t, nil
  }
  ```

### Stage 3 — `cloudflare.go` + `run.go`: deploy lee env var, sin `Store`

- `DeployWorker` y `DeployPages` ([cloudflare.go:76](../cloudflare.go#L76),
  [cloudflare.go:276](../cloudflare.go#L276)) cambian de `(store Store)` a `()` y usan
  `g.token()` en vez de `g.GetToken(store)`.
- `RunDeploy` ([run.go:89](../run.go#L89)): quitar `NewKeyringStore()` y
  `g.Auth(store, in)`. Validar `g.token()` al inicio; si falta, devolver el error
  accionable. El parámetro `in io.Reader` deja de usarse → quitarlo.
- `RunAuth` ([run.go:13](../run.go#L13)): quitar `--reset` (ya no hay nada que borrar).
  `goflare auth` y `goflare auth --check` validan el token de la env var.

### Stage 4 — Eliminar `goflare d1 init` por completo

La DB D1 se crea **manualmente en el dashboard de Cloudflare**, y su `D1_DATABASE_ID`
se registra **manualmente como variable de entorno del proyecto en GitHub** (`vars`).
goflare no automatiza nada de esto.

- Borrar `d1init.go` y `tests/d1init_test.go`.
- `run.go`: borrar `RunD1InitCmd` ([run.go:157](../run.go#L157)) y todo helper exclusivo
  de d1 init (`cfD1Manager`, `execGHRunner`, `fileEnvWriter`, `RunD1Init`) si no los usa
  nadie más.
- `cmd/goflare/main.go`: borrar el `case "d1"` ([main.go:44](../cmd/goflare/main.go#L44)).
- `config.go`: `D1_DATABASE_ID` ya no se escribe ni se gestiona localmente. El runtime
  del Worker lo lee de su env var de Cloudflare. Mantener `D1DatabaseID` en `Config`
  solo si el código de runtime/deploy lo necesita; si no, quitarlo.
- Revisar que `gh` CLI ya no sea dependencia de ningún flujo de goflare.

### Stage 5 — Eliminar `goflare init`

Tras quitar token, D1 y ENTRY/PUBLIC_DIR (convención), `init` solo escribiría una línea
`PROJECT_NAME` en `.env` — no justifica un comando. Se elimina; el dev copia
`.env.example` a `.env` y edita `PROJECT_NAME`.

- Borrar `init.go` y `tests/init_test.go`.
- `run.go`: borrar `RunInit` ([run.go:46](../run.go#L46)). Si `WriteEnvFile` /
  `UpdateGitignore` / `Init` quedan sin consumidores, borrarlos también.
- `cmd/goflare/main.go`: borrar el `case "init"` ([main.go:30](../cmd/goflare/main.go#L30)).

### Stage 6 — `cmd/goflare/main.go` + `Usage()`

- `auth`: quitar flag `--reset`; conservar `--check`.
- Quitar `init` y `d1` del usage.
- Actualizar `Usage()`: `auth` = "validate CLOUDFLARE_API_TOKEN from env"; documentar
  que `deploy` requiere la env var y está pensado para CI. Comandos finales:
  `auth`, `build`, `deploy`.

### Stage 7 — Test de integración: quitar keyring

- `d1/d1_integration_test.go` `resolveToken` ([:38](../d1/d1_integration_test.go#L38)):
  eliminar el import `go-keyring` y la lectura del keyring. Token solo desde
  `os.Getenv(envKeyToken)`; si vacío → `t.Skip`.

### Stage 8 — `.github/workflows/test.yml`: arreglar el job

Problemas actuales:
- `if: ${{ secrets.CLOUDFLARE_API_TOKEN != '' }}` ([test.yml:25](../.github/workflows/test.yml#L25))
  es **inválido**: GitHub no permite el contexto `secrets` en `if:` (el propio
  `CI_D1_SECRETS.md` lo documenta). El step nunca evalúa bien la condición.

Arreglo: quitar el `if:`, pasar siempre los secrets/vars como env (string vacío si no
están), y dejar que el test se auto-omita con `t.Skip` cuando el token esté vacío.

```yaml
      - name: Integration tests (D1)
        env:
          CLOUDFLARE_API_TOKEN: ${{ secrets.CLOUDFLARE_API_TOKEN }}
          CLOUDFLARE_ACCOUNT_ID: ${{ secrets.CLOUDFLARE_ACCOUNT_ID }}
          D1_DATABASE_ID: ${{ vars.D1_DATABASE_ID }}
        run: go test -tags=integration -run TestD1Integration ./d1/ -v
```

Notas:
- `CLOUDFLARE_API_TOKEN` y `CLOUDFLARE_ACCOUNT_ID` son **secrets**; `D1_DATABASE_ID` es
  una **variable** (`vars`).
- No se instala TinyGo: ni el test de integración ni los tests unitarios (que usan
  modo pages-static / entradas fake) ejecutan compilación real de edge.wasm.
- El step `go test ./...` ya cubre los tests unitarios; el de integración corre aparte.

### Stage 9 — Convención: `ENTRY` y `PUBLIC_DIR` dejan de ser configurables

- `config.go`: eliminar `EnvKeyEntry` y `EnvKeyPublicDir` del parser de `.env` y de los
  fallbacks de OS env. `Entry` y `PublicDir` ya no se leen de config.
- `applyDefaults` ([config.go:128](../config.go#L128)): fijar por convención —
  - `Entry = "edge"` si existe `edge/main.go` (ya está).
  - `PublicDir = "web/public"` si existe ese directorio (nuevo auto-detect; antes solo
    venía del prompt de `init`).
- `goflare.go`: los comentarios de `Entry`/`PublicDir` en `Config` pasan a "convención,
  no configurable". (Nota: `init.go` ya se elimina en Stage 5.)

### Stage 10 — Documentación y diagramas

#### `.env.example`

Una sola entrada. El agente actualiza el archivo:

```
# Única config local. Identidad del proyecto Cloudflare (Pages/Worker/D1 db default).
PROJECT_NAME=my-app

# NO van aquí — se gestionan manualmente en GitHub:
#   Settings → Secrets:   CLOUDFLARE_API_TOKEN, CLOUDFLARE_ACCOUNT_ID
#   Settings → Variables: D1_DATABASE_ID  (crear la DB en Cloudflare dash, copiar el ID)
# ENTRY (edge/) y PUBLIC_DIR (web/public/) son convención fija, no configurables.
```

#### Archivos a **borrar**

| Archivo | Motivo |
|---|---|
| `docs/SECRETS_PLAN.md` | Superseded por este plan. Toda la propuesta de keyring + secrets push queda descartada. |
| `docs/D1.md` | Documentaba `goflare d1 init`. Ya no existe el comando. |
| `docs/diagrams/CLOUDFLARE_GH_ENV_FLOW.md` | Documentaba el flujo de `goflare d1 init` + configuración automática de secrets. Eliminado junto con el comando. |
| `docs/diagrams/AUTH_FLOW.md` | Documentaba el flujo de keyring (prompt → validar → guardar). Con el nuevo modelo ya no hay flujo de auth que diagramar — `goflare auth --check` es un one-liner. |

#### Archivos a **reescribir**

**`docs/QUICK_REFERENCE.md`** — es la referencia principal, cambios mayores:
- Tabla de comandos: quitar `init`, `d1`; `auth` pasa a "valida `CLOUDFLARE_API_TOKEN`
  del entorno (sin guardar nada)"; `deploy` = "CI only, requiere env var".
- Tabla de configuración `.env`: dejar solo `PROJECT_NAME`. Quitar `CLOUDFLARE_ACCOUNT_ID`,
  `WORKER_NAME`, `DOMAIN`, `ENTRY`, `PUBLIC_DIR`, `FUNCTIONS_DIR`, `COMPILER_MODE` como
  filas de `.env` (algunos ya no son configurables; el resto se gestiona en GitHub/CF).
- Añadir sección "GitHub Secrets / Variables requeridos":

  | Nombre | Tipo | Dónde obtenerlo |
  |---|---|---|
  | `CLOUDFLARE_API_TOKEN` | Secret | dash.cloudflare.com → Profile → API Tokens → Create Token (Edit Workers) |
  | `CLOUDFLARE_ACCOUNT_ID` | Secret | Dashboard CF → barra lateral derecha |
  | `D1_DATABASE_ID` | Variable | Workers & Pages → D1 → tu base → Database ID |

**`docs/CI_D1_SECRETS.md`** — reescribir:
- Quitar "Forma automática" (era `goflare d1 init`).
- Quitar toda mención a keyring / `goflare auth` que guarda token.
- Dejar solo: pasos manuales para crear la DB D1 en el dashboard y registrar los
  tres valores en GitHub (dos Secrets + una Variable), con la imagen ya existente
  (`img/github_env_managment.png`).
- Corregir la tabla de comportamiento por entorno: "Local con `goflare auth`" → eliminar;
  "CI sin secrets / PR de fork" → sigue siendo `t.Skip`.

**`docs/CI_GITHUB_ACTIONS.md`** — reescribir:
- Quitar toda referencia a deploy local y a keyring.
- Actualizar el setup de secrets: `CLOUDFLARE_ACCOUNT_ID` pasa a Secret (no var).
- Simplificar: el flujo principal ES GitHub Actions, ya no es "alternativa avanzada".

**`docs/ARCHITECTURE.md`** — actualizar:
- Quitar `goflare init` y `goflare auth` (keyring) del diagrama de componentes y flujo.
- Añadir nota sobre convención `edge/` y `web/public/`.

**`docs/diagrams/DEPLOY_FLOW.md`** — actualizar:
- Verificar y quitar cualquier paso de "leer token del keyring". El token ahora viene
  exclusivamente de `CLOUDFLARE_API_TOKEN` env var.

**`docs/diagrams/goflare-generic.md`** — actualizar:
- Quitar nodos de `init`, `auth` con keyring, `d1 init`. CLI queda como tres entradas:
  `auth --check`, `build`, `deploy`.

**`README.md`** — actualizar:
- Sección "Getting started" / "Usage": quitar `goflare init`, `goflare auth` (keyring),
  `goflare d1 init`.
- Añadir sección breve "Setup en GitHub" apuntando a `CI_D1_SECRETS.md`.
- Comandos: `auth --check` / `build` / `deploy`.

---

## Resumen de archivos

| Archivo | Acción |
|---|---|
| `store.go` | Eliminar `KeyringStore`/`NewKeyringStore`/import keyring; quizá borrar archivo |
| `auth.go` | Quitar prompt + keyring; `token()` lee env var; `--check` valida |
| `cloudflare.go` | `DeployWorker`/`DeployPages` sin `Store`, usan `g.token()` |
| `run.go` | `RunDeploy`/`RunAuth` sin keyring; borrar `RunD1InitCmd` y helpers d1; `Usage()` |
| `d1init.go` + `tests/d1init_test.go` | **Borrar** |
| `init.go` + `tests/init_test.go` | **Borrar**; borrar `RunInit`/`WriteEnvFile`/`UpdateGitignore` si quedan huérfanos |
| `cmd/goflare/main.go` | Borrar `case "init"` y `case "d1"`; quitar `auth --reset` |
| `config.go` | Quitar `ENTRY`/`PUBLIC_DIR` del parser; auto-detect `web/public/`; `D1_DATABASE_ID` ya no se gestiona local |
| `d1/d1_integration_test.go` | `resolveToken` sin keyring |
| `.github/workflows/test.yml` | Quitar `if: secrets`; `ACCOUNT_ID` → `secrets` |
| `.env.example` | Una sola línea `PROJECT_NAME`; todo lo demás va a GitHub |
| `docs/SECRETS_PLAN.md` | **Borrar** |
| `docs/D1.md` | **Borrar** |
| `docs/diagrams/CLOUDFLARE_GH_ENV_FLOW.md` | **Borrar** |
| `docs/diagrams/AUTH_FLOW.md` | **Borrar** |
| `docs/QUICK_REFERENCE.md` | Reescribir (tabla comandos + tabla config + nueva tabla GitHub Secrets) |
| `docs/CI_D1_SECRETS.md` | Reescribir (solo flujo manual; quitar d1 init + keyring) |
| `docs/CI_GITHUB_ACTIONS.md` | Reescribir (ACCOUNT_ID → Secret; sin referencias a local/keyring) |
| `docs/ARCHITECTURE.md` | Actualizar (quitar init/auth/keyring; añadir convención dirs) |
| `docs/diagrams/DEPLOY_FLOW.md` | Actualizar (token de env var, no keyring) |
| `docs/diagrams/goflare-generic.md` | Actualizar (CLI = auth/build/deploy) |
| `README.md` | Actualizar (quitar init/auth/d1; añadir "Setup en GitHub") |

---

## Verification

```bash
gotest
```

- Compila sin el import `go-keyring` en ningún paquete `!wasm`.
- `goflare auth` sin env var → error accionable; con `CLOUDFLARE_API_TOKEN=...` válido → "Token OK".
- `goflare deploy` sin env var → falla con el mensaje que apunta a GitHub Secrets.
- `go test -tags=integration -run TestD1Integration ./d1/` → corre si hay token, `Skip` si no.
- Búsqueda `grep -rn "keyring\|secrets push\|SecretManager" .` no devuelve código vivo.
