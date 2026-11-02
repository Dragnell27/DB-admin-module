package engine

import (
	"encoding/json"
	"errors"
	"testing"

	"admin-backend/internal/modules"
)

// makeSelectField arma un FieldSpec de tipo "select" con las opciones
// Guatemala/El Salvador, reutilizado por los casos de validación del select.
func makeSelectField(defaultValue string, required bool) modules.FieldSpec {
	return modules.FieldSpec{
		Name:     "country",
		Type:     "string",
		Required: required,
		Source:   "select",
		Options: []modules.SelectOption{
			{Label: "Guatemala", Value: "gt"},
			{Label: "El Salvador", Value: "sv"},
		},
		DefaultValue: defaultValue,
	}
}

// TestValidateInputSaltaConstante verifica que ValidateInput NO falla cuando un
// campo Required tiene Source=="constant" y no está presente en el input: su
// valor es literal y no se valida contra el input.
func TestValidateInputSaltaConstante(t *testing.T) {
	fields := []modules.FieldSpec{
		{Name: "country", Type: "string", Required: true, Source: "constant", ConstValue: "gt"},
	}
	if err := ValidateInput(fields, map[string]any{}); err != nil {
		t.Fatalf("ValidateInput no debía fallar para un campo constante Required: %v", err)
	}
}

// TestValidateInputSelectObligatorioSinDefault: select Required sin default y
// sin input → error; con default → ok.
func TestValidateInputSelectObligatorioSinDefault(t *testing.T) {
	sinDefault := []modules.FieldSpec{makeSelectField("", true)}
	if err := ValidateInput(sinDefault, map[string]any{}); err == nil {
		t.Fatalf("ValidateInput debía fallar para un select obligatorio sin default ni input")
	}

	conDefault := []modules.FieldSpec{makeSelectField("gt", true)}
	if err := ValidateInput(conDefault, map[string]any{}); err != nil {
		t.Fatalf("ValidateInput no debía fallar para un select obligatorio con default: %v", err)
	}
}

// TestValidateInputSelectValorFueraDeOpciones: un valor que no está en Options
// es rechazado; un valor de la lista es aceptado.
func TestValidateInputSelectValorFueraDeOpciones(t *testing.T) {
	fields := []modules.FieldSpec{makeSelectField("", true)}

	if err := ValidateInput(fields, map[string]any{"country": "mx"}); err == nil {
		t.Fatalf("ValidateInput debía rechazar un valor fuera de Options")
	}
	if err := ValidateInput(fields, map[string]any{"country": "sv"}); err != nil {
		t.Fatalf("ValidateInput no debía fallar para un valor de Options: %v", err)
	}
}

// TestValidateInputKeyPassthroughAceptaNoString fija el acoplamiento por literal
// entre el paquete modules (sentinel FieldTypeKeyPassthrough) y engine.checkType:
// bajo ese tipo, ValidateInput solo valida PRESENCIA y NO la forma del valor, de
// modo que una PK tomada verbatim de la fila (float64 numérico, string o una
// fecha no RFC3339/YYYY-MM-DD) se acepta; ausente se rechaza (HIGH-1, R12.1/R12.2).
// Si alguien cambia el valor del sentinel o los case de checkType, este test lo
// detecta antes de que rompa editar/eliminar en silencio.
func TestValidateInputKeyPassthroughAceptaNoString(t *testing.T) {
	fields := []modules.FieldSpec{
		{Name: "id", Type: modules.FieldTypeKeyPassthrough, Required: true, Source: modules.FieldSourceRow},
	}

	aceptados := []struct {
		name  string
		input map[string]any
	}{
		{"float64 (PK numérica típica)", map[string]any{"id": float64(7)}},
		{"string arbitrario", map[string]any{"id": "abc-123"}},
		{"fecha no RFC3339 ni YYYY-MM-DD", map[string]any{"id": "31/12/2024"}},
	}
	for _, tc := range aceptados {
		t.Run("acepta "+tc.name, func(t *testing.T) {
			if err := ValidateInput(fields, tc.input); err != nil {
				t.Fatalf("ValidateInput debía aceptar %s bajo el sentinel: %v", tc.name, err)
			}
		})
	}

	t.Run("rechaza PK ausente", func(t *testing.T) {
		if err := ValidateInput(fields, map[string]any{}); err == nil {
			t.Fatalf("ValidateInput debía rechazar la PK ausente (valida presencia)")
		}
	})
}

// makeDynamicField arma un FieldSpec de tipo "dynamic" con un origen dinámico
// mínimo (tabla/id/etiqueta), reutilizado por los casos de validación dynamic.
func makeDynamicField(required bool) modules.FieldSpec {
	return modules.FieldSpec{
		Name:     "pais_id",
		Type:     "number",
		Required: required,
		Source:   modules.FieldSourceDynamic,
		DynamicSource: &modules.DynamicSource{
			Table:       "paises",
			IDColumn:    "id",
			LabelColumn: "nombre",
		},
	}
}

// TestValidateInputDynamicAceptaIDSinTypeErr verifica que la rama dynamic solo
// valida presencia: un id numérico (float64 de un JSON) o un id string se
// aceptan sin error de tipo, porque el id FK viaja como parámetro ligado.
func TestValidateInputDynamicAceptaIDSinTypeErr(t *testing.T) {
	fields := []modules.FieldSpec{makeDynamicField(true)}

	aceptados := []struct {
		name  string
		input map[string]any
	}{
		{"id numérico (float64)", map[string]any{"pais_id": float64(42)}},
		{"id string", map[string]any{"pais_id": "42"}},
	}
	for _, tc := range aceptados {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateInput(fields, tc.input); err != nil {
				t.Fatalf("ValidateInput debía aceptar %s en un campo dynamic: %v", tc.name, err)
			}
		})
	}
}

// TestValidateInputDynamicObligatorioAusente: un campo dynamic obligatorio
// ausente produce un error que envuelve ErrValidation.
func TestValidateInputDynamicObligatorioAusente(t *testing.T) {
	fields := []modules.FieldSpec{makeDynamicField(true)}
	err := ValidateInput(fields, map[string]any{})
	if err == nil {
		t.Fatalf("ValidateInput debía fallar para un campo dynamic obligatorio ausente")
	}
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("el error debe envolver ErrValidation, got %v", err)
	}
}

// TestValidateInputDynamicOpcionalAusente: un campo dynamic opcional ausente se
// acepta (se omite).
func TestValidateInputDynamicOpcionalAusente(t *testing.T) {
	fields := []modules.FieldSpec{makeDynamicField(false)}
	if err := ValidateInput(fields, map[string]any{}); err != nil {
		t.Fatalf("ValidateInput no debía fallar para un campo dynamic opcional ausente: %v", err)
	}
}

// TestEditFieldSpecsPropagaDynamic fija que EditFieldSpecs propaga un FieldSpec
// con Source=="dynamic" (de una acción edit) a la validación con su origen
// INTACTO: la columna editable declarada como dynamic se reusa tal cual, de modo
// que ValidateInput la valide por presencia (no por tipo).
func TestEditFieldSpecsPropagaDynamic(t *testing.T) {
	cols := []modules.ResolvedColumn{
		{Field: "id", IsPrimaryKey: true},
		{Field: "pais_id", Editable: true},
	}
	act := modules.Action{
		Kind: modules.ActionKindEdit,
		InputFields: []modules.FieldSpec{
			{
				Name:   "pais_id",
				Source: modules.FieldSourceDynamic,
				Type:   "number",
				DynamicSource: &modules.DynamicSource{
					Table: "paises", IDColumn: "id", LabelColumn: "nombre",
				},
			},
		},
	}

	specs := modules.EditFieldSpecs(cols, act, modules.MultiTableSpec{})

	var dyn *modules.FieldSpec
	for i := range specs {
		if specs[i].Name == "pais_id" {
			dyn = &specs[i]
			break
		}
	}
	if dyn == nil {
		t.Fatalf("EditFieldSpecs no propagó el campo dynamic pais_id: %+v", specs)
	}
	if dyn.Source != modules.FieldSourceDynamic {
		t.Fatalf("Source propagado = %q, want %q", dyn.Source, modules.FieldSourceDynamic)
	}
	if dyn.DynamicSource == nil || dyn.DynamicSource.Table != "paises" {
		t.Fatalf("DynamicSource no se propagó intacto: %+v", dyn.DynamicSource)
	}

	// La validación debe aceptar un id no-string (float64) por presencia.
	input := map[string]any{"id": float64(7), "pais_id": float64(3)}
	if err := ValidateInput(specs, input); err != nil {
		t.Fatalf("ValidateInput debía aceptar el id dynamic por presencia: %v", err)
	}
}

// TestFieldSpecRetrocompatibleSinSelect: un JSON viejo de input_fields (sin
// options/defaultValue) deserializa a FieldSpec con Options==nil y
// DefaultValue=="" (deserialización retrocompatible, sin migración).
func TestFieldSpecRetrocompatibleSinSelect(t *testing.T) {
	raw := []byte(`[{"name":"country","type":"string","required":true,"location":"header","source":"constant","constValue":"gt"}]`)
	var specs []modules.FieldSpec
	if err := json.Unmarshal(raw, &specs); err != nil {
		t.Fatalf("json.Unmarshal devolvió error: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("len(specs) = %d, want 1", len(specs))
	}
	if specs[0].Options != nil {
		t.Fatalf("Options = %v, want nil", specs[0].Options)
	}
	if specs[0].DefaultValue != "" {
		t.Fatalf("DefaultValue = %q, want \"\"", specs[0].DefaultValue)
	}
}
