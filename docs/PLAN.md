> Este plan se despacha vía el flujo CodeJob. Ver skill: agents-workflow.
> Orquestado por `tinywasm/docs/ROUTER_ADAPTER_MASTER_PLAN.md` — **Fase 3 (propagación)**.

# PLAN — cola de ejecución de `goflare`

> **La cola está vacía: no hay nada que despachar.** Las dos etapas están aplicadas en el
> árbol. Este archivo queda como índice histórico. Antes de añadir una etapa nueva, lee
> "Estado actual" más abajo.

| Orden | Plan | Estado | Asunto |
|-------|------|--------|--------|
| 1 | [PLAN_STAGE_1_ROUTER.md](PLAN_STAGE_1_ROUTER.md) | ✅ **COMPLETADA** (PR #18, mergeado) | `goflare` deja de ser dueño del contrato de enrutado y pasa a implementarlo: borra el fork `goflare/router`, reconstruye `devserver/` sobre `server/httpd`, renombra `pages/` → `edge/` y endurece la detección de modo. |
| 2 | [PLAN_STAGE_2_FILES.md](PLAN_STAGE_2_FILES.md) | ✅ **COMPLETADA** (PR #19) | Subir y servir archivos en el borde: `PublicAsset`/`PublicDir`, cuerpo binario y perezoso en `workers/request.go`, bucket R2 (`r2/`) y el helper de subida (`files/`). |

## Estado actual

El PR #19 dejó los pasos 0, 1 y 4–6 hechos, pero **los pasos 2 y 3 quedaron sin implementar**:
`filetype` y `unixid` se añadieron al `go.mod` y ningún `.go` los importaba. Se completaron
después, junto con dos arreglos que el plan no había previsto. Las decisiones tomadas:

- **Los pasos 2 y 3 viven en un paquete nuevo, [`files/`](../files/files.go)**, no en `r2/`.
  El plan pedía que `r2/` no importara nada salvo `syscall/js` y `fmt`, y la validación
  necesita `filetype`, `unixid` y `router`. `files.Store` monta las dos rutas (`Mount`):
  subir exige el permiso `files`/`write`, servir es público. Así la política de seguridad
  —el tipo sale de los bytes, la clave la genera el servidor, nada de SVG— se escribe **una
  vez** y no la recopia cada módulo consumidor.

- **Los tests wasm de `tests/` no se ejecutaban.** `r2_test.go` llevaba `//go:build wasm`,
  pero sus hermanos del paquete no llevaban `//go:build !wasm`, así que bajo `GOOS=js` el
  paquete entero no compilaba y la ida y vuelta binaria —el test que *define* la Etapa 2—
  nunca corría. Los host-only ya llevan el tag.

- **`gotest` no podía estar verde en este repo, y no era culpa del repo.** Su paso wasm
  hacía `go test ./...` bajo `GOOS=js`, que arrastra los paquetes host-only (la raíz,
  `cmd/goflare`) y revienta con *"build constraints exclude all Go files"*. Arreglado
  **aguas arriba** en `devflow` (`ParseWasmTestPackages`): el runner ahora selecciona los
  paquetes que de verdad compilan para wasm. Requiere `devflow` ≥ la versión que lo incluya.

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
objetivo `wasm` **no** es `./...`. La raíz, `devserver/` y `cmd/` son host-only por diseño
(ver la tabla de dos objetivos en [`AGENTS.md`](../AGENTS.md)), así que `GOOS=js go build ./...`
falla siempre con *"build constraints exclude all Go files"*. Compila solo la columna `wasm`:

```bash
go build ./...                                                        # host
GOOS=js GOARCH=wasm go build ./edge/... ./d1/... ./workers/... ./cloudflare/... ./r2/... ./files/...
gotest
```

`tests/` es el paquete mixto: sus archivos host-only llevan `//go:build !wasm` y los del
borde `//go:build wasm`. Si añades un test ahí, **etiquétalo**: sin tag acaba en las dos
columnas y rompe la que no le corresponde.
