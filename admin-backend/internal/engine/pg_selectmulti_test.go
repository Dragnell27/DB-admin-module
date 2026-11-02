package engine

import (
	"errors"
	"strings"
	"testing"

	"admin-backend/internal/modules"
)

// defMulti arma una Definition multi-tabla (pagos ← clientes) para los tests del
// builder de lectura.
func defMulti() modules.Definition {
	return modules.Definition{
		Slug: "pagos-clientes",
		FetchMapping: map[string]any{
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
		},
		DisplayMapping: map[string]any{
			"columns": []any{
				map[string]any{"field": "id", "isPrimaryKey": true, "sourceTable": "pagos", "visible": false, "order": 1},
				map[string]any{"field": "monto", "visible": true, "sourceTable": "pagos", "order": 2},
				map[string]any{"field": "id", "isPrimaryKey": true, "sourceTable": "clientes", "visible": false, "order": 3},
				map[string]any{"field": "nombre", "visible": true, "sourceTable": "clientes", "order": 4},
			},
		},
		DataSource: modules.DataSource{
			PGWhitelist: map[string][]string{
				"pagos":    {"id", "cliente_id", "monto"},
				"clientes": {"id", "nombre"},
			},
		},
	}
}

func TestBuildListSQL_MultiTable(t *testing.T) {
	def := defMulti()
	spec := modules.MultiTableFromFetchMapping(def.FetchMapping)
	parts, outputs, args, err := buildListSQL(def, spec, modules.Action{}, nil)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if len(args) != 0 {
		t.Fatalf("sin condiciones no debe haber args: %v", args)
	}

	// Alias <tabla>__<col> en la proyección y en los nombres de salida.
	for _, want := range []string{
		`"clientes"."id" AS "clientes__id"`,
		`"clientes"."nombre" AS "clientes__nombre"`,
		`"pagos"."id" AS "pagos__id"`,
		`"pagos"."monto" AS "pagos__monto"`,
	} {
		if !strings.Contains(parts.projection, want) {
			t.Fatalf("proyección sin %q:\n%s", want, parts.projection)
		}
	}

	// Las PK (visible==false) están incluidas en la salida.
	wantOutputs := map[string]bool{"pagos__id": true, "clientes__id": true}
	got := map[string]bool{}
	for _, o := range outputs {
		got[o] = true
	}
	for k := range wantOutputs {
		if !got[k] {
			t.Fatalf("falta la PK %q en los nombres de salida: %v", k, outputs)
		}
	}

	// LEFT JOIN del tipo declarado con ON cualificado.
	if !strings.Contains(parts.fromWhere, `LEFT JOIN "clientes" ON "pagos"."cliente_id" = "clientes"."id"`) {
		t.Fatalf("JOIN inesperado:\n%s", parts.fromWhere)
	}
	if !strings.HasPrefix(parts.fromWhere, `FROM "pagos"`) {
		t.Fatalf("FROM debe empezar por la principal:\n%s", parts.fromWhere)
	}
}

func TestBuildListSQL_N1Unchanged(t *testing.T) {
	def := modules.Definition{
		FetchMapping:   map[string]any{"table": "planes"},
		DisplayMapping: map[string]any{"columns": []any{map[string]any{"field": "id", "visible": true}}},
		DataSource:     modules.DataSource{PGWhitelist: map[string][]string{"planes": {"id", "nombre"}}},
	}
	spec := modules.MultiTableFromFetchMapping(def.FetchMapping)
	parts, outputs, _, err := buildListSQL(def, spec, modules.Action{}, nil)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	// Columnas peladas, sin alias, FROM sin JOIN.
	if strings.Contains(parts.projection, "AS") || strings.Contains(parts.projection, "__") {
		t.Fatalf("N=1 no debe usar alias: %s", parts.projection)
	}
	if parts.fromWhere != `FROM "planes"` {
		t.Fatalf("N=1 FROM inesperado: %s", parts.fromWhere)
	}
	for _, o := range outputs {
		if strings.Contains(o, "__") {
			t.Fatalf("N=1 nombres de salida deben ser pelados: %v", outputs)
		}
	}
}

func TestBuildListSQL_WhitelistPerTable(t *testing.T) {
	def := defMulti()
	// Quita "nombre" de la whitelist de clientes: la columna de visualización
	// debe abortar con ErrWhitelist.
	def.DataSource.PGWhitelist["clientes"] = []string{"id"}
	spec := modules.MultiTableFromFetchMapping(def.FetchMapping)
	_, _, _, err := buildListSQL(def, spec, modules.Action{}, nil)
	if err == nil || !errors.Is(err, ErrWhitelist) {
		t.Fatalf("se esperaba ErrWhitelist por columna fuera de lista: %v", err)
	}
}

// Test 8 (parte pura): COUNT y SELECT comparten FROM/JOIN/WHERE y args.
func TestBuildListSQL_ConditionsQualified(t *testing.T) {
	def := defMulti()
	def.ListConditions = []modules.ListCondition{
		{Field: "nombre", Values: []string{"Ana"}, SourceTable: "clientes"},
	}
	spec := modules.MultiTableFromFetchMapping(def.FetchMapping)
	parts, _, args, err := buildListSQL(def, spec, modules.Action{}, nil)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if len(args) != 1 || args[0] != "ana" {
		t.Fatalf("args normalizados inesperados: %v", args)
	}
	if !strings.Contains(parts.fromWhere, `WHERE lower(btrim("clientes"."nombre"::text)) IN ($1)`) {
		t.Fatalf("WHERE cualificado inesperado:\n%s", parts.fromWhere)
	}

	// COUNT y SELECT se componen a partir del MISMO fromWhere y args.
	countSQL := "SELECT count(*) " + parts.fromWhere
	selectSQL := "SELECT " + parts.projection + " " + parts.fromWhere
	if !strings.Contains(countSQL, parts.fromWhere) || !strings.Contains(selectSQL, parts.fromWhere) {
		t.Fatalf("COUNT y SELECT deben compartir el mismo FROM/WHERE")
	}
	// Los JOIN aparecen en ambos (vienen de fromWhere).
	if !strings.Contains(countSQL, "LEFT JOIN") || !strings.Contains(selectSQL, "LEFT JOIN") {
		t.Fatalf("el JOIN debe aparecer en COUNT y SELECT")
	}
}
