package modules

import "testing"

// Verifica la decodificación tolerante de fetchMapping a MultiTableSpec.
func TestMultiTableFromFetchMapping(t *testing.T) {
	t.Run("N=1: solo table ⇒ slices vacíos, IsMultiTable=false", func(t *testing.T) {
		spec := MultiTableFromFetchMapping(map[string]any{"table": "planes"})
		if spec.PrincipalTable != "planes" {
			t.Fatalf("principal: got %q", spec.PrincipalTable)
		}
		if len(spec.ForeignTables) != 0 || len(spec.Relationships) != 0 {
			t.Fatalf("se esperaban slices vacíos: %+v", spec)
		}
		if spec.IsMultiTable() {
			t.Fatalf("N=1 no debe ser multi-tabla")
		}
	})

	t.Run("multi-tabla: decodifica foráneas y relaciones", func(t *testing.T) {
		fm := map[string]any{
			"table": "pagos",
			"foreignTables": []any{
				map[string]any{"table": "clientes"},
			},
			"relationships": []any{
				map[string]any{
					"childTable":  "pagos",
					"parentTable": "clientes",
					"on":          []any{map[string]any{"child": "cliente_id", "parent": "id"}},
					"joinType":    "left",
				},
			},
		}
		spec := MultiTableFromFetchMapping(fm)
		if spec.PrincipalTable != "pagos" {
			t.Fatalf("principal: got %q", spec.PrincipalTable)
		}
		if len(spec.ForeignTables) != 1 || spec.ForeignTables[0].Table != "clientes" {
			t.Fatalf("foráneas inesperadas: %+v", spec.ForeignTables)
		}
		if len(spec.Relationships) != 1 {
			t.Fatalf("relaciones inesperadas: %+v", spec.Relationships)
		}
		r := spec.Relationships[0]
		if r.ChildTable != "pagos" || r.ParentTable != "clientes" || r.JoinType != "left" {
			t.Fatalf("relación mal decodificada: %+v", r)
		}
		if len(r.On) != 1 || r.On[0].Child != "cliente_id" || r.On[0].Parent != "id" {
			t.Fatalf("ON mal decodificado: %+v", r.On)
		}
		if !spec.IsMultiTable() {
			t.Fatalf("debe ser multi-tabla")
		}
	})

	t.Run("JSONB malformado ⇒ spec N=1 sin panic", func(t *testing.T) {
		fm := map[string]any{"table": "planes", "relationships": "no-es-array"}
		spec := MultiTableFromFetchMapping(fm)
		if spec.PrincipalTable != "planes" {
			t.Fatalf("principal: got %q", spec.PrincipalTable)
		}
		if len(spec.Relationships) != 0 {
			t.Fatalf("relaciones malformadas deben quedar vacías: %+v", spec.Relationships)
		}
	})

	t.Run("fm nil ⇒ spec vacía", func(t *testing.T) {
		spec := MultiTableFromFetchMapping(nil)
		if spec.PrincipalTable != "" || spec.IsMultiTable() {
			t.Fatalf("spec no vacía: %+v", spec)
		}
	})
}

func TestNormalizeJoinType(t *testing.T) {
	cases := map[string]string{
		"left":    "left",
		"inner":   "inner",
		"":        "inner",
		"RIGHT":   "inner",
		"unknown": "inner",
		"LEFT":    "inner", // sensible a may/min: solo el literal exacto "left"
	}
	for in, want := range cases {
		if got := normalizeJoinType(in); got != want {
			t.Fatalf("normalizeJoinType(%q)=%q, want %q", in, got, want)
		}
	}
}

func TestMultiTableTables(t *testing.T) {
	spec := MultiTableSpec{
		PrincipalTable: "pagos",
		ForeignTables:  []ForeignTable{{Table: "clientes"}, {Table: "planes"}, {Table: "clientes"}},
	}
	got := spec.Tables()
	want := []string{"pagos", "clientes", "planes"}
	if len(got) != len(want) {
		t.Fatalf("tables: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tables[%d]: got %q want %q", i, got[i], want[i])
		}
	}
}
