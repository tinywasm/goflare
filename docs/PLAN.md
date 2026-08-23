---
PLAN: "feat!: un solo camino de despliegue y verificacion posterior del Worker"
EXECUTOR: jules
REVIEWER: none
STATUS: review
SESSION: 10867801983736286679
PR: https://github.com/tinywasm/goflare/pull/23
---

> Este plan se despacha con el flujo CodeJob. Ver skill: agents-workflow.

# Plan — `tinywasm/goflare`: del Direct Upload de Pages al Worker con assets

## El problema, medido en producción

`goflare deploy` en modo `pages-functions` **nunca ejecutó una Function**. El
despliegue reporta éxito y el sitio queda roto de una forma silenciosa.

`DeployPages` (`cloudflare.go`) recorre `functions/` y mete sus archivos en el
**manifiesto de assets** de Pages:

```go
// Include Pages Functions artifacts if present.
if g.Config.FunctionsDir != "" {
    if _, statErr := os.Stat(g.Config.FunctionsDir); statErr == nil {
        if err := collectDir(g.Config.FunctionsDir, "/functions/"); err != nil {
            return err
        }
    }
}
```

El manifiesto de Pages solo describe **archivos estáticos**. Las Pages Functions
se compilan dentro del pipeline de Cloudflare (integración con Git) o se envían
ya compiladas como un `_worker.bundle` en el multipart del despliegue. Subir
`[[path]].mjs` por el manifiesto no crea una Function: crea un archivo público.

Verificado sobre `veltylabs/misitio` desplegado:

```
$ curl -s https://misitio.velty.cl/api/health | head -c 40
<!doctype html><html>            ← el shell estático, no el JSON del Worker

$ curl -sI https://misitio.velty.cl/functions/edge.wasm | head -2
HTTP/2 200
content-type: application/wasm   ← el binario del servidor, público
```

Dos consecuencias: **ninguna ruta del Worker responde** (todo cae al asset
estático), y **el Worker compilado queda descargable** por cualquiera.

## La decisión: un solo camino de despliegue

`goflare` pasa a tener **una sola función de despliegue**: subir assets estáticos y,
si el proyecto tiene servidor, adjuntar el script del Worker en el mismo `PUT`.
Desaparecen `DeployPages` y `DeployWorker` como caminos separados.

Los tres casos que hoy son tres funciones se vuelven tres **formas del mismo
despliegue**:

| Proyecto | `assets` en el metadata | `main_module` + partes de script |
|---|---|---|
| Sitio estático (sin `edge/main.go`) | sí | **no** |
| App con servidor (misitio) | sí | sí |
| Worker sin archivos públicos | no | sí |

**Verificado contra la API real el 2026-08-23**, no deducido de la
documentación: se desplegó un Worker con solo assets —manifiesto de un archivo,
las tres fases completas— **sin `main_module` y sin parte de script**, y la API
respondió `success: true`. El Worker de prueba se borró después.

### Por qué Workers y no Pages

- **Pages no puede expresar un servidor por API.** Su manifiesto solo transporta
  archivos estáticos; las Functions se compilan en el pipeline de Cloudflare o
  viajan como un `_worker.bundle` que produce wrangler. Sostener Pages como
  camino único obligaría a reimplementar ese empaquetado en Go: reconstruir
  aguas abajo lo que otra herramienta ya posee.
- **Workers expresa los tres casos** con el multipart que este repositorio ya
  construye en `DeployWorker`.
- **El costo no cambia para un sitio estático**: *"Requests to static assets are
  free and unlimited"* (<https://developers.cloudflare.com/workers/platform/pricing/>).
  Una visita a un archivo no invoca el Worker ni se factura.
- **Los bindings y el dominio viajan en el despliegue**, no en formularios del
  panel que el despliegue no puede ver. El paso manual de "configurar el proyecto
  recién creado" desaparece.

### Lo que NO cambia

Los paquetes de runtime `goflare/edge` y `goflare/workers` **se quedan como
están**. No son dos formas de hacer lo mismo: `workers` expone las primitivas
(`Request`, `Response`) y `edge` es el adaptador de `router` construido sobre
ellas. Este plan no toca ningún archivo bajo `edge/` ni `workers/` salvo el
detalle de la cabecera de identidad, si se decide incluirla.

## Contrato de la API de Cloudflare (tres fases)

Documentación: <https://developers.cloudflare.com/workers/static-assets/direct-upload/>

**Fase 1 — sesión de subida.** El hash es `sha256(base64(contenido) + extensión_sin_punto)`
en hexadecimal, **truncado a 32 caracteres**. `size` es el tamaño en bytes del
archivo original. Las claves del manifiesto empiezan con `/` y usan `/` como
separador en todas las plataformas.

```
POST /accounts/{account_id}/workers/scripts/{script_name}/assets-upload-session
Authorization: Bearer <API_TOKEN>
Content-Type: application/json

{"manifest": {"/index.html": {"hash": "08f1dfda4574284ab3c21666d1a0b2c3", "size": 4337}}}
```

Respuesta: `{"result": {"jwt": "<UPLOAD_TOKEN>", "buckets": [["hash1","hash2"], ["hash3"]]}}`.

Los `buckets` agrupan qué hashes subir juntos. **Un archivo que Cloudflare ya
tiene no aparece en ningún bucket**: si `buckets` viene vacío no hay nada que
subir y se salta la fase 2, reusando el JWT de la fase 1 como token de
finalización.

**Fase 2 — subida.** Un `POST` por bucket, `multipart/form-data`, autenticado con
el **JWT de la fase 1** (no con el token de la cuenta). El **nombre de cada parte
es el hash**, el cuerpo es el contenido en base64, y el `Content-Type` de la parte
es el que Cloudflare servirá después.

```
POST /accounts/{account_id}/workers/assets/upload?base64=true
Authorization: Bearer <UPLOAD_TOKEN>
```

La última respuesta (`201`) trae `{"result": {"jwt": "<COMPLETION_TOKEN>"}}`. Ese
token vale una hora y es el que se adjunta en la fase 3.

**Fase 3 — despliegue.** El multipart que ya arma `DeployWorker`, con el campo
`metadata` extendido:

```json
{
  "main_module": "edge.js",
  "compatibility_date": "2026-08-01",
  "assets": {
    "jwt": "<COMPLETION_TOKEN>",
    "config": {
      "html_handling": "auto-trailing-slash",
      "not_found_handling": "single-page-application",
      "run_worker_first": ["/api/*"]
    }
  },
  "bindings": [ ... d1 y r2 como hoy ... ]
}
```

### Por qué `run_worker_first` es el corazón de este plan

Por defecto **los assets ganan**: si la URL coincide con un archivo se sirve el
archivo y el Worker no corre. Con `not_found_handling: "single-page-application"`
todo lo que no coincide devuelve `index.html` con `200`.

Sin `run_worker_first`, `/api/health` no coincide con ningún archivo, cae en la
regla de SPA y devuelve el HTML — **exactamente el bug que este plan arregla,
reproducido en la configuración nueva**. `run_worker_first` lista los prefijos
que van al Worker antes que a los assets. Acepta comodín `*` y negación con `!`,
y cada patrón debe empezar con `/` o `!/`.

### Esa lista es una constante de `goflare`, no configuración del proyecto

Los prefijos **no son arbitrarios**: `/api/` es la convención de las rutas de
aplicación en este ecosistema, y `/oauth/` lo fija `tinywasm/user` al montar sus
rutas de OAuth2. Los dos son constantes del framework, así que viven aquí:

```go
// WorkerFirstRoutes son los prefijos que Cloudflare debe enviar al Worker antes
// que a los assets estaticos. No es configuracion del proyecto: /api/ es la
// convencion de rutas de tinywasm/router y /oauth/ lo monta tinywasm/user. Un
// proyecto que use el ecosistema queda correcto sin declarar nada.
var WorkerFirstRoutes = []string{"/api/*", "/oauth/*"}
```

Se descartaron dos alternativas, y conviene que quede escrito por qué:

- **Variable de entorno `WORKER_FIRST_ROUTES`.** Duplica en `.env` un dato que
  vive en el código de la aplicación. Nadie contrasta las dos fuentes: agregar
  una ruta y olvidar la variable devuelve el shell estático con `200` — el mismo
  fallo silencioso que este plan existe para eliminar.
- **`run_worker_first: true`** (el Worker corre siempre y delega en los assets
  cuando no tiene ruta). Elimina la lista por completo y es imposible de
  equivocar, pero obliga a arrancar el WASM para servir cada archivo estático:
  seis arranques por carga de un panel, solo para contestar "esta no es mía".
  Servir un archivo no debe despertar código.

El caso que la constante no cubre —una aplicación que monte rutas fuera de esos
dos prefijos— no queda mudo: lo detecta la verificación de la **etapa 6**.

## Reglas de código — obligatorias

### Nada de literales repetidos

```
REGLA: toda cadena repetida (clave de entorno, ruta, prefijo, nombre de campo,
endpoint) es una constante con nombre en el paquete. Prohibido el literal suelto
en la lógica.
```

- Claves de entorno → constantes exportadas: `const EnvKeyCompatibilityDate = "COMPATIBILITY_DATE"`.
- Valores por defecto → constantes exportadas: `const DefaultCompatibilityDate = "2026-08-01"`.
- Rutas de la API → constantes o `fmt.Sprintf` sobre una plantilla con nombre, nunca el literal repetido en dos funciones.

### `cmd/` delgado

```
REGLA: cmd/goflare/main.go solo hace parseo de argumentos, inyección de
dependencias e impresión/salida. Toda condición o validación es una función
exportada de la librería.
```

### Contrato de ejecución

- Sin argumentos → ayuda por stdout y salida `0`.
- stdout = datos; **stderr = diagnóstico**. Los mensajes de progreso del
  despliegue van por `g.Logger`, nunca por `fmt.Println` directo.
- Salida distinta de `0` solo ante error real.

### Este repositorio usa la biblioteca estándar

`goflare` es herramienta de backend y compila con `//go:build !wasm`. Usa
`net/http`, `encoding/json`, `crypto/sha256` y `os` con normalidad. **No
"corrijas" esos imports hacia `tinywasm/fmt`**: esa regla aplica a paquetes que
llegan al binario WASM, y ninguno de los archivos que toca este plan lo hace. Los
archivos que sí compilan a WASM (`workers/`, `edge/`) no se tocan aquí.

---

## Etapa 1 — Un solo artefacto de build, sin ramas por modo

**Archivos:** `javascripts.go`, `build.go`, `mode.go`, `config.go`, `goflare.go`.

El `worker.mjs` embebido ya termina en `export default { fetch, scheduled, queue, onRequest }`,
que es exactamente la forma que un Worker necesita. El modo Pages lo **degradaba**
a `export { onRequest };`. Esa degradación desaparece, y con ella la ramificación
por modo: **todo proyecto con `edge/main.go` produce el mismo par de artefactos**,
`OutputDir/edge.js` y `OutputDir/edge.wasm`.

1. En `javascripts.go`, **borra** `generatePagesFunctionFile`, `pagesOnlyExport` y
   `functionsDir`. `bundleJS` pierde el parámetro `pagesOnly` y su rama. Queda
   `generateWorkerFile` como única productora del pegamento.

2. En `build.go`, **borra** `buildPagesFunctions`. La build queda con dos
   preguntas independientes, sin modos:

   ```
   ¿existe edge/main.go?          → compila edge.wasm + edge.js en OutputDir
   ¿existe PublicDir con archivos? → no hace nada en build; los usa el deploy
   ```

   El control de tamaño (`checkWasmSize`, 1 MB duro) se mantiene sobre el
   `edge.wasm` de `OutputDir`.

3. En `mode.go`, la inspección de imports **deja de elegir camino y pasa a ser
   solo validación**. `ModePagesFunctions`, `ModePagesStatic` y `ModeWorkers`
   desaparecen junto con el tipo `Mode`; queda una función:

   ```go
   // validateEntry confirma que edge/main.go importa alguno de los dos paquetes
   // de runtime. No elige camino — el artefacto es el mismo para ambos — pero
   // un main.go que no importa ninguno compila y luego no registra handlers:
   // fallo silencioso en producción. Aquí se convierte en error de build.
   func validateEntry(entry string) error
   ```

   Conserva el texto de `ErrNoKnownImport`. **Borra `ErrAmbiguous`**: importar los
   dos paquetes ya no es ambiguo, porque no hay dos salidas entre las que elegir.

4. En `config.go` y `goflare.go`, **borra el campo `Config.FunctionsDir`** y su
   asignación por defecto `c.FunctionsDir = "functions"`.

**Criterios de aceptación (verificables con grep):**

```
grep -rn "FunctionsDir"              .  → vacío
grep -rn "pagesOnlyExport"           .  → vacío
grep -rn "generatePagesFunctionFile" .  → vacío
grep -rn "buildPagesFunctions"       .  → vacío
grep -rn "ModePagesFunctions\|ModePagesStatic\|ModeWorkers" . → vacío
grep -rn "pages-functions"           .  → vacío
grep -rn "ErrAmbiguous"              .  → vacío
```

**Test — `tests/build_worker_assets_test.go`:**

- Tras `g.Build()` con un `edge/main.go` que importa `goflare/edge`: existen
  `OutputDir/edge.js` y `OutputDir/edge.wasm`.
- Lo mismo importando `goflare/workers`: **los mismos dos archivos, idénticos en
  forma** — es la prueba de que la rama por modo se fue.
- **No** existe el directorio `functions/` en la raíz del proyecto de prueba.
- El contenido de `edge.js` contiene `export default` y **no** contiene `export{onRequest}`.
- Un `edge/main.go` que no importa ninguno de los dos falla la build con
  `ErrNoKnownImport`.

## Etapa 2 — `assets.go`: manifiesto y subida

**Archivo nuevo:** `assets.go` (con `//go:build !wasm`, como el resto del paquete).

```go
// assetEntry describe un archivo en el manifiesto de assets del Worker.
type assetEntry struct {
	Hash string `json:"hash"`
	Size int    `json:"size"`
}
```

Tres funciones, todas con receptor `*Goflare` salvo la primera:

1. `func assetHash(content []byte, ext string) string`
   `sha256(base64.StdEncoding.EncodeToString(content) + ext)`, hex, `[:32]`.
   `ext` llega **sin punto** (`html`, no `.html`).

   > `DeployPages` usaba `blake3` sobre la misma entrada, porque Pages y Workers
   > exigen algoritmos distintos. Esa función se borra en la etapa 3, así que
   > **no queda duplicación que unificar**: `assetHash` es el único hash del
   > repositorio. No lo parametrices por función de hash — una firma que acepta
   > cualquier hash no impide pasar el equivocado, y el error solo se ve como un
   > `500` de Cloudflare.

2. `func (g *Goflare) buildAssetManifest(dir string) (map[string]assetEntry, map[string]string, error)`
   Recorre `dir` con `filepath.Walk`, ignora directorios, y devuelve el manifiesto
   y un índice `hash → ruta absoluta` para la fase 2. Las claves del manifiesto
   son `"/" + filepath.ToSlash(rel)`.

   Si `dir` no existe: error `fmt.Errorf("assets directory missing: %s", dir)`.
   Si no hay archivos: error `fmt.Errorf("no assets found in %s", dir)`.

3. `func (g *Goflare) uploadAssets(client *CfClient, manifest map[string]assetEntry, byHash map[string]string) (string, error)`
   Ejecuta las fases 1 y 2 y devuelve el **token de finalización**.

   - Fase 1 contra `/accounts/{id}/workers/scripts/{script}/assets-upload-session`
     con el `client` recibido (token de la cuenta).
   - Si la respuesta trae `buckets` vacío o ausente → devuelve el `jwt` de la
     fase 1 sin subir nada.
   - Fase 2: **un `CfClient` distinto**, con `Token` = el JWT de la fase 1, contra
     `/workers/assets/upload?base64=true`. Una parte por hash, nombre de parte =
     hash, cuerpo = base64 del contenido, `Content-Type` de la parte =
     `detectContentType(ruta)` (la función ya existe en `cloudflare.go`).
   - Guarda el `jwt` de cada respuesta; el último no vacío es el de finalización.
   - Si tras subir todos los buckets no hubo ningún `jwt`:
     `fmt.Errorf("assets uploaded but no completion token received")`.

**Test — `tests/assets_test.go`:**

- `assetHash` devuelve 32 caracteres hexadecimales, y coincide con el sha256
  calculado a mano en el test sobre `base64(contenido)+ext`.
- El manifiesto usa claves con `/` inicial y separadores `/` incluso construyendo
  rutas con `filepath.Join`.
- Con `MockHTTPServer` (patrón de `tests/deploy_pages_output_test.go`): si la fase
  1 responde `buckets: []`, **no** se llama a `/workers/assets/upload` y se
  devuelve el jwt de la fase 1.
- Con dos buckets, se hacen dos `POST` y se devuelve el jwt de la última respuesta.

---

## Etapa 3 — `Deploy`: la única función de despliegue

**Archivo:** `cloudflare.go`.

`DeployPages` y `DeployWorker` **se borran**. En su lugar queda una sola:

```go
// Deploy sube el sitio a Cloudflare como un Worker con assets estaticos.
// Los assets van si PublicDir tiene archivos; el script va si OutputDir tiene
// edge.js. Al menos uno de los dos debe existir.
func (g *Goflare) Deploy() error
```

Orden interno, sin ramas por modo:

1. Si `PublicDir` tiene archivos → `buildAssetManifest` + `uploadAssets` (etapa 2)
   → `metadata.assets`.
2. Si existe `OutputDir/edge.js` → `metadata.main_module = "edge.js"` y las partes
   `edge.js` y `edge.wasm` del multipart.
3. Si **ninguna** de las dos se cumple:
   `fmt.Errorf("nothing to deploy: neither %s nor %s/edge.js exist", g.Config.PublicDir, g.Config.OutputDir)`.
4. `PUT /accounts/{id}/workers/scripts/{name}` con el multipart armado.
5. Dominio (etapa 4).

Un despliegue sin assets omite la clave `assets` entera; uno sin script omite
`main_module` y no adjunta partes. **Probado: la API acepta las dos formas.**

`metadata` incorpora:

  ```go
  metadata["compatibility_date"] = g.compatibilityDate()
  metadata["assets"] = map[string]any{
      "jwt": completionToken,
      "config": map[string]any{
          "html_handling":      HTMLHandlingDefault,
          "not_found_handling": g.notFoundHandling(),
          "run_worker_first":   WorkerFirstRoutes,
      },
  }
  ```

### Qué pasa con los bindings que ya existen — verificado contra la API real

Probado el 2026-08-23 sobre un Worker desechable en una cuenta real, subiendo el
script tres veces y leyendo `GET /accounts/{id}/workers/scripts/{name}/settings`
entre cada una:

| Tipo de binding | Si el `metadata` del despliegue **no** lo declara |
|---|---|
| `secret_text` | **sobrevive**. Cloudflare preserva los secretos entre despliegues. |
| `plain_text`, `d1`, `r2_bucket` | **desaparece**. El `metadata` es la verdad para todo lo que no sea secreto. |

`{"type":"inherit","name":"X"}` es válido y preserva un binding no secreto sin
repetir su valor.

Consecuencias para este plan:

- **No hace falta maquinaria de `inherit` para los secretos**: no se pierden. No
  implementes lectura previa de bindings ni un `INHERIT_BINDINGS`.
- **Sí hay que documentarlo**: una variable cargada como texto plano en el panel
  de Cloudflare se borra en el siguiente `goflare deploy`. Eso va en
  `docs/BUILD_WORKER_ASSETS.md` con esta frase textual: *"Las variables de
  ejecución se cargan como Secret. Una variable de texto plano creada en el panel
  desaparece en el siguiente despliegue: el `metadata` del despliegue es la
  fuente de verdad para todo binding que no sea secreto."*

Constantes y lectores nuevos en `config.go`:

| Constante | Valor |
|---|---|
| `EnvKeyCompatibilityDate` | `"COMPATIBILITY_DATE"` |
| `EnvKeyNotFoundHandling` | `"NOT_FOUND_HANDLING"` |
| `DefaultCompatibilityDate` | `"2026-08-01"` |
| `DefaultNotFoundHandling` | `"single-page-application"` |
| `HTMLHandlingDefault` | `"auto-trailing-slash"` |

`WorkerFirstRoutes` se declara en `config.go` como se muestra arriba y se envía
tal cual como array JSON. **No** agregues clave de entorno, ni lectura de `.env`,
ni validación de patrones: son literales del repositorio, revisados en el diff.

En `run.go` y `goflare.go` **desaparece el despacho por modo**: el comando
`deploy` llama a `Deploy()` y nada más.

**Criterios de aceptación:**

```
grep -rn "DeployPages\|DeployWorkerAssets\|DeployWorker\b" . → solo Deploy
grep -rn "/pages/"        .  → vacío
grep -rn "blake3"         .  → vacío   (se va con el manifiesto de Pages)
```

Con `blake3` fuera, quita también `lukechampine.com/blake3` de `go.mod` si no
queda otro uso.

**Test — `tests/deploy_test.go`** (con `MockHTTPServer`). Los tres casos de la
tabla de decisión, que son la prueba de que el camino es uno solo:

- **Solo assets** (sin `edge.js`): el `metadata` **no** trae `main_module` y no se
  adjunta ninguna parte de script.
- **Solo script** (sin `PublicDir`): el `metadata` **no** trae la clave `assets` y
  no se llama a `assets-upload-session`.
- **Ambos** (el caso de misitio): las tres fases y el `PUT` completo.
- Sin ninguno de los dos: error `nothing to deploy: …`.

Y sobre el caso completo:

- Se llaman las tres rutas **en orden**: `assets-upload-session`, `workers/assets/upload`, `workers/scripts/{name}`.
- El `metadata` del `PUT` contiene `main_module: "edge.js"`, el `compatibility_date`
  por defecto, `assets.jwt` igual al token de finalización que devolvió el mock, y
  `assets.config.run_worker_first == ["/api/*", "/oauth/*"]`.
- Con `D1_DATABASE_ID` y `R2_BUCKET_ID` definidos, los bindings D1 y R2 siguen
  presentes en `metadata.bindings` (no se perdieron al refactorizar).
- La fase 2 se autentica con el JWT de la fase 1, **no** con `CLOUDFLARE_API_TOKEN`.

---

## Etapa 4 — Dominio propio del Worker

**Archivo:** `cloudflare.go`.

`configurePagesDomain` sirve solo a Pages. Añade `configureWorkerDomain`, que se
llama al final de `DeployWorkerAssets` cuando `g.Config.Domain != ""`, con el
mismo tratamiento tolerante que hoy: si falla, se registra con `g.Logger` y el
despliegue **no** falla.

1. `GET /zones` y elegir la zona cuyo `name` sea el **sufijo más largo** del
   dominio configurado. Nada de derivar el apex cortando por puntos: `velty.cl`
   funciona pero `algo.co.uk` no, y no hay lista de sufijos públicos en este
   repositorio. Si ninguna zona calza:

   ```
   fmt.Errorf("no zone in the account matches domain %s", g.Config.Domain)
   ```

2. `PUT /accounts/{id}/workers/domains` con
   `{"environment":"production","hostname":<Domain>,"service":<WorkerName>,"zone_id":<id>}`.
   Es idempotente: repetir el despliegue no duplica nada.

**Test — `tests/deploy_worker_domain_test.go`:** con dos zonas en la respuesta del
mock (`velty.cl` y `misitio.velty.cl.otra`), se elige la de sufijo más largo que
calce, y el `PUT` lleva `service` igual al `WorkerName`.

---

## Etapa 5 — Documentación

- **Borra** `docs/BUILD_PAGES_FUNCTIONS.md`. Su contenido queda obsoleto entero:
  describe el directorio `functions/` y afirma que Cloudflare despliega solo "si
  tienes la integración con Git configurada", que es justamente la trampa.
- **Crea** `docs/BUILD_WORKER_ASSETS.md`: el modo, los artefactos (`.build/edge.js`,
  `.build/edge.wasm`), las tres fases de la API, la tabla de variables nuevas y la
  explicación de `run_worker_first` con un ejemplo de qué pasa si falta. Incluye
  dos apartados más, que se escriben al cerrar la etapa 6:
  - **La verificación posterior al despliegue**: qué comprueba, por qué mira la
    cabecera y no el código de estado, y qué hacer cuando falla.
  - El síntoma de una ruta fuera de convención, con esta frase textual: *"Si una
    ruta de tu aplicación devuelve el HTML del sitio en vez de su respuesta, está
    fuera de los prefijos de `WorkerFirstRoutes`."*
- `docs/BUILD_PAGES.md` se queda: describe el modo estático, que sigue vivo.
- `docs/QUICK_REFERENCE.md`: agrega las tres variables nuevas a la tabla de `.env`.
- `README.md`: donde diga `functions/`, corregir a `.build/`.

**Criterio:** `grep -rn "functions/" docs README.md` → vacío.

---

---

## Etapa 6 — El despliegue verifica que el Worker contesta

**Archivos:** `assets/worker.mjs`, `javascripts.go`, `cloudflare.go`, `goflare.go`.

### El problema que cierra

`tests/deploy_pages_output_test.go` afirmaba que se llamó a
`/pages/assets/upload` contra un servidor simulado. Esa afirmación fue **cierta
durante toda la caída**: la llamada se hacía, la subida funcionaba, y el sitio
estaba roto. El test nace de la misma creencia que el código —que subir
`functions/` crea una Function— así que solo podía confirmarla.

Un desplegador no se puede probar contra dobles: su producto es un efecto en el
sistema de otro. La única prueba con forma de consumidor es **desplegar y pedir
una URL**. Esta etapa la incorpora al propio `Deploy`.

Y tiene que comprobar **quién** respondió, no si respondió: el despliegue roto
devolvía `200` con el HTML del shell. El código de estado mentía.

### 6.1 — El pegamento se identifica

En `assets/worker.mjs`, `fetch` devuelve hoy la respuesta del wasm tal cual.
Pasa a envolverla para agregar la cabecera:

```js
async function fetch(req, env, ctx) {
  const binding = {};
  await run(createRuntimeContext({ env, ctx, binding }));
  const res = await binding.handleRequest(req);
  const out = new Response(res.body, res);
  out.headers.set("x-goflare", "__GOFLARE_VERSION__");
  return out;
}
```

En el mismo archivo, **borra `onRequest`** —la función y su entrada en el
`export default`—: era la forma de Pages Functions y ese camino ya no existe.
Cada Worker la arrastraba sin usarla.

En `javascripts.go`, `bundleJS` sustituye el marcador `__GOFLARE_VERSION__` por
el valor real antes de minificar, con la misma cirugía de cadenas que ya hace.
El valor lo da una función nueva en `goflare.go`:

```go
// HeaderIdentity es la cabecera con la que un Worker desplegado por goflare se
// identifica. Su presencia prueba que la respuesta la produjo el Worker y no la
// capa de archivos estaticos.
const HeaderIdentity = "x-goflare"

// identityValue devuelve la version del modulo goflare en ejecucion, o "dev"
// cuando no hay informacion de build (checkout local).
func identityValue() string
```

Implementación: `runtime/debug.ReadBuildInfo()`; usa `bi.Main.Version` si no está
vacío y no es `"(devel)"`; si no, `"dev"`. **No** declares una constante de
versión a mano: una cifra que hay que acordarse de subir miente en cuanto
alguien la olvida.

**Criterio:** `grep -rn "onRequest" .` → vacío.

### 6.2 — `Deploy` pide una URL y falla si no la trae

Al final de `Deploy`, después del dominio:

1. **Si el despliegue no llevaba script** (solo assets), no hay Worker que
   verificar: se salta, sin aviso.
2. Determina la URL pública:
   - `g.SiteURL` si está definido (**es el punto de inyección para los tests**;
     campo exportado nuevo en `Goflare`, vacío en producción).
   - Si no, `https://` + `g.Config.Domain` cuando hay dominio.
   - Si no, `GET /accounts/{id}/workers/subdomain` y arma
     `https://<WorkerName>.<subdomain>.workers.dev`.
   - Si no consigue ninguna, registra con `g.Logger` que no pudo verificar y
     devuelve `nil`. **No inventes una URL.**
3. `GET <url>/api/__goflare_probe`, reintentando con `g.retry(5, g.RetryBackoff, …)`
   — un despliegue recién creado tarda segundos en responder.
4. La ruta cae bajo `/api/*`, así que `WorkerFirstRoutes` la manda al Worker. El
   router de la aplicación devolverá `404`: **da igual el estado**. Lo único que
   se comprueba es que la respuesta traiga `HeaderIdentity`.
5. Si tras los reintentos la cabecera no está:

   ```go
   return fmt.Errorf(
       "deploy verification failed: %s responded without the %s header — "+
           "the files were uploaded but the Worker is not serving requests",
       probeURL, HeaderIdentity)
   ```

**No intenta revertir nada**: el despliegue ya ocurrió y Cloudflare no tiene
vuelta atrás en esta API. Falla el comando para que CI se ponga rojo en el mismo
minuto, con alguien mirando — que es exactamente lo que no pasó el 2026-08-23.

### 6.3 — Tests

**Archivo:** `tests/deploy_verify_test.go`.

Con `MockHTTPServer` haciendo de sitio publicado, inyectado por `g.SiteURL`:

- El sitio responde `404` **con** `x-goflare` → `Deploy` devuelve `nil`. Es el
  caso sano: la ruta no existe pero el Worker contestó.
- El sitio responde `200` **sin** la cabecera, con `Content-Type: text/html` —
  **la reproducción exacta del fallo de producción** → `Deploy` devuelve el error
  de arriba.
- Despliegue de solo assets (sin `edge.js`) → no se pide ninguna URL: el mock no
  registra visitas.
- El valor de la cabecera no se compara contra nada: se verifica **presencia**,
  no versión.

Y en `tests/build_worker_assets_test.go`, una comprobación más: el `edge.js`
generado contiene `"x-goflare"` y **no** contiene `__GOFLARE_VERSION__` — el
marcador fue sustituido.

---

## Tabla de etapas

| # | Qué | Archivos | Cierra cuando |
|---|---|---|---|
| 1 | Un solo artefacto de build | `javascripts.go`, `build.go`, `mode.go`, `config.go`, `goflare.go` | los siete greps de la etapa 1 salen vacíos y `build_worker_assets_test.go` pasa |
| 2 | Manifiesto y subida de assets | `assets.go` (nuevo), `tests/assets_test.go` | `assets_test.go` pasa, incluido el caso de `buckets` vacío |
| 3 | `Deploy` único | `cloudflare.go`, `config.go`, `run.go`, `goflare.go` | `deploy_test.go` pasa con los cuatro casos y `grep -rn "/pages/" .` vacío |
| 4 | Dominio del Worker | `cloudflare.go` | `deploy_worker_domain_test.go` pasa |
| 5 | Documentación | `docs/`, `README.md` | `grep -rn "functions/" docs README.md` vacío |
| 6 | Verificación posterior al despliegue | `assets/worker.mjs`, `javascripts.go`, `cloudflare.go`, `goflare.go` | `deploy_verify_test.go` pasa con los tres casos y `grep -rn "onRequest" .` vacío |

Las etapas son **secuenciales**: 2 necesita los artefactos de la 1, 3 necesita las
funciones de la 2, 4 y 5 cierran sobre 3, y 6 necesita el `Deploy` de la 3.

## Verificación final

```bash
go vet ./...
go test ./...
grep -rn "pages-functions\|FunctionsDir\|pagesOnlyExport\|DeployPages\|blake3\|onRequest" .   # vacío
```

Debe quedar **una** función de despliegue exportada. Si el diff toca algún archivo
bajo `workers/` o `edge/`, algo se salió del plan: esos paquetes son el runtime de
la aplicación y este plan solo cambia build y despliegue.
