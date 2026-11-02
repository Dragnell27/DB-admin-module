package connections

import (
	"bytes"
	"errors"
	"testing"

	"admin-backend/internal/secrets"
)

// testSealer construye un Sealer con una clave de 32 bytes de prueba.
func testSealer(t *testing.T) *secrets.Sealer {
	t.Helper()
	key := bytes.Repeat([]byte{0x11}, secrets.KeySize)
	s, err := secrets.New(key)
	if err != nil {
		t.Fatalf("no se pudo construir el Sealer de prueba: %v", err)
	}
	return s
}

func TestValidateFields(t *testing.T) {
	cases := []struct {
		name    string
		fields  []TemplateField
		wantErr bool
	}{
		{"sin campos", nil, true},
		{"clave vacía", []TemplateField{{Key: "", Kind: FieldFixed}}, true},
		{"claves duplicadas", []TemplateField{
			{Key: "host", Kind: FieldFixed, Role: RoleHost},
			{Key: "host", Kind: FieldVariable, Role: RoleHost},
		}, true},
		{"role inválido", []TemplateField{{Key: "x", Kind: FieldFixed, Role: "foo"}}, true},
		{"marca inválida", []TemplateField{{Key: "x", Kind: "weird", Role: RoleHost}}, true},
		{"role vacío ok", []TemplateField{{Key: "x", Kind: FieldFixed, Role: ""}}, false},
		{"válido", []TemplateField{{Key: "dbname", Kind: FieldVariable, Role: RoleDBName}}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateFields(c.fields)
			if c.wantErr && !errors.Is(err, ErrValidation) {
				t.Errorf("se esperaba ErrValidation, got %v", err)
			}
			if !c.wantErr && err != nil {
				t.Errorf("no se esperaba error, got %v", err)
			}
		})
	}
}

func TestMergeFixedValues_Create(t *testing.T) {
	fields := []TemplateField{
		{Key: "host", Kind: FieldFixed, Role: RoleHost},
		{Key: "dbname", Kind: FieldVariable, Role: RoleDBName},
	}
	// FIJO vacío en creación -> ErrValidation.
	_, err := mergeFixedValues(fields, nil, FixedValues{"host": ""})
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("FIJO vacío en creación debe fallar, got %v", err)
	}
	// FIJO con valor en creación -> ok.
	merged, err := mergeFixedValues(fields, nil, FixedValues{"host": "h"})
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if merged["host"] != "h" {
		t.Errorf("merged = %+v", merged)
	}
	// La clave VARIABLE no entra en el bundle fijo.
	if _, ok := merged["dbname"]; ok {
		t.Error("la clave variable no debe estar en el bundle fijo")
	}
}

func TestMergeFixedValues_Edit(t *testing.T) {
	fields := []TemplateField{
		{Key: "host", Kind: FieldFixed, Role: RoleHost},
		{Key: "password", Kind: FieldFixed, Role: RolePassword},
	}
	prev := FixedValues{"host": "h-old", "password": "p-old"}

	// FIJO existente vacío en edición -> conservar prev.
	merged, err := mergeFixedValues(fields, prev, FixedValues{"host": "", "password": "p-new"})
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if merged["host"] != "h-old" {
		t.Errorf("host debía conservarse, got %q", merged["host"])
	}
	if merged["password"] != "p-new" {
		t.Errorf("password debía reemplazarse, got %q", merged["password"])
	}

	// FIJO nuevo (no en prev) vacío en edición -> ErrValidation.
	fieldsWithNew := append(fields, TemplateField{Key: "user", Kind: FieldFixed, Role: RoleUser})
	_, err = mergeFixedValues(fieldsWithNew, prev, FixedValues{"host": "", "password": "", "user": ""})
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("FIJO nuevo vacío en edición debe fallar, got %v", err)
	}
}

func TestMergeFixedValues_DropsOrphanKeys(t *testing.T) {
	// Una clave presente en prev pero ya no declarada se descarta.
	fields := []TemplateField{
		{Key: "host", Kind: FieldFixed, Role: RoleHost},
	}
	prev := FixedValues{"host": "h", "obsoleto": "x"}
	merged, err := mergeFixedValues(fields, prev, FixedValues{})
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if _, ok := merged["obsoleto"]; ok {
		t.Error("la clave huérfana debía descartarse")
	}
	if merged["host"] != "h" {
		t.Errorf("host debía conservarse, got %q", merged["host"])
	}
}

func TestSealOpenFixed_RoundTrip(t *testing.T) {
	r := &gormRepository{sealer: testSealer(t)}
	fixed := FixedValues{"host": "10.0.0.5", "password": "s3cr3t"}
	sealed, err := r.sealFixed(fixed)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if len(sealed) == 0 {
		t.Fatal("el bundle sellado no debería estar vacío")
	}
	got, err := r.openFixed(sealed)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if got["host"] != "10.0.0.5" || got["password"] != "s3cr3t" {
		t.Errorf("round-trip inesperado: %+v", got)
	}
	// El bundle sellado nunca es igual al plaintext.
	if bytes.Contains(sealed, []byte("s3cr3t")) {
		t.Error("el secreto no debería aparecer en claro en el bundle sellado")
	}
}

func TestToTemplate_NeverLeaksFixedValues(t *testing.T) {
	r := &gormRepository{sealer: testSealer(t)}
	sealed, _ := r.sealFixed(FixedValues{"password": "s3cr3t"})
	row := templateRow{
		ID:         "id-1",
		Name:       "mod-x",
		Fields:     encodeFields([]TemplateField{{Key: "password", Kind: FieldFixed, Role: RolePassword}}),
		SecretsEnc: sealed,
	}
	tmpl := toTemplate(row)
	if !tmpl.HasFixedValues {
		t.Error("HasFixedValues debería ser true")
	}
	// La vista pública no tiene un campo para valores; solo estructura.
	for _, f := range tmpl.Fields {
		if f.Key == "password" && f.Suggested != "" {
			t.Error("la estructura pública no debe cargar valores fijos")
		}
	}
}
