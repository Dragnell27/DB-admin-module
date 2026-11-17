package engine

import (
	"errors"
	"reflect"
	"testing"

	"admin-backend/internal/modules"
)

// SQL y args EXACTOS de cada operador, en modo texto y raw.
func TestBuildPredicateSQL(t *testing.T) {
	cases := []struct {
		name     string
		op       modules.Operator
		values   []string
		mode     colMode
		wantSQL  string
		wantArgs []any
	}{
		{"texto eq", modules.OpEq, []string{" ABC "}, modeText, `lower(btrim("c"::text)) = $3`, []any{"abc"}},
		{"texto neq", modules.OpNeq, []string{"ABC"}, modeText, `lower(btrim("c"::text)) <> $3`, []any{"abc"}},
		{"texto in", modules.OpIn, []string{"A", " b "}, modeText, `lower(btrim("c"::text)) IN ($3, $4)`, []any{"a", "b"}},
		{"texto not_in", modules.OpNotIn, []string{"A", "B"}, modeText, `lower(btrim("c"::text)) NOT IN ($3, $4)`, []any{"a", "b"}},
		{"texto contains", modules.OpContains, []string{" Ab "}, modeText, `btrim("c"::text) ILIKE $3 ESCAPE '\'`, []any{"%Ab%"}},
		{"texto starts_with", modules.OpStartsWith, []string{"Ab"}, modeText, `btrim("c"::text) ILIKE $3 ESCAPE '\'`, []any{"Ab%"}},
		{"texto ends_with", modules.OpEndsWith, []string{"Ab"}, modeText, `btrim("c"::text) ILIKE $3 ESCAPE '\'`, []any{"%Ab"}},
		{"texto is_null", modules.OpIsNull, nil, modeText, `"c" IS NULL`, nil},
		{"texto is_not_null", modules.OpIsNotNull, nil, modeText, `"c" IS NOT NULL`, nil},

		{"raw eq", modules.OpEq, []string{" 1 "}, modeRaw, `"c" = $3`, []any{" 1 "}},
		{"raw neq", modules.OpNeq, []string{"1"}, modeRaw, `"c" <> $3`, []any{"1"}},
		{"raw in", modules.OpIn, []string{"10001", "10002"}, modeRaw, `"c" IN ($3, $4)`, []any{"10001", "10002"}},
		{"raw not_in", modules.OpNotIn, []string{"1", "2"}, modeRaw, `"c" NOT IN ($3, $4)`, []any{"1", "2"}},
		{"raw contains", modules.OpContains, []string{"Ab"}, modeRaw, `"c"::text ILIKE $3 ESCAPE '\'`, []any{"%Ab%"}},
		{"raw starts_with", modules.OpStartsWith, []string{"Ab"}, modeRaw, `"c"::text ILIKE $3 ESCAPE '\'`, []any{"Ab%"}},
		{"raw ends_with", modules.OpEndsWith, []string{"Ab"}, modeRaw, `"c"::text ILIKE $3 ESCAPE '\'`, []any{"%Ab"}},
		{"raw is_null", modules.OpIsNull, nil, modeRaw, `"c" IS NULL`, nil},
		{"raw is_not_null", modules.OpIsNotNull, nil, modeRaw, `"c" IS NOT NULL`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sql, args, next, err := buildPredicate(tc.op, `"c"`, tc.values, 3, tc.mode, true)
			if err != nil {
				t.Fatalf("error inesperado: %v", err)
			}
			if sql != tc.wantSQL {
				t.Fatalf("sql\n  got:  %s\n  want: %s", sql, tc.wantSQL)
			}
			if len(args) != len(tc.wantArgs) || (len(args) > 0 && !reflect.DeepEqual(args, tc.wantArgs)) {
				t.Fatalf("args\n  got:  %v\n  want: %v", args, tc.wantArgs)
			}
			if next != 3+len(tc.wantArgs) {
				t.Fatalf("next = %d, want %d", next, 3+len(tc.wantArgs))
			}
		})
	}
}

func TestBuildPredicateEscapesLikeWildcards(t *testing.T) {
	if got := esc(`50%_a\b`); got != `50\%\_a\\b` {
		t.Fatalf("esc = %q", got)
	}
	_, args, _, err := buildPredicate(modules.OpContains, `"c"`, []string{`50%_a\b`}, 1, modeText, true)
	if err != nil {
		t.Fatal(err)
	}
	if args[0] != `%50\%\_a\\b%` {
		t.Fatalf("arg = %q", args[0])
	}
}

func TestBuildPredicateNumbering(t *testing.T) {
	// Lista mixta: in de 2 + is_null + contains ⇒ $1,$2 / sin $ / $3.
	var next = 1
	var all []string
	for _, p := range []struct {
		op modules.Operator
		v  []string
	}{
		{modules.OpIn, []string{"a", "b"}},
		{modules.OpIsNull, nil},
		{modules.OpContains, []string{"x"}},
	} {
		sql, _, n, err := buildPredicate(p.op, `"c"`, p.v, next, modeText, true)
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, sql)
		next = n
	}
	want := []string{
		`lower(btrim("c"::text)) IN ($1, $2)`,
		`"c" IS NULL`,
		`btrim("c"::text) ILIKE $3 ESCAPE '\'`,
	}
	if !reflect.DeepEqual(all, want) {
		t.Fatalf("got %v want %v", all, want)
	}
	if next != 4 {
		t.Fatalf("next = %d, want 4", next)
	}
}

func TestBuildPredicateValidation(t *testing.T) {
	bad := []struct {
		name   string
		op     modules.Operator
		values []string
	}{
		{"desconocido", modules.Operator("1; DROP TABLE x"), []string{"a"}},
		{"vacío", modules.Operator(""), []string{"a"}},
		{"eq sin valores", modules.OpEq, nil},
		{"eq con dos", modules.OpEq, []string{"a", "b"}},
		{"in sin valores", modules.OpIn, nil},
		{"is_null con valor", modules.OpIsNull, []string{"a"}},
		{"contains vacío (strict)", modules.OpContains, []string{" "}},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			sql, args, _, err := buildPredicate(tc.op, `"c"`, tc.values, 1, modeText, true)
			if !errors.Is(err, ErrValidation) {
				t.Fatalf("esperaba ErrValidation, got %v", err)
			}
			if sql != "" || args != nil {
				t.Fatalf("en error no debe producirse SQL: %q %v", sql, args)
			}
		})
	}

	t.Run("strict=false tolera valores vacíos", func(t *testing.T) {
		sql, args, _, err := buildPredicate(modules.OpIn, `"c"`, []string{""}, 1, modeText, false)
		if err != nil {
			t.Fatal(err)
		}
		if sql != `lower(btrim("c"::text)) IN ($1)` || !reflect.DeepEqual(args, []any{""}) {
			t.Fatalf("got %q %v", sql, args)
		}
	})
}

// Guarda: todo operador del enum debe tener generador en el motor.
func TestEveryOperatorHasGenerator(t *testing.T) {
	for _, op := range modules.AllOperators() {
		if predicateGenerators[op] == nil {
			t.Errorf("el operador %q no tiene generador", op)
		}
	}
	if len(predicateGenerators) != len(modules.AllOperators()) {
		t.Errorf("predicateGenerators (%d) y AllOperators (%d) divergen", len(predicateGenerators), len(modules.AllOperators()))
	}
}
