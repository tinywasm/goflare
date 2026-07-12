---
message: "refactor: adopt tinywasm/router contract; devserver on server/httpd; pages -> edge"
---

> Este plan se despacha vía el flujo CodeJob. Ver skill: agents-workflow.

# PLAN — cola de ejecución de `goflare`

> Si te han dicho *"ejecuta el plan descrito en docs/PLAN.md"*, ejecuta **ÚNICAMENTE la
> Etapa 1**, el único plan de la tabla de abajo. Es autocontenido: tiene todo el contexto,
> los contratos y los criterios de aceptación que necesitas.

| Orden | Plan | Asunto |
|-------|------|--------|
| 1 | [PLAN_STAGE_1_ROUTER.md](PLAN_STAGE_1_ROUTER.md) | `goflare` deja de ser dueño del contrato de enrutado y pasa a implementarlo: borra el fork `goflare/router`, reconstruye `devserver/` sobre `server/httpd`, renombra `pages/` → `edge/` y endurece la detección de modo. |

## ⛔ Lo que NO debes hacer en este despacho

- **NO ejecutes `PLAN_STAGE_2_FILES.md`.** Ese archivo existe en `docs/`, pero es la **etapa
  siguiente** y se despacha por separado, **después** de que esta se publique. La Etapa 2
  registra sus rutas sobre el router que construyes aquí y da por hecho que el paquete del
  borde ya se llama `edge/`: adelantarla obliga a rehacerla. Ignóralo por completo.
- **NO toques `goflare-demo`.** Es otro repo. Se romperá con estos cambios, y eso es
  **esperado**: se migra con su propio plan, después.

## Antes de tocar código

- [`AGENTS.md`](../AGENTS.md) — reglas del arnés: los **dos objetivos de compilación**
  (`wasm` = el Worker, sin stdlib; `!wasm` = herramientas de host, donde la stdlib **sí** es
  correcta), `gotest` en vez de `go test`, y fallo ruidoso siempre.
- [`docs/TESTING.md`](TESTING.md) — cómo se prueba código que corre dentro de un Worker de
  Cloudflare **sin desplegar nada**. Desplegar **no es un test**.

Al terminar, `gotest` debe estar en verde y los dos objetivos deben compilar:

```bash
go build ./... && GOOS=js GOARCH=wasm go build ./...
gotest
```
