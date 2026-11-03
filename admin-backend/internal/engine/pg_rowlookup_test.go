package engine

import (
	"errors"
	"testing"
)

func lookupWL() map[string][]string {
	return map[string][]string{
		"gt_valores_padre": {"padr_numero", "depe_numero", "valor_objeto"},
	}
}

// SQL exacto: columnas en el orden de selectColumns, $1 en el WHERE, LIMIT 2.
// El builder ya NO devuelve args (el valor local lo liga el llamador): firma
// (string, error).
func TestBuildRowLookupSQL_Exact(t *testing.T) {
	q, err := buildRowLookupSQL(lookupWL(), "gt_valores_padre", "valor_objeto", []string{"padr_numero", "depe_numero"})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	want := `SELECT "padr_numero", "depe_numero" FROM "gt_valores_padre" WHERE "valor_objeto" = $1 LIMIT 2`
	if q != want {
		t.Fatalf("SQL inesperado:\n got %s\nwant %s", q, want)
	}
}

// ErrWhitelist por tabla fuera de lista.
func TestBuildRowLookupSQL_TableNotWhitelisted(t *testing.T) {
	_, err := buildRowLookupSQL(lookupWL(), "otra_tabla", "valor_objeto", []string{"padr_numero"})
	if err == nil || !errors.Is(err, ErrWhitelist) {
		t.Fatalf("se esperaba ErrWhitelist por tabla: %v", err)
	}
}

// ErrWhitelist por keyColumn fuera de lista.
func TestBuildRowLookupSQL_KeyColumnNotWhitelisted(t *testing.T) {
	_, err := buildRowLookupSQL(lookupWL(), "gt_valores_padre", "columna_mala", []string{"padr_numero"})
	if err == nil || !errors.Is(err, ErrWhitelist) {
		t.Fatalf("se esperaba ErrWhitelist por keyColumn: %v", err)
	}
}

// ErrWhitelist por columna de selectColumns fuera de lista.
func TestBuildRowLookupSQL_SelectColumnNotWhitelisted(t *testing.T) {
	_, err := buildRowLookupSQL(lookupWL(), "gt_valores_padre", "valor_objeto", []string{"padr_numero", "columna_mala"})
	if err == nil || !errors.Is(err, ErrWhitelist) {
		t.Fatalf("se esperaba ErrWhitelist por columna de selectColumns: %v", err)
	}
}

// ErrValidation por selectColumns vacío (sin producir SQL).
func TestBuildRowLookupSQL_EmptySelectColumns(t *testing.T) {
	_, err := buildRowLookupSQL(lookupWL(), "gt_valores_padre", "valor_objeto", nil)
	if err == nil || !errors.Is(err, ErrValidation) {
		t.Fatalf("se esperaba ErrValidation por selectColumns vacío: %v", err)
	}
}
