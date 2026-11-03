package api

import (
	"testing"

	"admin-backend/internal/modules"
)

// findDynamicField resuelve un campo capture-from-catalog (incluido un
// __capture_*) buscando dentro de la acción indicada por ?action=, y expone su
// catálogo en Strategy.Catalog (no en DynamicSource).
func TestFindDynamicField_CaptureFromCatalog(t *testing.T) {
	actions := []modules.Action{
		{
			ActionKey: "activar",
			InputFields: []modules.FieldSpec{
				{Name: "__capture_comercio", Source: modules.FieldSourceFillStrategy, Strategy: &modules.FillStrategy{
					Kind:    modules.StrategyCaptureFromCatalog,
					Catalog: &modules.DynamicSource{Table: "cl_comercios_sapp", IDColumn: "come_numero", LabelColumn: "nombre"},
				}},
			},
		},
	}
	f, ok := findDynamicField(actions, "__capture_comercio", "activar")
	if !ok {
		t.Fatalf("se esperaba encontrar la captura __capture_comercio")
	}
	if f.DynamicSource != nil {
		t.Fatalf("una captura no lleva DynamicSource; el catálogo va en Strategy.Catalog")
	}
	if f.Strategy == nil || f.Strategy.Catalog == nil || f.Strategy.Catalog.Table != "cl_comercios_sapp" {
		t.Fatalf("el catálogo de la captura es inesperado: %+v", f.Strategy)
	}
}

// Sin ?action, una captura también se encuentra por nombre (primera coincidencia).
func TestFindDynamicField_CaptureWithoutAction(t *testing.T) {
	actions := []modules.Action{
		{ActionKey: "activar", InputFields: []modules.FieldSpec{
			{Name: "valor_lista", Source: modules.FieldSourceFillStrategy, Strategy: &modules.FillStrategy{
				Kind:    modules.StrategyCaptureFromCatalog,
				Catalog: &modules.DynamicSource{Table: "gt_valores", IDColumn: "valo_numero", LabelColumn: "valor_es"},
			}},
		}},
	}
	if _, ok := findDynamicField(actions, "valor_lista", ""); !ok {
		t.Fatalf("se esperaba encontrar la captura valor_lista sin ?action")
	}
}

// Una captura derive-* (sin Catalog) NO se considera dinámica ⇒ no se encuentra.
func TestFindDynamicField_DeriveNotFound(t *testing.T) {
	actions := []modules.Action{
		{ActionKey: "activar", InputFields: []modules.FieldSpec{
			{Name: "list_numero_h", Source: modules.FieldSourceFillStrategy, Strategy: &modules.FillStrategy{
				Kind: modules.StrategyDeriveFromOption,
			}},
		}},
	}
	if _, ok := findDynamicField(actions, "list_numero_h", "activar"); ok {
		t.Fatalf("una derivación no debe resolver como campo dinámico de opciones")
	}
}
