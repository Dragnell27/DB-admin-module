package modules

import (
	"reflect"
	"testing"
)

func TestColumnsFromDisplayMappingWithPrincipal_DefaultSourceTable(t *testing.T) {
	dm := map[string]any{
		"columns": []any{
			map[string]any{"field": "id", "isPrimaryKey": true, "order": 1},
			map[string]any{"field": "nombre", "order": 2, "sourceTable": "clientes"},
		},
	}
	cols := ColumnsFromDisplayMappingWithPrincipal(dm, "pagos")
	if len(cols) != 2 {
		t.Fatalf("se esperaban 2 columnas: %+v", cols)
	}
	if cols[0].Field != "id" || cols[0].SourceTable != "pagos" {
		t.Fatalf("columna sin sourceTable debe tomar la principal: %+v", cols[0])
	}
	if cols[1].Field != "nombre" || cols[1].SourceTable != "clientes" {
		t.Fatalf("columna con sourceTable declarada debe conservarla: %+v", cols[1])
	}
}

func TestColumnsFromDisplayMapping_N1OrderUnchanged(t *testing.T) {
	// En N=1 (principal == ""), SourceTable queda vacío y el orden es idéntico al
	// histórico (Order, Field).
	dm := map[string]any{
		"columns": []any{
			map[string]any{"field": "b", "order": 1},
			map[string]any{"field": "a", "order": 1},
			map[string]any{"field": "c", "order": 0},
		},
	}
	cols := ColumnsFromDisplayMapping(dm)
	got := []string{cols[0].Field, cols[1].Field, cols[2].Field}
	want := []string{"c", "a", "b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("orden N=1 inesperado: got %v want %v", got, want)
	}
	for _, c := range cols {
		if c.SourceTable != "" {
			t.Fatalf("N=1 no debe resolver sourceTable: %+v", c)
		}
	}
}

func multiCols() []ResolvedColumn {
	return []ResolvedColumn{
		{Field: "id", IsPrimaryKey: true, SourceTable: "pagos"},
		{Field: "monto", Editable: true, SourceTable: "pagos"},
		{Field: "estado", Editable: true, SourceTable: "pagos"},
		{Field: "id", IsPrimaryKey: true, SourceTable: "clientes"},
		{Field: "estado", Editable: true, SourceTable: "clientes"}, // homónima con pagos.estado
	}
}

func TestColumnsByTable(t *testing.T) {
	grouped := ColumnsByTable(multiCols())
	if len(grouped["pagos"]) != 3 {
		t.Fatalf("pagos debe tener 3 columnas: %+v", grouped["pagos"])
	}
	if len(grouped["clientes"]) != 2 {
		t.Fatalf("clientes debe tener 2 columnas: %+v", grouped["clientes"])
	}
}

func TestPrimaryKeyColumnsByTable(t *testing.T) {
	pk := PrimaryKeyColumnsByTable(multiCols())
	if !reflect.DeepEqual(pk["pagos"], []string{"id"}) {
		t.Fatalf("pk pagos: %v", pk["pagos"])
	}
	if !reflect.DeepEqual(pk["clientes"], []string{"id"}) {
		t.Fatalf("pk clientes: %v", pk["clientes"])
	}
}

func TestActionWriteColumnsByTable_Qualified(t *testing.T) {
	cols := multiCols()

	t.Run("campo homónimo cualificado por sourceTable va a su tabla", func(t *testing.T) {
		act := Action{Kind: ActionKindEdit, InputFields: []FieldSpec{
			{Name: "estado", SourceTable: "clientes", Source: FieldSourceInput},
			{Name: "monto", SourceTable: "pagos", Source: FieldSourceInput},
		}}
		got := ActionWriteColumnsByTable(cols, act)
		if !reflect.DeepEqual(got["clientes"], []string{"estado"}) {
			t.Fatalf("clientes: %v", got["clientes"])
		}
		if !reflect.DeepEqual(got["pagos"], []string{"monto"}) {
			t.Fatalf("pagos: %v", got["pagos"])
		}
	})

	t.Run("PK nunca entra como objetivo", func(t *testing.T) {
		act := Action{Kind: ActionKindEdit, InputFields: []FieldSpec{
			{Name: "id", SourceTable: "pagos", Source: FieldSourceInput},
			{Name: "monto", SourceTable: "pagos", Source: FieldSourceInput},
		}}
		got := ActionWriteColumnsByTable(cols, act)
		if !reflect.DeepEqual(got["pagos"], []string{"monto"}) {
			t.Fatalf("PK no debe entrar: %v", got["pagos"])
		}
	})

	t.Run("homónimo sin sourceTable se ignora", func(t *testing.T) {
		act := Action{Kind: ActionKindEdit, InputFields: []FieldSpec{
			{Name: "estado", Source: FieldSourceInput}, // homónimo, sin tabla
		}}
		got := ActionWriteColumnsByTable(cols, act)
		if len(got) != 0 {
			t.Fatalf("homónimo sin tabla no debe emparejar: %v", got)
		}
	})

	t.Run("nombre único sin sourceTable empareja", func(t *testing.T) {
		act := Action{Kind: ActionKindEdit, InputFields: []FieldSpec{
			{Name: "monto", Source: FieldSourceInput}, // único
		}}
		got := ActionWriteColumnsByTable(cols, act)
		if !reflect.DeepEqual(got["pagos"], []string{"monto"}) {
			t.Fatalf("único debe emparejar: %v", got["pagos"])
		}
	})
}
