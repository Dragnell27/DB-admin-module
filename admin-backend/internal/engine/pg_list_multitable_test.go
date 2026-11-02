package engine

import (
	"context"
	"strings"
	"testing"

	"admin-backend/internal/modules"
)

// Test 8: COUNT y SELECT de un módulo multi-tabla con condición cualificada
// comparten el MISMO WHERE y JOIN; el total y la paginación son coherentes.
func TestList_MultiTable_CountSelectConsistent(t *testing.T) {
	ctx := context.Background()
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
			map[string]any{"field": "id", "isPrimaryKey": true, "sourceTable": "pagos", "visible": true, "order": 1},
			map[string]any{"field": "monto", "visible": true, "sourceTable": "pagos", "order": 2},
			map[string]any{"field": "nombre", "visible": true, "sourceTable": "clientes", "order": 3},
			map[string]any{"field": "id", "isPrimaryKey": true, "sourceTable": "clientes", "visible": false, "order": 4},
		}},
		DataSource: modules.DataSource{PGWhitelist: map[string][]string{
			"pagos":    {"id", "cliente_id", "monto"},
			"clientes": {"id", "nombre"},
		}},
		ListConditions: []modules.ListCondition{{Field: "nombre", Values: []string{"Ana"}, SourceTable: "clientes"}},
		Actions:        []modules.Action{{ActionKey: "list", Kind: modules.ActionKindList}},
	}

	_, page, err := e.List(ctx, def, "", ListQuery{Page: 1, RecordsPerPage: 10})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if page.Page != 1 || page.RecordsPerPage != 10 {
		t.Fatalf("paginación inesperada: %+v", page)
	}

	var countSQL, selectSQL string
	for _, q := range drv.queries {
		up := strings.ToUpper(strings.TrimSpace(q))
		switch {
		case strings.HasPrefix(up, "SELECT COUNT"):
			countSQL = q
		case strings.HasPrefix(up, "SELECT"):
			selectSQL = q
		}
	}
	if countSQL == "" || selectSQL == "" {
		t.Fatalf("faltan COUNT o SELECT: %v", drv.queries)
	}
	// El MISMO WHERE cualificado aparece en ambos.
	wantWhere := `WHERE lower(btrim("clientes"."nombre"::text)) IN ($1)`
	if !strings.Contains(countSQL, wantWhere) || !strings.Contains(selectSQL, wantWhere) {
		t.Fatalf("COUNT y SELECT deben compartir el WHERE cualificado\ncount=%s\nselect=%s", countSQL, selectSQL)
	}
	// El JOIN aparece en ambos.
	if !strings.Contains(countSQL, "JOIN") || !strings.Contains(selectSQL, "JOIN") {
		t.Fatalf("el JOIN debe aparecer en COUNT y SELECT\ncount=%s\nselect=%s", countSQL, selectSQL)
	}
	// LIMIT/OFFSET solo en el SELECT de datos, numerados después del WHERE.
	if !strings.Contains(selectSQL, "LIMIT $2 OFFSET $3") {
		t.Fatalf("LIMIT/OFFSET deben numerarse tras el WHERE: %s", selectSQL)
	}
	if strings.Contains(countSQL, "LIMIT") {
		t.Fatalf("el COUNT no debe llevar LIMIT: %s", countSQL)
	}
}
