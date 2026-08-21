---
PLAN: "feat!: el build de páginas usa la tubería completa de sitec"
TAG: v0.5.0
EXECUTOR: jules
REVIEWER: none
STATUS: review
SESSION: 16358991460281098374
PR: https://github.com/tinywasm/goflare/pull/22
---

> Plan autocontenido: todo lo necesario para ejecutarlo está aquí.
> Se despacha con el flujo CodeJob. Ver skill: agents-workflow.
> Reglas del repo: [`AGENTS.md`](../AGENTS.md) en la raíz — léelo antes de tocar
> nada (dos objetivos de compilación, tests en `tests/`, fallo ruidoso).

# Plan — `goflare build` deja de armar los assets a mano

**Requisito previo**, porque este entorno no lo trae instalado:

```bash
go install github.com/tinywasm/devflow/cmd/gotest@latest
```

## 1. El problema, con el caso real que lo destapó

`veltylabs/misitio` es un panel construido con `goflare` en modo Pages
Functions. Declara su tema en `config/css.go` y ejecuta `goflare build`. El
resultado: **`web/public/style.css` de 0 bytes**, `favicon.svg` de 0 bytes, y el
tema del proyecto en ninguna parte.

No es un fallo de ese proyecto. Es que **`goflare build` nunca busca lo que el
proyecto declara.**

Mira lo que hace hoy `buildPages()` en [`build.go`](../build.go):

1. comprueba que exista `PublicDir`,
2. si existe `web/client.go`, lo compila a `client.wasm`,
3. y vuelca a disco un `sitec.AssetMin` que `New()` sembró en
   [`goflare.go`](../goflare.go) con **una sola cosa**: el bootstrap JS
   (`assetMin.UpdateSSRModule("tinywasm/js/bootstrap", …)`).

Ese `AssetMin` es la capa de bajo nivel de `sitec` — el buzón donde se depositan
assets ya extraídos. **Nadie extrae nada.** El escáner de `sitec`, el que
recorre el módulo y recoge los productores del proyecto (`RootCSS()`,
`RenderCSS()`, `RenderHTML()`, `IconSvg()`, `Fonts()`, `RenderPages()`), no se
invoca en ningún punto de este repo. Compruébalo tú mismo antes de tocar código:

```sh
grep -rn "ExtractAll\|sitec.Build(" --include=*.go . | grep -v docs/
# hoy: vacío
```

Consecuencia: un proyecto puede declarar todos los assets que quiera; `goflare`
emite un `style.css` vacío y un `index.html` sin nada dentro de `#app`.

### La prueba de que la tubería correcta ya existe y funciona

Sobre ese mismo proyecto, ejecutando el CLI de `sitec` en vez de `goflare`:

```sh
go run github.com/tinywasm/sitec/cmd/sitec build -o web/public
# style.css: 4374 bytes — los tokens del tema del proyecto
```

Y cuando el proyecto declaraba un `config/css.go` sin productor, esa misma
tubería **falló ruidosamente**, que es justo lo que este repo pide en su
`AGENTS.md`:

```
ssr: package github.com/veltylabs/misitio/config has css.go but declares no
RootCSS() or RenderCSS(); expected: func (w *T) RenderCSS() *css.Stylesheet
```

`goflare` no falla ni acierta: **ignora**. Eso es lo que hay que corregir.

## 2. Lo que ya te da `sitec` — no reimplementes nada de esto

`github.com/tinywasm/sitec` (ya es dependencia directa, `v0.1.10`) expone la
tubería completa en dos llamadas. Esta es la API real, copiada de su código:

```go
// Build ejecuta el pipeline completo en memoria. No escribe a disco.
func Build(cfg BuildConfig) (*Output, error)

type BuildConfig struct {
	RootDir        string // Raíz del módulo (donde vive go.mod). Obligatorio.
	Mode           Mode   // ModeRelease | ModeDev
	OutputDir      string // Relativo a RootDir. Vacío ⇒ "web/public" (Release)
	SiteURL        string
	AppName        string
	StaticAssets   []string
	ImageQuality   int
	AssetLibraries []string
	Log            func(...any)
}

// WriteTo vuelca los artefactos al FS indicado.
func (s *Output) WriteTo(fs FS) error
func NewOsFS() FS
```

Lo que `Build` hace por dentro, y que por tanto **desaparece de `goflare`**:

- valida que la raíz sea un proyecto Go real (`ValidateProject`),
- recorre el módulo y extrae los productores de todos los paquetes,
- **compila el WASM del frontend** si existe `web/client.go` — con TinyGo en
  `ModeRelease` y con el Go estándar en `ModeDev`,
- arma `style.css`, `script.js`, el sprite SVG, las fuentes, las imágenes y el
  `index.html`,
- copia los activos estáticos declarados.

**Anti-footgun:** `Build` devuelve el sitio **en memoria**. Si no llamas a
`WriteTo`, no se escribe nada y el build "pasa" sin producir un solo archivo.

## 3. Qué se cambia

Tres archivos de código, todos del objetivo `!wasm` (herramienta de host).

### 3.1 — `goflare.go`: la costura y el borrado del `AssetMin`

**Añade** (junto al resto de tipos exportados):

```go
// SiteOutput es el sitio ya compilado, listo para volcarse a disco.
type SiteOutput interface {
	WriteTo(fs sitec.FS) error
}

// SiteBuilder compila el sitio estático del proyecto.
//
// Es una costura deliberada: la tubería real de sitec exige un módulo Go
// válido en disco y un compilador instalado, y los tests de este repo no
// tienen ninguna de las dos cosas. La implementación real es buildSite.
type SiteBuilder func(cfg sitec.BuildConfig) (SiteOutput, error)

// buildSite es la implementación real: la tubería completa de sitec.
func buildSite(cfg sitec.BuildConfig) (SiteOutput, error) {
	out, err := sitec.Build(cfg)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// SetSiteBuilder sustituye el compilador de sitio. Pensado para tests; en
// producción nadie lo llama y se usa buildSite.
func (g *Goflare) SetSiteBuilder(b SiteBuilder) {
	if b != nil {
		g.siteBuilder = b
	}
}
```

**Anti-footgun:** `return sitec.Build(cfg)` **no compila** — Go no convierte
`(*sitec.Output, error)` a `(SiteOutput, error)` en un retorno múltiple.
Escríbelo con la variable intermedia, tal como está arriba.

**Modifica** el struct `Goflare`:

- **borra** el campo `assetMin *sitec.AssetMin`,
- **añade** el campo `siteBuilder SiteBuilder`.

**Modifica** `New(cfg *Config) *Goflare`:

- inicializa `siteBuilder: buildSite` en el literal del struct,
- **borra** el bloque `if cfg.PublicDir != "" { … }` que llamaba a
  `syncJSRuntime`, `sitec.NewAssetMin`, `SetFS` y `UpdateSSRModule`. Ese bloque
  entero se va: `sitec.Build` siembra el bootstrap y fija el runtime JS por su
  cuenta.

**Borra** la función `syncJSRuntime` completa: `New` era su único llamador y
`sitec` ya hace ese `js.SetRuntime` dentro de su compilador de WASM. Si al
borrarla el import `github.com/tinywasm/js` queda sin usar, quítalo también.

**Modifica** `SetLog`: elimina la propagación a `g.assetMin` (el log ahora viaja
en `BuildConfig.Log`). El resto del método se queda igual.

### 3.2 — `build.go`: `buildPages` pasa a ser una llamada

Sustituye **el cuerpo entero** de `buildPages()` por:

```go
func (g *Goflare) buildPages() error {
	// 1. Verify that PUBLIC_DIR exists
	if _, err := os.Stat(g.Config.PublicDir); os.IsNotExist(err) {
		return fmt.Errorf("public dir does not exist: %s", g.Config.PublicDir)
	}

	// 2. sitec recorre el modulo, recoge lo que el proyecto declara, compila
	//    el WASM del frontend y arma el sitio completo en memoria.
	g.Logger("building site →", g.Config.PublicDir)
	site, err := g.siteBuilder(sitec.BuildConfig{
		RootDir:   moduleRoot,
		Mode:      siteMode(g.Config.CompilerMode),
		OutputDir: g.Config.PublicDir,
		AppName:   g.Config.ProjectName,
		Log:       g.Logger,
	})
	if err != nil {
		return fmt.Errorf("site build failed: %w", err)
	}

	// 3. Nada existe hasta que se vuelca: Build() trabaja en memoria.
	if err := site.WriteTo(sitec.NewOsFS()); err != nil {
		return fmt.Errorf("failed to write site artifacts: %w", err)
	}

	return nil
}
```

Y añade, junto a las constantes que ya existen en `build.go`
(`moduleRoot`, `dirWeb`, `fileClientGo`):

```go
// CompilerModeStdlib compila el frontend con el Go estandar en vez de TinyGo:
// binario grande, compilacion rapida, sin minificar. Es el modo de desarrollo.
const CompilerModeStdlib = "L"

// siteMode traduce el modo de compilador de goflare al de sitec.
func siteMode(compilerMode string) sitec.Mode {
	if compilerMode == CompilerModeStdlib {
		return sitec.ModeDev
	}
	return sitec.ModeRelease
}
```

Con eso, en `buildPages` quedan **sin usar** `dirWeb`, `fileClientGo`,
`frontEntry`, `frontBuilder` y todo el paso 2 anterior: **bórralos**. Las dos
constantes `dirWeb` y `fileClientGo` desaparecen del repo si ningún otro archivo
las usa — compruébalo con `grep` y no las dejes huérfanas.

**`moduleRoot` se queda**: lo sigue usando la llamada nueva.

### 3.3 — Lo que NO se toca

`buildPagesFunctions()`, `buildWorker()`, `generateWasmFile()` y `edgeBuilder`
son el camino del **Worker** (`edge/main.go` → `functions/edge.wasm`), que tiene
su propio límite de 1 MB y su propio compilador. **No los toques.** Este plan
sólo cambia el camino de las páginas estáticas.

## 4. Tests

`gotest`, nunca `go test`. Todos en `tests/`, `package goflare_test`, sólo
aserciones de la stdlib.

### 4.1 — Un doble de sitio, en `tests/`

Escribe un `SiteBuilder` falso reutilizable que registre la `BuildConfig` que
recibió y escriba los archivos que le pidas:

```go
type fakeSite struct {
	cfg     sitec.BuildConfig
	written bool
	files   map[string][]byte // ruta relativa a OutputDir → contenido
	err     error             // si no es nil, Build falla con este error
}
```

Su `WriteTo` escribe `files` bajo `cfg.OutputDir` y marca `written = true`.

### 4.2 — Tests nuevos

| Test | Qué fija |
|---|---|
| `TestBuildPages_PassesModuleRootAndPublicDir` | La `BuildConfig` que recibe `sitec`: `RootDir == "."`, `OutputDir == Config.PublicDir`, `AppName == Config.ProjectName`, `Log != nil`. |
| `TestBuildPages_ModeFollowsCompilerMode` | `CompilerMode: "L"` → `sitec.ModeDev`; `""` y `"S"` → `sitec.ModeRelease`. |
| `TestBuildPages_WritesArtifactsToDisk` | Tras `Build()`, los archivos del doble están en `PublicDir`. Fija el paso que es fácil olvidar: `WriteTo`. |
| `TestBuildPages_PropagatesSiteBuildError` | Si el `SiteBuilder` devuelve error, `Build()` devuelve error y el mensaje **contiene el del builder**. Fallo ruidoso: nada de degradar a build vacío. |

### 4.3 — Tests existentes que hay que adaptar

Estos cinco llegan hoy a `buildPages` y se apoyaban en el `AssetMin` sembrado.
Cada uno debe inyectar el doble con `SetSiteBuilder` antes de llamar a `Build()`:

- `tests/build_test.go` → `TestBuild_PagesOnly`
- `tests/build_output_test.go` → `TestBuild_NoDist`,
  `TestBuild_OutputDirContainsOnlyWorkerArtifacts`,
  `TestBuild_PublicDirGetsGeneratedIndex`
- `tests/build_frontend_client_test.go` → `TestBuild_FrontendWasmFromModuleRoot`

`TestBuild_PublicDirGetsGeneratedIndex` comprobaba que `index.html` lo genera la
tubería y no el usuario: mantén esa intención haciendo que el doble escriba un
`index.html` propio y afirmando que el del usuario quedó pisado.

`TestBuild_FrontendWasmFromModuleRoot` comprobaba que el WASM del frontend se
resolvía desde la raíz del módulo. Esa responsabilidad **se muda a `sitec`**, que
ya la tiene cubierta con su propio test. Aquí conviértelo en lo único que sigue
siendo de `goflare`: que `RootDir` sea la raíz del módulo y no `filepath.Dir(PublicDir)`.
Si eso lo cubre ya `TestBuildPages_PassesModuleRootAndPublicDir`, **borra el
archivo** en vez de dejar un test duplicado.

Los otros tres de `build_test.go` (`TestBuild_NothingToBuild`,
`TestBuild_MissingEntry`, `TestBuild_MissingPublicDir`) fallan antes de llegar a
`sitec`: no deberían necesitar cambios. Si alguno los necesita, es señal de que
el orden de validaciones cambió — no lo cambies.

### 4.4 — Un test de integración, con el pipeline de verdad

`tests/build_site_integration_test.go`, con `//go:build integration` en la
primera línea (el mismo patrón que `tests/pages_test.go`), para que no entre en
la suite por defecto:

- `t.Chdir` a un `t.TempDir()`;
- escribe un módulo mínimo: `go.mod` (`module example.com/site`, `go 1.25.2`),
  `web/client.go` (`//go:build wasm`, `package main`, `func main() {}`),
  `web/public/` vacío y un paquete `config/` con
  `//go:build !wasm`, un tipo y `func (p *Panel) RootCSS() *css.Stylesheet`;
- `CompilerMode: CompilerModeStdlib` para no depender de TinyGo;
- afirma que `web/public/style.css` **no está vacío** y que `index.html`
  contiene `<div id="app">`.

Es el test que habría cazado este defecto. Si el módulo de prueba resulta
imposible de resolver sin red, deja el test escrito y márcalo con `t.Skip` que
**nombre la causa**; no lo borres.

## 5. Documentación — obligatoria, no opcional

- [`docs/BUILD_PAGES.md`](BUILD_PAGES.md) §Build Process: hoy describe los tres
  pasos viejos ("Compile Frontend WASM", "Generate Assets"). Reescríbelo: el
  build verifica `PublicDir` y delega en la tubería de `sitec`, que extrae lo que
  el proyecto declara, compila el WASM del frontend y emite el sitio. Di
  explícitamente que **un proyecto que declara un `css.go` sin productor rompe el
  build, a propósito**.
- [`docs/ARCHITECTURE.md`](ARCHITECTURE.md) §3 "Build Pipeline": actualiza la
  descripción del camino de páginas y menciona la costura `SiteBuilder`.
- [`README.md`](../README.md): re-indexa si añadiste algún documento.

Ningún documento debe citar `docs/PLAN.md`: este archivo se borra al publicar.

## 6. Criterios de aceptación

- [ ] `gotest` en verde (vet, race, wasm incluidos).
- [ ] `gofmt -l .` vacío.
- [ ] `grep -rn "assetMin\|AssetMin" --include=*.go . | grep -v docs/` → vacío.
- [ ] `grep -rn "syncJSRuntime" --include=*.go .` → vacío.
- [ ] `grep -rn "sitec.Build(" --include=*.go .` → aparece **una sola vez**, en
      `buildSite` (`goflare.go`).
- [ ] `grep -rn "filepath.Dir(g.Config.PublicDir)" --include=*.go .` → vacío.
- [ ] `buildPagesFunctions`, `buildWorker` y `generateWasmFile` siguen intactos:
      `git diff` no los toca.
- [ ] Los cuatro tests nuevos de §4.2 existen y pasan.

## 7. Anti-footguns

1. **Este cambio es todo `!wasm`** — herramienta de host. La stdlib (`os`,
   `fmt`, `path/filepath`) es correcta y esperada aquí. La regla de "sin stdlib"
   del ecosistema aplica sólo a `edge/`, `workers/`, `d1/` y `r2/`. **No
   "arregles" esos imports.**
2. **No inventes una degradación elegante.** Si `sitec` devuelve error, `goflare`
   devuelve error. Un sitio a medias desplegado es peor que un build roto.
3. **`Build` en memoria + `WriteTo`**: son dos pasos, y olvidar el segundo
   produce un build verde sin archivos.
4. **No toques el camino del Worker.** Comparten archivo (`build.go`) pero no
   comparten problema.
5. `docs/PLAN.md` (este archivo) no se renombra ni se borra, y su bloque de
   frontmatter —`PLAN`, `TAG`, `EXECUTOR`, `STATUS`, `SESSION`, `PR`— no se
   edita a mano.
