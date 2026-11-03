package engine

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"admin-backend/internal/modules"
)

// fakeResult implementa sql.Result para los dobles.
type fakeResult struct{ affected int64 }

func (r fakeResult) LastInsertId() (int64, error) { return 0, nil }
func (r fakeResult) RowsAffected() (int64, error) { return r.affected, nil }

// execRecord captura una sentencia ejecutada por el doble.
type execRecord struct {
	query string
	args  []any
}

// fakeTx es un doble de txExecer que registra cada ExecContext y puede forzar un
// fallo en la n-ésima sentencia (failAt, 1-indexado; 0 = nunca falla).
type fakeTx struct {
	execs  []execRecord
	failAt int
}

func (f *fakeTx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	f.execs = append(f.execs, execRecord{query: query, args: args})
	if f.failAt != 0 && len(f.execs) == f.failAt {
		return nil, errors.New("fallo simulado de la sentencia")
	}
	return fakeResult{affected: 1}, nil
}

// QueryContext satisface la ampliación aditiva de txExecer (fase 1). Los tests
// de execMultiUpdate/execMultiDelete/execSingleInsert NO ejecutan SELECT, así
// que el stub devuelve un error: no puede fabricar *sql.Rows (tipo concreto). La
// resolución de estrategias se prueba con un *sql.Tx real sobre el fake driver.
func (f *fakeTx) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return nil, errors.New("no soportado")
}

// writeCols arma las columnas resueltas multi-tabla para pagos ← clientes.
func writeCols() []modules.ResolvedColumn {
	return []modules.ResolvedColumn{
		{Field: "id", IsPrimaryKey: true, SourceTable: "pagos"},
		{Field: "cliente_id", Editable: true, SourceTable: "pagos"},
		{Field: "monto", Editable: true, SourceTable: "pagos"},
		{Field: "id", IsPrimaryKey: true, SourceTable: "clientes"},
		{Field: "nombre", Editable: true, SourceTable: "clientes"},
	}
}

func writeWL() map[string][]string {
	return map[string][]string{
		"pagos":    {"id", "cliente_id", "monto"},
		"clientes": {"id", "nombre"},
	}
}

func writeSpec() modules.MultiTableSpec {
	return modules.MultiTableFromFetchMapping(map[string]any{
		"table":         "pagos",
		"foreignTables": []any{map[string]any{"table": "clientes"}},
		"relationships": []any{
			map[string]any{"childTable": "pagos", "parentTable": "clientes", "on": []any{map[string]any{"child": "cliente_id", "parent": "id"}}, "joinType": "inner"},
		},
	})
}

// --- Test 7: edit multi-tabla emite un UPDATE por tabla ---------------------

func TestExecMultiUpdate_PerTable(t *testing.T) {
	ctx := context.Background()
	tx := &fakeTx{}
	act := modules.Action{Kind: modules.ActionKindEdit, SQLOp: "UPDATE", InputFields: []modules.FieldSpec{
		{Name: "monto", SourceTable: "pagos", Source: modules.FieldSourceInput},
		{Name: "nombre", SourceTable: "clientes", Source: modules.FieldSourceInput},
	}}
	input := map[string]any{
		"pagos__id":    7,
		"clientes__id": 3,
		"monto":        100,
		"nombre":       "Ana",
	}
	n, err := execMultiUpdate(ctx, tx, writeWL(), writeCols(), writeSpec(), act, input)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if n != 2 {
		t.Fatalf("rowsAffected agregado: got %d want 2", n)
	}
	if len(tx.execs) != 2 {
		t.Fatalf("se esperaban 2 UPDATE, got %d: %+v", len(tx.execs), tx.execs)
	}
	// Orden estable alfabético: clientes antes que pagos.
	if !strings.HasPrefix(tx.execs[0].query, `UPDATE "clientes" SET "nombre"`) {
		t.Fatalf("primer UPDATE inesperado: %s", tx.execs[0].query)
	}
	if !strings.HasPrefix(tx.execs[1].query, `UPDATE "pagos" SET "monto"`) {
		t.Fatalf("segundo UPDATE inesperado: %s", tx.execs[1].query)
	}
	// La PK solo en el WHERE con el nombre REAL "id" (no el alias).
	for _, e := range tx.execs {
		if strings.Contains(e.query, "__") {
			t.Fatalf("el SQL no debe contener el alias: %s", e.query)
		}
		if !strings.Contains(e.query, `WHERE "id" = `) {
			t.Fatalf("WHERE por PK real esperado: %s", e.query)
		}
	}
}

// Test 3: el UPDATE nunca pone una PK en el SET (incluida PK homónima).
func TestExecMultiUpdate_PKNeverInSet(t *testing.T) {
	ctx := context.Background()
	tx := &fakeTx{}
	// La acción intenta colar "id" (PK) y una homónima: no deben entrar al SET.
	act := modules.Action{Kind: modules.ActionKindEdit, SQLOp: "UPDATE", InputFields: []modules.FieldSpec{
		{Name: "id", SourceTable: "pagos", Source: modules.FieldSourceInput},
		{Name: "monto", SourceTable: "pagos", Source: modules.FieldSourceInput},
	}}
	input := map[string]any{"pagos__id": 7, "id": 999, "monto": 100}
	_, err := execMultiUpdate(ctx, tx, writeWL(), writeCols(), writeSpec(), act, input)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if len(tx.execs) != 1 {
		t.Fatalf("se esperaba 1 UPDATE (solo pagos): %+v", tx.execs)
	}
	q := tx.execs[0].query
	if strings.Contains(q, `SET "id"`) {
		t.Fatalf("la PK no debe entrar en el SET: %s", q)
	}
	if q != `UPDATE "pagos" SET "monto" = $1 WHERE "id" = $2` {
		t.Fatalf("SQL inesperado: %s", q)
	}
}

// §5.2/R9.7: una tabla con setCols vacío se omite y su PK NO se exige.
func TestExecMultiUpdate_OmitsTableNoSetCols(t *testing.T) {
	ctx := context.Background()
	tx := &fakeTx{}
	act := modules.Action{Kind: modules.ActionKindEdit, SQLOp: "UPDATE", InputFields: []modules.FieldSpec{
		{Name: "monto", SourceTable: "pagos", Source: modules.FieldSourceInput},
		{Name: "nombre", SourceTable: "clientes", Source: modules.FieldSourceInput},
	}}
	// Solo viene "monto" (pagos). clientes NO tiene columnas en el input ⇒ se
	// omite y NO se exige su PK (clientes__id ausente).
	input := map[string]any{"pagos__id": 7, "monto": 100}
	_, err := execMultiUpdate(ctx, tx, writeWL(), writeCols(), writeSpec(), act, input)
	if err != nil {
		t.Fatalf("una tabla sin setCols debe omitirse sin exigir PK: %v", err)
	}
	if len(tx.execs) != 1 || !strings.Contains(tx.execs[0].query, `UPDATE "pagos"`) {
		t.Fatalf("se esperaba solo el UPDATE de pagos: %+v", tx.execs)
	}
}

// Test 1: DELETE emite un DELETE por tabla por PK en orden hijo→padre.
func TestExecMultiDelete_ChildToParentOrder(t *testing.T) {
	ctx := context.Background()
	tx := &fakeTx{}
	// DeleteTopTable=clientes ⇒ orden [pagos (hijo), clientes (padre)].
	act := modules.Action{Kind: modules.ActionKindDelete, SQLOp: "DELETE", DeleteTopTable: "clientes"}
	input := map[string]any{"pagos__id": 7, "clientes__id": 3}
	n, err := execMultiDelete(ctx, tx, writeWL(), writeCols(), writeSpec(), act, input)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if n != 2 {
		t.Fatalf("rowsAffected agregado: got %d want 2", n)
	}
	if len(tx.execs) != 2 {
		t.Fatalf("se esperaban 2 DELETE: %+v", tx.execs)
	}
	if tx.execs[0].query != `DELETE FROM "pagos" WHERE "id" = $1` {
		t.Fatalf("primer DELETE (hijo) inesperado: %s", tx.execs[0].query)
	}
	if tx.execs[1].query != `DELETE FROM "clientes" WHERE "id" = $1` {
		t.Fatalf("segundo DELETE (padre) inesperado: %s", tx.execs[1].query)
	}
	// S1: nunca JOIN/USING en un DELETE.
	for _, e := range tx.execs {
		if strings.Contains(strings.ToUpper(e.query), "JOIN") || strings.Contains(strings.ToUpper(e.query), "USING") {
			t.Fatalf("un DELETE no debe usar JOIN/USING: %s", e.query)
		}
	}
}

// Test 2: una PK ausente aborta el DELETE SIN ejecutar ninguna sentencia.
func TestExecMultiDelete_MissingPKAborts(t *testing.T) {
	ctx := context.Background()
	t.Run("falta la PK de una tabla ⇒ 0 sentencias", func(t *testing.T) {
		tx := &fakeTx{}
		act := modules.Action{Kind: modules.ActionKindDelete, SQLOp: "DELETE", DeleteTopTable: "clientes"}
		input := map[string]any{"pagos__id": 7} // falta clientes__id
		_, err := execMultiDelete(ctx, tx, writeWL(), writeCols(), writeSpec(), act, input)
		if err == nil {
			t.Fatalf("se esperaba abortar por PK ausente")
		}
		if len(tx.execs) != 0 {
			t.Fatalf("no debe ejecutarse NINGUNA sentencia: %+v", tx.execs)
		}
	})

	t.Run("PK homónima: no se reutiliza el valor de otra tabla", func(t *testing.T) {
		tx := &fakeTx{}
		act := modules.Action{Kind: modules.ActionKindDelete, SQLOp: "DELETE", DeleteTopTable: "clientes"}
		// Solo viene pagos__id; clientes__id ausente. El motor NO debe reutilizar
		// pagos__id para clientes (prohibido el fallback pelado).
		input := map[string]any{"pagos__id": 7, "id": 7}
		_, err := execMultiDelete(ctx, tx, writeWL(), writeCols(), writeSpec(), act, input)
		if err == nil {
			t.Fatalf("se esperaba abortar: clientes__id ausente")
		}
		if len(tx.execs) != 0 {
			t.Fatalf("0 sentencias esperadas: %+v", tx.execs)
		}
	})

	t.Run("valor de PK vacío tras TrimSpace ⇒ aborta", func(t *testing.T) {
		tx := &fakeTx{}
		act := modules.Action{Kind: modules.ActionKindDelete, SQLOp: "DELETE", DeleteTopTable: "clientes"}
		input := map[string]any{"pagos__id": 7, "clientes__id": "  "}
		_, err := execMultiDelete(ctx, tx, writeWL(), writeCols(), writeSpec(), act, input)
		if err == nil || len(tx.execs) != 0 {
			t.Fatalf("PK vacía debe abortar sin ejecutar: err=%v execs=%+v", err, tx.execs)
		}
	})

	t.Run("valor de PK 0 es válido (no aborta)", func(t *testing.T) {
		tx := &fakeTx{}
		act := modules.Action{Kind: modules.ActionKindDelete, SQLOp: "DELETE", DeleteTopTable: "clientes"}
		input := map[string]any{"pagos__id": 0, "clientes__id": 0}
		_, err := execMultiDelete(ctx, tx, writeWL(), writeCols(), writeSpec(), act, input)
		if err != nil {
			t.Fatalf("un id 0 es válido: %v", err)
		}
		if len(tx.execs) != 2 {
			t.Fatalf("se esperaban 2 DELETE: %+v", tx.execs)
		}
	})
}

// S5: fallo en la 2ª sentencia ⇒ el error se propaga (el dispatcher hace ROLLBACK).
func TestExecMultiDelete_FailsSecondStatement(t *testing.T) {
	ctx := context.Background()
	tx := &fakeTx{failAt: 2}
	act := modules.Action{Kind: modules.ActionKindDelete, SQLOp: "DELETE", DeleteTopTable: "clientes"}
	input := map[string]any{"pagos__id": 7, "clientes__id": 3}
	_, err := execMultiDelete(ctx, tx, writeWL(), writeCols(), writeSpec(), act, input)
	if err == nil {
		t.Fatalf("se esperaba error en la 2ª sentencia")
	}
	// La 1ª sí se intentó (es responsabilidad del dispatcher revertir la tx).
	if len(tx.execs) != 2 {
		t.Fatalf("se esperaban 2 intentos: %+v", tx.execs)
	}
}

// S7: PK fuera de la whitelist ⇒ ErrWhitelist, 0 sentencias.
func TestExecMultiDelete_PKNotWhitelisted(t *testing.T) {
	ctx := context.Background()
	tx := &fakeTx{}
	wl := writeWL()
	wl["clientes"] = []string{"nombre"} // "id" (PK) fuera de la whitelist
	act := modules.Action{Kind: modules.ActionKindDelete, SQLOp: "DELETE", DeleteTopTable: "clientes"}
	input := map[string]any{"pagos__id": 7, "clientes__id": 3}
	_, err := execMultiDelete(ctx, tx, wl, writeCols(), writeSpec(), act, input)
	if err == nil || !errors.Is(err, ErrWhitelist) {
		t.Fatalf("se esperaba ErrWhitelist: %v", err)
	}
	if len(tx.execs) != 0 {
		t.Fatalf("0 sentencias esperadas: %+v", tx.execs)
	}
}

// §5.4: INSERT a la tabla objetivo elegida, sin JOIN.
func TestExecSingleInsert_TargetTable(t *testing.T) {
	ctx := context.Background()
	tx := &fakeTx{}
	act := modules.Action{Kind: modules.ActionKindCreate, SQLOp: "INSERT", TargetTable: "clientes", InputFields: []modules.FieldSpec{
		{Name: "nombre", Source: modules.FieldSourceInput},
	}}
	input := map[string]any{"nombre": "Ana"}
	n, err := execSingleInsert(ctx, tx, writeWL(), writeCols(), writeSpec(), act, input)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if n != 1 {
		t.Fatalf("rowsAffected: got %d want 1", n)
	}
	if len(tx.execs) != 1 {
		t.Fatalf("se esperaba 1 INSERT: %+v", tx.execs)
	}
	q := tx.execs[0].query
	if !strings.HasPrefix(q, `INSERT INTO "clientes"`) {
		t.Fatalf("INSERT a la tabla objetivo esperado: %s", q)
	}
	if strings.Contains(strings.ToUpper(q), "JOIN") {
		t.Fatalf("un INSERT no debe usar JOIN: %s", q)
	}
}

// §5.4: INSERT sin TargetTable ⇒ la principal.
func TestExecSingleInsert_DefaultsToPrincipal(t *testing.T) {
	ctx := context.Background()
	tx := &fakeTx{}
	act := modules.Action{Kind: modules.ActionKindCreate, SQLOp: "INSERT", InputFields: []modules.FieldSpec{
		{Name: "monto", Source: modules.FieldSourceInput},
	}}
	input := map[string]any{"monto": 50}
	_, err := execSingleInsert(ctx, tx, writeWL(), writeCols(), writeSpec(), act, input)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if len(tx.execs) != 1 || !strings.HasPrefix(tx.execs[0].query, `INSERT INTO "pagos"`) {
		t.Fatalf("INSERT debe ir a la principal: %+v", tx.execs)
	}
}

// Test 7 (CA9/CA10): "Inactivar" con constante y objetivo dynamic enrutado a su tabla.
func TestExecMultiUpdate_ConstantAndDynamicRouting(t *testing.T) {
	ctx := context.Background()
	tx := &fakeTx{}
	cols := []modules.ResolvedColumn{
		{Field: "id", IsPrimaryKey: true, SourceTable: "pagos"},
		{Field: "cliente_id", Editable: true, SourceTable: "pagos"},
		{Field: "id", IsPrimaryKey: true, SourceTable: "clientes"},
		{Field: "estado", Editable: true, SourceTable: "clientes"},
	}
	wl := map[string][]string{
		"pagos":    {"id", "cliente_id"},
		"clientes": {"id", "estado"},
	}
	act := modules.Action{Kind: modules.ActionKindEdit, SQLOp: "UPDATE", InputFields: []modules.FieldSpec{
		{Name: "estado", SourceTable: "clientes", Source: modules.FieldSourceConstant, ConstValue: "2"},
		{Name: "cliente_id", SourceTable: "pagos", Source: modules.FieldSourceDynamic},
	}}
	// applyConstantFields lo hace el dispatcher; aquí lo simulamos para estado.
	input := map[string]any{
		"pagos__id":    7,
		"clientes__id": 3,
		"estado":       "2",        // materializado por constante
		"cliente_id":   float64(9), // id elegido en un dynamic
	}
	_, err := execMultiUpdate(ctx, tx, wl, cols, writeSpec(), act, input)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if len(tx.execs) != 2 {
		t.Fatalf("se esperaban 2 UPDATE: %+v", tx.execs)
	}
	var clientesQ, pagosQ string
	for _, e := range tx.execs {
		switch {
		case strings.Contains(e.query, `UPDATE "clientes"`):
			clientesQ = e.query
		case strings.Contains(e.query, `UPDATE "pagos"`):
			pagosQ = e.query
		}
	}
	if !strings.Contains(clientesQ, `SET "estado" = $1`) {
		t.Fatalf("estado constante debe ir a clientes: %s", clientesQ)
	}
	if !strings.Contains(pagosQ, `SET "cliente_id" = $1`) {
		t.Fatalf("el id dynamic debe enrutarse a pagos: %s", pagosQ)
	}
}
