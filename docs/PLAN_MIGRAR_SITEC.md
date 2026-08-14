---
PLAN: "fix!: migrar de tinywasm/assetmin (archivado) a tinywasm/sitec"
EXECUTOR: jules
REVIEWER: none
---

> Este plan se despacha con el flujo CodeJob. Ver skill: agents-workflow.
>
> Forma parte de un cambio con ruptura coordinado desde
> https://github.com/tinywasm/core/blob/main/docs/PLAN.md
> **Bloqueado por** https://github.com/tinywasm/sitec/blob/main/docs/PLAN.md

# Plan — este repo compila contra una librería archivada

## Qué pasó

**`github.com/tinywasm/assetmin` está archivado.** Su contenido se repartió por
responsabilidad:

| Lo que hacía | Dónde vive ahora |
|---|---|
| compilar los assets del sitio (CSS, JS, sprite, fuentes, shell HTML) | `github.com/tinywasm/sitec` |
| deduplicación de sprites | `github.com/tinywasm/svg/sprite` |
| carga en segundo plano, reintentos, vigilancia de archivos | `github.com/tinywasm/app` |
| minificación | `github.com/tinywasm/sitec` (era un campo y tres registros de `tdewolff/minify`) |

Motivo: `assetmin` no era una librería de minificación. Tenía 74 métodos sobre
un solo tipo, importaba `tinywasm/router` (servir HTTP) y `tinywasm/tui`
(terminal), y poseía el DTO que **producía** otro repo — el productor importaba a
su consumidor para nombrar su propio resultado.

## Qué usa este repo

```go
// goflare.go:48
assetMin *assetmin.AssetMin  // generates script.js + style.css — nil if no PublicDir

// goflare.go:131
g.assetMin = assetmin.NewAssetMin(&assetmin.Config{...})

// goflare.go:134
g.assetMin.UpdateSSRModule("tinywasm/js/bootstrap", "", []*js.Script{js.PageBootstrap()}, "", nil)

// goflare.go:153
g.assetMin.SetLog(f)

// build.go:212
g.assetMin.FlushToDisk()
```

Es **la tubería completa de compilación de assets**, no minificación. Por eso el
destino es `sitec` y no un minificador suelto.

## Qué hacer

### Etapa 1 — cambiar la dependencia

1. `go.mod`: quitar `github.com/tinywasm/assetmin`, añadir
   `github.com/tinywasm/sitec` en la versión publicada tras su etapa 7.
2. `goflare.go` y `build.go`: cambiar el import.

**No confíes en una redirección de GitHub.** El repo está archivado, no
renombrado: la ruta antigua deja de resolver.

### Etapa 2 — adaptar las llamadas

El equivalente uno a uno lo define `sitec` en su etapa 7. Cuando ejecutes este
plan esa API ya existe; **no la inventes ni la adivines** — léela en
https://github.com/tinywasm/sitec/blob/main/docs/PLAN.md y en el código
publicado.

Correspondencia esperada:

| Antes | Después |
|---|---|
| `assetmin.NewAssetMin(&assetmin.Config{…})` | el constructor del pipeline de `sitec` |
| `UpdateSSRModule(name, css, scripts, html, icons)` | registro de la contribución de un módulo, etapa `emit` |
| `FlushToDisk()` | `emit` contra el adaptador `osFS` |
| `SetLog(f)` | igual |

**Detalle que no debes perder:** este repo escribe a disco (`FlushToDisk` en
`build.go:212`) porque produce un artefacto desplegable. En `sitec` eso es el
sink `osFS`, **no** el modo por defecto — el por defecto es memoria, para que
probar un componente no ensucie el disco. Selecciona `osFS` explícitamente.

### Etapa 3 — verificar el artefacto, no solo la compilación

`go build ./...` pasando no demuestra nada aquí: lo que importa es que el
`PublicDir` generado siga conteniendo `script.js` y `style.css` con el mismo
contenido que antes de migrar.

**Aceptación:**

- `grep -rn "tinywasm/assetmin" .` devuelve vacío, incluidos `go.mod` y
  comentarios.
- Una compilación real produce `script.js` y `style.css` no vacíos en el
  `PublicDir`.
- El bootstrap `tinywasm/js/bootstrap` sigue presente en el `script.js`
  resultante — es la contribución que inyecta `goflare.go:134` y es fácil de
  perder al adaptar la llamada.

## Restricciones

- Este repo es herramienta de backend: usa la biblioteca estándar
  legítimamente. La regla de "nada de biblioteca estándar en código WASM" **no
  aplica** — no "arregles" esos imports.
- Todo string repetido es una constante con nombre.
- Sin carpetas `internal/`.
- Cambio con ruptura: sin alias ni capas de compatibilidad hacia `assetmin`.
- No añadir llamadas a `gopush` ni a `codejob`.

## Etapas

| # | Alcance | Archivos | Aceptación |
|---|---|---|---|
| 1 | Cambiar la dependencia | `go.mod`, `goflare.go`, `build.go` | `grep -rn "tinywasm/assetmin" .` vacío |
| 2 | Adaptar las llamadas; sink `osFS` explícito | `goflare.go`, `build.go` | compila y `FlushToDisk` equivalente escribe |
| 3 | Verificar el artefacto | — | `script.js` y `style.css` no vacíos, con el bootstrap dentro |
