package modules

import "testing"

func efCols() []ResolvedColumn {
	return []ResolvedColumn{
		{Field: "id", IsPrimaryKey: true, SourceTable: "pagos"},
		{Field: "monto", Editable: true, SourceTable: "pagos"},
		{Field: "id", IsPrimaryKey: true, SourceTable: "clientes"},
		{Field: "nombre", Editable: true, SourceTable: "clientes"},
	}
}

func efSpec() MultiTableSpec {
	return MultiTableFromFetchMapping(map[string]any{
		"table":         "pagos",
		"foreignTables": []any{map[string]any{"table": "clientes"}},
		"relationships": []any{
			map[string]any{"childTable": "pagos", "parentTable": "clientes", "on": []any{map[string]any{"child": "cliente_id", "parent": "id"}}, "joinType": "inner"},
		},
	})
}

func hasFieldName(specs []FieldSpec, name string) bool {
	for _, s := range specs {
		if s.Name == name {
			return true
		}
	}
	return false
}

func TestEditFieldSpecs_MultiTableAliasedPK(t *testing.T) {
	t.Run("edit: PK aliased por cada tabla afectada", func(t *testing.T) {
		act := Action{Kind: ActionKindEdit, InputFields: []FieldSpec{
			{Name: "monto", SourceTable: "pagos", Source: FieldSourceInput},
			{Name: "nombre", SourceTable: "clientes", Source: FieldSourceInput},
		}}
		specs := EditFieldSpecs(efCols(), act, efSpec())
		if !hasFieldName(specs, "pagos__id") || !hasFieldName(specs, "clientes__id") {
			t.Fatalf("se esperaban PK aliased pagos__id y clientes__id: %+v", specs)
		}
		if !hasFieldName(specs, "monto") || !hasFieldName(specs, "nombre") {
			t.Fatalf("faltan campos objetivo: %+v", specs)
		}
	})

	t.Run("delete: union de PK de DeleteOrder aliased", func(t *testing.T) {
		act := Action{Kind: ActionKindDelete, DeleteTopTable: "clientes"}
		specs := EditFieldSpecs(efCols(), act, efSpec())
		if !hasFieldName(specs, "pagos__id") || !hasFieldName(specs, "clientes__id") {
			t.Fatalf("delete debe exigir PK aliased de ambas tablas: %+v", specs)
		}
		// delete: solo PK, sin campos objetivo.
		for _, s := range specs {
			if s.Name == "monto" || s.Name == "nombre" {
				t.Fatalf("delete no debe emitir campos objetivo: %+v", specs)
			}
		}
	})
}

func TestEditFieldSpecs_N1PeeledPK(t *testing.T) {
	cols := []ResolvedColumn{
		{Field: "id", IsPrimaryKey: true},
		{Field: "monto", Editable: true},
	}
	specs := EditFieldSpecs(cols, Action{Kind: ActionKindDelete}, MultiTableSpec{})
	if len(specs) != 1 || specs[0].Name != "id" {
		t.Fatalf("N=1 debe emitir PK pelada 'id': %+v", specs)
	}
}

// Fallback union-máxima: ante un árbol inválido en ejecución (delete), se
// emiten las PK de TODAS las tablas declaradas (nunca menos).
func TestEditFieldSpecs_BrokenTreeUnionMax(t *testing.T) {
	// Árbol con ciclo FK: BuildRelationTree fallará.
	spec := MultiTableFromFetchMapping(map[string]any{
		"table":         "pagos",
		"foreignTables": []any{map[string]any{"table": "clientes"}},
		"relationships": []any{
			map[string]any{"childTable": "pagos", "parentTable": "clientes", "joinType": "inner"},
			map[string]any{"childTable": "clientes", "parentTable": "pagos", "joinType": "inner"},
		},
	})
	act := Action{Kind: ActionKindDelete, DeleteTopTable: "clientes"}
	specs := EditFieldSpecs(efCols(), act, spec)
	if !hasFieldName(specs, "pagos__id") || !hasFieldName(specs, "clientes__id") {
		t.Fatalf("ante árbol roto debe exigir la union-máxima de PK: %+v", specs)
	}
}
