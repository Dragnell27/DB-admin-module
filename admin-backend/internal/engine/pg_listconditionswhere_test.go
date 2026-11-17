package engine

import (
	"errors"
	"reflect"
	"strconv"
	"testing"

	"admin-backend/internal/modules"
)

// whitelist común de las pruebas del constructor de WHERE de condiciones.
var condWhitelist = map[string][]string{
	"medios_pago": {"id", "nombre", "list_numero", "estado", "eliminado"},
}

// Verifica que buildListConditionsWhere (operador ausente ⇒ in) compone
// SQL parametrizado: IN por columna (OR), AND entre columnas,
// normalización lower+trim de los valores ligados, numeración de marcadores
// desde startParam y validación de lista blanca SIN ejecutar SQL.
func TestBuildListConditionsWhere(t *testing.T) {
	cases := []struct {
		name       string
		conditions []modules.ListCondition
		startParam int
		wantClause string
		wantArgs   []any
		wantErr    bool
	}{
		{
			name:       "sin condiciones ⇒ clause vacío y args nil (SELECT/COUNT sin cambios)",
			conditions: nil,
			startParam: 1,
			wantClause: "",
			wantArgs:   nil,
		},
		{
			name: "valor único ⇒ WHERE con IN de un marcador (escenario del bug)",
			conditions: []modules.ListCondition{
				{Field: "list_numero", Values: []string{"10001"}},
			},
			startParam: 1,
			wantClause: `lower(btrim("list_numero"::text)) IN ($1)`,
			wantArgs:   []any{"10001"},
		},
		{
			name: "varios valores en una columna ⇒ IN/OR con args en orden",
			conditions: []modules.ListCondition{
				{Field: "estado", Values: []string{"1", "2"}},
			},
			startParam: 1,
			wantClause: `lower(btrim("estado"::text)) IN ($1, $2)`,
			wantArgs:   []any{"1", "2"},
		},
		{
			name: "varias columnas ⇒ fragmentos unidos por AND con marcadores incrementales",
			conditions: []modules.ListCondition{
				{Field: "list_numero", Values: []string{"10001"}},
				{Field: "estado", Values: []string{"1", "2"}},
			},
			startParam: 1,
			wantClause: `lower(btrim("list_numero"::text)) IN ($1) AND lower(btrim("estado"::text)) IN ($2, $3)`,
			wantArgs:   []any{"10001", "1", "2"},
		},
		{
			name: "condición con Values vacío o Field vacío se salta",
			conditions: []modules.ListCondition{
				{Field: "list_numero", Values: nil},
				{Field: "", Values: []string{"x"}},
				{Field: "estado", Values: []string{"1"}},
			},
			startParam: 1,
			wantClause: `lower(btrim("estado"::text)) IN ($1)`,
			wantArgs:   []any{"1"},
		},
		{
			name: "normalización lower+trim del valor ligado",
			conditions: []modules.ListCondition{
				{Field: "estado", Values: []string{"  ACTIVO "}},
			},
			startParam: 1,
			wantClause: `lower(btrim("estado"::text)) IN ($1)`,
			wantArgs:   []any{"activo"},
		},
		{
			name: "startParam desplaza la numeración (camino del SELECT de datos)",
			conditions: []modules.ListCondition{
				{Field: "estado", Values: []string{"1", "2"}},
			},
			startParam: 3,
			wantClause: `lower(btrim("estado"::text)) IN ($3, $4)`,
			wantArgs:   []any{"1", "2"},
		},
		{
			name: "columna fuera de la lista blanca ⇒ ErrWhitelist sin SQL",
			conditions: []modules.ListCondition{
				{Field: "secreto", Values: []string{"x"}},
			},
			startParam: 1,
			wantErr:    true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clause, args, err := buildListConditionsWhere(condWhitelist, "medios_pago", tc.conditions, tc.startParam)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("se esperaba error, got nil (clause=%q)", clause)
				}
				if !errors.Is(err, ErrWhitelist) {
					t.Fatalf("el error debe envolver ErrWhitelist, got %v", err)
				}
				if clause != "" || args != nil {
					t.Fatalf("en error no debe producirse SQL: clause=%q args=%v", clause, args)
				}
				return
			}
			if err != nil {
				t.Fatalf("error inesperado: %v", err)
			}
			if clause != tc.wantClause {
				t.Fatalf("clause\n  got:  %s\n  want: %s", clause, tc.wantClause)
			}
			if tc.wantArgs == nil {
				if len(args) != 0 {
					t.Fatalf("args: se esperaba vacío, got %v", args)
				}
			} else if !reflect.DeepEqual(args, tc.wantArgs) {
				t.Fatalf("args\n  got:  %v\n  want: %v", args, tc.wantArgs)
			}
		})
	}
}

// Verifica que el WHERE construido se aplica al SELECT de datos y al COUNT(*)
// con los MISMOS args: el COUNT solo lleva los args del WHERE, y el SELECT de
// datos los lleva PRIMERO y luego LIMIT/OFFSET renumerados después.
func TestBuildListConditionsWhereSharedBetweenDataAndCount(t *testing.T) {
	conditions := []modules.ListCondition{
		{Field: "list_numero", Values: []string{"10001"}},
	}
	clause, condArgs, err := buildListConditionsWhere(condWhitelist, "medios_pago", conditions, 1)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	whereSQL := " WHERE " + clause

	// COUNT: solo args del WHERE.
	countSQL := "SELECT count(*) FROM \"medios_pago\"" + whereSQL
	wantCount := `SELECT count(*) FROM "medios_pago" WHERE lower(btrim("list_numero"::text)) IN ($1)`
	if countSQL != wantCount {
		t.Fatalf("countSQL\n  got:  %s\n  want: %s", countSQL, wantCount)
	}

	// DATA: WHERE primero, LIMIT/OFFSET renumerados después.
	dataSQL := "SELECT \"list_numero\" FROM \"medios_pago\"" + whereSQL +
		" LIMIT $" + strconv.Itoa(len(condArgs)+1) + " OFFSET $" + strconv.Itoa(len(condArgs)+2)
	wantData := `SELECT "list_numero" FROM "medios_pago" WHERE lower(btrim("list_numero"::text)) IN ($1) LIMIT $2 OFFSET $3`
	if dataSQL != wantData {
		t.Fatalf("dataSQL\n  got:  %s\n  want: %s", dataSQL, wantData)
	}

	if len(condArgs) != 1 || condArgs[0] != "10001" {
		t.Fatalf("condArgs inesperados: %v", condArgs)
	}
}

// Operadores en condiciones de listado (N=1 y multi-tabla): SQL y args exactos,
// retrocompat sin operador y rechazos sin producir SQL.
func TestBuildListConditionsWhereOperators(t *testing.T) {
	cases := []struct {
		name     string
		cond     modules.ListCondition
		wantN1   string
		wantMult string
		wantArgs []any
	}{
		{"legado sin operador ⇒ in", modules.ListCondition{Field: "estado", Values: []string{" A ", "B"}},
			`lower(btrim("estado"::text)) IN ($1, $2)`, `lower(btrim("medios_pago"."estado"::text)) IN ($1, $2)`, []any{"a", "b"}},
		{"legado Values vacío [\"\"] ⇒ IN ('')", modules.ListCondition{Field: "estado", Values: []string{""}},
			`lower(btrim("estado"::text)) IN ($1)`, `lower(btrim("medios_pago"."estado"::text)) IN ($1)`, []any{""}},
		{"eq", modules.ListCondition{Field: "estado", Operator: modules.OpEq, Values: []string{"A"}},
			`lower(btrim("estado"::text)) = $1`, `lower(btrim("medios_pago"."estado"::text)) = $1`, []any{"a"}},
		{"neq", modules.ListCondition{Field: "estado", Operator: modules.OpNeq, Values: []string{"A"}},
			`lower(btrim("estado"::text)) <> $1`, `lower(btrim("medios_pago"."estado"::text)) <> $1`, []any{"a"}},
		{"in", modules.ListCondition{Field: "estado", Operator: modules.OpIn, Values: []string{"A", "B"}},
			`lower(btrim("estado"::text)) IN ($1, $2)`, `lower(btrim("medios_pago"."estado"::text)) IN ($1, $2)`, []any{"a", "b"}},
		{"not_in", modules.ListCondition{Field: "estado", Operator: modules.OpNotIn, Values: []string{"A", "B"}},
			`lower(btrim("estado"::text)) NOT IN ($1, $2)`, `lower(btrim("medios_pago"."estado"::text)) NOT IN ($1, $2)`, []any{"a", "b"}},
		{"contains", modules.ListCondition{Field: "estado", Operator: modules.OpContains, Values: []string{"a_b"}},
			`btrim("estado"::text) ILIKE $1 ESCAPE '\'`, `btrim("medios_pago"."estado"::text) ILIKE $1 ESCAPE '\'`, []any{`%a\_b%`}},
		{"starts_with", modules.ListCondition{Field: "estado", Operator: modules.OpStartsWith, Values: []string{"a"}},
			`btrim("estado"::text) ILIKE $1 ESCAPE '\'`, `btrim("medios_pago"."estado"::text) ILIKE $1 ESCAPE '\'`, []any{`a%`}},
		{"ends_with", modules.ListCondition{Field: "estado", Operator: modules.OpEndsWith, Values: []string{"a"}},
			`btrim("estado"::text) ILIKE $1 ESCAPE '\'`, `btrim("medios_pago"."estado"::text) ILIKE $1 ESCAPE '\'`, []any{`%a`}},
		{"is_null sin valores se emite", modules.ListCondition{Field: "estado", Operator: modules.OpIsNull},
			`"estado" IS NULL`, `"medios_pago"."estado" IS NULL`, nil},
		{"is_not_null sin valores se emite", modules.ListCondition{Field: "estado", Operator: modules.OpIsNotNull},
			`"estado" IS NOT NULL`, `"medios_pago"."estado" IS NOT NULL`, nil},
	}
	spec := multiSpec("medios_pago", "clientes")
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sql, args, err := buildListConditionsWhere(condWhitelist, "medios_pago", []modules.ListCondition{tc.cond}, 1)
			if err != nil {
				t.Fatalf("N=1: %v", err)
			}
			if sql != tc.wantN1 || !reflect.DeepEqual(args, tc.wantArgs) && (len(args) != 0 || len(tc.wantArgs) != 0) {
				t.Fatalf("N=1\n  got:  %s %v\n  want: %s %v", sql, args, tc.wantN1, tc.wantArgs)
			}
			wl := map[string][]string{"medios_pago": condWhitelist["medios_pago"], "clientes": {"id"}}
			sql, args, err = buildListConditionsWhereMulti(wl, nil, spec, []modules.ListCondition{tc.cond}, 1)
			if err != nil {
				t.Fatalf("multi: %v", err)
			}
			if sql != tc.wantMult || !reflect.DeepEqual(args, tc.wantArgs) && (len(args) != 0 || len(tc.wantArgs) != 0) {
				t.Fatalf("multi\n  got:  %s %v\n  want: %s %v", sql, args, tc.wantMult, tc.wantArgs)
			}
		})
	}
}

func TestBuildListConditionsWhereOperatorErrors(t *testing.T) {
	t.Run("desconocido ⇒ ErrValidation", func(t *testing.T) {
		sql, args, err := buildListConditionsWhere(condWhitelist, "medios_pago",
			[]modules.ListCondition{{Field: "estado", Operator: "x; DROP", Values: []string{"a"}}}, 1)
		if !errors.Is(err, ErrValidation) || sql != "" || args != nil {
			t.Fatalf("got %q %v %v", sql, args, err)
		}
	})
	t.Run("operador explícito con aridad errónea ⇒ ErrValidation", func(t *testing.T) {
		for _, c := range []modules.ListCondition{
			{Field: "estado", Operator: modules.OpIn},
			{Field: "estado", Operator: modules.OpEq, Values: []string{"a", "b"}},
			{Field: "estado", Operator: modules.OpIsNull, Values: []string{"a"}},
			{Field: "estado", Operator: modules.OpContains, Values: []string{""}},
		} {
			if _, _, err := buildListConditionsWhere(condWhitelist, "medios_pago", []modules.ListCondition{c}, 1); !errors.Is(err, ErrValidation) {
				t.Errorf("%+v: esperaba ErrValidation, got %v", c, err)
			}
		}
	})
	t.Run("fuera de whitelist ⇒ ErrWhitelist", func(t *testing.T) {
		_, _, err := buildListConditionsWhere(condWhitelist, "medios_pago",
			[]modules.ListCondition{{Field: "fantasma", Operator: modules.OpIsNull}}, 1)
		if !errors.Is(err, ErrWhitelist) {
			t.Fatalf("esperaba ErrWhitelist, got %v", err)
		}
	})
	t.Run("legado sin valores se omite", func(t *testing.T) {
		sql, args, err := buildListConditionsWhere(condWhitelist, "medios_pago",
			[]modules.ListCondition{{Field: "estado"}, {Field: "", Values: []string{"a"}}}, 1)
		if err != nil || sql != "" || args != nil {
			t.Fatalf("got %q %v %v", sql, args, err)
		}
	})
	t.Run("numeración continua con mezcla de operadores", func(t *testing.T) {
		sql, args, err := buildListConditionsWhere(condWhitelist, "medios_pago", []modules.ListCondition{
			{Field: "estado", Operator: modules.OpIn, Values: []string{"a", "b"}},
			{Field: "eliminado", Operator: modules.OpIsNull},
			{Field: "nombre", Operator: modules.OpContains, Values: []string{"x"}},
		}, 5)
		want := `lower(btrim("estado"::text)) IN ($5, $6) AND "eliminado" IS NULL AND btrim("nombre"::text) ILIKE $7 ESCAPE '\'`
		if err != nil || sql != want || len(args) != 3 {
			t.Fatalf("got %q %v %v", sql, args, err)
		}
	})
}
