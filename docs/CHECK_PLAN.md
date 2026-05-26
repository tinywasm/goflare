> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.

# Plan: goflare/d1 — Consolidar en 2 constructores (`NewEdge` + `NewLocal`)

## Contexto y motivación

`goflare/d1` tiene hoy dos constructores y se planteaba un tercero. Tres es redundante.
Los constructores deben mapear a los **contextos de build**, y hay exactamente dos:

- `NewEdge(binding)` → **wasm/edge** (hoy llamado `New`). El runtime de Cloudflare
  entrega D1 por binding JS; no hay connection string. Inevitable.
- **host (`!wasm`)** → necesita UNA forma de conectar.

> **Nombres simétricos**: se renombra `New` → `NewEdge` para que ambos constructores
> queden calificados por su entorno (`NewEdge`/`NewLocal`), en vez de un `New` sin marca
> que oculta que es para el edge. "edge" ya es el vocabulario del repo (`edge/`,
> `edge.wasm`).

Hoy el host usa `NewDirect(token, accountID, dbID)` (REST a la **D1 real**). Eso es un
tercer eje ortogonal al build —un cliente host a un servicio remoto— y obliga a tener
credenciales de Cloudflare para desarrollar/testear, escribiendo en producción. Sus dos
usos reales los cubre mejor otra cosa:

- **Dev / lógica** → SQLite local: mismo dialecto que D1 (D1 *es* SQLite y `orm` usa el
  mismo `sqlt.NewCompiler()` en todos los contextos), sin credenciales, offline.
- **Validación real de producción** → el e2e por **HTTP** (deploy → POST → GET → assert),
  que ejercita el binding real + la D1 real de punta a punta — lo que usa el usuario.

**Decisión**: consolidar en **2 constructores** — `NewEdge` (edge) + `NewLocal` (host
SQLite). **Eliminar `NewDirect`** y el adaptador REST (198 líneas). `tinywasm/sqlite` ya
resuelve lo difícil: `sqlite.Open(dsn) (*orm.DB, error)` (driver `modernc.org/sqlite`,
Go puro).

---

## Stages

### Stage 0 — Renombrar `New` → `NewEdge` (`d1/adapter_wasm.go`, `//go:build wasm`)

```go
// NewEdge opens the named D1 binding (Cloudflare edge runtime) and returns an *orm.DB.
func NewEdge(bindingName string) (*orm.DB, error) {
	// ...cuerpo actual de New, sin cambios...
}
```

Único consumidor en este repo: ninguno (el edge lo usa el demo). Actualizar
referencias en docs (`docs/D1.md`) de `d1.New(` → `d1.NewEdge(`.

### Stage 1 — `d1/local.go` (nuevo, `//go:build !wasm`)

```go
//go:build !wasm

package d1

import (
	"github.com/tinywasm/orm"
	"github.com/tinywasm/sqlite"
)

// NewLocal opens a local SQLite database for host use (dev + tests) — no Cloudflare
// credentials, no network. D1 is SQLite under the hood and orm uses the same sqlt
// compiler in every context, so behavior matches the edge (New). Only the data
// location differs.
//
// path is a SQLite DSN: a file ("goflare-local.db") to persist across restarts, or
// ":memory:" for an ephemeral per-process database (tests).
func NewLocal(path string) (*orm.DB, error) {
	return sqlite.Open(path)
}
```

### Stage 2 — Eliminar `NewDirect` y el adaptador REST

- Borrar `d1/client.go` (define `NewDirect` + `directAdapter` REST, ~198 líneas).
- Verificar que nada más en `d1/` referencia esos símbolos (`d1RestClient`, etc.).

### Stage 3 — Convertir el test de integración en test local

`d1/d1_integration_test.go` usa `NewDirect` + credenciales reales con tag
`//go:build integration`. Reescribir como round-trip local **sin credenciales ni tag**
(`d1/local_test.go`, `//go:build !wasm`), usando `NewLocal(":memory:")`:

```go
//go:build !wasm

package d1_test

import (
	"testing"

	"github.com/tinywasm/fmt"
	"github.com/tinywasm/goflare/d1"
	"github.com/tinywasm/orm"
)

type item struct {
	ID   int
	Name string
}

func (m *item) ModelName() string { return "items" }
func (m *item) Schema() []fmt.Field {
	return []fmt.Field{
		{Name: "id", DB: &fmt.FieldDB{PK: true, AutoInc: true}},
		{Name: "name"},
	}
}
func (m *item) Pointers() []any { return []any{&m.ID, &m.Name} }

func TestNewLocal_RoundTrip(t *testing.T) {
	db, err := d1.NewLocal(":memory:")
	if err != nil {
		t.Fatalf("NewLocal: %v", err)
	}
	defer db.Close()

	if err := db.CreateTable(&item{}); err != nil {
		t.Fatalf("CreateTable: %v", err)
	}
	if err := db.Create(&item{Name: "hello"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got := &item{}
	if err := db.Query(got).Where("name").Eq("hello").ReadOne(); err != nil {
		t.Fatalf("ReadOne: %v", err)
	}
	if got.Name != "hello" {
		t.Errorf("got %q, want hello", got.Name)
	}

	// Update / Delete para cubrir el ciclo completo (orm.Eq como condición).
	got.Name = "world"
	if err := db.Update(got, orm.Eq("id", got.ID)); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if err := db.Delete(&item{}, orm.Eq("id", got.ID)); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}
```

Borrar `d1/d1_integration_test.go`.

### Stage 4 — `go.mod`

Añadir `github.com/tinywasm/sqlite` (arrastra `modernc.org/sqlite`, Go puro; solo
`!wasm`). `go mod tidy` — debería eliminar deps que solo usaba el adaptador REST si
quedaran huérfanas (el REST usaba stdlib `net/http`/`encoding/json`, así que no hay
cambios de módulo por ese lado).

### Stage 5 — `.github/workflows/test.yml`

Eliminar el step "Integration tests (D1)" (ya no hay test que requiera credenciales
Cloudflare). El nuevo `TestNewLocal_RoundTrip` corre dentro del `go test ./...` normal,
offline. Quitar el bloque `env:` con `CLOUDFLARE_*`/`D1_DATABASE_ID` de ese workflow.

---

## Resumen de archivos

| Archivo | Acción |
|---|---|
| `d1/adapter_wasm.go` | Renombrar `New` → `NewEdge` |
| `d1/local.go` | Nuevo — `NewLocal(path) (*orm.DB, error)` wrapping `sqlite.Open` |
| `d1/client.go` | **Borrar** — `NewDirect` + adaptador REST |
| `d1/d1_integration_test.go` | **Borrar** → reemplazado por `d1/local_test.go` |
| `d1/local_test.go` | Nuevo — round-trip CRUD contra `:memory:`, sin credenciales |
| `go.mod` | Añadir `github.com/tinywasm/sqlite`; `go mod tidy` |
| `.github/workflows/test.yml` | Quitar step "Integration tests (D1)" + env Cloudflare |
| `docs/D1.md` | Actualizar refs `d1.New(` → `d1.NewEdge(` |

---

## Verification

```bash
go test ./...        # TestNewLocal_RoundTrip pasa offline, sin credenciales
go vet ./...
GOOS=js GOARCH=wasm go build ./...   # el edge no arrastra tinywasm/sqlite
grep -rn "NewDirect\|directAdapter" .   # debe dar vacío (símbolos eliminados)
grep -rn "d1\.New(" .                    # debe dar vacío (renombrado a NewEdge)
```

## Seguimiento (fuera de este repo)

Tras publicar la nueva versión de goflare, en `tinywasm/goflare-demo`:

1. `modules/contact/db_wasm.go`: `d1.New("DB")` → `d1.NewEdge("DB")` (2 llamadas).
2. `modules/contact/db_host.go`: `d1.NewDirect(...env...)` → `d1.NewLocal("goflare-local.db")`.
   Gitignorar `*.db`. Prueba local sin credenciales.
3. `tests/e2e/contact_e2e_test.go`: ya no puede usar `NewDirect`. Reescribir la
   verificación para que vaya por **HTTP**: tras el POST, hacer `GET /api/contacto` y
   afirmar que la submission aparece en el array JSON. Así el e2e valida el stack real
   (binding + D1) por la misma vía que usa el usuario, sin credenciales D1 en el runner.
4. El job de deploy del demo conserva `CLOUDFLARE_*` (los necesita para desplegar), pero
   el paso de verificación deja de necesitar `D1_DATABASE_ID`.
