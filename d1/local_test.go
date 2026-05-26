//go:build !wasm

package d1_test

import (
	"testing"

	"github.com/tinywasm/fmt"
	"github.com/tinywasm/goflare/d1"
	"github.com/tinywasm/orm"
)

type item struct {
	ID   int64
	Name string
}

func (m *item) ModelName() string { return "items" }
func (m *item) Schema() []fmt.Field {
	return []fmt.Field{
		{Name: "id", Type: fmt.FieldInt, DB: &fmt.FieldDB{PK: true}},
		{Name: "name", Type: fmt.FieldText, DB: &fmt.FieldDB{}},
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
	if err := db.Create(&item{ID: 1, Name: "hello"}); err != nil {
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

	// Verify update
	got2 := &item{}
	if err := db.Query(got2).Where("id").Eq(1).ReadOne(); err != nil {
		t.Fatalf("ReadOne after update: %v", err)
	}
	if got2.Name != "world" {
		t.Errorf("got %q, want world", got2.Name)
	}

	if err := db.Delete(&item{}, orm.Eq("id", 1)); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Verify delete
	got3 := &item{}
	err = db.Query(got3).Where("id").Eq(got.ID).ReadOne()
	if err != orm.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
