---
message: "feat: files.Store can key an object by its owner — one file per identity, replaced on re-upload"
---

> Este plan se despacha vía el flujo CodeJob. Ver skill: agents-workflow.
> Orquestado por `tinywasm/docs/DEMO_FOUR_APIS_MASTER_PLAN.md` — **Fase F**.

← [Etapa 3](PLAN_STAGE_3_EDGE_ACCESS.md) | Índice → [PLAN.md](PLAN.md)

# Etapa 4 — `files`: un archivo por dueño, reemplazado al subir otro

Autocontenido, en español.

## El caso que hoy no se puede escribir

"Cada usuario tiene **un** avatar. Si sube otro, sustituye al anterior."

Es el caso más común que existe, y hoy `files.Store` **no lo permite**: la clave la genera
`unixid` y es distinta en cada subida (`files/files.go`):

```go
// The key comes from the server. The client's filename is text it chose.
key := s.ids.GetNewID() + t.Ext
```

Diez subidas del mismo usuario = **diez objetos** en el bucket, y nueve basura que nadie borra
jamás. El consumidor no puede arreglarlo: si le dejas elegir la clave al cliente, vuelves a
abrir el agujero que esta política cerró (el nombre que manda el cliente es tan poco fiable
como su `Content-Type`).

La clave la sigue eligiendo el servidor. Lo que cambia es **de dónde la deduce**.

## Cambios

### 1. `PerOwner()` — la clave es la identidad del que sube

```go
// PerOwner keys the object by its uploader: ONE object per identity, replaced whenever that
// identity uploads again. The natural shape of an avatar, a profile photo, a signature.
//
// The key is the caller's UserID and nothing else — no extension. The type is NOT lost: it
// was deduced from the bytes and travels in the object's metadata, which is exactly where
// serve() already reads it from. An extension would defeat the point: uploading a .png and
// then a .jpg would produce TWO keys, and the first would be orphaned forever.
//
// It is only reachable on a guarded route (upload already Requires files/Create), so the
// identity is never empty by construction.
func (s *Store) PerOwner() *Store {
	s.perOwner = true
	return s
}
```

En `upload`, la clave pasa a ser:

```go
key := s.ids.GetNewID() + t.Ext
if s.perOwner {
	// The identity is guaranteed non-empty: the gate rejected the anonymous caller before
	// this handler ever ran.
	key = ctx.UserID()
}
```

El `Put` a R2 ya escribe el `Content-Type` real (`t.MIME`) en la metadata, y `serve` ya lo lee
de ahí. **No hay que tocar ninguno de los dos**: por eso la clave puede ir sin extensión.

### 2. El reemplazo es la semántica de R2, no código nuestro

`bucket.Put(key, ...)` sobre una clave existente **sobrescribe**. No escribas un `Delete`
previo: sería una ventana en la que el usuario no tiene archivo, y una llamada de red de más.

### 3. Documentar el contrato que esto crea

En el doc del paquete, dilo explícito: con `PerOwner()`, la clave de un usuario **es
adivinable** (es su id). Eso está bien —servir es público por diseño, un `<img src>` no manda
cabeceras— pero el consumidor tiene que saberlo: **no metas ahí nada que no quieras que sea
públicamente legible por quien conozca el id del usuario.**

## Anti-footguns

- **No dejes que el cliente elija la clave.** Ni con `PerOwner`, ni sin él, ni "solo el
  nombre". Es la política de seguridad de este paquete y no se negocia.
- **No añadas la extensión a la clave con `PerOwner`.** Parece inofensivo y rompe justo lo que
  el modo existe para dar: `foto.png` y `foto.jpg` serían dos objetos.
- **No hagas `PerOwner` el modo por defecto.** El modo actual (clave generada) es el correcto
  para galerías, adjuntos y todo lo que sea "muchos por usuario".

## Criterios de aceptación

- `gotest` pasa, **y la suite wasm se ejecuta de verdad** (`gotest -tinygo`).
- Test nuevo: **el mismo dueño sube dos imágenes distintas y el bucket acaba con UN objeto**,
  cuyo contenido es el de la segunda, byte a byte.
- Test nuevo: **dos dueños distintos suben y acaban con dos objetos**, uno cada uno.
- Test nuevo: subir `.png` y luego `.jpg` con el mismo dueño deja **un** objeto, servido con
  el `Content-Type` de la segunda (`image/jpeg`) — leído de la metadata, no de la extensión.
- El modo por defecto no cambia: sin `PerOwner()`, dos subidas del mismo dueño siguen
  produciendo dos claves distintas.
- La validación por bytes mágicos sigue intacta: un HTML disfrazado de PNG es **415** y **no
  escribe nada**, también en modo `PerOwner`.
