package engine

import (
	"context"
	"database/sql/driver"
	"strconv"
	"strings"
	"testing"

	"admin-backend/internal/modules"
)

// fsActionFiltered es fsAction pero con FILTROS en el catálogo del medio de pago
// (list_numero=10001), reproduciendo el caso real del módulo
// qa-medios-pago-x-comercio: gt_valores.valo_numero NO es único entre listas, así
// que la relectura por id DEBE acotarse a la sublista (como hace buildOptionsSQL
// al generar las opciones). El resto (comercio + derive-via-fk) es idéntico.
func fsActionFiltered() modules.Action {
	act := fsAction()
	for i := range act.InputFields {
		s := act.InputFields[i].Strategy
		if s != nil && s.CaptureKey == "medio_pago" && s.Catalog != nil {
			s.Catalog.Filters = []modules.ListCondition{{Field: "list_numero", Values: []string{"10001"}}}
		}
	}
	return act
}

// TestResolveFillStrategies_CatalogFilters_WhereAndBind congela el fix de
// E422_DERIVE por valo_numero no único entre listas: la relectura
// capture-from-catalog del medio de pago debe emitir el filtro del catálogo en el
// WHERE (clave en $1, filtro list_numero en $2) y ligar AMBOS: el id elegido y el
// valor del filtro. El formato del WHERE de filtros replica el de buildOptionsSQL.
func TestResolveFillStrategies_CatalogFilters_WhereAndBind(t *testing.T) {
	drv := &recordedDriver{}

	var valoresSQL string
	var valoresArgs []any
	drv.queryResponder = func(query string, args []driver.NamedValue) ([]string, [][]driver.Value, bool) {
		switch {
		case strings.Contains(query, `FROM "gt_valores"`):
			valoresSQL = query
			valoresArgs = nil
			for _, a := range args {
				valoresArgs = append(valoresArgs, a.Value)
			}
			return []string{"codigo", "list_numero", "valo_numero", "valor_es"},
				[][]driver.Value{{"MP01", int64(10001), int64(55), "Tarjeta"}}, true
		case strings.Contains(query, `FROM "gt_valores_padre"`):
			return []string{"depe_numero", "padr_numero"},
				[][]driver.Value{{int64(777), int64(999)}}, true
		}
		return nil, nil, false
	}

	tx, done := openFakeTx(t, drv)
	defer done()

	input := map[string]any{
		"__capture_comercio": int64(42),
		"valor_lista":        int64(55), // id del medio de pago elegido
	}
	out, err := resolveFillStrategies(context.Background(), tx, fsWL(), fsActionFiltered(), input)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	// (a) SQL EXACTO de la relectura del catálogo: clave en $1 + filtro del
	// catálogo en $2 (mismo formato lower(btrim(...)) IN (...) que buildOptionsSQL).
	// WL de gt_valores en orden alfabético: codigo, list_numero, valo_numero, valor_es.
	wantSQL := `SELECT "codigo", "list_numero", "valo_numero", "valor_es" FROM "gt_valores" WHERE "valo_numero" = $1 AND lower(btrim("list_numero"::text)) IN ($2) LIMIT 2`
	if valoresSQL != wantSQL {
		t.Fatalf("SQL de la relectura del catálogo inesperado:\n got %s\nwant %s", valoresSQL, wantSQL)
	}

	// (b) el bind lleva tanto el id elegido ($1 == 55) como el valor del filtro
	// ($2 == "10001", normalizado lower+trim).
	if len(valoresArgs) != 2 || valoresArgs[0] != int64(55) || valoresArgs[1] != "10001" {
		t.Fatalf("bind de la relectura inesperado, got %#v (want [55 \"10001\"])", valoresArgs)
	}

	// Sanidad: el fix no rompe el resto del flujo.
	if out["valor_lista"] != "MP01" || out["list_numero_h"] != int64(10001) {
		t.Fatalf("flujo roto por el fix: valor_lista=%v list_numero_h=%v", out["valor_lista"], out["list_numero_h"])
	}
}

// TestResolveFillStrategies_CatalogFilters_RegressionNoE422 es el caso de
// regresión del bug: en gt_valores existen DOS filas con el mismo valo_numero=55
// pero distinto list_numero (10001 y 20002). SIN el filtro el WHERE devolvía 2
// filas y abortaba con ErrDeriveRelation (E422_DERIVE). CON el filtro aplicado la
// relectura queda acotada a list_numero=10001 y devuelve 1 fila. El responder
// honra el filtro ligado en $2 para simular el comportamiento real de la BD.
func TestResolveFillStrategies_CatalogFilters_RegressionNoE422(t *testing.T) {
	drv := &recordedDriver{}
	drv.queryResponder = func(query string, args []driver.NamedValue) ([]string, [][]driver.Value, bool) {
		switch {
		case strings.Contains(query, `FROM "gt_valores"`):
			cols := []string{"codigo", "list_numero", "valo_numero", "valor_es"}
			// Universo: dos filas comparten valo_numero=55, distinto list_numero.
			universe := [][]driver.Value{
				{"MP01", int64(10001), int64(55), "Tarjeta"},
				{"MP99", int64(20002), int64(55), "Otra lista"},
			}
			// El filtro del catálogo viaja en $2 (lower+trim); solo lo aplicamos si
			// la relectura lo incluyó (WHERE con AND ... IN ($2)).
			if strings.Contains(query, "IN ($2)") && len(args) == 2 {
				want := args[1].Value // "10001"
				filtered := universe[:0:0]
				for _, row := range universe {
					if toText(row[1]) == want {
						filtered = append(filtered, row)
					}
				}
				return cols, filtered, true
			}
			// Sin filtro: devolvería las 2 filas (el bug original ⇒ E422).
			return cols, universe, true
		case strings.Contains(query, `FROM "gt_valores_padre"`):
			return []string{"depe_numero", "padr_numero"},
				[][]driver.Value{{int64(777), int64(999)}}, true
		}
		return nil, nil, false
	}

	tx, done := openFakeTx(t, drv)
	defer done()

	input := map[string]any{"__capture_comercio": int64(42), "valor_lista": int64(55)}
	out, err := resolveFillStrategies(context.Background(), tx, fsWL(), fsActionFiltered(), input)
	if err != nil {
		t.Fatalf("con el filtro aplicado la relectura debe dar 1 fila, no ErrDeriveRelation: %v", err)
	}

	// La fila resuelta es la de list_numero=10001 (NO la de 20002).
	if out["list_numero_h"] != int64(10001) {
		t.Fatalf("la relectura debe resolver la sublista 10001, got %v", out["list_numero_h"])
	}
	if out["valor_lista"] != "MP01" {
		t.Fatalf("valor_lista debe ser el codigo de la fila 10001, got %v", out["valor_lista"])
	}
}

// toText normaliza el list_numero de una fila (driver.Value) a texto con la
// MISMA semántica que buildListConditionsWhere (lower+trim) para compararlo con
// el arg ligado en el test de regresión.
func toText(v driver.Value) string {
	switch t := v.(type) {
	case string:
		return strings.ToLower(strings.TrimSpace(t))
	case int64:
		return strconv.FormatInt(t, 10)
	default:
		return ""
	}
}
