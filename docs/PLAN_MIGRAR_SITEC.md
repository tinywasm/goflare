---
PLAN: "fix!: migrar de tinywasm/assetmin y tinywasm/client (archivados) a tinywasm/sitec"
EXECUTOR: jules
REVIEWER: none
---

> Este plan se despacha con el flujo CodeJob. Ver skill: agents-workflow.
>
> Forma parte de un cambio con ruptura coordinado desde
> https://github.com/tinywasm/core/blob/main/docs/PLAN.md
>
> **YA NO ESTÁ BLOQUEADO.** `sitec` está publicado en **v0.0.58** con toda la
> API que este plan necesita, incluida la parametrización del `WasmBuilder`
> (2026-08-17). No hay que esperar a ninguna etapa de `sitec`: lee su código
> publicado.

# Plan — este repo compila contra DOS librerías archivadas

## Qué pasó

**`github.com/tinywasm/assetmin` y `github.com/tinywasm/client` están
archivados.** Ambos se reemplazan por `github.com/tinywasm/sitec`.

Hay una segunda razón, urgente, para hacerlo ya: `client` arrastra una versión
antigua de `tinywasm/devflow`, y por ahí entra **`github.com/zalando/go-keyring`**
al árbol de dependencias de `goflare`. El resto del ecosistema
(`keyring`, `devflow`, `app`, `deploy`) ya eliminó esa dependencia; `goflare` es
de los últimos que la conserva, y **solo por este import archivado**.
Comprobación: `go mod why github.com/zalando/go-keyring`.

### `assetmin` — la tubería de assets

Su contenido se repartió por responsabilidad:

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

### `client` — el compilador Go→wasm

`client` compilaba Go a WebAssembly. Eso vive ahora en `sitec` como el puerto
`WasmBuilder`. Este repo lo usa **dos veces**, con propósitos distintos:

```go
// goflare.go:77  — el worker del borde: main.go → edge.wasm, siempre TinyGo
edgeCompiler := client.New(&client.Config{SourceDir: …, OutputDir: …})
edgeCompiler.SetAppRootDir("")
edgeCompiler.UseDiskStorage()
edgeCompiler.UseProductionTinyGo()
edgeCompiler.SetMainInputFile("main.go")
edgeCompiler.SetOutputName("edge")

// goflare.go:120 — el frontend: web/client.go → client.wasm, modo configurable
browserCompiler := client.New(&client.Config{SourceDir: …, OutputDir: cfg.PublicDir})
browserCompiler.SetAppRootDir("")
browserCompiler.UseDiskStorage()
browserCompiler.SetMode(cfg.CompilerMode)

// invocación
g.edgeCompiler.RecompileMainWasm()   // wasm.go:16
g.browserCompiler.Compile()          // build.go:204
g.edgeCompiler.SetLog(f)             // goflare.go:147
g.edgeCompiler.Change(newValue)      // goflare.go:168
```

**El API de `sitec` es más pequeño a propósito**, y la diferencia de modelo es
lo único difícil de esta migración:

| `client` | `sitec` |
|---|---|
| `client.New(&Config{SourceDir, OutputDir})` | `sitec.NewWasmBuilder(stdlib bool, sitec.WasmBuildOptions{Entry, OutputName})` |
| `SetMainInputFile("main.go")` | `WasmBuildOptions.Entry = "main.go"` |
| `SetOutputName("edge")` | `WasmBuildOptions.OutputName = "edge"` |
| `UseProductionTinyGo()` | `stdlib = false` |
| `SetMode("S"/"M"/"L")` | `stdlib` booleano — ver "Modo del compilador" |
| `RecompileMainWasm()` / `Compile()` | `Build(dir) (WasmOutput, error)` |
| `UseDiskStorage()` + `OutputDir` | **no existe: `Build` devuelve los bytes en memoria** |
| `SetAppRootDir("")` | no existe; `Build(dir)` recibe el directorio directamente |

**Lo que debes escribir tú:** `sitec.WasmBuilder.Build` **no escribe archivos**.
Devuelve `WasmOutput{Binary []byte, Filename string, Runtime string}`. Este repo
sí necesita el artefacto en disco (el staging del borde y el `PublicDir` del
frontend), así que la escritura pasa a ser código de `goflare`: crear el
directorio y volcar `out.Binary` en `filepath.Join(dir, out.Filename)`.

**No pierdas el runtime.** `WasmOutput.Runtime` es el glue JS
(`wasm_exec`) que corresponde al modo compilado, y va emparejado con el binario
a propósito: un `.wasm` de TinyGo servido con el loader de Go estándar **no
arranca**. Donde antes `client` lo colocaba solo, ahora hay que escribirlo o
pasarlo a `AssetMin.SetWasm(filename, runtime)`.

### Modo del compilador

`cfg.CompilerMode` es `"S" | "M" | "L"` y hoy se traduce con
`browserCompiler.SetMode(...)` y `syncJSRuntime(...)` (`goflare.go:56`).
`sitec` expone un booleano `stdlib`, no tres letras.

**Decide la correspondencia leyendo `syncJSRuntime` y `js.SetRuntime`**, no
inventando: mira qué runtime selecciona hoy cada letra y reprodúcelo exactamente.
El borde **siempre** es TinyGo (`stdlib = false`) — está justificado en el
comentario de `goflare.go:93`: Cloudflare impone 1 MiB de wasm en el plan
gratuito y Go estándar produce 2–10 MB. Ese comentario debe sobrevivir a la
migración.

## Qué hacer

### Etapa 1 — cambiar las dependencias

1. `go.mod`: quitar `github.com/tinywasm/assetmin` **y**
   `github.com/tinywasm/client`; añadir `github.com/tinywasm/sitec` **v0.0.58 o
   posterior** (antes de esa versión el `WasmBuilder` fijaba `web/client.go` y
   `client.wasm`, y el borde no podía usarlo).
2. `goflare.go`, `build.go` y `wasm.go`: cambiar los imports.
3. `go mod tidy` y comprobar que **zalando desaparece**:
   `go mod why github.com/zalando/go-keyring` debe decir que ya no se necesita.

**No confíes en una redirección de GitHub.** Los repos están archivados, no
renombrados: las rutas antiguas dejan de resolver.

### Etapa 2 — adaptar las llamadas de assets

`sitec` conserva los mismos nombres que `assetmin` para esta parte, así que es
casi un cambio de import:

| Antes | Después |
|---|---|
| `assetmin.NewAssetMin(&assetmin.Config{…})` | `sitec.NewAssetMin(&sitec.Config{…})` |
| `UpdateSSRModule(name, css, scripts, html, icons)` | igual |
| `FlushToDisk()` | igual |
| `SetLog(f)` | igual |

**Detalle que no debes perder:** este repo escribe a disco (`FlushToDisk` en
`build.go:212`) porque produce un artefacto desplegable. En `sitec` el sink se
elige con `SetFS`: `sitec.NewOsFS()` escribe, `sitec.NewMemFS()` no. Verifica
cuál trae `NewAssetMin` por defecto en la versión que uses y **selecciona
`NewOsFS()` explícitamente** en vez de confiar en el default.

### Etapa 2b — adaptar las dos compilaciones wasm

Reemplaza los dos `client.New(...)` por dos `sitec.NewWasmBuilder(...)` según la
tabla de correspondencia de arriba:

```go
// borde: siempre TinyGo
edgeBuilder := sitec.NewWasmBuilder(false, sitec.WasmBuildOptions{
    Entry:      "main.go",
    OutputName: "edge",
})

// frontend: modo según cfg.CompilerMode
frontBuilder := sitec.NewWasmBuilder(stdlibFor(cfg.CompilerMode), sitec.WasmBuildOptions{})
```

y sustituye `RecompileMainWasm()` / `Compile()` por `Build(dir)` **más la
escritura del resultado**, que ahora es responsabilidad de este repo (ver "Lo
que debes escribir tú").

`Change(newValue)` y `SetCompilerMode` (`goflare.go:163-168`) recompilaban al
vuelo. Como el builder ya no guarda estado, **el modo pasa a ser un parámetro
en el momento de construir**: guarda el modo en `Config` y crea el builder al
compilar, en vez de mutar uno de larga vida.

### Etapa 3 — verificar el artefacto, no solo la compilación

`go build ./...` pasando no demuestra nada aquí: lo que importa es que el
`PublicDir` generado siga conteniendo `script.js` y `style.css` con el mismo
contenido que antes de migrar.

**Aceptación:**

- `grep -rn "tinywasm/assetmin\|tinywasm/client" .` devuelve vacío, incluidos
  `go.mod` y comentarios.
- `go mod why github.com/zalando/go-keyring` confirma que ya no entra.
- Una compilación real produce `script.js` y `style.css` no vacíos en el
  `PublicDir`.
- El bootstrap `tinywasm/js/bootstrap` sigue presente en el `script.js`
  resultante — es la contribución que inyecta `goflare.go:134` y es fácil de
  perder al adaptar la llamada.
- **`edge.wasm` se genera con ese nombre exacto** en el staging. El nombre no es
  cosmético: el despliegue lo referencia. Un `client.wasm` ahí significa que se
  usó el builder por defecto en vez de pasar `OutputName: "edge"`.
- **El runtime JS emparejado se escribe junto a cada binario.** Un `.wasm` de
  TinyGo servido con el loader de Go estándar no arranca, y el fallo aparece
  solo en el navegador, no en la compilación.

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
| 1 | Cambiar las dependencias | `go.mod`, `goflare.go`, `build.go`, `wasm.go` | `grep -rn "tinywasm/assetmin\|tinywasm/client" .` vacío y zalando fuera |
| 2 | Adaptar los assets; sink `osFS` explícito | `goflare.go`, `build.go` | compila y `FlushToDisk` escribe |
| 2b | Adaptar las dos compilaciones wasm + escribir binario y runtime | `goflare.go`, `build.go`, `wasm.go` | `edge.wasm` con ese nombre; runtime emparejado escrito |
| 3 | Verificar el artefacto | — | `script.js` y `style.css` no vacíos, con el bootstrap dentro |

## Si algo no encaja, para y reporta

`sitec` v0.0.58 tiene la API que este plan describe — está leída y verificada.
Si aun así encuentras que falta algo, **detente y repórtalo**: no escribas un
adaptador local que imite a `client`, no añadas un `replace` en `go.mod`
apuntando a una copia, y no dejes un tipo con "fake" o "stub" en el nombre.
Un sustituto local compila y pasa los tests, que es justo lo que lo hace
peligroso: parece terminado y no lo está.
