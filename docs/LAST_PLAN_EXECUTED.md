---
PLAN: "fix: el Response del Worker deja de arrastrar las tablas Unicode"
EXECUTOR: jules
REVIEWER: none
---

> Este plan se despacha con el flujo CodeJob. Ver skill: agents-workflow.

# Plan — `tinywasm/goflare`: quitar `bytes` del Worker

## El problema, en un campo

`workers/response.go` acumula el cuerpo de la respuesta en un `bytes.Buffer`:

```go
import (
	"bytes"
	"syscall/js"
)

type Response struct {
	status  int
	headers map[string]string
	buf     bytes.Buffer   // ← esto
}
```

`bytes` importa `unicode`, y `unicode` trae sus tablas de rangos completas
—categorías, scripts, mayúsculas/minúsculas—. **Medido sobre un Worker real
(`veltylabs/misitio`) son 93.733 bytes**, casi un 13 % del binario, por un campo
que sólo necesita acumular bytes y entregarlos al final.

El límite de `functions/edge.wasm` en Cloudflare es **1 MB duro**: el despliegue
falla, no degrada. Cada KB del Worker es presupuesto que una aplicación no puede
gastar en su propia lógica.

## El cambio

```go
import (
	"syscall/js"
)

type Response struct {
	status  int
	headers map[string]string
	buf     []byte
}

// Write agrega bytes al cuerpo de la respuesta.
func (w *Response) Write(b []byte) (int, error) {
	w.buf = append(w.buf, b...)
	return len(b), nil
}

// WriteString agrega una cadena al cuerpo de la respuesta.
func (w *Response) WriteString(s string) (int, error) {
	w.buf = append(w.buf, s...)
	return len(s), nil
}
```

Y en `build()`, `w.buf.Bytes()` pasa a ser `w.buf`.

`append([]byte, string...)` es válido en Go y no copia por un camino distinto al
de `bytes.Buffer`; `Write` y `WriteString` conservan su firma
`(int, error)` para seguir satisfaciendo `io.Writer` **estructuralmente**, sin
que este paquete importe `io`.

`buf` empieza en `nil`: `append` sobre un slice nil asigna en la primera
escritura, que es lo que `bytes.Buffer` hacía de todos modos. **No lo
preasignes con `make`** — un tamaño inventado desperdicia memoria en las
respuestas cortas, que son la mayoría.

## Verificación de que no quedan puertas abiertas

Este arreglo es la mitad de un par. La otra mitad vive en `tinywasm/user` y sale
en su propio plan. Están **acopladas**: `unicode` entra al binario por más de un
camino, y medido, quitar sólo este campo con la otra puerta abierta rinde
**2.119 bytes**; con las dos cerradas, **93.733**.

Consecuencia para quien ejecute este plan: **no te alarmes si la medición local
da una ganancia pequeña.** No es que el cambio no sirva; es que el otro camino
sigue abierto en la aplicación con la que midas. El criterio de aceptación de
abajo mide sobre el paquete, no sobre una aplicación, justamente por eso.

## Barrido del resto del repositorio

`bytes` no es el único riesgo. Revisa **todo el código que compile para el
Worker** —`workers/`, `edge/`, `d1/`, `r2/`, `cloudflare/`, `log/`— y sustituye
cualquier import de estos paquetes de la biblioteca estándar:

| Si encuentras | Usa |
|---|---|
| `bytes` | `[]byte` con `append` |
| `strings` | `github.com/tinywasm/fmt` (`HasPrefix`, `Contains`, `Convert(s).TrimPrefix(p).String()`) |
| `strconv` | `github.com/tinywasm/fmt` |
| `fmt` | `github.com/tinywasm/fmt` |
| `errors` | `fmt.Err(...)` / `fmt.Errf(...)` de `tinywasm/fmt` |
| `encoding/json` | `github.com/tinywasm/json` |

**Anti-footgun:** esto aplica al código que se compila para el Worker. Los
`_test.go` y cualquier herramienta de build de este repo compilan con Go estándar
y **usan la stdlib legítimamente**. No "arregles" sus imports.

## Criterios de aceptación

- [ ] `GOOS=js GOARCH=wasm go list -deps ./workers/` **no contiene** `bytes`,
      `unicode`, `strings`, `strconv`, `fmt` ni `errors`.
- [ ] Lo mismo para `./edge/`, `./d1/`, `./r2/`, `./cloudflare/` y `./log/`.
- [ ] `Write`, `WriteString` y `Header` conservan sus firmas: ningún consumidor
      cambia una línea.
- [ ] Los tests actuales del repositorio pasan sin modificarse.
- [ ] Un test nuevo comprueba que el cuerpo sale intacto: tres `Write` y dos
      `WriteString` alternados producen exactamente la concatenación esperada,
      incluidos bytes no ASCII (UTF-8 multibyte) y un `Write` de slice vacío.

## Fuera de alcance

Cualquier otro cambio en la API de `goflare`. Este plan es una sustitución de
tipo y un barrido de imports, nada más.
