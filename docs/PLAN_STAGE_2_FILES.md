> Este plan se despacha vía el flujo CodeJob. Ver skill: agents-workflow.

Etapa 2 de 2 · ← [Etapa 1](PLAN_STAGE_1_ROUTER.md) · Índice → [PLAN.md](PLAN.md)

# Etapa 2 — Subir y servir archivos en el borde: cuerpo binario + bucket R2

**Qué hace esta etapa, en una frase:** arregla un bug de **corrupción silenciosa** de datos
binarios y añade el almacenamiento de objetos que hoy no existe.

**Prerrequisito:** la Etapa 1 está aplicada y publicada. El paquete del borde se llama
`edge/` y habla el contrato `github.com/tinywasm/router`.

**Lee antes de tocar código:** [`AGENTS.md`](../AGENTS.md) en la raíz del repo — reglas del
arnés (los dos objetivos de compilación, `gotest` en vez de `go test`, fallo ruidoso) y
[`docs/TESTING.md`](TESTING.md) — cómo se prueba sin desplegar a Cloudflare.

---

## El problema (léelo antes de tocar nada)

`goflare` **no puede subir archivos hoy**, y el motivo no es que falte una función:

**1. El cuerpo de la petición se destruye si es binario.** En `workers/request.go`, la
función `newRequest` llama **siempre** a `req.text()`, que decodifica el cuerpo como UTF-8.
Cualquier byte que no forme UTF-8 válido —o sea, casi todos los de un JPG, un PNG o un PDF—
se sustituye por el carácter de reemplazo `U+FFFD`. **No hay error: el archivo se guarda
corrupto.** La firma `Request.Body() []byte` miente: devuelve texto re-encodeado, no los
bytes que llegaron.

**2. No hay almacenamiento de objetos.** El paquete `d1/` envuelve el binding de D1, pero no
existe el equivalente para un bucket. `cloudflare/env_wasm.go` solo lee variables de entorno
como `string`.

## Decisiones ya tomadas (no las reabras)

- **Almacén: R2** (object storage de Cloudflare, accedido por *binding* igual que D1).
  Descartados D1 (es SQLite: límites por fila, y los binarios degradan las consultas) y KV
  (valores pequeños, consistencia eventual).
- **Transporte: cuerpo crudo** (`PUT` con los bytes tal cual), **NO `multipart/form-data`**.
  Un parser de multipart dentro del wasm engorda el binario, y este ecosistema tiene un plan
  activo de reducción de tamaño. El frontend ya es Go/wasm con `tinywasm/fetch`: mandar bytes
  crudos no le cuesta nada.

## ⚠️ Anti-footgun

Todo lo que tocas en esta etapa es `//go:build wasm`: `workers/`, el nuevo `r2/`, `edge/`.
Aquí **sí** rige la regla tinywasm: **nada de librería estándar** — usa `tinywasm/fmt`, no
`errors`/`fmt`/`strconv`. (Los archivos `!wasm` del repo —`build.go`, `mode.go`,
`cloudflare.go`— sí usan stdlib legítimamente; **no los toques ni los "arregles"**.)

---

## Paso 1 — Cuerpo binario **y perezoso** en `workers/request.go`

Dos cambios en la misma función, y el segundo es de seguridad.

**1a. Binario.** Cambia la lectura del cuerpo para que use **`req.arrayBuffer()`** en vez de
`req.text()`, copiando los bytes con **`js.CopyBytesToGo`**.

- Mantén el **mismo patrón bloqueante** que ya usa el código: canal + `js.Func` para `then`
  y `catch`, liberadas con `Release()` al resolverse la promesa. No inventes un mecanismo de
  concurrencia nuevo — cambia la *fuente* de los bytes, no la mecánica.
- **Borra la función `readBodyText` por completo.** No la conserves "por compatibilidad": un
  cuerpo de texto es un caso particular de un cuerpo de bytes (`string(ctx.Body())`), no al
  revés. Dejar las dos rutas es dejar la trampa abierta.

**1b. Perezoso — el cuerpo NO se lee hasta que alguien lo pide.**

Hoy `newRequest` lee el cuerpo **entero** ([workers/workers.go](../workers/workers.go), en
`Handle`), y lo hace **antes** de que corra el enrutado y **antes** del RBAC. Consecuencia:
una petición **anónima** puede obligar al worker a bufear en memoria todo lo que le manden,
y el `Requires("files","write")` se evalúa *después*, cuando el daño ya está hecho. Un
límite de tamaño dentro del handler **llega tarde por diseño**.

Arreglo: `newRequest` **deja de leer el cuerpo**. `Request.Body()` lo lee en su **primera
llamada** y cachea el resultado (`body []byte` + un `bool` de "ya leído"). Así el handler
puede mirar `Content-Length`, responder **413** y no tocar un solo byte.

## Paso 2 — Validar que el archivo es legítimo: usa `tinywasm/filetype`

**NO escribas un validador.** Ya existe: **`github.com/tinywasm/filetype` (v0.0.2, publicada)**
hace exactamente esto y es isomórfica (sin build tags, sin stdlib, una sola dependencia). Solo
tienes que consumirla.

```go
import "github.com/tinywasm/filetype"

// Images es la política segura por defecto: solo imágenes ráster, nada ejecutable.
t, err := filetype.Images.Validate(data)
if err != nil {
	ctx.WriteStatus(415)   // el error NOMBRA lo que era en realidad: "filetype: PDF is not allowed"
	return
}
// t.MIME → "image/png"   (el tipo REAL, deducido de los bytes)
// t.Ext  → ".png"        (la extensión sale de los bytes, no del nombre que mandó el cliente)
```

Reglas, todas obligatorias:

- **Nunca confíes en el `Content-Type` que manda el cliente: es texto que él elige.** El tipo
  que **almacenas y sirves** es el que devolvió `filetype`, jamás el que declaró el cliente.
- **Usa `filetype.Images`** (PNG, JPEG, GIF, WebP). **No metas SVG ni HTML en la lista
  blanca:** un SVG **lleva JavaScript dentro** y, servido desde tu dominio, se ejecuta **en tu
  origen** — XSS de manual. La librería los **detecta a propósito** para poder rechazarlos por
  su nombre.
- Contenido no reconocido → **415, sin escribir NADA en el bucket.**

### ⚠️ Cómo NO validar (esto engordaría el wasm)

La regla que justifica que `filetype` exista: **validar por bytes mágicos, jamás
decodificando.** Decodificar un PNG "para comprobar que es válido" arrastra `image/png` +
`compress/zlib`: **cientos de KB** en un binario que este ecosistema está intentando
adelgazar. **Prohibidos en el borde:**

- `net/http.DetectContentType` — es stdlib (prohibida en wasm) y además sniffea a tipos
  HTML/texto que justamente no queremos.
- Cualquier paquete `image/*` para "validar".
- Cualquier librería de MIME.

## Paso 3 — La clave la genera el servidor, no el cliente

El nombre que manda el cliente es tan poco fiable como su `Content-Type`: una clave con
`../` o con barras te ensucia el bucket.

- **Genera la clave en el servidor** con `github.com/tinywasm/unixid` (`NewUnixID()` →
  `GetNewID()`), que **ya está en el grafo de dependencias**, tiene variante `//go:build
  wasm` y solo depende de `tinywasm/fmt` y `tinywasm/time`. Coste de tamaño: despreciable.
- La clave final es `<id><ext>`, donde `ext` **sale del sniffing** del paso 2 — nunca del
  nombre que mandó el cliente.
- El handler **devuelve la clave generada** en la respuesta, para que el cliente sepa dónde
  quedó el archivo. Si quieres conservar el nombre original, va como **metadato**, jamás como
  clave.

## Paso 4 — Respuesta binaria simétrica en `workers/response.go`

`Response.Write` ya acumula `[]byte`, pero **verifica que `build()` entrega esos bytes al
`Response` de JS sin pasar por `string`**. Si pasa por `string`, servir un archivo de vuelta
lo corrompe exactamente igual que al subirlo. Arréglalo si es el caso (usa
`js.CopyBytesToJS` sobre un `Uint8Array`).

Sin este paso, la ida y vuelta sigue rota aunque el paso 1 esté bien.

## Paso 5 — Paquete nuevo `r2/` — espejo exacto de `d1/`

Crea `r2/bucket.go` y `r2/errors.go`. Estructura calcada de `d1/` (mismo estilo de
adaptador delgado sobre `syscall/js`, sin lógica de negocio).

```go
// package r2 (github.com/tinywasm/goflare/r2)   //go:build wasm

type Bucket struct{ /* js.Value del binding */ }

// NewEdge obtiene el bucket desde el binding declarado en wrangler (p. ej. "FILES").
// Falla ruidosamente si el binding no existe: un bucket ausente es un error de
// configuración, NUNCA un *Bucket nulo devuelto en silencio.
func NewEdge(binding string) (*Bucket, error)

func (b *Bucket) Put(key string, data []byte, contentType string) error
func (b *Bucket) Get(key string) (data []byte, contentType string, err error)
func (b *Bucket) Delete(key string) error
func (b *Bucket) List(prefix string) ([]ObjectInfo, error)

type ObjectInfo struct {
	Key         string
	Size        int64
	ContentType string
}
```

Reglas:
- El binding se lee de `context.env.<BINDING>` (el objeto global que inyecta el runtime),
  igual que hace `cloudflare/env_wasm.go` con las variables de entorno.
- Todas las llamadas a R2 devuelven promesas de JS: resuélvelas con el **mismo patrón
  bloqueante** que ya usa `d1/`.
- **`Put` recibe `[]byte`, nunca `string`.** Es el punto exacto donde el bug del paso 1 se
  volvería a colar.
- **El paquete `r2/` no importa nada más** que `syscall/js` y `tinywasm/fmt`. (Ojo: esto es
  una restricción **de este paquete**, no del módulo — la etapa sí añade dos dependencias al
  `go.mod`, ver abajo.)

### `go.mod` — las dos dependencias que esta etapa SÍ añade

```bash
go get github.com/tinywasm/filetype@v0.0.2   # validación por bytes mágicos (paso 2)
go get github.com/tinywasm/unixid            # generación de la clave (paso 3)
```

Las dos son isomórficas y ligeras: `filetype` no tiene build tags y solo depende de
`tinywasm/fmt`; `unixid` tiene variante `//go:build wasm` y depende de `tinywasm/fmt` y
`tinywasm/time`. **Ninguna decodifica nada ni arrastra stdlib**, que es justo el motivo de
elegirlas.

## Paso 6 — Declarar el binding R2 en la configuración generada

`goflare` genera la configuración de wrangler y ya declara ahí el binding de D1. Añade el
del bucket R2 de la misma forma (archivos `javascripts.go` / plantillas de generación).

## Paso 7 — Compilar y probar

```bash
GOOS=js GOARCH=wasm go build ./...
gotest                               # NUNCA `go test`
```

## Cómo se prueba esto (no lo improvises)

**Lee [TESTING.md](TESTING.md) antes de escribir un test.** Es especialmente importante en
esta etapa, porque la tentación es "hay que desplegar a Cloudflare para probar R2". **No.**

- **Los tests van en `tests/`**, como `package goflare_test` — es la convención del repo. La
  API de `r2/` es pública (`NewEdge`, `Put`, `Get`), así que se prueba en caja negra desde
  ahí. El fake de `context.env` va como helper compartido en `tests/`, no duplicado en cada
  archivo.

- El binding de R2 se lee de `js.Global().Get("context").Get("env").Get("FILES")`. Esa es
  **toda** la frontera. En el test se inyecta un `context.env` falso **escrito en Go** con
  `syscall/js`, y el código real corre entero contra él: interop, promesas y copia de bytes.
  `TESTING.md` trae el fake de bucket R2 listo para copiar.
- **El fake almacena `[]byte`, jamás `string`.** Un fake que pase por `string` escondería
  exactamente el bug de corrupción que esta etapa arregla, y el test pasaría en verde
  mintiendo.
- **El fake devuelve Promesas**, porque el binding real lo hace. Si devuelve valores
  síncronos, un manejo de promesas roto pasaría el test.
- **Nada de wrangler en el bucle.** El único test que lo justifica es el de humo (Nivel 3):
  subir un binario a R2 real de miniflare (`wrangler dev --local`, que **no despliega**) y
  recuperarlo, para demostrar que el fake no miente. Uno, no una suite.

---

## Cómo se usan los archivos desde un módulo (para tu test, y como referencia)

**No se añade NADA al contrato `tinywasm/router`.** No existe `Param()` ni hará falta: se
evaluó y se descartó por innecesario. Con `Body()` (ya binario-seguro tras el paso 1) y
`Path()` es suficiente.

La clave del archivo se saca de la ruta, apoyándose en el **match por prefijo** que la Etapa
1 añadió al router del borde: un patrón **terminado en `/`** captura todo lo que cuelga de
él, y `ctx.Path()` devuelve la **ruta concreta** de la petición.

**Subir — el cliente NO elige ni el nombre ni el tipo.** La ruta de subida es el prefijo
desnudo; la clave la devuelve el servidor:

```go
const (
	filesPrefix = "/api/files/"
	maxFileSize = 10 << 20 // 10 MiB — constante, nunca un literal suelto
)

r.Put(filesPrefix, func(ctx router.Context) {
	// 1. Tamaño ANTES de leer el cuerpo. Body() es perezoso (paso 1b): aquí no se
	//    ha bufeado ni un byte todavía.
	if n := contentLength(ctx); n > maxFileSize {
		ctx.WriteStatus(413)
		return
	}

	data := ctx.Body()

	// 2. El tipo sale de los BYTES, no de la cabecera del cliente.
	t, err := filetype.Images.Validate(data)
	if err != nil {
		ctx.WriteStatus(415) // tipo no permitido — no se escribe NADA en el bucket
		return
	}

	// 3. La clave la genera el servidor. Nunca el nombre que mandó el cliente.
	key := id.GetNewID() + t.Ext

	if err := bucket.Put(key, data, t.MIME); err != nil {
		ctx.WriteStatus(502)
		return
	}
	ctx.WriteStatus(201)
	ctx.Write([]byte(key)) // el cliente se entera aquí de dónde quedó su archivo
}).Requires("files", "write")   // subir es privado
```

**Servir — la clave viaja en la ruta, y se sirve con las cabeceras de seguridad:**

```go
r.Get(filesPrefix, func(ctx router.Context) {
	key := ctx.Path()[len(filesPrefix):]   // sin importar "strings"
	if key == "" {
		ctx.WriteStatus(400)
		return
	}
	data, ct, err := bucket.Get(key)
	if err != nil {
		ctx.WriteStatus(404)
		return
	}
	ctx.SetHeader("Content-Type", ct)              // el tipo que dedujimos al subir
	ctx.SetHeader("X-Content-Type-Options", "nosniff") // el navegador no adivina el tipo
	ctx.Write(data)
}).Public()   // servir es público: un <img src> no puede mandar cabeceras
```

**Por qué la clave va en la URL y no en una cabecera:** servir un archivo lo dispara el
navegador con `<img src="/api/files/1721…jpg">`, y ahí **el navegador no manda cabeceras
personalizadas**. La clave tiene que viajar en la ruta.

Recuerda la regla de la Etapa 1: **privado por defecto**. Una ruta sin `.Public()` ni
`.Requires()` responde 403.

---

## Reglas de calidad (obligatorias)

- **Sin strings repetidos en la lógica:** nombres de binding, claves y cabeceras repetidas
  van a constantes con nombre.
- **Nada de stdlib** en `wasm` — `tinywasm/fmt` para errores.
- **Fallo ruidoso siempre:** un binding ausente, una promesa rechazada o un `Get` de una
  clave inexistente devuelven error con contexto. Jamás un `nil` mudo.

## Criterios de aceptación (verificables)

```bash
# 1. No queda ninguna ruta de lectura como texto
grep -rn "readBodyText\|\.Call(\"text\")" workers/   # → vacío

# 2. El paquete r2 existe, compila para el borde y los tests pasan
GOOS=js GOARCH=wasm go build ./...
gotest
```

Y el test que **define esta etapa**:

- **Ida y vuelta binaria, byte a byte.** Sube un archivo cuyos bytes **no sean UTF-8 válido**
  (por ejemplo `0xFF 0xFE 0x00 0x80`), recupéralo, y compara con `bytes.Equal` contra el
  original.

  **Este test FALLA con el código actual.** Es exactamente el que demuestra que el bug
  existía y que lo arreglaste. Si no lo escribes, esta etapa no está hecha.

Además:
- **`r2.NewEdge` falla ruidosamente** cuando el binding no está declarado: devuelve un error
  que **nombra el binding que buscaba**, no un `*Bucket` nulo.
- **El `.wasm` del borde no crece de forma apreciable.** Es el resultado esperado de haber
  elegido cuerpo crudo en vez de multipart. Si crece mucho, alguien metió un parser.

---

## Consecuencia fuera de este repo (no es tu trabajo)

La verificación real —subir una imagen desde el navegador y verla servida de vuelta sin
corromperse— ocurre en `goflare-demo`, en su propio plan, después de publicar esta etapa.

Referencia (opcional): https://github.com/tinywasm/goflare-demo/blob/main/docs/PLAN.md
