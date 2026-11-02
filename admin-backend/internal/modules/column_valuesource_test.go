package modules

import (
	"reflect"
	"testing"
)

// findCol localiza la ResolvedColumn por Field en el slice (orden estable). Falla
// el test si no existe.
func findCol(t *testing.T, cols []ResolvedColumn, field string) ResolvedColumn {
	t.Helper()
	for _, c := range cols {
		if c.Field == field {
			return c
		}
	}
	t.Fatalf("no se encontró la columna %q en %#v", field, cols)
	return ResolvedColumn{}
}

// TestColumnValueSourceLectura verifica la deserialización y normalización
// TOLERANTE de valueSource en la capa de lectura
// (ColumnsFromDisplayMappingWithPrincipal → normalizeColumnValueSource):
// estática preserva Options, dinámica preserva DynamicSource, Kind inválido o
// estática vacía ⇒ nil, columna sin valueSource ⇒ nil.
func TestColumnValueSourceLectura(t *testing.T) {
	dm := map[string]any{"columns": []any{
		map[string]any{
			"field": "estado",
			"order": 1,
			"valueSource": map[string]any{
				"kind": "static",
				"options": []any{
					map[string]any{"label": "Activo", "value": "1"},
					map[string]any{"label": "Inactivo", "value": "0"},
				},
			},
		},
		map[string]any{
			"field": "plan_id",
			"order": 2,
			"valueSource": map[string]any{
				"kind": "dynamic",
				"dynamicSource": map[string]any{
					"table":       "planes",
					"idColumn":    "id",
					"labelColumn": "nombre",
				},
			},
		},
		map[string]any{
			"field":       "invalida",
			"order":       3,
			"valueSource": map[string]any{"kind": "otro"},
		},
		map[string]any{
			"field":       "estatica_vacia",
			"order":       4,
			"valueSource": map[string]any{"kind": "static"},
		},
		map[string]any{
			"field": "sin_fuente",
			"order": 5,
		},
	}}

	cols := ColumnsFromDisplayMapping(dm)

	t.Run("static preserva Options", func(t *testing.T) {
		got := findCol(t, cols, "estado").ValueSource
		want := &ColumnValueSource{
			Kind: "static",
			Options: []SelectOption{
				{Label: "Activo", Value: "1"},
				{Label: "Inactivo", Value: "0"},
			},
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("static\n  got:  %#v\n  want: %#v", got, want)
		}
	})

	t.Run("dynamic preserva DynamicSource", func(t *testing.T) {
		got := findCol(t, cols, "plan_id").ValueSource
		want := &ColumnValueSource{
			Kind: "dynamic",
			DynamicSource: &DynamicSource{
				Table:       "planes",
				IDColumn:    "id",
				LabelColumn: "nombre",
			},
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("dynamic\n  got:  %#v\n  want: %#v", got, want)
		}
	})

	t.Run("Kind inválido ⇒ nil", func(t *testing.T) {
		if got := findCol(t, cols, "invalida").ValueSource; got != nil {
			t.Fatalf("Kind inválido debía normalizar a nil, got %#v", got)
		}
	})

	t.Run("static con Options vacío ⇒ nil", func(t *testing.T) {
		if got := findCol(t, cols, "estatica_vacia").ValueSource; got != nil {
			t.Fatalf("static vacío debía normalizar a nil, got %#v", got)
		}
	})

	t.Run("columna sin valueSource ⇒ nil", func(t *testing.T) {
		if got := findCol(t, cols, "sin_fuente").ValueSource; got != nil {
			t.Fatalf("columna sin valueSource debía dar nil, got %#v", got)
		}
	})
}

// TestColumnValueSourcePrincipalResuelto verifica que una columna sin
// sourceTable resuelve a la tabla principal igual que hoy (retrocompat del
// emparejamiento) y que su valueSource se normaliza como corresponde.
func TestColumnValueSourcePrincipalResuelto(t *testing.T) {
	dm := map[string]any{"columns": []any{
		map[string]any{
			"field": "estado",
			"valueSource": map[string]any{
				"kind":    "static",
				"options": []any{map[string]any{"label": "A", "value": "1"}},
			},
		},
	}}

	cols := ColumnsFromDisplayMappingWithPrincipal(dm, "pagos")
	c := findCol(t, cols, "estado")
	if c.SourceTable != "pagos" {
		t.Fatalf("sourceTable vacío debía resolver a la principal 'pagos', got %q", c.SourceTable)
	}
	if c.ValueSource == nil || c.ValueSource.Kind != "static" {
		t.Fatalf("se esperaba valueSource static, got %#v", c.ValueSource)
	}
}
