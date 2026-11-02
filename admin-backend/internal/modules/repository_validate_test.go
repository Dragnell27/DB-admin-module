package modules

import "testing"

// validDef arma una Definition mínima válida salvo por el tipo de origen, que
// el caller fija para ejercitar validateDefinition.
func validDef(sourceType string) *Definition {
	return &Definition{
		Slug:       "mi-modulo",
		Name:       "Mi Módulo",
		SourceType: sourceType,
	}
}

// TestValidateDefinitionAceptaPostgres verifica que una definición de origen
// PostgreSQL (el único soportado) pasa la validación.
func TestValidateDefinitionAceptaPostgres(t *testing.T) {
	if err := validateDefinition(validDef(SourcePostgres)); err != nil {
		t.Fatalf("validateDefinition rechazó un origen postgres válido: %v", err)
	}
}

// TestValidateDefinitionRechazaAPI verifica que una definición de origen "api"
// se rechaza: el origen API HTTP fue retirado del constructor de módulos.
func TestValidateDefinitionRechazaAPI(t *testing.T) {
	if err := validateDefinition(validDef("api")); err == nil {
		t.Fatalf("validateDefinition debía rechazar un origen api")
	}
}

// TestValidateDefinitionRechazaOrigenDesconocido verifica que un tipo de origen
// arbitrario también se rechaza.
func TestValidateDefinitionRechazaOrigenDesconocido(t *testing.T) {
	if err := validateDefinition(validDef("graphql")); err == nil {
		t.Fatalf("validateDefinition debía rechazar un origen desconocido")
	}
}
