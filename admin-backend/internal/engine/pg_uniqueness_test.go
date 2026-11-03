package engine

import (
	"context"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"admin-backend/internal/modules"
)

// uniqWL es la whitelist de la tabla objetivo del pre-check de unicidad.
func uniqWL() map[string][]string {
	return map[string][]string{
		"gt_valores_hijo": {"hijo_numero", "valor_lista", "padr_numero", "valor_objeto"},
	}
}

// --- Builder buildUniquenessSQL --------------------------------------------

// Columnas con valor ⇒ "col = $N"; numeración $N en orden de columnas.
func TestBuildUniquenessSQL_AllValues(t *testing.T) {
	input := map[string]any{"padr_numero": int64(999), "valor_lista": "MP01"}
	sql, args, err := buildUniquenessSQL(uniqWL(), "gt_valores_hijo", []string{"padr_numero", "valor_lista"}, input)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	want := `SELECT 1 FROM "gt_valores_hijo" WHERE "padr_numero" = $1 AND "valor_lista" = $2 LIMIT 1`
	if sql != want {
		t.Fatalf("SQL inesperado:\n got %q\nwant %q", sql, want)
	}
	if len(args) != 2 || args[0] != int64(999) || args[1] != "MP01" {
		t.Fatalf("args inesperados: %#v", args)
	}
}

// Columna ausente en el input ⇒ "col IS NOT DISTINCT FROM $N" con NULL ligado.
func TestBuildUniquenessSQL_AbsentColumnIsNotDistinct(t *testing.T) {
	input := map[string]any{"padr_numero": int64(999), "valor_lista": "MP01"}
	// valor_objeto NO está en el input ⇒ se trata como NULL.
	sql, args, err := buildUniquenessSQL(uniqWL(), "gt_valores_hijo", []string{"padr_numero", "valor_lista", "valor_objeto"}, input)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	want := `SELECT 1 FROM "gt_valores_hijo" WHERE "padr_numero" = $1 AND "valor_lista" = $2 AND "valor_objeto" IS NOT DISTINCT FROM $3 LIMIT 1`
	if sql != want {
		t.Fatalf("SQL inesperado:\n got %q\nwant %q", sql, want)
	}
	if len(args) != 3 || args[2] != nil {
		t.Fatalf("el tercer arg debe ser NULL: %#v", args)
	}
}

// Columna presente con valor nil ⇒ también "IS NOT DISTINCT FROM".
func TestBuildUniquenessSQL_NilValueIsNotDistinct(t *testing.T) {
	input := map[string]any{"padr_numero": int64(999), "valor_objeto": nil}
	sql, args, err := buildUniquenessSQL(uniqWL(), "gt_valores_hijo", []string{"padr_numero", "valor_objeto"}, input)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	want := `SELECT 1 FROM "gt_valores_hijo" WHERE "padr_numero" = $1 AND "valor_objeto" IS NOT DISTINCT FROM $2 LIMIT 1`
	if sql != want {
		t.Fatalf("SQL inesperado:\n got %q\nwant %q", sql, want)
	}
	if len(args) != 2 || args[1] != nil {
		t.Fatalf("el segundo arg debe ser NULL: %#v", args)
	}
}

// Columna fuera de la whitelist ⇒ ErrWhitelist sin producir SQL.
func TestBuildUniquenessSQL_ColumnOutOfWhitelist(t *testing.T) {
	_, _, err := buildUniquenessSQL(uniqWL(), "gt_valores_hijo", []string{"columna_mala"}, map[string]any{"columna_mala": 1})
	if !errors.Is(err, ErrWhitelist) {
		t.Fatalf("se esperaba ErrWhitelist, got %v", err)
	}
}

// Tabla fuera de la whitelist ⇒ ErrWhitelist.
func TestBuildUniquenessSQL_TableOutOfWhitelist(t *testing.T) {
	_, _, err := buildUniquenessSQL(uniqWL(), "tabla_mala", []string{"padr_numero"}, nil)
	if !errors.Is(err, ErrWhitelist) {
		t.Fatalf("se esperaba ErrWhitelist, got %v", err)
	}
}

// Sin columnas ⇒ ErrValidation.
func TestBuildUniquenessSQL_NoColumns(t *testing.T) {
	_, _, err := buildUniquenessSQL(uniqWL(), "gt_valores_hijo", nil, nil)
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("se esperaba ErrValidation, got %v", err)
	}
}

// --- Pre-check checkUniqueness sobre un *sql.Tx real del fake driver --------

func uniqAction() modules.Action {
	return modules.Action{
		ActionKey:   "activar",
		Kind:        modules.ActionKindCreate,
		SQLOp:       "INSERT",
		TargetTable: "gt_valores_hijo",
		Uniqueness: &modules.UniquenessCheck{
			Columns: []string{"padr_numero", "valor_lista"},
			Message: "Ese medio de pago ya está activo para el comercio",
		},
	}
}

// Existe una fila ⇒ ErrUniqueness (no se inserta; el llamador revierte).
func TestCheckUniqueness_Exists(t *testing.T) {
	drv := &recordedDriver{}
	drv.queryResponder = func(query string, args []driver.NamedValue) ([]string, [][]driver.Value, bool) {
		if strings.Contains(query, `FROM "gt_valores_hijo"`) {
			return []string{"?column?"}, [][]driver.Value{{int64(1)}}, true
		}
		return nil, nil, false
	}
	tx, done := openFakeTx(t, drv)
	defer done()

	input := map[string]any{"padr_numero": int64(999), "valor_lista": "MP01"}
	err := checkUniqueness(context.Background(), tx, uniqWL(), uniqAction(), input)
	if !errors.Is(err, ErrUniqueness) {
		t.Fatalf("se esperaba ErrUniqueness, got %v", err)
	}
	if !strings.Contains(err.Error(), "Ese medio de pago ya está activo para el comercio") {
		t.Fatalf("el error debe llevar el mensaje de negocio: %v", err)
	}
}

// No existe fila ⇒ nil (se procede al INSERT).
func TestCheckUniqueness_NotExists(t *testing.T) {
	drv := &recordedDriver{}
	drv.queryResponder = func(query string, args []driver.NamedValue) ([]string, [][]driver.Value, bool) {
		if strings.Contains(query, `FROM "gt_valores_hijo"`) {
			return []string{"?column?"}, [][]driver.Value{}, true // 0 filas
		}
		return nil, nil, false
	}
	tx, done := openFakeTx(t, drv)
	defer done()

	input := map[string]any{"padr_numero": int64(999), "valor_lista": "MP01"}
	if err := checkUniqueness(context.Background(), tx, uniqWL(), uniqAction(), input); err != nil {
		t.Fatalf("sin fila existente no debe haber error: %v", err)
	}
}

// Columna ausente (valor_objeto NULL) + no existe fila ⇒ nil, y el SELECT usa
// IS NOT DISTINCT FROM para la columna sin valor.
func TestCheckUniqueness_AbsentColumnNoRow(t *testing.T) {
	drv := &recordedDriver{}
	var seen string
	drv.queryResponder = func(query string, args []driver.NamedValue) ([]string, [][]driver.Value, bool) {
		if strings.Contains(query, `FROM "gt_valores_hijo"`) {
			seen = query
			return []string{"?column?"}, [][]driver.Value{}, true
		}
		return nil, nil, false
	}
	tx, done := openFakeTx(t, drv)
	defer done()

	act := uniqAction()
	act.Uniqueness.Columns = []string{"padr_numero", "valor_lista", "valor_objeto"}
	input := map[string]any{"padr_numero": int64(999), "valor_lista": "MP01"} // valor_objeto ausente
	if err := checkUniqueness(context.Background(), tx, uniqWL(), act, input); err != nil {
		t.Fatalf("sin fila existente no debe haber error: %v", err)
	}
	if !strings.Contains(seen, `"valor_objeto" IS NOT DISTINCT FROM`) {
		t.Fatalf("la columna ausente debe usar IS NOT DISTINCT FROM: %q", seen)
	}
}

// act.Uniqueness == nil ⇒ no-op (ni SELECT ni error).
func TestCheckUniqueness_NilUniqueness(t *testing.T) {
	drv := &recordedDriver{}
	tx, done := openFakeTx(t, drv)
	defer done()
	act := uniqAction()
	act.Uniqueness = nil
	if err := checkUniqueness(context.Background(), tx, uniqWL(), act, map[string]any{}); err != nil {
		t.Fatalf("sin unicidad declarada no debe haber error: %v", err)
	}
	if len(drv.queries) != 0 {
		t.Fatalf("no debe emitirse ningún SELECT: %v", drv.queries)
	}
}

// --- isUniqueViolation ------------------------------------------------------

func TestIsUniqueViolation(t *testing.T) {
	if !isUniqueViolation(&pgconn.PgError{Code: "23505"}) {
		t.Fatalf("23505 debe reconocerse como violación de unicidad")
	}
	// Envuelto con %w también se reconoce.
	wrapped := errors.Join(errors.New("contexto"), &pgconn.PgError{Code: "23505"})
	if !isUniqueViolation(wrapped) {
		t.Fatalf("23505 envuelto debe reconocerse")
	}
	if isUniqueViolation(&pgconn.PgError{Code: "23503"}) {
		t.Fatalf("otro SQLSTATE NO debe reconocerse como unicidad")
	}
	if isUniqueViolation(errors.New("error cualquiera")) {
		t.Fatalf("un error no-pg NO debe reconocerse como unicidad")
	}
}

// --- uniquenessMessage ------------------------------------------------------

func TestUniquenessMessage(t *testing.T) {
	if got := uniquenessMessage(&modules.UniquenessCheck{Message: "mi mensaje"}); got != "mi mensaje" {
		t.Fatalf("mensaje del admin inesperado: %q", got)
	}
	if got := uniquenessMessage(&modules.UniquenessCheck{}); got == "" {
		t.Fatalf("mensaje vacío debe caer a un genérico no vacío")
	}
	if got := uniquenessMessage(nil); got == "" {
		t.Fatalf("nil debe caer a un genérico no vacío")
	}
}
