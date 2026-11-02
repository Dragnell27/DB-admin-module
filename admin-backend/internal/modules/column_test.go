package modules

import (
	"reflect"
	"testing"
)

func boolPtr(b bool) *bool { return &b }

// Verifica la extracción y normalización de columnas desde displayMapping,
// incluidos los defaults de compatibilidad (R8) y la regla PK ⇒ no editable.
func TestColumnsFromDisplayMapping(t *testing.T) {
	cases := []struct {
		name string
		dm   map[string]any
		want []ResolvedColumn
	}{
		{
			name: "sin clave columns devuelve nil",
			dm:   map[string]any{"otra": 1},
			want: nil,
		},
		{
			name: "dm nil devuelve nil",
			dm:   nil,
			want: nil,
		},
		{
			name: "columna antigua (sin banderas) aplica defaults",
			dm: map[string]any{"columns": []any{
				map[string]any{"field": "id", "label": "ID", "order": 0},
			}},
			want: []ResolvedColumn{
				{Field: "id", Label: "ID", Order: 0, Visible: true, IsPrimaryKey: false, Editable: true},
			},
		},
		{
			name: "PK fuerza editable=false aunque venga editable=true",
			dm: map[string]any{"columns": []any{
				map[string]any{"field": "id", "isPrimaryKey": true, "editable": true},
			}},
			want: []ResolvedColumn{
				{Field: "id", Visible: true, IsPrimaryKey: true, Editable: false},
			},
		},
		{
			name: "visible=false se respeta",
			dm: map[string]any{"columns": []any{
				map[string]any{"field": "oculta", "visible": false},
			}},
			want: []ResolvedColumn{
				{Field: "oculta", Visible: false, IsPrimaryKey: false, Editable: true},
			},
		},
		{
			name: "orden estable por order y field",
			dm: map[string]any{"columns": []any{
				map[string]any{"field": "b", "order": 1},
				map[string]any{"field": "a", "order": 1},
				map[string]any{"field": "c", "order": 0},
			}},
			want: []ResolvedColumn{
				{Field: "c", Order: 0, Visible: true, Editable: true},
				{Field: "a", Order: 1, Visible: true, Editable: true},
				{Field: "b", Order: 1, Visible: true, Editable: true},
			},
		},
		{
			name: "columna sin field se descarta",
			dm: map[string]any{"columns": []any{
				map[string]any{"field": "", "order": 0},
				map[string]any{"field": "ok", "order": 1},
			}},
			want: []ResolvedColumn{
				{Field: "ok", Order: 1, Visible: true, Editable: true},
			},
		},
		{
			name: "claves de búsqueda viejas (searchable/searchMatch/searchRequired) se ignoran sin error (retrocompat)",
			dm: map[string]any{"columns": []any{
				map[string]any{"field": "nombre", "order": 0, "searchable": true, "searchMatch": "partial", "searchRequired": true},
			}},
			want: []ResolvedColumn{
				{Field: "nombre", Order: 0, Visible: true, Editable: true},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ColumnsFromDisplayMapping(tc.dm)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("ColumnsFromDisplayMapping\n  got:  %#v\n  want: %#v", got, tc.want)
			}
		})
	}
}

// Verifica PrimaryKeyColumns y EditableColumns con clave simple y compuesta.
func TestPrimaryKeyAndEditableColumns(t *testing.T) {
	cols := []ResolvedColumn{
		{Field: "id", IsPrimaryKey: true, Editable: false},
		{Field: "pais", IsPrimaryKey: true, Editable: false},
		{Field: "monto", Editable: true},
		{Field: "estado", Editable: true},
		{Field: "creado", Editable: false},
	}

	gotPK := PrimaryKeyColumns(cols)
	wantPK := []string{"id", "pais"}
	if !reflect.DeepEqual(gotPK, wantPK) {
		t.Fatalf("PrimaryKeyColumns got %v want %v", gotPK, wantPK)
	}

	gotEd := EditableColumns(cols)
	wantEd := []string{"estado", "monto"}
	if !reflect.DeepEqual(gotEd, wantEd) {
		t.Fatalf("EditableColumns got %v want %v", gotEd, wantEd)
	}
}

// Verifica que EditableColumns NUNCA incluye una columna PK aunque estuviera
// marcada como editable por una definición incoherente (la normalización ya la
// fuerza a false, pero EditableColumns refuerza excluyendo PK).
func TestEditableColumnsExcludesPK(t *testing.T) {
	cols := []ResolvedColumn{
		{Field: "id", IsPrimaryKey: true, Editable: true}, // incoherente a propósito
		{Field: "monto", Editable: true},
	}
	got := EditableColumns(cols)
	want := []string{"monto"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("EditableColumns got %v want %v", got, want)
	}
}

// Verifica ActionWriteColumns: sin InputFields ⇒ todas las editables
// (compatibilidad); con subconjunto ⇒ intersección; columnas no editables, PK o
// inexistentes listadas por la acción se descartan (techo de seguridad).
func TestActionWriteColumns(t *testing.T) {
	cols := []ResolvedColumn{
		{Field: "id", IsPrimaryKey: true},
		{Field: "monto", Editable: true},
		{Field: "estado", Editable: true},
		{Field: "creado", Editable: false},
	}

	t.Run("sin InputFields ⇒ todas las editables (compatibilidad)", func(t *testing.T) {
		got := ActionWriteColumns(cols, Action{Kind: ActionKindEdit})
		want := []string{"estado", "monto"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v want %v", got, want)
		}
	})

	t.Run("subconjunto ⇒ intersección con editables", func(t *testing.T) {
		act := Action{Kind: ActionKindEdit, InputFields: []FieldSpec{{Name: "estado"}}}
		got := ActionWriteColumns(cols, act)
		want := []string{"estado"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v want %v", got, want)
		}
	})

	t.Run("columna no editable/PK/inexistente listada ⇒ descartada (techo)", func(t *testing.T) {
		act := Action{Kind: ActionKindEdit, InputFields: []FieldSpec{
			{Name: "id"},       // PK: descartada
			{Name: "creado"},   // no editable: descartada
			{Name: "fantasma"}, // inexistente: descartada
			{Name: "monto"},    // editable: se conserva
		}}
		got := ActionWriteColumns(cols, act)
		want := []string{"monto"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v want %v", got, want)
		}
	})
}

// Verifica los FieldSpec derivados COMBINANDO columnas + acción: sin InputFields
// ⇒ PK + un capturado por editable (compatibilidad); con InputFields ⇒ PK + los
// specs declarados (constant/select reusados) solo para columnas objetivo;
// delete ⇒ solo PK.
func TestEditFieldSpecs(t *testing.T) {
	cols := []ResolvedColumn{
		// PK con Format:"date" a propósito: NO debe heredar el tipo del formato,
		// se emite con el sentinel FieldTypeKeyPassthrough.
		{Field: "id", IsPrimaryKey: true, Format: "date"},
		{Field: "monto", Editable: true, Format: "date"},
		{Field: "estado", Editable: true},
		{Field: "nota", Editable: false},
	}

	t.Run("edit sin InputFields ⇒ PK + capturado por editable (compatibilidad)", func(t *testing.T) {
		got := EditFieldSpecs(cols, Action{Kind: ActionKindEdit}, MultiTableSpec{})
		want := []FieldSpec{
			{Name: "id", Type: FieldTypeKeyPassthrough, Required: true, Source: FieldSourceRow, RowField: "id"},
			{Name: "monto", Type: "date", Required: false, Source: FieldSourceInput},
			{Name: "estado", Type: "string", Required: false, Source: FieldSourceInput},
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("EditFieldSpecs(edit sin config)\n  got:  %#v\n  want: %#v", got, want)
		}
	})

	t.Run("edit con InputFields ⇒ reusa el spec declarado (constant) solo para objetivo", func(t *testing.T) {
		act := Action{Kind: ActionKindEdit, InputFields: []FieldSpec{
			{Name: "estado", Source: FieldSourceConstant, ConstValue: "2"},
		}}
		got := EditFieldSpecs(cols, act, MultiTableSpec{})
		want := []FieldSpec{
			{Name: "id", Type: FieldTypeKeyPassthrough, Required: true, Source: FieldSourceRow, RowField: "id"},
			{Name: "estado", Source: FieldSourceConstant, ConstValue: "2"},
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("EditFieldSpecs(edit con config)\n  got:  %#v\n  want: %#v", got, want)
		}
	})

	t.Run("delete ⇒ solo PK", func(t *testing.T) {
		got := EditFieldSpecs(cols, Action{Kind: ActionKindDelete}, MultiTableSpec{})
		want := []FieldSpec{
			{Name: "id", Type: FieldTypeKeyPassthrough, Required: true, Source: FieldSourceRow, RowField: "id"},
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("EditFieldSpecs(delete)\n  got:  %#v\n  want: %#v", got, want)
		}
	})
}

// Verifica la validación de guardado: una acción edit/delete sin columna PK es
// rechazada con ErrNoPrimaryKey; con PK o módulo solo-lista, pasa.
func TestValidateDefinitionPrimaryKey(t *testing.T) {
	base := func(cols []any, kind string) *Definition {
		return &Definition{
			Slug:           "mod-x",
			Name:           "Mod X",
			SourceType:     SourcePostgres,
			DisplayMapping: map[string]any{"columns": cols},
			Actions:        []Action{{ActionKey: kind, Kind: kind}},
		}
	}

	t.Run("edit sin PK ⇒ ErrNoPrimaryKey", func(t *testing.T) {
		def := base([]any{map[string]any{"field": "id"}}, ActionKindEdit)
		if err := validateDefinition(def); err == nil || err != ErrNoPrimaryKey {
			t.Fatalf("se esperaba ErrNoPrimaryKey, got %v", err)
		}
	})

	t.Run("delete con PK ⇒ OK", func(t *testing.T) {
		def := base([]any{map[string]any{"field": "id", "isPrimaryKey": true}}, ActionKindDelete)
		if err := validateDefinition(def); err != nil {
			t.Fatalf("se esperaba nil, got %v", err)
		}
	})

	t.Run("solo-lista sin PK ⇒ OK", func(t *testing.T) {
		def := base([]any{map[string]any{"field": "id"}}, ActionKindList)
		if err := validateDefinition(def); err != nil {
			t.Fatalf("se esperaba nil, got %v", err)
		}
	})
}
