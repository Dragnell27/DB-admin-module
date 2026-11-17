package modules

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// condDef arma una definición N=1 válida con un catálogo dinámico de columna, un
// campo de acción con catálogo y una estrategia capture-from-catalog, para
// ejercitar cada superficie de operadores.
func condDef() *Definition {
	return &Definition{
		Slug:         "mi-modulo",
		Name:         "Mi Módulo",
		SourceType:   SourcePostgres,
		FetchMapping: map[string]any{"table": "t"},
		DisplayMapping: map[string]any{"columns": []any{
			map[string]any{"field": "id", "visible": true},
			map[string]any{"field": "cat", "visible": true, "valueSource": map[string]any{
				"kind": "dynamic",
				"dynamicSource": map[string]any{
					"table": "c", "idColumn": "id", "labelColumn": "nombre",
				},
			}},
		}},
		DataSource: DataSource{PGWhitelist: map[string][]string{
			"t": {"id", "cat", "estado"},
			"c": {"id", "nombre", "activo"},
		}},
	}
}

func TestValidateConditionOperators(t *testing.T) {
	dyn := func(f ListCondition) *DynamicSource {
		return &DynamicSource{Table: "c", IDColumn: "id", LabelColumn: "nombre", Filters: []ListCondition{f}}
	}
	bad := ListCondition{Field: "activo", Operator: "zzz", Values: []string{"1"}}

	tests := []struct {
		name   string
		mutate func(d *Definition)
		wantOK bool
		inMsg  string
	}{
		{"legado sin operador (Values vacío)", func(d *Definition) {
			d.ListConditions = []ListCondition{{Field: "estado"}}
		}, true, ""},
		{"legado sin operador (Values [\"\"])", func(d *Definition) {
			d.ListConditions = []ListCondition{{Field: "estado", Values: []string{""}}}
		}, true, ""},
		{"operadores válidos", func(d *Definition) {
			d.ListConditions = []ListCondition{
				{Field: "estado", Operator: OpNotIn, Values: []string{"a", "b"}},
				{Field: "estado", Operator: OpIsNull},
				{Field: "estado", Operator: OpContains, Values: []string{"x"}},
			}
		}, true, ""},
		{"condición de listado con operador desconocido", func(d *Definition) {
			d.ListConditions = []ListCondition{{Field: "estado", Operator: "zzz", Values: []string{"a"}}}
		}, false, "condición de listado #1 (estado)"},
		{"in con 0 valores", func(d *Definition) {
			d.ListConditions = []ListCondition{{Field: "estado", Operator: OpIn}}
		}, false, "al menos"},
		{"contains con valor vacío", func(d *Definition) {
			d.ListConditions = []ListCondition{{Field: "estado", Operator: OpContains, Values: []string{" "}}}
		}, false, "vacíos"},
		{"is_null con valor", func(d *Definition) {
			d.ListConditions = []ListCondition{{Field: "estado", Operator: OpIsNull, Values: []string{"a"}}}
		}, false, "no admite valores"},
		{"eq con dos valores", func(d *Definition) {
			d.ListConditions = []ListCondition{{Field: "estado", Operator: OpEq, Values: []string{"a", "b"}}}
		}, false, "exactamente 1"},
		{"filtro del catálogo de una columna", func(d *Definition) {
			d.DisplayMapping["columns"].([]any)[1].(map[string]any)["valueSource"].(map[string]any)["dynamicSource"].(map[string]any)["filters"] = []any{
				map[string]any{"field": "activo", "operator": "zzz", "values": []any{"1"}},
			}
		}, false, "catálogo de la columna"},
		{"filtro del catálogo de un campo de acción", func(d *Definition) {
			d.Actions = []Action{{ActionKey: "custom:x", InputFields: []FieldSpec{{Name: "cat", DynamicSource: dyn(bad)}}}}
		}, false, "catálogo del campo"},
		{"filtro del catálogo de una estrategia", func(d *Definition) {
			d.Actions = []Action{{ActionKey: "custom:x", InputFields: []FieldSpec{{Name: "cat", Strategy: &FillStrategy{Kind: StrategyCaptureFromCatalog, Catalog: dyn(bad)}}}}}
		}, false, "estrategia"},
		{"operador de campo de acción desconocido", func(d *Definition) {
			d.Actions = []Action{{ActionKey: "custom:x", InputFields: []FieldSpec{{Name: "estado", Operator: "zzz"}}}}
		}, false, "acción \"custom:x\""},
		{"operador de campo de acción válido", func(d *Definition) {
			d.Actions = []Action{{ActionKey: "custom:x", InputFields: []FieldSpec{{Name: "estado", Operator: OpStartsWith}}}}
		}, true, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := condDef()
			tc.mutate(d)
			err := validateConditionOperators(d)
			if tc.wantOK {
				if err != nil {
					t.Fatalf("no esperaba error: %v", err)
				}
				return
			}
			if !errors.Is(err, ErrInvalidCondition) {
				t.Fatalf("esperaba ErrInvalidCondition, got %v", err)
			}
			if !strings.Contains(err.Error(), tc.inMsg) {
				t.Errorf("el mensaje debe contener %q: %v", tc.inMsg, err)
			}
		})
	}
}

func relDef(constants ...map[string]any) *Definition {
	d := condDef()
	d.FetchMapping = map[string]any{
		"table":         "t",
		"foreignTables": []any{map[string]any{"table": "c"}},
		"relationships": []any{map[string]any{
			"childTable": "t", "parentTable": "c", "joinType": "left",
			"on":          []any{map[string]any{"child": "cat", "parent": "id"}},
			"onConstants": toAnySlice(constants),
		}},
	}
	return d
}

func toAnySlice(in []map[string]any) []any {
	out := make([]any, len(in))
	for i, m := range in {
		out[i] = m
	}
	return out
}

func TestValidateConditionOperatorsOnConstants(t *testing.T) {
	jc := func(extra map[string]any) map[string]any {
		m := map[string]any{"table": "c", "column": "activo", "value": ""}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	tests := []struct {
		name   string
		jc     map[string]any
		wantOK bool
		inMsg  string
	}{
		{"legado sin operador con value vacío", jc(nil), true, ""},
		{"eq con value", jc(map[string]any{"operator": "eq", "value": "1"}), true, ""},
		{"in con values", jc(map[string]any{"operator": "in", "values": []any{"1", "2"}}), true, ""},
		{"is_null sin valor", jc(map[string]any{"operator": "is_null"}), true, ""},
		{"operador desconocido", jc(map[string]any{"operator": "zzz", "value": "1"}), false, "desconocido"},
		{"eq con value vacío", jc(map[string]any{"operator": "eq"}), false, "vacíos"},
		{"in sin values", jc(map[string]any{"operator": "in"}), false, "al menos"},
		{"in con Value", jc(map[string]any{"operator": "in", "value": "1", "values": []any{"2"}}), false, "solo la lista de valores"},
		{"eq con Values", jc(map[string]any{"operator": "eq", "value": "1", "values": []any{"2"}}), false, "solo el valor"},
		{"is_null con Value", jc(map[string]any{"operator": "is_null", "value": "1"}), false, "no admite valor"},
		{"is_not_null con Values", jc(map[string]any{"operator": "is_not_null", "values": []any{"1"}}), false, "no admite valor"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateConditionOperators(relDef(tc.jc))
			if tc.wantOK {
				if err != nil {
					t.Fatalf("no esperaba error: %v", err)
				}
				return
			}
			if !errors.Is(err, ErrInvalidCondition) {
				t.Fatalf("esperaba ErrInvalidCondition, got %v", err)
			}
			if !strings.Contains(err.Error(), tc.inMsg) {
				t.Errorf("el mensaje debe contener %q: %v", tc.inMsg, err)
			}
		})
	}
}

// validateDefinition (N=1) rechaza una condición inválida con ErrInvalidCondition
// y acepta la definición legada sin operadores.
func TestValidateDefinitionConditionOperators(t *testing.T) {
	d := condDef()
	if err := validateDefinition(d); err != nil {
		t.Fatalf("la definición legada debe ser válida: %v", err)
	}
	d.ListConditions = []ListCondition{{Field: "estado", Operator: "zzz", Values: []string{"a"}}}
	if err := validateDefinition(d); !errors.Is(err, ErrInvalidCondition) {
		t.Fatalf("esperaba ErrInvalidCondition, got %v", err)
	}
}

// Round-trip JSON de los filtros de un catálogo: sin operador no se añade la
// clave; con operador se conserva.
func TestDynamicSourceFiltersOperatorRoundTrip(t *testing.T) {
	var legacy DynamicSource
	if err := json.Unmarshal([]byte(`{"table":"c","idColumn":"id","labelColumn":"n","filters":[{"field":"a","values":["1"]}]}`), &legacy); err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(legacy)
	if strings.Contains(string(out), "operator") {
		t.Errorf("no debe añadir operator: %s", out)
	}
	var withOp DynamicSource
	if err := json.Unmarshal([]byte(`{"table":"c","idColumn":"id","labelColumn":"n","filters":[{"field":"a","operator":"is_not_null","values":[]}]}`), &withOp); err != nil {
		t.Fatal(err)
	}
	if withOp.Filters[0].Operator != OpIsNotNull {
		t.Fatalf("operator = %q", withOp.Filters[0].Operator)
	}
	out, _ = json.Marshal(withOp)
	if !strings.Contains(string(out), `"operator":"is_not_null"`) {
		t.Errorf("debe conservar operator: %s", out)
	}
}
