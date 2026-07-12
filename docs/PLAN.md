---
message: "feat: raw binary request body + R2 bucket binding for edge file uploads"
---

> Este plan se despacha vía el flujo CodeJob. Ver skill: agents-workflow.
> Orquestado por `tinywasm/docs/ROUTER_ADAPTER_MASTER_PLAN.md` — **Fase 3 (propagación)**.

# PLAN — cola de ejecución de `goflare`

> Si te han dicho *"ejecuta el plan descrito en docs/PLAN.md"*, ejecuta **ÚNICAMENTE la
> Etapa 2**. La Etapa 1 ya está aplicada en el árbol y se lista abajo solo como contexto:
> **no la rehagas**. La Etapa 2 es autocontenida — tiene todo el contexto, los contratos y
> los criterios de aceptación que necesitas.

| Orden | Plan | Estado | Asunto |
|-------|------|--------|--------|
| 1 | [PLAN_STAGE_1_ROUTER.md](PLAN_STAGE_1_ROUTER.md) | ✅ **COMPLETADA** | `goflare` deja de ser dueño del contrato de enrutado y pasa a implementarlo: borra el fork `goflare/router`, reconstruye `devserver/` sobre `server/httpd`, renombra `pages/` → `edge/` y endurece la detección de modo. |
| 2 | [PLAN_STAGE_2_FILES.md](PLAN_STAGE_2_FILES.md) | ☐ **PENDIENTE** | Subir y servir archivos en el borde: arregla la corrupción silenciosa del cuerpo binario en `workers/request.go` y añade el bucket R2 (`r2/`). |

## ⛔ Compuerta — no despachar la Etapa 2 todavía

La Etapa 2 declara como prerrequisito que la **Etapa 1 esté aplicada y publicada**. Hoy la
Etapa 1 vive en el **PR #18, sin mergear**. Mergea y publica primero; despachar antes deja
la Etapa 2 apoyada en un `edge/` que aún no existe en `main`.

## Etapa 1 — qué quedó hecho (contexto, no trabajo)

- El subpaquete `router/` ha sido eliminado.
- `devserver/` ahora usa `server/httpd`.
- `pages/` renombrado a `edge/`.
- `edge` implementa el contrato `tinywasm/router` (cookies, RBAC, prefijos).
- `inferMode` usa `go/parser` y tiene tests internos pasando.
- Los archivos de la raíz que dependen de la struct `Goflare` (host-only) llevan `//go:build !wasm`.

El conflicto de `Kind` que bloqueaba la build WASM era aguas arriba, no de este repo:
`model@v0.0.8` introdujo `Kind`, que chocaba con el `Kind` de `fmt` porque `jsvalue` hacía
**dot-import de ambos**. Resuelto en `jsvalue@v0.0.15` cualificando el import de `fmt`, tal
como ya exigía el `AGENTS.md` de aquel repo.

## ⛔ Lo que NO debes hacer

- **NO toques `goflare-demo`.** Es otro repo. Se rompe con estos cambios, y eso es
  **esperado**: se migra con su propio plan, después.

## Antes de tocar código

- [`AGENTS.md`](../AGENTS.md) — reglas del arnés: los **dos objetivos de compilación**
  (`wasm` = el Worker, sin stdlib; `!wasm` = herramientas de host, donde la stdlib **sí** es
  correcta), `gotest` en vez de `go test`, y fallo ruidoso siempre.
- [`docs/TESTING.md`](TESTING.md) — cómo se prueba código que corre dentro de un Worker de
  Cloudflare **sin desplegar nada**. Desplegar **no es un test**.

Al terminar, `gotest` debe estar en verde y los dos objetivos deben compilar. Ojo: el
objetivo `wasm` **no** es `./...`. La raíz, `devserver/` y `tests/` son host-only por diseño
(ver la tabla de dos objetivos en [`AGENTS.md`](../AGENTS.md)), así que `GOOS=js go build ./...`
falla siempre con *"build constraints exclude all Go files"*. Compila solo la columna `wasm`
(la Etapa 2 añade `./r2/...` a esa lista):

```bash
go build ./...                                                        # host
GOOS=js GOARCH=wasm go build ./edge/... ./d1/... ./workers/... ./cloudflare/...
gotest
```
