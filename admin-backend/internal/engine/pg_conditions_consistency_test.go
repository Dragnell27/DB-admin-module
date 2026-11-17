package engine

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"admin-backend/internal/modules"
)

var placeholderRe = regexp.MustCompile(`\$\d+`)

// operatorSample devuelve valores válidos de muestra para el operador.
func operatorSample(op modules.Operator) []string {
	switch op {
	case modules.OpIn, modules.OpNotIn:
		return []string{"A", "b"}
	case modules.OpIsNull, modules.OpIsNotNull:
		return nil
	default:
		return []string{"A_b"}
	}
}

// Las opciones, la relectura de catálogo y las etiquetas auxiliares aplican el
// MISMO predicado (y los mismos args de filtro) para cada operador; solo cambia
// la numeración propia de cada consumidor.
func TestCatalogFiltersConsistencyAcrossConsumers(t *testing.T) {
	wl := map[string][]string{"cat": {"id", "nombre", "activo"}}
	for _, op := range modules.AllOperators() {
		t.Run(string(op), func(t *testing.T) {
			filters := []modules.ListCondition{{Field: "activo", Operator: op, Values: operatorSample(op)}}
			src := modules.DynamicSource{Table: "cat", IDColumn: "id", LabelColumn: "nombre", Filters: filters}

			// Opciones: filtros desde $1, LIMIT $len(args)+1.
			optSQL, optArgs, err := buildOptionsSQL(wl, src)
			if err != nil {
				t.Fatalf("opciones: %v", err)
			}
			// Relectura: clave en $1, filtros desde $2, LIMIT 2 literal.
			rowSQL, rowArgs, err := buildRowLookupSQL(wl, "cat", "id", []string{"id", "nombre"}, filters)
			if err != nil {
				t.Fatalf("relectura: %v", err)
			}
			// Aux: ids $1..$k, filtros desde $k+1, LIMIT literal.
			ids := []any{1, 2, 3}
			auxSQL, auxArgs, err := buildAuxLabelsSQL(wl, &src, ids)
			if err != nil {
				t.Fatalf("aux: %v", err)
			}

			pred1, filterArgs, _ := buildListConditionsWhere(wl, "cat", filters, 1)
			pred2, _, _ := buildListConditionsWhere(wl, "cat", filters, 2)
			pred4, _, _ := buildListConditionsWhere(wl, "cat", filters, len(ids)+1)

			for name, c := range map[string]struct{ sql, pred string }{
				"opciones":  {optSQL, pred1},
				"relectura": {rowSQL, pred2},
				"aux":       {auxSQL, pred4},
			} {
				if !strings.Contains(c.sql, c.pred) {
					t.Errorf("%s no contiene el predicado %q:\n%s", name, c.pred, c.sql)
				}
			}
			if a, b, c := placeholderRe.ReplaceAllString(pred1, "$#"), placeholderRe.ReplaceAllString(pred2, "$#"), placeholderRe.ReplaceAllString(pred4, "$#"); a != b || b != c {
				t.Errorf("el predicado normalizado difiere: %q %q %q", a, b, c)
			}

			// Args de filtro iguales en los tres consumidores.
			if len(filterArgs) > 0 {
				if got := optArgs[:len(filterArgs)]; !reflect.DeepEqual(got, filterArgs) {
					t.Errorf("args de opciones %v != %v", got, filterArgs)
				}
				if !reflect.DeepEqual(rowArgs, filterArgs) {
					t.Errorf("args de relectura %v != %v", rowArgs, filterArgs)
				}
				if got := auxArgs[len(ids):]; !reflect.DeepEqual(got, filterArgs) {
					t.Errorf("args de aux %v != %v", got, filterArgs)
				}
			}

			// Numeración y LIMIT propios de cada consumidor.
			if want := fmt.Sprintf("LIMIT $%d", len(optArgs)); !strings.HasSuffix(optSQL, want) {
				t.Errorf("opciones debe terminar en %q: %s", want, optSQL)
			}
			if !strings.HasSuffix(rowSQL, "LIMIT 2") {
				t.Errorf("relectura debe terminar en LIMIT 2: %s", rowSQL)
			}
			if want := fmt.Sprintf("LIMIT %d", DefaultOptionsLimit); !strings.HasSuffix(auxSQL, want) {
				t.Errorf("aux debe terminar en %q: %s", want, auxSQL)
			}
		})
	}
}

func TestBuildAuxLabelsSQLNoIDs(t *testing.T) {
	wl := map[string][]string{"cat": {"id", "nombre"}}
	q, args, err := buildAuxLabelsSQL(wl, &modules.DynamicSource{Table: "cat", IDColumn: "id", LabelColumn: "nombre"}, nil)
	if err != nil || q != "" || args != nil {
		t.Fatalf("got %q %v %v", q, args, err)
	}
}
