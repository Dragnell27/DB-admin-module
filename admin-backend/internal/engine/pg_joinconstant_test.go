package engine

import (
	"errors"
	"strings"
	"testing"

	"admin-backend/internal/modules"
)

// defMultiConstant arma una Definición multi-tabla (pagos ← clientes) con una
// condición constante en el ON del JOIN: "clientes"."estado" = $N.
func defMultiConstant() modules.Definition {
	def := defMulti()
	def.DataSource.PGWhitelist["clientes"] = []string{"id", "nombre", "estado"}
	def.FetchMapping["relationships"] = []any{
		map[string]any{
			"childTable":  "pagos",
			"parentTable": "clientes",
			"on":          []any{map[string]any{"child": "cliente_id", "parent": "id"}},
			"onConstants": []any{map[string]any{"table": "clientes", "column": "estado", "value": "1"}},
			"joinType":    "left",
		},
	}
	return def
}

// TestBuildJoinClause_OnConstant verifica que una constante del ON se renderiza
// como "<tabla>"."<col>" = $N, con el valor LIGADO (no interpolado) y la
// numeración arrancando en startParam.
func TestBuildJoinClause_OnConstant(t *testing.T) {
	def := defMultiConstant()
	spec := modules.MultiTableFromFetchMapping(def.FetchMapping)
	from, args, err := buildJoinClause(def.DataSource.PGWhitelist, spec, 1)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	want := `LEFT JOIN "clientes" ON "pagos"."cliente_id" = "clientes"."id" AND "clientes"."estado" = $1`
	if !strings.Contains(from, want) {
		t.Fatalf("ON con constante inesperado:\n%s", from)
	}
	if len(args) != 1 || args[0] != "1" {
		t.Fatalf("el valor debe ir ligado como string sin interpolar: %v", args)
	}
	// El valor NO debe aparecer interpolado en el texto SQL.
	if strings.Contains(from, "= 1 ") || strings.Contains(from, "= '1'") {
		t.Fatalf("el valor constante NO debe interpolarse:\n%s", from)
	}
}

// TestBuildJoinClause_StartParam confirma que startParam desplaza la numeración
// de los $N del ON.
func TestBuildJoinClause_StartParam(t *testing.T) {
	def := defMultiConstant()
	spec := modules.MultiTableFromFetchMapping(def.FetchMapping)
	from, args, err := buildJoinClause(def.DataSource.PGWhitelist, spec, 5)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if !strings.Contains(from, `"clientes"."estado" = $5`) {
		t.Fatalf("la numeración debe arrancar en startParam:\n%s", from)
	}
	if len(args) != 1 {
		t.Fatalf("un único arg esperado: %v", args)
	}
}

// TestBuildJoinClause_OnConstantOutOfWhitelist aborta con ErrWhitelist cuando la
// columna de la constante no está en la lista blanca y NO produce SQL.
func TestBuildJoinClause_OnConstantOutOfWhitelist(t *testing.T) {
	def := defMultiConstant()
	// "estado" fuera de la lista blanca de clientes.
	def.DataSource.PGWhitelist["clientes"] = []string{"id", "nombre"}
	spec := modules.MultiTableFromFetchMapping(def.FetchMapping)
	from, args, err := buildJoinClause(def.DataSource.PGWhitelist, spec, 1)
	if err == nil || !errors.Is(err, ErrWhitelist) {
		t.Fatalf("se esperaba ErrWhitelist: %v", err)
	}
	if from != "" || args != nil {
		t.Fatalf("no debe producir SQL ni args al abortar: from=%q args=%v", from, args)
	}
}

// TestBuildJoinClause_OnConstantForeignTable aborta con ErrWhitelist cuando la
// tabla de la constante es ajena a la relación.
func TestBuildJoinClause_OnConstantForeignTable(t *testing.T) {
	def := defMultiConstant()
	rels := def.FetchMapping["relationships"].([]any)
	rels[0].(map[string]any)["onConstants"] = []any{
		map[string]any{"table": "pagos_ajena", "column": "estado", "value": "1"},
	}
	spec := modules.MultiTableFromFetchMapping(def.FetchMapping)
	_, _, err := buildJoinClause(def.DataSource.PGWhitelist, spec, 1)
	if err == nil || !errors.Is(err, ErrWhitelist) {
		t.Fatalf("se esperaba ErrWhitelist por tabla ajena: %v", err)
	}
}

// TestBuildJoinClause_MultiplePairs confirma que varios pares columna=columna se
// unen con AND en el ON.
func TestBuildJoinClause_MultiplePairs(t *testing.T) {
	def := defMulti()
	def.DataSource.PGWhitelist["pagos"] = []string{"id", "cliente_id", "monto", "pais"}
	def.DataSource.PGWhitelist["clientes"] = []string{"id", "nombre", "pais"}
	def.FetchMapping["relationships"] = []any{
		map[string]any{
			"childTable":  "pagos",
			"parentTable": "clientes",
			"on": []any{
				map[string]any{"child": "cliente_id", "parent": "id"},
				map[string]any{"child": "pais", "parent": "pais"},
			},
			"joinType": "left",
		},
	}
	spec := modules.MultiTableFromFetchMapping(def.FetchMapping)
	from, _, err := buildJoinClause(def.DataSource.PGWhitelist, spec, 1)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	want := `ON "pagos"."cliente_id" = "clientes"."id" AND "pagos"."pais" = "clientes"."pais"`
	if !strings.Contains(from, want) {
		t.Fatalf("pares múltiples deben unirse con AND:\n%s", from)
	}
}

// TestBuildJoinClause_BackwardCompat garantiza que una relación SIN OnConstants
// produce el ON byte-por-byte de hoy y cero args.
func TestBuildJoinClause_BackwardCompat(t *testing.T) {
	def := defMulti()
	spec := modules.MultiTableFromFetchMapping(def.FetchMapping)
	from, args, err := buildJoinClause(def.DataSource.PGWhitelist, spec, 1)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if len(args) != 0 {
		t.Fatalf("sin constantes no debe haber args: %v", args)
	}
	if !strings.Contains(from, `LEFT JOIN "clientes" ON "pagos"."cliente_id" = "clientes"."id"`) {
		t.Fatalf("ON retrocompatible inesperado:\n%s", from)
	}
}

// TestBuildListSQL_OnConstantParamOrder verifica que el $N del ON precede al del
// WHERE y que COUNT y SELECT comparten el mismo fromWhere y condArgs.
func TestBuildListSQL_OnConstantParamOrder(t *testing.T) {
	def := defMultiConstant()
	def.ListConditions = []modules.ListCondition{
		{Field: "nombre", Values: []string{"Ana"}, SourceTable: "clientes"},
	}
	spec := modules.MultiTableFromFetchMapping(def.FetchMapping)
	parts, _, condArgs, err := buildListSQL(def, spec, modules.Action{}, nil)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	// La constante del ON ocupa $1 y el WHERE se desplaza a $2.
	if !strings.Contains(parts.fromWhere, `"clientes"."estado" = $1`) {
		t.Fatalf("la constante del ON debe ser $1:\n%s", parts.fromWhere)
	}
	if !strings.Contains(parts.fromWhere, `WHERE lower(btrim("clientes"."nombre"::text)) IN ($2)`) {
		t.Fatalf("el WHERE debe desplazarse a $2:\n%s", parts.fromWhere)
	}
	// condArgs en orden JOIN ON → WHERE: ["1", "ana"].
	if len(condArgs) != 2 || condArgs[0] != "1" || condArgs[1] != "ana" {
		t.Fatalf("orden de condArgs inesperado: %v", condArgs)
	}
	// COUNT y SELECT comparten fromWhere y, por tanto, condArgs y numeración $N.
	countSQL := "SELECT count(*) " + parts.fromWhere
	selectSQL := "SELECT " + parts.projection + " " + parts.fromWhere
	if !strings.Contains(countSQL, `"clientes"."estado" = $1`) ||
		!strings.Contains(selectSQL, `"clientes"."estado" = $1`) {
		t.Fatalf("COUNT y SELECT deben compartir el $N del ON")
	}
}

// TestBuildListSQL_FourTableLeftJoinWithConstant reproduce la forma de la
// consulta objetivo del usuario: cuatro LEFT JOIN encadenados con la constante
// list_numero = 10001 en el ON del último JOIN.
func TestBuildListSQL_FourTableLeftJoinWithConstant(t *testing.T) {
	def := modules.Definition{
		Slug: "comercios-valores",
		FetchMapping: map[string]any{
			"table": "cl_comercios_sapp",
			"foreignTables": []any{
				map[string]any{"table": "gt_valores_padre"},
				map[string]any{"table": "gt_valores_hijo"},
				map[string]any{"table": "gt_valores"},
			},
			"relationships": []any{
				map[string]any{
					"childTable":  "cl_comercios_sapp",
					"parentTable": "gt_valores_padre",
					"on":          []any{map[string]any{"child": "come_numero", "parent": "valor_objeto"}},
					"joinType":    "left",
				},
				map[string]any{
					"childTable":  "gt_valores_hijo",
					"parentTable": "gt_valores_padre",
					"on":          []any{map[string]any{"child": "padr_numero", "parent": "padr_numero"}},
					"joinType":    "left",
				},
				map[string]any{
					"childTable":  "gt_valores_hijo",
					"parentTable": "gt_valores",
					"on":          []any{map[string]any{"child": "valor_lista", "parent": "codigo"}},
					"onConstants": []any{map[string]any{"table": "gt_valores", "column": "list_numero", "value": "10001"}},
					"joinType":    "left",
				},
			},
		},
		DisplayMapping: map[string]any{
			"columns": []any{
				map[string]any{"field": "comercio_numero", "isPrimaryKey": true, "sourceTable": "cl_comercios_sapp", "visible": false, "order": 1},
				map[string]any{"field": "valor_lista", "visible": true, "sourceTable": "gt_valores_hijo", "order": 2},
				map[string]any{"field": "valor_es", "visible": true, "sourceTable": "gt_valores", "order": 3},
			},
		},
		DataSource: modules.DataSource{
			PGWhitelist: map[string][]string{
				"cl_comercios_sapp": {"comercio_numero", "come_numero"},
				"gt_valores_padre":  {"padr_numero", "valor_objeto"},
				"gt_valores_hijo":   {"padr_numero", "valor_lista"},
				"gt_valores":        {"codigo", "list_numero", "valor_es"},
			},
		},
	}
	spec := modules.MultiTableFromFetchMapping(def.FetchMapping)
	parts, _, condArgs, err := buildListSQL(def, spec, modules.Action{}, nil)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	// Tres LEFT JOIN y la constante en el ON del JOIN de gt_valores.
	if strings.Count(parts.fromWhere, "LEFT JOIN") != 3 {
		t.Fatalf("se esperaban 3 LEFT JOIN:\n%s", parts.fromWhere)
	}
	if !strings.Contains(parts.fromWhere,
		`ON "gt_valores_hijo"."valor_lista" = "gt_valores"."codigo" AND "gt_valores"."list_numero" = $1`) {
		t.Fatalf("la constante debe vivir en el ON del último JOIN:\n%s", parts.fromWhere)
	}
	if len(condArgs) != 1 || condArgs[0] != "10001" {
		t.Fatalf("el valor 10001 debe ir ligado como $1: %v", condArgs)
	}
}

// onConstantSpec devuelve la spec con las constantes dadas en el ON (pagos ← clientes).
func onConstantSpec(consts ...map[string]any) (map[string][]string, modules.MultiTableSpec) {
	def := defMultiConstant()
	list := make([]any, len(consts))
	for i, c := range consts {
		list[i] = c
	}
	def.FetchMapping["relationships"] = []any{
		map[string]any{
			"childTable":  "pagos",
			"parentTable": "clientes",
			"on":          []any{map[string]any{"child": "cliente_id", "parent": "id"}},
			"onConstants": list,
			"joinType":    "left",
		},
	}
	return def.DataSource.PGWhitelist, modules.MultiTableFromFetchMapping(def.FetchMapping)
}

// Cada operador en las constantes del ON: SQL (modo raw) y args exactos, con $N
// correlativos tras otras constantes y los pares ON intactos.
func TestBuildJoinClause_OnConstantOperators(t *testing.T) {
	const base = `LEFT JOIN "clientes" ON "pagos"."cliente_id" = "clientes"."id"`
	c := func(extra map[string]any) map[string]any {
		m := map[string]any{"table": "clientes", "column": "estado", "value": ""}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	cases := []struct {
		name     string
		consts   []map[string]any
		wantTail string
		wantArgs []any
	}{
		{"sin operador ⇒ =", []map[string]any{c(map[string]any{"value": "1"})}, ` AND "clientes"."estado" = $1`, []any{"1"}},
		{"Value vacío sin operador ⇒ = con arg vacío", []map[string]any{c(nil)}, ` AND "clientes"."estado" = $1`, []any{""}},
		{"eq", []map[string]any{c(map[string]any{"operator": "eq", "value": "1"})}, ` AND "clientes"."estado" = $1`, []any{"1"}},
		{"neq", []map[string]any{c(map[string]any{"operator": "neq", "value": "1"})}, ` AND "clientes"."estado" <> $1`, []any{"1"}},
		{"in", []map[string]any{c(map[string]any{"operator": "in", "values": []any{"10001", "10002"}})}, ` AND "clientes"."estado" IN ($1, $2)`, []any{"10001", "10002"}},
		{"not_in", []map[string]any{c(map[string]any{"operator": "not_in", "values": []any{"1"}})}, ` AND "clientes"."estado" NOT IN ($1)`, []any{"1"}},
		{"contains", []map[string]any{c(map[string]any{"operator": "contains", "value": "a%"})}, ` AND "clientes"."estado"::text ILIKE $1 ESCAPE '\'`, []any{`%a\%%`}},
		{"is_null", []map[string]any{c(map[string]any{"operator": "is_null"})}, ` AND "clientes"."estado" IS NULL`, nil},
		{"is_not_null", []map[string]any{c(map[string]any{"operator": "is_not_null"})}, ` AND "clientes"."estado" IS NOT NULL`, nil},
		{"correlativos tras otras constantes", []map[string]any{
			c(map[string]any{"value": "1"}),
			c(map[string]any{"column": "nombre", "operator": "is_null"}),
			c(map[string]any{"operator": "in", "values": []any{"a", "b"}}),
		}, ` AND "clientes"."estado" = $1 AND "clientes"."nombre" IS NULL AND "clientes"."estado" IN ($2, $3)`, []any{"1", "a", "b"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wl, spec := onConstantSpec(tc.consts...)
			from, args, err := buildJoinClause(wl, spec, 1)
			if err != nil {
				t.Fatalf("error inesperado: %v", err)
			}
			if !strings.HasSuffix(from, base+tc.wantTail) {
				t.Fatalf("from\n  got:  %s\n  want sufijo: %s", from, base+tc.wantTail)
			}
			if len(args) != len(tc.wantArgs) {
				t.Fatalf("args got %v want %v", args, tc.wantArgs)
			}
			for i := range args {
				if args[i] != tc.wantArgs[i] {
					t.Fatalf("args got %v want %v", args, tc.wantArgs)
				}
			}
		})
	}
}

func TestBuildJoinClause_OnConstantOperatorErrors(t *testing.T) {
	c := func(extra map[string]any) map[string]any {
		m := map[string]any{"table": "clientes", "column": "estado", "value": ""}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	for name, jc := range map[string]map[string]any{
		"operador desconocido":     c(map[string]any{"operator": "zzz", "value": "1"}),
		"Value vacío con operador": c(map[string]any{"operator": "eq"}),
		"in sin valores":           c(map[string]any{"operator": "in"}),
	} {
		t.Run(name, func(t *testing.T) {
			wl, spec := onConstantSpec(jc)
			from, args, err := buildJoinClause(wl, spec, 1)
			if name == "is_null con valores" {
				// OperatorValues ignora Values en nulidad: se emite sin valores.
				if err != nil {
					t.Fatalf("no esperaba error: %v", err)
				}
				return
			}
			if !errors.Is(err, ErrValidation) || from != "" || args != nil {
				t.Fatalf("esperaba ErrValidation sin SQL, got %q %v %v", from, args, err)
			}
		})
	}
}
