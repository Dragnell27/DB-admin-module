package modules

import (
	"errors"
	"testing"
)

// valueSourceDef arma un módulo N=1 válido (sin acciones edit/delete para no
// exigir PK por otros motivos) con una sola columna cuyo valueSource el caller
// inyecta. La whitelist declara la tabla principal 'planes' y la tabla de
// catálogo 'estados' (estas sí/no se incluyen según el test).
func valueSourceDef(vs map[string]any) *Definition {
	col := map[string]any{"field": "estado", "order": 1}
	if vs != nil {
		col["valueSource"] = vs
	}
	return &Definition{
		Slug:           "planes",
		Name:           "Planes",
		SourceType:     SourcePostgres,
		FetchMapping:   map[string]any{"table": "planes"},
		DisplayMapping: map[string]any{"columns": []any{col}},
		DataSource: DataSource{
			SourceType: SourcePostgres,
			PGWhitelist: map[string][]string{
				"planes":  {"estado"},
				"estados": {"id", "nombre"},
			},
			ConnectionTemplateID: "tpl-1",
		},
	}
}

// TestValidateDefinition_ValueSourceDynamicFueraWhitelist es la prueba EXPLÍCITA
// del fix N=1: una columna dinámica cuya tabla NO está en la whitelist se
// rechaza con un error que satisface errors.Is(err, ErrMultiTableValidation),
// aun en un módulo de una sola tabla (donde validateMultiTable es no-op).
func TestValidateDefinition_ValueSourceDynamicFueraWhitelist(t *testing.T) {
	def := valueSourceDef(map[string]any{
		"kind": "dynamic",
		"dynamicSource": map[string]any{
			"table":       "fantasma", // no está en la whitelist
			"idColumn":    "id",
			"labelColumn": "nombre",
		},
	})
	err := validateDefinition(def)
	if err == nil {
		t.Fatalf("se esperaba rechazo por tabla fuera de whitelist")
	}
	if !errors.Is(err, ErrMultiTableValidation) {
		t.Fatalf("el error debía satisfacer errors.Is(ErrMultiTableValidation), got %v", err)
	}
}

// TestValidateDefinition_ValueSourceDynamicColumnaFueraWhitelist cubre el caso de
// tabla whitelisteada pero con una columna id/label que no está en la whitelist
// de esa tabla.
func TestValidateDefinition_ValueSourceDynamicColumnaFueraWhitelist(t *testing.T) {
	def := valueSourceDef(map[string]any{
		"kind": "dynamic",
		"dynamicSource": map[string]any{
			"table":       "estados",
			"idColumn":    "id",
			"labelColumn": "descripcion", // no está en whitelist[estados]
		},
	})
	err := validateDefinition(def)
	if !errors.Is(err, ErrMultiTableValidation) {
		t.Fatalf("se esperaba ErrMultiTableValidation por columna fuera de whitelist, got %v", err)
	}
}

// TestValidateDefinition_ValueSourceDynamicIncompleto verifica que una fuente
// dinámica sin columna de id o de etiqueta se rechaza.
func TestValidateDefinition_ValueSourceDynamicIncompleto(t *testing.T) {
	def := valueSourceDef(map[string]any{
		"kind": "dynamic",
		"dynamicSource": map[string]any{
			"table":    "estados",
			"idColumn": "id",
			// labelColumn ausente
		},
	})
	err := validateDefinition(def)
	if !errors.Is(err, ErrMultiTableValidation) {
		t.Fatalf("se esperaba ErrMultiTableValidation por fuente incompleta, got %v", err)
	}
}

// TestValidateDefinition_ValueSourceDynamicDentroWhitelist verifica que una
// fuente dinámica con tabla/id/label whitelisteados pasa.
func TestValidateDefinition_ValueSourceDynamicDentroWhitelist(t *testing.T) {
	def := valueSourceDef(map[string]any{
		"kind": "dynamic",
		"dynamicSource": map[string]any{
			"table":       "estados",
			"idColumn":    "id",
			"labelColumn": "nombre",
		},
	})
	if err := validateDefinition(def); err != nil {
		t.Fatalf("una fuente dinámica whitelisteada no debía fallar: %v", err)
	}
}

// TestValidateDefinition_SinValueSourceRetrocompat verifica que una columna sin
// valueSource (definición histórica) pasa sin cambios (retrocompat).
func TestValidateDefinition_SinValueSourceRetrocompat(t *testing.T) {
	def := valueSourceDef(nil)
	if err := validateDefinition(def); err != nil {
		t.Fatalf("una columna sin valueSource no debía fallar: %v", err)
	}
}
