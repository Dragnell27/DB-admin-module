package engine

import (
	"errors"
	"testing"

	"admin-backend/internal/modules"
)

// capture-from-catalog Required ausente ⇒ ErrValidation (presencia como dynamic).
func TestValidateInput_FillStrategyCaptureRequiredMissing(t *testing.T) {
	fields := []modules.FieldSpec{
		{Name: "valor_lista", Required: true, Source: modules.FieldSourceFillStrategy, Strategy: &modules.FillStrategy{
			Kind: modules.StrategyCaptureFromCatalog,
		}},
	}
	err := ValidateInput(fields, map[string]any{})
	if err == nil || !errors.Is(err, ErrValidation) {
		t.Fatalf("se esperaba ErrValidation por captura obligatoria ausente: %v", err)
	}
}

// capture-from-catalog presente ⇒ aceptado sin checkType.
func TestValidateInput_FillStrategyCapturePresent(t *testing.T) {
	fields := []modules.FieldSpec{
		{Name: "valor_lista", Required: true, Source: modules.FieldSourceFillStrategy, Strategy: &modules.FillStrategy{
			Kind: modules.StrategyCaptureFromCatalog,
		}},
	}
	if err := ValidateInput(fields, map[string]any{"valor_lista": int64(55)}); err != nil {
		t.Fatalf("una captura presente no debe fallar: %v", err)
	}
}

// derive-* NO se validan contra el input aunque estén marcados Required.
func TestValidateInput_FillStrategyDeriveSkipped(t *testing.T) {
	fields := []modules.FieldSpec{
		{Name: "list_numero_h", Required: true, Source: modules.FieldSourceFillStrategy, Strategy: &modules.FillStrategy{
			Kind: modules.StrategyDeriveFromOption, SourceCaptureKey: "medio_pago", Attribute: "list_numero",
		}},
		{Name: "padr_numero", Required: true, Source: modules.FieldSourceFillStrategy, Strategy: &modules.FillStrategy{
			Kind: modules.StrategyDeriveViaFK,
		}},
	}
	if err := ValidateInput(fields, map[string]any{}); err != nil {
		t.Fatalf("las derivaciones no deben validarse contra el input: %v", err)
	}
}
