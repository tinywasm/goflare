---
PLAN: "fix: goflare compila contra el ecosistema actual (router, orm, unixid, sqlt)"
EXECUTOR: jules
REVIEWER: none
STATUS: review
SESSION: 3282759745990873760
PR: https://github.com/tinywasm/goflare/pull/21
---

> Este plan se despacha con el flujo CodeJob. Ver skill: agents-workflow.
>
> **Este plan es autocontenido.** No necesitas leer PRs anteriores ni otros
> repos, y no debes tocar otros repos desde aquí.

# Plan — `goflare` compila contra el ecosistema actual

`goflare v0.4.3` **no compila dentro de ningún consumidor actual.** No es un
riesgo futuro: es el estado de hoy, y bloquea el arranque de `veltylabs/misitio`,
cuya etapa 1 entera consiste en desplegar un Worker con `goflare/edge`.

## La medición

El repo compila **consigo mismo** porque su `go.mod` pinnea versiones viejas
(`router v0.1.14`, `orm v0.9.28`, `unixid v0.2.23`, `sqlt v0.0.7`). Un consumidor
real trae las actuales por MVS —`server`, `orm` y `router` ya están ahí— y
entonces:

```
$ GOOS=js GOARCH=wasm go build ./edge/ ./d1/ ./r2/ ./files/
files/files.go:144:15: s.ids.GetNewID undefined (type *unixid.UnixID has no field or method GetNewID)
d1/adapter.go:22:37: too many arguments in call to orm.New
	have (*adapter, *sqlt.compiler)
	want (storage.Conn)
d1/adapter.go:31:59: undefined: orm.Scanner
d1/adapter.go:45:57: undefined: orm.Rows
d1/rows.go:82:11: undefined: orm.Rows
# github.com/tinywasm/sqlt
.../sqlt@v0.0.7/compiler.go:14:33: undefined: orm.Query
.../sqlt@v0.0.7/compiler.go:14:60: undefined: orm.Plan
```

Tres rupturas independientes, todas aguas arriba, todas ya resueltas en las
librerías; falta **adoptarlas aquí**.

**Lo que el PR #20 ya arregló y no hay que rehacer:** `edge` implementa
`Accepts` y `Decode`, así que la conformidad con `router` está completa.
`edge/` **no se toca en este plan**.

---

## 0. Reglas de desarrollo

`edge/`, `d1/`, `r2/` y `files/` son **wasm** (`//go:build wasm`). Por lo tanto:

- **Sin `fmt`, `errors`, `strconv`, `strings`, `log`** de la stdlib. Usa
  `github.com/tinywasm/fmt`.
- **Sin `map[K]V`**, sin `reflect`, sin `encoding/json`.
- **Anti-footgun:** la raíz (`goflare.go`, `build.go`, `run.go`, `cloudflare.go`)
  y `cmd/` son **host-only** (`//go:build !wasm`) y usan la stdlib
  legítimamente. `devserver/` también. **No les apliques las reglas de arriba.**
- Código en inglés; documentación y comentarios de prosa en español.
- Sin strings mágicos: todo string repetido es una constante nombrada.

---

## 1. `go.mod` — adoptar las versiones actuales

Sube exactamente estas cuatro:

```
github.com/tinywasm/router  v0.1.14 -> v0.1.23
github.com/tinywasm/orm     v0.9.28 -> v0.11.7
github.com/tinywasm/unixid  v0.2.23 -> v0.2.26
github.com/tinywasm/sqlt    v0.0.7  -> v0.0.8
```

Con `go get` seguido de `go mod tidy`. `github.com/tinywasm/storage` pasa a ser
dependencia **directa** (§3 la importa); deja que `tidy` la coloque.

**No subas nada más.** Si `tidy` arrastra otra cosa por transitividad, déjala,
pero no vayas a buscar actualizaciones que este plan no pide.

---

## 2. `files/files.go` — `unixid` renombró el generador

`unixid v0.2.26` renombró `GetNewID()` a `NewID()`. Una línea, la 144:

```go
key := s.ids.NewID() + t.Ext
```

**Verificación:** `grep -rn "GetNewID" .` → vacío.

---

## 3. `d1/` — portar al contrato `storage.Conn`

Es el cambio con sustancia. `orm` dejó de recibir ejecutor y compilador como
**dos argumentos** y ahora recibe **uno solo**: `storage.Conn`, que es la unión
de `Executor` y `Compiler`.

El porqué está escrito en el propio `storage/conn.go` y vale repetirlo, porque
es lo que hace obvio el porte: un `Executor` de un backend emparejado con un
`Compiler` de otro era un estado ilegal que la firma de dos argumentos permitía
representar. `Conn` lo vuelve imposible.

### 3.1 `d1/adapter.go`

El adaptador ya implementaba `Executor` (`Exec`, `QueryRow`, `Query`, `Close`).
Ahora tiene que implementar también `Compiler`, y la forma más corta es
**embeber la interfaz** y dejar que el compilador de `sqlt` la satisfaga:

```go
import (
	"syscall/js"

	"github.com/tinywasm/await"
	"github.com/tinywasm/jsvalue"
	"github.com/tinywasm/orm"
	"github.com/tinywasm/sqlt"
	"github.com/tinywasm/storage"
)

type adapter struct {
	dbObj js.Value
	storage.Compiler
}
```

**Anti-footgun:** se embebe la **interfaz** `storage.Compiler`, no el tipo que
devuelve `sqlt.NewCompiler()` — ese tipo es **no exportado** (`*sqlt.compiler`) y
no se puede embeber por nombre desde otro paquete.

El constructor pasa a un solo argumento:

```go
return orm.New(&adapter{dbObj: v, Compiler: sqlt.NewCompiler()}), nil
```

Y las dos firmas que nombraban tipos que `orm` ya no reexporta:

```go
func (a *adapter) QueryRow(query string, args ...any) storage.Scanner
func (a *adapter) Query(query string, args ...any) (storage.Rows, error)
```

`orm.ErrNotFound` **sigue existiendo** y se queda como está: no lo cambies.

### 3.2 `d1/rows.go`

La aserción de conformidad de la línea 82:

```go
var _ storage.Rows = (*d1Rows)(nil)
```

Cambia el import de `orm` por `storage` en ese archivo. Si tras el cambio `orm`
queda sin usar ahí, quítalo — `go vet` lo va a marcar.

**Verificación:** `grep -rn "orm\.Scanner\|orm\.Rows\|orm\.Query\b\|orm\.Plan" .` → vacío.

---

## 4. Tests

No hay tests nuevos que escribir: este plan no agrega comportamiento, adopta
contratos. Lo que sí es obligatorio es que **la batería existente siga verde**,
incluida la de conformidad del borde (`tests/edge_conformance_test.go`), que es
la que demuestra que `edge` sigue satisfaciendo `router`.

`d1/` no tiene test propio porque su adaptador sólo existe bajo `GOOS=js` contra
un binding real de Cloudflare. **El compilador es su test**: el criterio de
aceptación de §6 que construye `./d1/` para wasm es lo que verifica el porte.
No inventes un mock de `js.Value` para cubrirlo — no aporta y ata el test a la
implementación.

---

## 5. Etapas

| # | Etapa | Archivos | Resultado |
|---|---|---|---|
| 1 | Versiones al día | `go.mod`, `go.sum` | las cuatro subidas, `storage` directa |
| 2 | `unixid` | `files/files.go` | `NewID()` |
| 3 | Contrato `storage.Conn` | `d1/adapter.go`, `d1/rows.go` | un solo argumento en `orm.New` |
| 4 | Documentación | `docs/ARCHITECTURE.md` | la tabla de dependencias al día, si la cita |

---

## 6. Criterios de aceptación

- [ ] `go build ./...` limpio.
- [ ] `go vet ./...` limpio.
- [ ] `go test ./...` en verde.
- [ ] `GOOS=js GOARCH=wasm go build ./edge/ ./d1/ ./r2/ ./files/` **sin errores**
      — es el criterio que define este plan.
- [ ] `grep -rn "GetNewID" .` → vacío.
- [ ] `grep -rn "orm\.Scanner\|orm\.Rows\|orm\.Query\b\|orm\.Plan" .` → vacío.
- [ ] `grep -n "tinywasm/sqlt v0.0.8\|tinywasm/router v0.1.23\|tinywasm/orm v0.11.7\|tinywasm/unixid v0.2.26" go.mod` → cuatro líneas.
- [ ] `git diff --stat` **no** toca `edge/edge.go`: la conformidad con `router`
      ya la resolvió el PR #20.
- [ ] `grep -n "func NewEdge" d1/adapter.go` → la firma pública
      `NewEdge(bindingName string) (*orm.DB, error)` **no cambia**.

## 7. Fuera de alcance

- `edge/` — ya conforma con `router`. No lo toques.
- Los consumidores de `goflare` (`tinywasm/deploy`, `goflare-demo`, `app`): la
  cascada de publicación los actualiza, y si alguno necesita ajustes son suyos,
  no de este repo.
- Agregar capacidades: `Op`/`OpRegistry` está reportado como no soportado por la
  batería de conformidad y **queda así**; implementarlo es otro plan.
- Tocar `devserver/` o la raíz host-only.
