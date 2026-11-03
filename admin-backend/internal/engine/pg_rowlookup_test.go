package engine

import (
	"errors"
	"reflect"
	"testing"

	"admin-backend/internal/modules"
)

func lookupWL() map[string][]string {
	return map[string][]string{
		"gt_valores_padre": {"padr_numero", "depe_numero", "valor_objeto"},
		"gt_valores":       {"valo_numero", "list_numero", "codigo", "valor_es"},
	}
}

// SQL exacto SIN filtros: columnas en el orden de selectColumns, $1 en el WHERE,
// LIMIT 2. Con filters nil los args devueltos son nil (camino del salto
// derive-via-fk).
func TestBuildRowLookupSQL_Exact(t *testing.T) {
	q, args, err := buildRowLookupSQL(lookupWL(), "gt_valores_padre", "valor_objeto", []string{"padr_numero", "depe_numero"}, nil)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	want := `SELECT "padr_numero", "depe_numero" FROM "gt_valores_padre" WHERE "valor_objeto" = $1 LIMIT 2`
	if q != want {
		t.Fatalf("SQL inesperado:\n got %s\nwant %s", q, want)
	}
	if args != nil {
		t.Fatalf("sin filtros no debe devolver args: %v", args)
	}
}

// SQL exacto CON filtros del catálogo: el WHERE lleva la clave en $1 y el filtro
// (lower(btrim(...)) IN (...)) numerado DESPUÉS ($2); los args devueltos son solo
// los del filtro (el valor de la clave lo liga el llamador como $1). Usa como
// referencia el formato del WHERE de buildOptionsSQL (pg_options_test.go).
func TestBuildRowLookupSQL_WithCatalogFilters(t *testing.T) {
	q, args, err := buildRowLookupSQL(lookupWL(), "gt_valores", "valo_numero",
		[]string{"codigo", "list_numero", "valo_numero", "valor_es"},
		[]modules.ListCondition{{Field: "list_numero", Values: []string{"10001"}}},
	)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	want := `SELECT "codigo", "list_numero", "valo_numero", "valor_es" FROM "gt_valores" WHERE "valo_numero" = $1 AND lower(btrim("list_numero"::text)) IN ($2) LIMIT 2`
	if q != want {
		t.Fatalf("SQL inesperado:\n got %s\nwant %s", q, want)
	}
	if !reflect.DeepEqual(args, []any{"10001"}) {
		t.Fatalf("args del filtro inesperados: %v", args)
	}
}

// ErrWhitelist por columna de filtro fuera de lista blanca ⇒ SIN producir SQL.
func TestBuildRowLookupSQL_FilterColumnNotWhitelisted(t *testing.T) {
	q, args, err := buildRowLookupSQL(lookupWL(), "gt_valores", "valo_numero",
		[]string{"codigo"},
		[]modules.ListCondition{{Field: "columna_mala", Values: []string{"10001"}}},
	)
	if err == nil || !errors.Is(err, ErrWhitelist) {
		t.Fatalf("se esperaba ErrWhitelist por columna de filtro: %v", err)
	}
	if q != "" || args != nil {
		t.Fatalf("en error no debe producirse SQL: sql=%q args=%v", q, args)
	}
}

// ErrWhitelist por tabla fuera de lista.
func TestBuildRowLookupSQL_TableNotWhitelisted(t *testing.T) {
	_, _, err := buildRowLookupSQL(lookupWL(), "otra_tabla", "valor_objeto", []string{"padr_numero"}, nil)
	if err == nil || !errors.Is(err, ErrWhitelist) {
		t.Fatalf("se esperaba ErrWhitelist por tabla: %v", err)
	}
}

// ErrWhitelist por keyColumn fuera de lista.
func TestBuildRowLookupSQL_KeyColumnNotWhitelisted(t *testing.T) {
	_, _, err := buildRowLookupSQL(lookupWL(), "gt_valores_padre", "columna_mala", []string{"padr_numero"}, nil)
	if err == nil || !errors.Is(err, ErrWhitelist) {
		t.Fatalf("se esperaba ErrWhitelist por keyColumn: %v", err)
	}
}

// ErrWhitelist por columna de selectColumns fuera de lista.
func TestBuildRowLookupSQL_SelectColumnNotWhitelisted(t *testing.T) {
	_, _, err := buildRowLookupSQL(lookupWL(), "gt_valores_padre", "valor_objeto", []string{"padr_numero", "columna_mala"}, nil)
	if err == nil || !errors.Is(err, ErrWhitelist) {
		t.Fatalf("se esperaba ErrWhitelist por columna de selectColumns: %v", err)
	}
}

// ErrValidation por selectColumns vacío (sin producir SQL).
func TestBuildRowLookupSQL_EmptySelectColumns(t *testing.T) {
	_, _, err := buildRowLookupSQL(lookupWL(), "gt_valores_padre", "valor_objeto", nil, nil)
	if err == nil || !errors.Is(err, ErrValidation) {
		t.Fatalf("se esperaba ErrValidation por selectColumns vacío: %v", err)
	}
}
