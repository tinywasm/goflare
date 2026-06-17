# [DESCARTADO] PLAN — Desplegar Pages Functions vía Direct Upload (`_worker.bundle`)

> **❌ NO IMPLEMENTAR ESTE PLAN.** Se adoptó la **Opción C**: goflare construye y
> **wrangler despliega** (wrangler ya es dueño del protocolo Direct Upload, incluido el
> `_worker.bundle` y `_routes.json`). Reimplementarlo en Go sería reinventar wrangler.
> El workflow del demo (`goflare-demo/.github/workflows/deploy.yml`, generado desde
> `workflow/spec.go`) ahora hace `goflare build` + `npx wrangler@4 pages deploy`.
> Este doc se conserva solo como referencia del formato del bundle.
>
> **Lo que SÍ salió de aquí y se aplicó a goflare:**
> - v0.3.2–v0.3.4: fixes del cliente Direct Upload (405 upload-token, hash blake3, manifest multipart).
> - v0.3.5: fix del bundle JS — `worker.mjs` redeclaraba `mod` (colisión con el import
>   inyectado); esbuild de wrangler lo detectó. Renombrado a `cachedModule`. Este bug
>   era latente: la función nunca se había ejecutado (se subía como estático).
>
> **Estado tras Opción C (validado en CI):** deploy ✓, estático+routing ✓, la función
> SE EJECUTA (binding D1 ✓, tabla creada ✓). Pendiente: `POST /api/contacto` lanza
> excepción runtime (HTTP 500 / code 1101) = panic de Go en el handler (json.Decode o
> db.Create por la vía wasm). Requiere `wrangler tail` o repro local para el mensaje.

> **Repo destino (histórico):** `tinywasm/goflare`.
> **Síntoma original:** el sitio estático vivía en `*.pages.dev` pero
> **`POST /api/contacto` devolvía HTTP 405** porque la Function se subía como estático.

---

## 1. Estado actual (qué YA está arreglado)

El flujo Direct Upload de `cloudflare.go`/`DeployPages` tenía 3 bugs, corregidos y
publicados:

| Versión | Bug | Fix |
|---------|-----|-----|
| v0.3.2 | `POST /uploadToken` → 405 | `GET /upload-token` (kebab-case, método GET) |
| v0.3.3 | hash de assets con `sha256` → 500 (1101) | `blake3(base64(content)+ext).hex()[:32]` (dep `lukechampine.com/blake3`) |
| v0.3.4 | manifest del deployment como JSON → 400 (8000096) | `multipart/form-data` con campo `manifest` |

Resultado: el job `deploy` pasa y el sitio estático sirve en `pages.dev` (HTTP 200).
**Falta** que las Functions se ejecuten.

## 2. Causa raíz

`goflare` fue diseñado para **CF Git Integration** (ver comentario en
`build.go` `buildPagesFunctions`): produce `functions/[[path]].mjs` + `functions/edge.wasm`
y asume que Cloudflare compila el directorio `functions/` al desplegar desde Git.

Pero el CI usa `goflare deploy` = **Direct Upload API**, que **NO compila** `functions/`.
Hoy `DeployPages` sube `functions/` como **assets estáticos** (`collectDir(g.Config.FunctionsDir, "/functions/")` en `cloudflare.go`). Por eso el `.mjs` queda como
archivo estático y `POST /api/contacto` lo maneja el servidor estático → **405**.

En Direct Upload, las Functions deben ir como un **`_worker.bundle`** (módulo worker ya
compilado) adjunto al `formData` del deployment, más un **`_routes.json`** que indica
qué rutas invocan al worker.

Ventaja para goflare: **ya tiene el worker compilado**. `javascripts.go` `bundleJS`
genera un módulo ESM autocontenido (inlinea `wasm_exec` + `runtime.mjs` + `worker.mjs`,
importa `./edge.wasm` y `cloudflare:sockets`). El shape "workers" (`generateWorkerFile`,
`pagesOnly=false`) ya exporta `export default { fetch, ... }`, que es justo lo que pide
el `main_module` del bundle. **No hace falta esbuild**: solo serializar lo que ya existe.

## 3. Formato exacto de `_worker.bundle` (verificado contra wrangler)

El `_worker.bundle` es **multipart/form-data anidado** (su propio boundary) que se
adjunta como un campo-archivo dentro del `formData` del deployment. Fuente:
`@cloudflare/deploy-helpers/.../create-worker-upload-form.ts` +
`wrangler/src/api/pages/create-worker-bundle-contents.ts`.

Partes del bundle (para worker tipo **esm / modules**):

1. **`metadata`** (valor JSON):
   ```json
   {
     "main_module": "edge.js",
     "compatibility_date": "2024-xx-xx",
     "compatibility_flags": []
   }
   ```
   - Para Pages **sin** wrangler config, `createUploadWorkerBundleContents` **borra**
     `bindings` del metadata (los bindings D1/KV los inyecta el proyecto Pages, no el bundle).
   - `compatibility_date` es necesario (el worker usa `cloudflare:sockets`).

2. **Módulo principal**: parte con `name="edge.js"`, `filename="edge.js"`,
   `Content-Type: application/javascript+module`, contenido = el bundle ESM con
   `export default { fetch }`.

3. **Módulo wasm**: parte con `name="edge.wasm"`, `filename="edge.wasm"`,
   `Content-Type: application/wasm`, contenido = bytes de `edge.wasm`.
   (El `import mod from "./edge.wasm"` del bundle resuelve al módulo llamado `edge.wasm`.)

Mapa MIME de módulos (`moduleTypeMimeType`):

| tipo | Content-Type |
|------|--------------|
| esm | `application/javascript+module` |
| commonjs | `application/javascript` |
| compiled-wasm | `application/wasm` |
| text | `text/plain` |
| buffer/data | `application/octet-stream` |

> En Go: construir con `mime/multipart` un `multipart.Writer`; cada parte usa
> `CreatePart` con headers `Content-Disposition: form-data; name="..."; filename="..."`
> y `Content-Type:` del mapa. La parte `metadata` solo lleva `name="metadata"` (JSON).
> El resultado (bytes) es el contenido del campo `_worker.bundle` del deployment.

## 4. Formato de `_routes.json` (Pages)

```json
{ "version": 1, "include": ["/api/*"], "exclude": [] }
```

- `include`: globs que **invocan al worker** (máx 100). El resto se sirve estático.
- `exclude`: globs servidos estáticos aunque casen con `include` (precedencia).

**Decisión de routing (importante):** el router de goflare (`pages/pages.go` `Serve`)
devuelve **404** para rutas no registradas — NO hace fallthrough a estático. Por eso
`include` debe acotarse a las rutas dinámicas (p.ej. `/api/*`); si fuera `/*`, el worker
interceptaría `/` y rompería el sitio estático.

→ goflare necesita saber qué rutas son dinámicas. Opciones (ver §6).

## 5. Cambios concretos en `goflare`

En `cloudflare.go` → `DeployPages`:

1. **Dejar de subir `functions/` como assets**: quitar el `collectDir(FunctionsDir, "/functions/")`.
2. Tras subir los assets estáticos, **construir `_worker.bundle`**:
   - leer `functions/[[path]].mjs` o, mejor, generar el shape `default { fetch }`
     (revisar: hoy el modo pages genera `onRequest`-only con `pagesOnlyExport`; para el
     bundle conviene `main_module` con `export default { fetch }`. Evaluar reusar
     `generateWorkerFile`/`bundleJS(pagesOnly=false)`).
   - leer `functions/edge.wasm`.
   - serializar el multipart del §3.
3. **Generar `_routes.json`** (§4).
4. En el `formData` del deployment añadir, junto a `manifest`+`branch`:
   - `_worker.bundle` (File con los bytes del §3)
   - `_routes.json` (File)
   - (opcional) `_headers`, `_redirects` si existen en PublicDir.
5. Añadir tests en `tests/deploy_pages_test.go`: el mock debe aceptar el deployment
   multipart y verificar presencia de los campos `_worker.bundle` y `_routes.json`.

## 6. Decisiones de diseño abiertas (confirmar con el dev antes de implementar)

1. **Fuente de `_routes.json`**: ¿campo de config `Routes []string` (default `["/*"]`)?
   ¿o leer un `_routes.json` que el dev ponga en `PublicDir` (como wrangler)? ¿o derivar
   de las rutas registradas? Para el demo, `["/api/*"]` basta.
2. **`compatibility_date`**: ¿campo de config con default a una fecha reciente fija?
3. **Binding D1 `DB`**: el `edge/main.go` del demo hace `d1.NewEdge("DB")`. El binding
   debe existir en el proyecto Pages. ¿`goflare deploy` lo provisiona (API de
   `pages/projects/:name` con `deployment_configs.production.d1_databases`) o se asume
   configurado en el dashboard? (Fuera del alcance de este plan, pero bloquea el e2e.)
4. **Coexistencia worker + estático**: confirmado que con `_routes.json` acotado, Pages
   sirve estático para lo excluido y solo enruta `include` al worker.

## 7. Pendiente aparte (no es este plan)

- **Dominio custom** `goflare-demo.tinywasm.app` → HTTP 000 (no resuelve). El e2e apunta
  ahí. Requiere custom domain en el proyecto Pages + registro DNS en la zona `tinywasm.app`.
  Revisar `configurePagesDomain` en `cloudflare.go`.

## 8. Referencias (wrangler/workers-sdk, rama main)

- `packages/deploy-helpers/src/deploy/helpers/create-worker-upload-form.ts` — formato del bundle + `moduleTypeMimeType`.
- `packages/wrangler/src/api/pages/create-worker-bundle-contents.ts` — `_worker.bundle` = form serializado a Blob.
- `packages/wrangler/src/api/pages/deploy.ts` — `formData.append("_worker.bundle"/"_routes.json"/...)`.
- Hash de assets: `blake3(base64(content)+ext).hex()[:32]` (histórico `wrangler@3.0.0/.../pages/hash.ts`).
