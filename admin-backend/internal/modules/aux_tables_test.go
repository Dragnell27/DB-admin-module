package modules

import (
	"errors"
	"reflect"
	"testing"
)

// auxBaseDef arma una definición N=1 VÁLIDA cuya columna `pais_id` resuelve su
// etiqueta contra una TABLA AUXILIAR (countries) vía valueSource.dynamic. La
// auxiliar NO está relacionada con la principal ni aparece en el grid: solo
// aporta su whitelist. La principal declara su whitelist SIN las columnas de la
// auxiliar, para comprobar que la fusión las añade.
func auxBaseDef() *Definition {
	return &Definition{
		Slug:       "qa-tabla-auxiliar",
		Name:       "QA tabla auxiliar",
		SourceType: SourcePostgres,
		FetchMapping: map[string]any{
			"table": "merchant",
			"auxTables": []any{
				map[string]any{"table": "countries", "columns": []any{"id", "name"}},
			},
		},
		DisplayMapping: map[string]any{
			"columns": []any{
				map[string]any{"field": "uuid", "isPrimaryKey": true, "order": 1},
				map[string]any{
					"field": "country_id",
					"order": 2,
					"valueSource": map[string]any{
						"kind": "dynamic",
						"dynamicSource": map[string]any{
							"table":       "countries",
							"idColumn":    "id",
							"labelColumn": "name",
						},
					},
				},
			},
		},
		DataSource: DataSource{
			SourceType: SourcePostgres,
			PGWhitelist: map[string][]string{
				"merchant": {"uuid", "merchant_name", "country_id"},
			},
			ConnectionTemplateID: "tpl-1",
		},
	}
}

// Una tabla auxiliar NO relacionada entra a la whitelist efectiva y NO dispara
// el grafo conexo ("no está enlazada"); una columna con valueSource.dynamic a
// esa auxiliar valida OK.
func TestValidateDefinition_AuxTableWhitelistedSinRelacion(t *testing.T) {
	def := auxBaseDef()
	if err := validateDefinition(def); err != nil {
		t.Fatalf("definición con tabla auxiliar no debe fallar: %v", err)
	}
	// La fusión debió añadir la auxiliar a la whitelist efectiva.
	got := def.DataSource.PGWhitelist["countries"]
	if !reflect.DeepEqual(got, []string{"id", "name"}) {
		t.Fatalf("la whitelist efectiva debe incluir la auxiliar countries{id,name}, got %v", got)
	}
	// La auxiliar NO entra a Tables() (no es tabla del árbol).
	spec := MultiTableFromFetchMapping(def.FetchMapping)
	for _, tbl := range spec.Tables() {
		if tbl == "countries" {
			t.Fatalf("la tabla auxiliar countries NO debe aparecer en Tables(): %v", spec.Tables())
		}
	}
	if spec.IsMultiTable() {
		t.Fatalf("una definición con solo auxiliares NO debe ser multi-tabla")
	}
}

// Un catálogo dinámico que referencia una columna FUERA de la whitelist de la
// auxiliar sigue siendo rechazado.
func TestValidateDefinition_AuxColumnOutOfWhitelist(t *testing.T) {
	def := auxBaseDef()
	// labelColumn apunta a una columna NO declarada en auxTables.columns.
	cols := def.DisplayMapping["columns"].([]any)
	vs := cols[1].(map[string]any)["valueSource"].(map[string]any)
	vs["dynamicSource"].(map[string]any)["labelColumn"] = "iso_code"
	err := validateDefinition(def)
	if err == nil || !errors.Is(err, ErrMultiTableValidation) {
		t.Fatalf("se esperaba ErrMultiTableValidation por columna fuera de la whitelist de la auxiliar, got %v", err)
	}
}

// Guarda: una tabla auxiliar con columns vacío ⇒ ErrMultiTableValidation.
func TestValidateDefinition_AuxTableSinColumnas(t *testing.T) {
	def := auxBaseDef()
	def.FetchMapping["auxTables"] = []any{
		map[string]any{"table": "countries", "columns": []any{}},
	}
	err := validateDefinition(def)
	if err == nil || !errors.Is(err, ErrMultiTableValidation) {
		t.Fatalf("se esperaba ErrMultiTableValidation por auxiliar sin columnas, got %v", err)
	}
}

// Guarda: una tabla auxiliar cuyo nombre choca con la principal ⇒ rechazo.
func TestValidateDefinition_AuxTableChocaConPrincipal(t *testing.T) {
	def := auxBaseDef()
	def.FetchMapping["auxTables"] = []any{
		map[string]any{"table": "merchant", "columns": []any{"uuid"}},
	}
	err := validateDefinition(def)
	if err == nil || !errors.Is(err, ErrMultiTableValidation) {
		t.Fatalf("se esperaba ErrMultiTableValidation por choque de nombre con la principal, got %v", err)
	}
}

// AuxWhitelist funde columnas sin duplicar y omite tablas con nombre vacío.
func TestAuxWhitelist_UnionSinDuplicados(t *testing.T) {
	spec := MultiTableFromFetchMapping(map[string]any{
		"table": "merchant",
		"auxTables": []any{
			map[string]any{"table": "countries", "columns": []any{"id", "name"}},
			map[string]any{"table": "countries", "columns": []any{"name", "iso_code"}},
			map[string]any{"table": "", "columns": []any{"x"}},
		},
	})
	got := spec.AuxWhitelist()
	want := map[string][]string{"countries": {"id", "name", "iso_code"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("AuxWhitelist\n got %v\nwant %v", got, want)
	}
}
