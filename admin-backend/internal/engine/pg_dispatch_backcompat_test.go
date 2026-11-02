package engine

import (
	"context"
	"strings"
	"testing"

	"admin-backend/internal/modules"
)

// Test 12: el despacho N=1 NO abre transacción; multi-tabla SÍ abre BeginTx.
func TestExec_DispatchTransaction(t *testing.T) {
	ctx := context.Background()

	t.Run("N=1: sin transacción", func(t *testing.T) {
		drv := &recordedDriver{}
		e := newRecordedExecutor(drv)
		def := modules.Definition{
			Slug:           "planes",
			SourceType:     modules.SourcePostgres,
			FetchMapping:   map[string]any{"table": "planes"},
			DisplayMapping: map[string]any{"columns": []any{map[string]any{"field": "id", "isPrimaryKey": true}, map[string]any{"field": "monto", "editable": true}}},
			DataSource:     modules.DataSource{PGWhitelist: map[string][]string{"planes": {"id", "monto"}}},
		}
		act := modules.Action{Kind: modules.ActionKindEdit, SQLOp: "UPDATE", InputFields: []modules.FieldSpec{{Name: "monto", Source: modules.FieldSourceInput}}}
		input := map[string]any{"id": 7, "monto": 100}
		_, _, err := e.Exec(ctx, def, "", act, input)
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		if drv.began != 0 {
			t.Fatalf("N=1 no debe abrir transacción, began=%d", drv.began)
		}
		if len(drv.execs) != 1 {
			t.Fatalf("N=1 debe emitir 1 sentencia: %v", drv.execs)
		}
		if !strings.HasPrefix(drv.execs[0], `UPDATE "planes"`) {
			t.Fatalf("SQL N=1 inesperado: %s", drv.execs[0])
		}
	})

	t.Run("multi-tabla: abre transacción y hace commit", func(t *testing.T) {
		drv := &recordedDriver{}
		e := newRecordedExecutor(drv)
		def := modules.Definition{
			Slug:       "pagos-clientes",
			SourceType: modules.SourcePostgres,
			FetchMapping: map[string]any{
				"table":         "pagos",
				"foreignTables": []any{map[string]any{"table": "clientes"}},
				"relationships": []any{map[string]any{"childTable": "pagos", "parentTable": "clientes", "on": []any{map[string]any{"child": "cliente_id", "parent": "id"}}, "joinType": "inner"}},
			},
			DisplayMapping: map[string]any{"columns": []any{
				map[string]any{"field": "id", "isPrimaryKey": true, "sourceTable": "pagos"},
				map[string]any{"field": "monto", "editable": true, "sourceTable": "pagos"},
				map[string]any{"field": "id", "isPrimaryKey": true, "sourceTable": "clientes"},
			}},
			DataSource: modules.DataSource{PGWhitelist: map[string][]string{"pagos": {"id", "cliente_id", "monto"}, "clientes": {"id"}}},
		}
		act := modules.Action{Kind: modules.ActionKindEdit, SQLOp: "UPDATE", InputFields: []modules.FieldSpec{{Name: "monto", SourceTable: "pagos", Source: modules.FieldSourceInput}}}
		input := map[string]any{"pagos__id": 7, "monto": 100}
		_, _, err := e.Exec(ctx, def, "", act, input)
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		if drv.began != 1 || drv.committed != 1 {
			t.Fatalf("multi-tabla debe abrir y confirmar 1 transacción: began=%d committed=%d", drv.began, drv.committed)
		}
	})
}

// TestFindListAction_IgnoresSearchKind confirma que findListAction empareja solo
// Kind==ActionKindList: una acción de Kind "search" (lectura con filtro, hermana
// de 'list') NO sustituye a la lista BASE. Con solo una acción search presente,
// findListAction no debe encontrar nada.
func TestFindListAction_IgnoresSearchKind(t *testing.T) {
	t.Run("search sola: no hay lista BASE", func(t *testing.T) {
		def := modules.Definition{
			Actions: []modules.Action{
				{ActionKey: "buscar", Kind: modules.ActionKindSearch, SQLOp: "SELECT"},
			},
		}
		if _, ok := findListAction(def); ok {
			t.Fatal("findListAction no debe emparejar una acción Kind=search")
		}
	})

	t.Run("search junto a list: elige la de Kind=list", func(t *testing.T) {
		def := modules.Definition{
			Actions: []modules.Action{
				{ActionKey: "buscar", Kind: modules.ActionKindSearch, SQLOp: "SELECT"},
				{ActionKey: "list", Kind: modules.ActionKindList, SQLOp: "SELECT"},
			},
		}
		got, ok := findListAction(def)
		if !ok {
			t.Fatal("findListAction debió encontrar la acción Kind=list")
		}
		if got.Kind != modules.ActionKindList {
			t.Fatalf("findListAction eligió Kind=%q, esperaba %q", got.Kind, modules.ActionKindList)
		}
	})
}

// Test 6: compatibilidad byte-por-byte N=1 en List (SELECT sin JOIN, peladas) y
// Exec (INSERT/UPDATE/DELETE sin transacción).
func TestBackcompat_N1(t *testing.T) {
	ctx := context.Background()
	def := modules.Definition{
		Slug:           "planes",
		SourceType:     modules.SourcePostgres,
		FetchMapping:   map[string]any{"table": "planes"},
		DisplayMapping: map[string]any{"columns": []any{map[string]any{"field": "id", "isPrimaryKey": true, "visible": true}, map[string]any{"field": "monto", "editable": true, "visible": true}}},
		DataSource:     modules.DataSource{PGWhitelist: map[string][]string{"planes": {"id", "monto"}}},
		Actions:        []modules.Action{{ActionKey: "list", Kind: modules.ActionKindList}},
	}

	t.Run("List N=1: SELECT sin JOIN ni alias", func(t *testing.T) {
		drv := &recordedDriver{}
		e := newRecordedExecutor(drv)
		_, _, err := e.List(ctx, def, "", ListQuery{Page: 1, RecordsPerPage: 10})
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		found := false
		for _, q := range drv.queries {
			if strings.Contains(q, "JOIN") || strings.Contains(q, "__") {
				t.Fatalf("N=1 List no debe usar JOIN ni alias: %s", q)
			}
			if strings.Contains(q, `FROM "planes"`) {
				found = true
			}
		}
		if !found {
			t.Fatalf("no se vio el SELECT esperado: %v", drv.queries)
		}
	})

	t.Run("Exec N=1: DELETE sin transacción", func(t *testing.T) {
		drv := &recordedDriver{}
		e := newRecordedExecutor(drv)
		act := modules.Action{Kind: modules.ActionKindDelete, SQLOp: "DELETE"}
		_, _, err := e.Exec(ctx, def, "", act, map[string]any{"id": 9})
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		if drv.began != 0 {
			t.Fatalf("N=1 Exec no debe abrir transacción: began=%d", drv.began)
		}
		if len(drv.execs) != 1 || drv.execs[0] != `DELETE FROM "planes" WHERE "id" = $1` {
			t.Fatalf("DELETE N=1 inesperado: %v", drv.execs)
		}
	})
}
