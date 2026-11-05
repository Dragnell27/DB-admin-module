package engine

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"

	"admin-backend/internal/modules"
)

// auxLookupWL es la lista blanca de las pruebas del lookup en lote: la auxiliar
// countries con sus columnas id/etiqueta/filtro.
func auxLookupWL() map[string][]string {
	return map[string][]string{
		"countries": {"id", "name", "active"},
	}
}

// openFakeDB abre un *sql.DB sobre el driver falso con el responder dado.
func openFakeDB(drv *recordedDriver) *sql.DB {
	return sql.OpenDB(fakeConnector{drv: drv})
}

// TestAuxLabelsForColumn_LoteUnaConsulta verifica que el lookup emite UNA sola
// consulta con los ids DISTINTOS ligados como $1..$k en orden estable de
// aparición, y resuelve id→etiqueta en el mapa de salida.
func TestAuxLabelsForColumn_LoteUnaConsulta(t *testing.T) {
	drv := &recordedDriver{}
	var gotArgs []driver.NamedValue
	drv.queryResponder = func(query string, args []driver.NamedValue) ([]string, [][]driver.Value, bool) {
		gotArgs = args
		return []string{"id", "name"}, [][]driver.Value{
			{int64(1), "Guatemala"},
			{int64(2), "Colombia"},
		}, true
	}
	db := openFakeDB(drv)
	defer db.Close()

	rows := []map[string]any{
		{"country_id": int64(1)},
		{"country_id": int64(2)},
		{"country_id": int64(1)}, // repetido: NO debe duplicar el id ligado
	}
	src := &modules.DynamicSource{Table: "countries", IDColumn: "id", LabelColumn: "name"}

	labels, err := auxLabelsForColumn(context.Background(), db, auxLookupWL(), src, rows, "country_id")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	// Una sola consulta.
	if len(drv.queries) != 1 {
		t.Fatalf("se esperaba exactamente 1 consulta, got %d: %v", len(drv.queries), drv.queries)
	}
	wantSQL := `SELECT "id", "name" FROM "countries" WHERE "id" IN ($1, $2) LIMIT 500`
	if drv.queries[0] != wantSQL {
		t.Fatalf("SQL inesperado:\n got %s\nwant %s", drv.queries[0], wantSQL)
	}
	// Ids distintos en orden de aparición: $1=1, $2=2 (el 1 repetido no añade $3).
	if len(gotArgs) != 2 {
		t.Fatalf("se esperaban 2 args (ids distintos), got %d: %v", len(gotArgs), gotArgs)
	}
	// Resolución id→etiqueta.
	if labels["1"] != "Guatemala" || labels["2"] != "Colombia" {
		t.Fatalf("resolución id→etiqueta inesperada: %v", labels)
	}
}

// TestAuxLabelsForColumn_ConFiltros verifica que los filtros del catálogo se
// numeran DESPUÉS de los ids ($k+1) y respetan la whitelist.
func TestAuxLabelsForColumn_ConFiltros(t *testing.T) {
	drv := &recordedDriver{}
	drv.queryResponder = func(query string, args []driver.NamedValue) ([]string, [][]driver.Value, bool) {
		return []string{"id", "name"}, [][]driver.Value{{int64(1), "Guatemala"}}, true
	}
	db := openFakeDB(drv)
	defer db.Close()

	rows := []map[string]any{{"country_id": int64(1)}}
	src := &modules.DynamicSource{
		Table: "countries", IDColumn: "id", LabelColumn: "name",
		Filters: []modules.ListCondition{{Field: "active", Values: []string{"1"}}},
	}

	if _, err := auxLabelsForColumn(context.Background(), db, auxLookupWL(), src, rows, "country_id"); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	wantSQL := `SELECT "id", "name" FROM "countries" WHERE "id" IN ($1) AND lower(btrim("active"::text)) IN ($2) LIMIT 500`
	if drv.queries[0] != wantSQL {
		t.Fatalf("SQL inesperado:\n got %s\nwant %s", drv.queries[0], wantSQL)
	}
}

// TestAuxLabelsForColumn_WhitelistTabla: tabla fuera de lista ⇒ ErrWhitelist SIN
// ejecutar consulta.
func TestAuxLabelsForColumn_WhitelistTabla(t *testing.T) {
	drv := &recordedDriver{}
	db := openFakeDB(drv)
	defer db.Close()

	rows := []map[string]any{{"country_id": int64(1)}}
	src := &modules.DynamicSource{Table: "secreta", IDColumn: "id", LabelColumn: "name"}

	_, err := auxLabelsForColumn(context.Background(), db, auxLookupWL(), src, rows, "country_id")
	if err == nil || !errors.Is(err, ErrWhitelist) {
		t.Fatalf("se esperaba ErrWhitelist por tabla: %v", err)
	}
	if len(drv.queries) != 0 {
		t.Fatalf("no debe ejecutarse consulta ante ErrWhitelist, got %v", drv.queries)
	}
}

// TestAuxLabelsForColumn_WhitelistColumna: columna etiqueta fuera de lista ⇒
// ErrWhitelist SIN consulta.
func TestAuxLabelsForColumn_WhitelistColumna(t *testing.T) {
	drv := &recordedDriver{}
	db := openFakeDB(drv)
	defer db.Close()

	rows := []map[string]any{{"country_id": int64(1)}}
	src := &modules.DynamicSource{Table: "countries", IDColumn: "id", LabelColumn: "iso_code"}

	_, err := auxLabelsForColumn(context.Background(), db, auxLookupWL(), src, rows, "country_id")
	if err == nil || !errors.Is(err, ErrWhitelist) {
		t.Fatalf("se esperaba ErrWhitelist por columna etiqueta: %v", err)
	}
	if len(drv.queries) != 0 {
		t.Fatalf("no debe ejecutarse consulta ante ErrWhitelist, got %v", drv.queries)
	}
}

// TestAuxLabelsForColumn_IdSinFila: un id que no resuelve queda ausente del mapa
// (el llamador degrada mostrando el id crudo).
func TestAuxLabelsForColumn_IdSinFila(t *testing.T) {
	drv := &recordedDriver{}
	drv.queryResponder = func(query string, args []driver.NamedValue) ([]string, [][]driver.Value, bool) {
		// Solo resuelve el id 1; el 2 no viene en el resultado.
		return []string{"id", "name"}, [][]driver.Value{{int64(1), "Guatemala"}}, true
	}
	db := openFakeDB(drv)
	defer db.Close()

	rows := []map[string]any{{"country_id": int64(1)}, {"country_id": int64(2)}}
	src := &modules.DynamicSource{Table: "countries", IDColumn: "id", LabelColumn: "name"}

	labels, err := auxLabelsForColumn(context.Background(), db, auxLookupWL(), src, rows, "country_id")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if labels["1"] != "Guatemala" {
		t.Fatalf("el id 1 debía resolver a Guatemala: %v", labels)
	}
	if _, ok := labels["2"]; ok {
		t.Fatalf("el id 2 no debía resolver (degradación al id crudo): %v", labels)
	}
}

// TestResolveAuxLabels_EscribeClaveSeparada verifica, de extremo a extremo con
// el executor, que la etiqueta se escribe en la clave separada "<col>__label"
// (N=1) sin pisar el id crudo, y que un id sin resolver NO añade la clave.
func TestResolveAuxLabels_EscribeClaveSeparada(t *testing.T) {
	drv := &recordedDriver{}
	drv.queryResponder = func(query string, args []driver.NamedValue) ([]string, [][]driver.Value, bool) {
		if strings.Contains(query, "IN (") {
			return []string{"id", "name"}, [][]driver.Value{{int64(1), "Guatemala"}}, true
		}
		return nil, nil, false
	}
	e := newRecordedExecutor(drv)

	def := modules.Definition{
		Slug:         "qa-tabla-auxiliar",
		FetchMapping: map[string]any{"table": "merchant"},
		DisplayMapping: map[string]any{
			"columns": []any{
				map[string]any{
					"field": "country_id",
					"valueSource": map[string]any{
						"kind": "dynamic",
						"dynamicSource": map[string]any{
							"table": "countries", "idColumn": "id", "labelColumn": "name",
						},
					},
				},
			},
		},
		DataSource: modules.DataSource{
			SourceType:  modules.SourcePostgres,
			PGWhitelist: map[string][]string{"merchant": {"country_id"}, "countries": {"id", "name"}},
		},
	}
	spec := modules.MultiTableFromFetchMapping(def.FetchMapping)

	rows := []map[string]any{
		{"country_id": int64(1)},
		{"country_id": int64(9)}, // sin fila: no debe añadir __label
	}
	e.resolveAuxLabels(context.Background(), def, spec, rows)

	if rows[0]["country_id"] != int64(1) {
		t.Fatalf("el id crudo no debe pisarse: %v", rows[0]["country_id"])
	}
	if rows[0]["country_id__label"] != "Guatemala" {
		t.Fatalf("la etiqueta debe ir en country_id__label: %v", rows[0]["country_id__label"])
	}
	if _, ok := rows[1]["country_id__label"]; ok {
		t.Fatalf("un id sin resolver no debe añadir __label: %v", rows[1])
	}
}
