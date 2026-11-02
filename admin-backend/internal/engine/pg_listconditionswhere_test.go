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

// Verifica que buildListConditionsWhere reproduce la semántica en memoria de
// rowMeetsAll en SQL parametrizado: IN por columna (OR), AND entre columnas,
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
			name: "condición con Values vacío o Field vacío se salta (paridad con rowMeetsAll)",
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
