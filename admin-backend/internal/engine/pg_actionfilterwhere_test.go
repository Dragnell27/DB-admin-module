package engine

import (
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"admin-backend/internal/modules"
)

// filterWhitelist es la lista blanca común de las pruebas del constructor de
// WHERE de filtro de acción SELECT.
var filterWhitelist = map[string][]string{
	"medios_pago": {"id", "nombre", "estado", "numero"},
	"clientes":    {"id", "estado", "nombre"},
}

// n1Spec devuelve una spec N=1 con la tabla principal real poblada (como la
// produce MultiTableFromFetchMapping desde fm["table"]) e IsMultiTable()==false.
func n1Spec(principal string) modules.MultiTableSpec {
	return modules.MultiTableSpec{PrincipalTable: principal}
}

// multiSpec devuelve una spec multi-tabla mínima (ForeignTables no vacío ⇒
// IsMultiTable()==true).
func multiSpec(principal string, foreign ...string) modules.MultiTableSpec {
	fts := make([]modules.ForeignTable, 0, len(foreign))
	for _, f := range foreign {
		fts = append(fts, modules.ForeignTable{Table: f})
	}
	return modules.MultiTableSpec{PrincipalTable: principal, ForeignTables: fts}
}

// selectAction construye una acción SELECT con los InputFields dados en el orden
// declarado (determinismo del WHERE de filtro).
func selectAction(fields ...modules.FieldSpec) modules.Action {
	return modules.Action{ActionKey: "custom:filtrar", Kind: modules.ActionKindCustom, SQLOp: "SELECT", InputFields: fields}
}

// Verifica la semántica por rama (exact/partial), la numeración desde
// startParam, la omisión de vacíos, el orden DECLARADO, el rechazo por lista
// blanca y la cualificación multi-tabla.
func TestBuildActionFilterWhere(t *testing.T) {
	cases := []struct {
		name       string
		act        modules.Action
		spec       modules.MultiTableSpec
		input      map[string]any
		startParam int
		wantClause string
		wantArgs   []any
		wantErr    error // nil o ErrWhitelist
	}{
		{
			name:       "acción sin campos ⇒ clause vacío y args nil",
			act:        selectAction(),
			spec:       n1Spec("medios_pago"),
			input:      map[string]any{},
			startParam: 1,
			wantClause: "",
			wantArgs:   nil,
		},
		{
			name:       "N=1 exact ⇒ lower(btrim(col)) = $N con arg lower+trim",
			act:        selectAction(modules.FieldSpec{Name: "estado", MatchType: modules.SearchMatchExact}),
			spec:       n1Spec("medios_pago"),
			input:      map[string]any{"estado": "  ACTIVO "},
			startParam: 1,
			wantClause: `lower(btrim("estado"::text)) = $1`,
			wantArgs:   []any{"activo"},
		},
		{
			name:       "N=1 partial ⇒ LIKE con comodines literales y arg solo-trim (sin lower en Go)",
			act:        selectAction(modules.FieldSpec{Name: "nombre", MatchType: modules.SearchMatchPartial}),
			spec:       n1Spec("medios_pago"),
			input:      map[string]any{"nombre": "  Visa "},
			startParam: 1,
			wantClause: `lower("nombre"::text) LIKE '%' || lower($1) || '%'`,
			wantArgs:   []any{"Visa"},
		},
		{
			name:       "MatchType ausente ⇒ exact por defecto",
			act:        selectAction(modules.FieldSpec{Name: "estado"}),
			spec:       n1Spec("medios_pago"),
			input:      map[string]any{"estado": "1"},
			startParam: 1,
			wantClause: `lower(btrim("estado"::text)) = $1`,
			wantArgs:   []any{"1"},
		},
		{
			name:       "startParam desplaza la numeración (continúa tras los estáticos)",
			act:        selectAction(modules.FieldSpec{Name: "estado", MatchType: modules.SearchMatchExact}),
			spec:       n1Spec("medios_pago"),
			input:      map[string]any{"estado": "1"},
			startParam: 3,
			wantClause: `lower(btrim("estado"::text)) = $3`,
			wantArgs:   []any{"1"},
		},
		{
			name: "varios campos llenos ⇒ AND en ORDEN DECLARADO de InputFields",
			act: selectAction(
				modules.FieldSpec{Name: "nombre", MatchType: modules.SearchMatchPartial},
				modules.FieldSpec{Name: "estado", MatchType: modules.SearchMatchExact},
			),
			spec:       n1Spec("medios_pago"),
			input:      map[string]any{"estado": "activo", "nombre": "vi"},
			startParam: 1,
			// orden DECLARADO: nombre primero, estado después (no alfabético).
			wantClause: `lower("nombre"::text) LIKE '%' || lower($1) || '%' AND lower(btrim("estado"::text)) = $2`,
			wantArgs:   []any{"vi", "activo"},
		},
		{
			name: "campo vacío/solo-espacios se omite (no añade predicado)",
			act: selectAction(
				modules.FieldSpec{Name: "estado", MatchType: modules.SearchMatchExact},
				modules.FieldSpec{Name: "nombre", MatchType: modules.SearchMatchExact},
			),
			spec:       n1Spec("medios_pago"),
			input:      map[string]any{"estado": "   ", "nombre": "visa"},
			startParam: 1,
			wantClause: `lower(btrim("nombre"::text)) = $1`,
			wantArgs:   []any{"visa"},
		},
		{
			name:       "campo ausente del input se omite",
			act:        selectAction(modules.FieldSpec{Name: "estado", MatchType: modules.SearchMatchExact}),
			spec:       n1Spec("medios_pago"),
			input:      map[string]any{},
			startParam: 1,
			wantClause: "",
			wantArgs:   nil,
		},
		{
			name:       "columna fuera de la lista blanca ⇒ ErrWhitelist sin SQL",
			act:        selectAction(modules.FieldSpec{Name: "fantasma", MatchType: modules.SearchMatchExact}),
			spec:       n1Spec("medios_pago"),
			input:      map[string]any{"fantasma": "x"},
			startParam: 1,
			wantErr:    ErrWhitelist,
		},
		{
			name:       "multi-tabla ⇒ fragmento cualificado por f.SourceTable",
			act:        selectAction(modules.FieldSpec{Name: "estado", SourceTable: "clientes", MatchType: modules.SearchMatchExact}),
			spec:       multiSpec("medios_pago", "clientes"),
			input:      map[string]any{"estado": "activo"},
			startParam: 1,
			wantClause: `lower(btrim("clientes"."estado"::text)) = $1`,
			wantArgs:   []any{"activo"},
		},
		{
			name:       "multi-tabla sin SourceTable ⇒ cualifica por la principal",
			act:        selectAction(modules.FieldSpec{Name: "estado", MatchType: modules.SearchMatchExact}),
			spec:       multiSpec("medios_pago", "clientes"),
			input:      map[string]any{"estado": "activo"},
			startParam: 1,
			wantClause: `lower(btrim("medios_pago"."estado"::text)) = $1`,
			wantArgs:   []any{"activo"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clause, args, err := buildActionFilterWhere(filterWhitelist, nil, tc.spec, tc.act, tc.input, tc.startParam)
			if tc.wantErr != nil {
				if err == nil {
					t.Fatalf("se esperaba error, got nil (clause=%q)", clause)
				}
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("el error debe envolver %v, got %v", tc.wantErr, err)
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

// Verifica la INVARIANTE de numeración "un parámetro por fragmento": len(args)
// == número de fragmentos; el primer $N es exactamente startParam; el último es
// startParam + len(args) - 1; los comodines '%' NO son parámetros.
func TestBuildActionFilterWhereNumberingInvariant(t *testing.T) {
	act := selectAction(
		modules.FieldSpec{Name: "estado", MatchType: modules.SearchMatchExact},
		modules.FieldSpec{Name: "nombre", MatchType: modules.SearchMatchPartial},
	)
	startParam := 4 // simula 3 args estáticos previos
	clause, args, err := buildActionFilterWhere(filterWhitelist, nil, n1Spec("medios_pago"), act,
		map[string]any{"estado": "a", "nombre": "b"}, startParam)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	fragments := strings.Split(clause, " AND ")
	if len(fragments) != len(args) {
		t.Fatalf("un parámetro por fragmento: %d fragmentos vs %d args", len(fragments), len(args))
	}
	if !strings.Contains(clause, "$"+strconv.Itoa(startParam)) {
		t.Fatalf("el primer marcador debe ser $%d: %s", startParam, clause)
	}
	last := startParam + len(args) - 1
	if !strings.Contains(clause, "$"+strconv.Itoa(last)) {
		t.Fatalf("el último marcador debe ser $%d: %s", last, clause)
	}
	beyond := startParam + len(args)
	if strings.Contains(clause, "$"+strconv.Itoa(beyond)) {
		t.Fatalf("no debe haber marcadores más allá de $%d (comodines no parametrizados): %s", last, clause)
	}
}

// Verifica la composición en buildListSQL: el WHERE fusiona (estáticas) AND
// (filtro), los args se concatenan en orden estáticos→filtro, y COUNT y SELECT
// de datos comparten byte-por-byte el fromWhere y los condArgs (R4.6).
func TestBuildListSQL_FilterComposition(t *testing.T) {
	def := modules.Definition{
		FetchMapping: map[string]any{"table": "medios_pago"},
		DisplayMapping: map[string]any{"columns": []any{
			map[string]any{"field": "id", "visible": true},
			map[string]any{"field": "estado", "visible": true},
		}},
		ListConditions: []modules.ListCondition{
			{Field: "eliminado", Values: []string{"-1"}},
		},
		DataSource: modules.DataSource{PGWhitelist: map[string][]string{
			"medios_pago": {"id", "estado", "eliminado"},
		}},
	}
	spec := modules.MultiTableFromFetchMapping(def.FetchMapping)
	act := selectAction(modules.FieldSpec{Name: "estado", MatchType: modules.SearchMatchExact})
	input := map[string]any{"estado": "ACTIVO"}

	parts, _, condArgs, err := buildListSQL(def, spec, act, input)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	// WHERE fusionado y parentizado: (estáticas) AND (filtro).
	wantWhere := ` WHERE (lower(btrim("eliminado"::text)) IN ($1)) AND (lower(btrim("estado"::text)) = $2)`
	if !strings.HasSuffix(parts.fromWhere, wantWhere) {
		t.Fatalf("fromWhere\n  got:  %s\n  want sufijo: %s", parts.fromWhere, wantWhere)
	}

	// args en orden estáticos→filtro.
	wantArgs := []any{"-1", "activo"}
	if !reflect.DeepEqual(condArgs, wantArgs) {
		t.Fatalf("condArgs\n  got:  %v\n  want: %v", condArgs, wantArgs)
	}

	// Primer $N del filtro == len(staticArgs)+1 == 2 (contigüidad).
	if !strings.Contains(parts.fromWhere, "= $2") {
		t.Fatalf("el primer $N del filtro debe ser $2: %s", parts.fromWhere)
	}

	// COUNT y SELECT comparten el mismo fromWhere y condArgs; el SELECT numera
	// LIMIT/OFFSET DESPUÉS (len(condArgs)+1/+2).
	countSQL := "SELECT count(*) " + parts.fromWhere
	dataSQL := "SELECT " + parts.projection + " " + parts.fromWhere +
		" LIMIT $" + strconv.Itoa(len(condArgs)+1) + " OFFSET $" + strconv.Itoa(len(condArgs)+2)
	if !strings.Contains(countSQL, wantWhere) || !strings.Contains(dataSQL, wantWhere) {
		t.Fatalf("COUNT y SELECT deben compartir el WHERE\ncount: %s\ndata:  %s", countSQL, dataSQL)
	}
	if !strings.HasSuffix(dataSQL, "LIMIT $3 OFFSET $4") {
		t.Fatalf("LIMIT/OFFSET deben numerarse tras los args del WHERE: %s", dataSQL)
	}
}

// Verifica la equivalencia List vs ListFiltered-sin-filtros: buildListSQL con la
// acción cero valor produce el MISMO fromWhere/args que con una acción SELECT
// cuyos campos están vacíos (no aportan predicado).
func TestBuildListSQL_EmptyActionEquivalentToBase(t *testing.T) {
	def := modules.Definition{
		FetchMapping: map[string]any{"table": "medios_pago"},
		DisplayMapping: map[string]any{"columns": []any{
			map[string]any{"field": "id", "visible": true},
			map[string]any{"field": "estado", "visible": true},
		}},
		ListConditions: []modules.ListCondition{
			{Field: "eliminado", Values: []string{"-1"}},
		},
		DataSource: modules.DataSource{PGWhitelist: map[string][]string{
			"medios_pago": {"id", "estado", "eliminado"},
		}},
	}
	spec := modules.MultiTableFromFetchMapping(def.FetchMapping)

	basePartes, baseOut, baseArgs, err := buildListSQL(def, spec, modules.Action{}, nil)
	if err != nil {
		t.Fatalf("base: error inesperado: %v", err)
	}

	// Acción SELECT con un campo cuyo valor viene vacío ⇒ sin predicado de filtro.
	act := selectAction(modules.FieldSpec{Name: "estado", MatchType: modules.SearchMatchExact})
	emptyPartes, emptyOut, emptyArgs, err := buildListSQL(def, spec, act, map[string]any{"estado": "   "})
	if err != nil {
		t.Fatalf("filtro vacío: error inesperado: %v", err)
	}

	if basePartes.fromWhere != emptyPartes.fromWhere || basePartes.projection != emptyPartes.projection {
		t.Fatalf("fromWhere/projection deben ser idénticos\n base:  %+v\n empty: %+v", basePartes, emptyPartes)
	}
	if !reflect.DeepEqual(baseArgs, emptyArgs) {
		t.Fatalf("args deben ser idénticos\n base:  %v\n empty: %v", baseArgs, emptyArgs)
	}
	if !reflect.DeepEqual(baseOut, emptyOut) {
		t.Fatalf("outputNames deben ser idénticos\n base:  %v\n empty: %v", baseOut, emptyOut)
	}
}

// Verifica que un módulo SIN condiciones ni acción emite el SQL de hoy
// byte-por-byte (retrocompat total del GET /list base).
func TestBuildListSQL_NoFilterUnchanged(t *testing.T) {
	def := modules.Definition{
		FetchMapping:   map[string]any{"table": "medios_pago"},
		DisplayMapping: map[string]any{"columns": []any{map[string]any{"field": "id", "visible": true}}},
		DataSource:     modules.DataSource{PGWhitelist: map[string][]string{"medios_pago": {"id", "nombre"}}},
	}
	spec := modules.MultiTableFromFetchMapping(def.FetchMapping)
	parts, _, condArgs, err := buildListSQL(def, spec, modules.Action{}, nil)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if parts.fromWhere != `FROM "medios_pago"` {
		t.Fatalf("fromWhere inesperado: %s", parts.fromWhere)
	}
	if len(condArgs) != 0 {
		t.Fatalf("sin filtro ni condiciones no debe haber args: %v", condArgs)
	}
}
