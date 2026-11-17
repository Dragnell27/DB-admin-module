package modules

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestAllOperators(t *testing.T) {
	ops := AllOperators()
	if len(ops) != 9 {
		t.Fatalf("esperaba 9 operadores, hay %d", len(ops))
	}
	seen := map[Operator]bool{}
	for _, op := range ops {
		if seen[op] {
			t.Errorf("operador duplicado %q", op)
		}
		seen[op] = true
		if _, ok := operatorArity[op]; !ok {
			t.Errorf("operador %q sin aridad", op)
		}
	}
	if len(operatorArity) != len(ops) {
		t.Errorf("operatorArity (%d) y AllOperators (%d) divergen", len(operatorArity), len(ops))
	}
}

func TestValidateOperatorValues(t *testing.T) {
	tests := []struct {
		name   string
		op     Operator
		values []string
		strict bool
		ok     bool
	}{
		{"eq un valor", OpEq, []string{"a"}, true, true},
		{"eq cero", OpEq, nil, true, false},
		{"eq dos", OpEq, []string{"a", "b"}, true, false},
		{"neq un valor", OpNeq, []string{"a"}, true, true},
		{"neq dos", OpNeq, []string{"a", "b"}, false, false},
		{"in varios", OpIn, []string{"a", "b"}, true, true},
		{"in cero", OpIn, nil, true, false},
		{"not_in varios", OpNotIn, []string{"a", "b", "c"}, true, true},
		{"not_in cero", OpNotIn, nil, false, false},
		{"contains un valor", OpContains, []string{"a"}, true, true},
		{"contains dos", OpContains, []string{"a", "b"}, true, false},
		{"starts_with un valor", OpStartsWith, []string{"a"}, true, true},
		{"starts_with cero", OpStartsWith, nil, true, false},
		{"ends_with un valor", OpEndsWith, []string{"a"}, true, true},
		{"ends_with dos", OpEndsWith, []string{"a", "b"}, true, false},
		{"is_null sin valores", OpIsNull, nil, true, true},
		{"is_null con valor", OpIsNull, []string{"a"}, true, false},
		{"is_not_null sin valores", OpIsNotNull, nil, true, true},
		{"is_not_null con valor", OpIsNotNull, []string{"a"}, false, false},
		{"desconocido", Operator("drop"), []string{"a"}, true, false},
		{"vacío sin operador", Operator(""), []string{"a"}, true, false},
		{"strict valor vacío", OpEq, []string{"  "}, true, false},
		{"no strict valor vacío", OpEq, []string{""}, false, true},
		{"strict in con vacío", OpIn, []string{"a", ""}, true, false},
		{"no strict in con vacío", OpIn, []string{""}, false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateOperatorValues(tc.op, tc.values, tc.strict)
			if tc.ok && err != nil {
				t.Fatalf("no esperaba error: %v", err)
			}
			if !tc.ok {
				if err == nil {
					t.Fatalf("esperaba error")
				}
				if !errors.Is(err, ErrInvalidCondition) {
					t.Errorf("el error no envuelve ErrInvalidCondition: %v", err)
				}
			}
		})
	}
}

func TestValidateOperator(t *testing.T) {
	if err := ValidateOperator(""); err != nil {
		t.Errorf("vacío debe ser válido: %v", err)
	}
	for _, op := range AllOperators() {
		if err := ValidateOperator(op); err != nil {
			t.Errorf("%q debe ser válido: %v", op, err)
		}
	}
	err := ValidateOperator("drop")
	if !errors.Is(err, ErrInvalidCondition) {
		t.Fatalf("esperaba ErrInvalidCondition, got %v", err)
	}
	if !strings.Contains(err.Error(), "eq, neq, in, not_in") {
		t.Errorf("el mensaje debe listar los operadores válidos: %v", err)
	}
}

func TestOperatorResolve(t *testing.T) {
	if got, err := Operator("").Resolve(OpIn); err != nil || got != OpIn {
		t.Errorf("Resolve('') = %q, %v", got, err)
	}
	if got, err := OpContains.Resolve(OpIn); err != nil || got != OpContains {
		t.Errorf("Resolve(contains) = %q, %v", got, err)
	}
	if _, err := Operator("x").Resolve(OpIn); !errors.Is(err, ErrInvalidCondition) {
		t.Errorf("esperaba ErrInvalidCondition, got %v", err)
	}
}

func TestFieldSpecEffectiveOperator(t *testing.T) {
	tests := []struct {
		name string
		f    FieldSpec
		want Operator
	}{
		{"vacío ⇒ eq", FieldSpec{}, OpEq},
		{"exact ⇒ eq", FieldSpec{MatchType: "exact"}, OpEq},
		{"desconocido ⇒ eq", FieldSpec{MatchType: "zzz"}, OpEq},
		{"partial ⇒ contains", FieldSpec{MatchType: SearchMatchPartial}, OpContains},
		{"operator gana a matchType", FieldSpec{Operator: OpStartsWith, MatchType: SearchMatchPartial}, OpStartsWith},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.f.EffectiveOperator(); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestJoinConstantOperatorValues(t *testing.T) {
	tests := []struct {
		name   string
		jc     JoinConstant
		wantOp Operator
		want   []string
	}{
		{"sin operador ⇒ eq con Value", JoinConstant{Value: "x"}, OpEq, []string{"x"}},
		{"sin operador y Value vacío", JoinConstant{}, OpEq, []string{""}},
		{"eq", JoinConstant{Operator: OpEq, Value: "x"}, OpEq, []string{"x"}},
		{"neq", JoinConstant{Operator: OpNeq, Value: "x"}, OpNeq, []string{"x"}},
		{"contains", JoinConstant{Operator: OpContains, Value: "x"}, OpContains, []string{"x"}},
		{"in usa Values", JoinConstant{Operator: OpIn, Values: []string{"a", "b"}}, OpIn, []string{"a", "b"}},
		{"not_in usa Values", JoinConstant{Operator: OpNotIn, Values: []string{"a"}}, OpNotIn, []string{"a"}},
		{"is_null sin valores", JoinConstant{Operator: OpIsNull, Value: "x"}, OpIsNull, nil},
		{"is_not_null sin valores", JoinConstant{Operator: OpIsNotNull}, OpIsNotNull, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			op, vals := tc.jc.OperatorValues()
			if op != tc.wantOp {
				t.Errorf("op = %q, want %q", op, tc.wantOp)
			}
			if len(vals) != len(tc.want) {
				t.Fatalf("vals = %v, want %v", vals, tc.want)
			}
			for i := range vals {
				if vals[i] != tc.want[i] {
					t.Errorf("vals = %v, want %v", vals, tc.want)
				}
			}
		})
	}
}

func TestOperatorJSONRoundTrip(t *testing.T) {
	// Definición antigua: sin clave operator, el reserializado no la añade.
	old := `{"field":"estado","values":["A"]}`
	var lc ListCondition
	if err := json.Unmarshal([]byte(old), &lc); err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(lc)
	if strings.Contains(string(out), "operator") {
		t.Errorf("no debe emitir operator: %s", out)
	}
	// Con operador se conserva.
	var lc2 ListCondition
	if err := json.Unmarshal([]byte(`{"field":"estado","operator":"not_in","values":["A","B"]}`), &lc2); err != nil {
		t.Fatal(err)
	}
	if lc2.Operator != OpNotIn {
		t.Fatalf("operator = %q", lc2.Operator)
	}
	out2, _ := json.Marshal(lc2)
	if !strings.Contains(string(out2), `"operator":"not_in"`) {
		t.Errorf("debe conservar operator: %s", out2)
	}
	// JoinConstant: values/operator con omitempty.
	jc := JoinConstant{Table: "t", Column: "c", Value: "x"}
	b, _ := json.Marshal(jc)
	if strings.Contains(string(b), "operator") || strings.Contains(string(b), "values") {
		t.Errorf("JoinConstant legado no debe emitir operator/values: %s", b)
	}
	b2, _ := json.Marshal(JoinConstant{Table: "t", Column: "c", Operator: OpIn, Values: []string{"1", "2"}})
	if !strings.Contains(string(b2), `"operator":"in"`) || !strings.Contains(string(b2), `"values":["1","2"]`) {
		t.Errorf("JoinConstant con operador debe emitir operator/values: %s", b2)
	}
}
