# Build & Deploy — Worker con Assets Estáticos

`goflare` compila y despliega aplicaciones a Cloudflare como un **Worker con assets estáticos** en un solo comando (`goflare deploy`).

---

## Modos de proyecto y artefactos

Cualquier proyecto con `edge/main.go` genera exactamente el mismo par de artefactos en `.build/`:

- `.build/edge.js` — el pegamento JavaScript empaquetado y minificado.
- `.build/edge.wasm` — el binario compilado de Go (máximo 1 MiB en el plan Free).

Si el proyecto contiene `web/public/`, sus archivos se procesan como assets estáticos.

| Estructura del proyecto | Assets | Script Worker |
|---|---|---|
| Sitio estático (sin `edge/main.go`) | Sí | No |
| App con servidor (`edge/main.go` + `web/public/`) | Sí | Sí |
| Worker sin frontend (`edge/main.go` sin `web/public/`) | No | Sí |

---

## Proceso de Despliegue (API de 3 Fases)

El despliegue combina la API Direct Upload de Assets de Cloudflare Workers con el `PUT` del Worker:

1. **Fase 1 — Sesión de subida (`assets-upload-session`):**
   `goflare` genera un manifiesto de hashes sha256 truncados a 32 caracteres hexadecimales para todos los archivos estáticos y solicita una sesión de subida a `/accounts/{account_id}/workers/scripts/{script_name}/assets-upload-session`.

2. **Fase 2 — Subida de bloques (`/workers/assets/upload`):**
   Si Cloudflare solicita subir archivos (buckets no vacíos), `goflare` sube los datos codificados en base64 usando el JWT obtenido en la Fase 1 como cabecera de autorización. Devuelve un **token de finalización** (validado por 1 hora).

3. **Fase 3 — Despliegue del Worker (`PUT /accounts/.../workers/scripts/{script_name}`):**
   Envía la petición multipart/form-data con los artefactos `.build/edge.js` y `.build/edge.wasm`, junto al campo `metadata` configurado:

```json
{
  "main_module": "edge.js",
  "compatibility_date": "2026-08-01",
  "assets": {
    "jwt": "<COMPLETION_TOKEN>",
    "config": {
      "html_handling": "auto-trailing-slash",
      "not_found_handling": "single-page-application",
      "run_worker_first": ["/api/*", "/oauth/*"]
    }
  },
  "bindings": []
}
```

---

## Enrutamiento y `WorkerFirstRoutes`

Por defecto, los assets estáticos tienen prioridad sobre el Worker. Con `not_found_handling: "single-page-application"`, cualquier ruta que no coincida con un archivo estático devolverá `index.html` con estado `200`.

Para asegurar que las peticiones API lleguen al Worker, `goflare` envía `run_worker_first: ["/api/*", "/oauth/*"]`.

> *Si una ruta de tu aplicación devuelve el HTML del sitio en vez de su respuesta, está fuera de los prefijos de `WorkerFirstRoutes`.*

---

## Secretos y Bindings

Los bindings de D1 y R2 se configuran automáticamente si se definen `D1_DATABASE_ID` o `R2_BUCKET_ID`.

> *Las variables de ejecución se cargan como Secret. Una variable de texto plano creada en el panel desaparece en el siguiente despliegue: el `metadata` del despliegue es la fuente de verdad para todo binding que no sea secreto.*

---

## Verificación posterior al despliegue

Tras completar el `PUT`, `goflare` realiza una sonda automática `GET /api/__goflare_probe` reintentando con backoff exponencial. Comprueba que la respuesta contenga la cabecera `x-goflare`. Si la cabecera está ausente, el despliegue falla y alerta al pipeline de CI.

---

## Variables de Entorno

| Variable | Descripción | Valor por defecto |
|---|---|---|
| `PROJECT_NAME` | Nombre del proyecto | Requerido |
| `CLOUDFLARE_ACCOUNT_ID` | ID de la cuenta Cloudflare | Requerido |
| `WORKER_NAME` | Nombre del Worker | `<PROJECT_NAME>-worker` |
| `DOMAIN` | Dominio personalizado | Opcional |
| `COMPATIBILITY_DATE` | Fecha de compatibilidad de Workers | `2026-08-01` |
| `NOT_FOUND_HANDLING` | Manejo de 404 para assets | `single-page-application` |
