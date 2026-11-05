package engine

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"

	"admin-backend/internal/modules"
)

// fsWL es la whitelist del caso real de estrategias de llenado.
func fsWL() map[string][]string {
	return map[string][]string{
		"gt_valores_hijo":   {"hijo_numero", "valor_lista", "list_numero_h", "padr_numero", "depe_numero", "usrio_numero", "eliminado"},
		"gt_valores":        {"valo_numero", "list_numero", "codigo", "valor_es"},
		"cl_comercios_sapp": {"come_numero", "nombre"},
		"gt_valores_padre":  {"padr_numero", "depe_numero", "valor_objeto"},
	}
}

// fsAction arma la acción INSERT del caso real (capturas + derivaciones).
func fsAction() modules.Action {
	return modules.Action{
		ActionKey:   "activar",
		Kind:        modules.ActionKindCreate,
		SQLOp:       "INSERT",
		TargetTable: "gt_valores_hijo",
		InputFields: []modules.FieldSpec{
			{Name: "__capture_comercio", Source: modules.FieldSourceFillStrategy, Strategy: &modules.FillStrategy{
				Kind:       modules.StrategyCaptureFromCatalog,
				CaptureKey: "comercio",
				Catalog:    &modules.DynamicSource{Table: "cl_comercios_sapp", IDColumn: "come_numero", LabelColumn: "nombre"},
			}},
			{Name: "valor_lista", Source: modules.FieldSourceFillStrategy, Strategy: &modules.FillStrategy{
				Kind:        modules.StrategyCaptureFromCatalog,
				CaptureKey:  "medio_pago",
				StoreColumn: "codigo",
				Catalog:     &modules.DynamicSource{Table: "gt_valores", IDColumn: "valo_numero", LabelColumn: "valor_es"},
			}},
			{Name: "list_numero_h", Source: modules.FieldSourceFillStrategy, Strategy: &modules.FillStrategy{
				Kind:             modules.StrategyDeriveFromOption,
				SourceCaptureKey: "medio_pago",
				Attribute:        "list_numero",
			}},
			{Name: "padr_numero", Source: modules.FieldSourceFillStrategy, Strategy: &modules.FillStrategy{
				Kind:    modules.StrategyDeriveViaFK,
				Jump:    &modules.FkJump{RelatedTable: "gt_valores_padre", RelatedColumn: "valor_objeto", LocalCaptureKey: "comercio", LocalAttribute: "come_numero"},
				Mapping: []modules.AttributeMap{{Attribute: "padr_numero", TargetColumn: "padr_numero"}},
			}},
			{Name: "depe_numero", Source: modules.FieldSourceFillStrategy, Strategy: &modules.FillStrategy{
				Kind:    modules.StrategyDeriveViaFK,
				Jump:    &modules.FkJump{RelatedTable: "gt_valores_padre", RelatedColumn: "valor_objeto", LocalCaptureKey: "comercio", LocalAttribute: "come_numero"},
				Mapping: []modules.AttributeMap{{Attribute: "depe_numero", TargetColumn: "depe_numero"}},
			}},
		},
	}
}

// openFakeTx abre un *sql.Tx REAL contra el driver falso con el responder dado.
func openFakeTx(t *testing.T, drv *recordedDriver) (*sql.Tx, func()) {
	t.Helper()
	db := sql.OpenDB(fakeConnector{drv: drv})
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("no se pudo abrir la tx: %v", err)
	}
	return tx, func() { _ = tx.Rollback(); _ = db.Close() }
}

// TestResolveFillStrategies_HappyPath cubre el flujo completo del caso real:
// StoreColumn!=IDColumn deja valor_lista==codigo; derive-from-option propaga
// list_numero; dedup de dos derive-via-fk con el MISMO salto ejecuta UN SELECT a
// gt_valores_padre y DOS escrituras; proyección final borra __capture_*.
func TestResolveFillStrategies_HappyPath(t *testing.T) {
	drv := &recordedDriver{}
	var padreSelects, valoresSelects int
	drv.queryResponder = func(query string, args []driver.NamedValue) ([]string, [][]driver.Value, bool) {
		switch {
		case strings.Contains(query, `FROM "gt_valores"`):
			valoresSelects++
			// Relectura del catálogo de medio_pago: devuelve todas las columnas WL.
			// WL de gt_valores (orden alfabético): codigo, list_numero, valo_numero, valor_es
			return []string{"codigo", "list_numero", "valo_numero", "valor_es"},
				[][]driver.Value{{"MP01", int64(10001), int64(55), "Tarjeta"}}, true
		case strings.Contains(query, `FROM "gt_valores_padre"`):
			padreSelects++
			// Proyección: atributos en orden alfabético (depe_numero, padr_numero).
			return []string{"depe_numero", "padr_numero"},
				[][]driver.Value{{int64(777), int64(999)}}, true
		}
		return nil, nil, false
	}

	tx, done := openFakeTx(t, drv)
	defer done()

	input := map[string]any{
		"__capture_comercio": int64(42), // id del comercio (IDColumn ⇒ sin relectura)
		"valor_lista":        int64(55), // id del medio de pago elegido
	}
	out, err := resolveFillStrategies(context.Background(), tx, fsWL(), fsAction(), input)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	// INV-5: StoreColumn=codigo ⇒ valor_lista==codigo (no el id).
	if out["valor_lista"] != "MP01" {
		t.Fatalf("valor_lista debe ser el codigo capturado, got %v", out["valor_lista"])
	}
	// derive-from-option: list_numero_h == list_numero de la fila releída.
	if out["list_numero_h"] != int64(10001) {
		t.Fatalf("list_numero_h derivado inesperado: %v", out["list_numero_h"])
	}
	// derive-via-fk: dos escrituras desde la fila única del salto.
	if out["padr_numero"] != int64(999) || out["depe_numero"] != int64(777) {
		t.Fatalf("derivaciones del salto inesperadas: padr=%v depe=%v", out["padr_numero"], out["depe_numero"])
	}
	// Dedup: UN solo SELECT a gt_valores_padre.
	if padreSelects != 1 {
		t.Fatalf("se esperaba 1 SELECT a gt_valores_padre (dedup), got %d", padreSelects)
	}
	// El catálogo de medio_pago se relee UNA sola vez (cacheado por CaptureKey),
	// aunque lo usan la captura (codigo) y el derive-from-option (list_numero).
	if valoresSelects != 1 {
		t.Fatalf("se esperaba 1 SELECT a gt_valores (reuso de la fila cacheada), got %d", valoresSelects)
	}
	// Proyección final: __capture_comercio BORRADO (no es columna de la objetivo).
	if _, ok := out["__capture_comercio"]; ok {
		t.Fatalf("la proyección final debe borrar __capture_comercio: %+v", out)
	}
	// Solo columnas de la tabla objetivo.
	for k := range out {
		switch k {
		case "valor_lista", "list_numero_h", "padr_numero", "depe_numero":
		default:
			t.Fatalf("clave inesperada tras la proyección final: %q", k)
		}
	}
}

// TestResolveFillStrategies_CaptureOnlyFlagNoPrefix (MEJORA 4): un campo
// solo-captura marcado con el flag explícito CaptureOnly y un Name SIN prefijo
// (p. ej. "comercio_sel") debe comportarse igual que el __capture_ histórico: el
// motor NO lo escribe a columna y la proyección final lo descarta, pero su valor
// SÍ alimenta las derivaciones derive-via-fk.
func TestResolveFillStrategies_CaptureOnlyFlagNoPrefix(t *testing.T) {
	drv := &recordedDriver{}
	var padreSelects int
	drv.queryResponder = func(query string, args []driver.NamedValue) ([]string, [][]driver.Value, bool) {
		switch {
		case strings.Contains(query, `FROM "gt_valores"`):
			return []string{"codigo", "list_numero", "valo_numero", "valor_es"},
				[][]driver.Value{{"MP01", int64(10001), int64(55), "Tarjeta"}}, true
		case strings.Contains(query, `FROM "gt_valores_padre"`):
			padreSelects++
			return []string{"depe_numero", "padr_numero"},
				[][]driver.Value{{int64(777), int64(999)}}, true
		}
		return nil, nil, false
	}

	// Renombra la captura del comercio a un name SIN prefijo + flag CaptureOnly y
	// ajusta el LocalCaptureKey no hace falta (es por CaptureKey, no por name).
	act := fsAction()
	for i := range act.InputFields {
		f := &act.InputFields[i]
		if f.Name == "__capture_comercio" {
			f.Name = "comercio_sel"
			f.CaptureOnly = true
		}
	}

	tx, done := openFakeTx(t, drv)
	defer done()

	input := map[string]any{
		"comercio_sel": int64(42), // captura por name SIN prefijo
		"valor_lista":  int64(55),
	}
	out, err := resolveFillStrategies(context.Background(), tx, fsWL(), act, input)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	// El valor alimentó la derivación (el salto se resolvió).
	if out["padr_numero"] != int64(999) || out["depe_numero"] != int64(777) {
		t.Fatalf("la captura por flag debe alimentar derive-via-fk: padr=%v depe=%v", out["padr_numero"], out["depe_numero"])
	}
	if padreSelects != 1 {
		t.Fatalf("se esperaba 1 SELECT a gt_valores_padre, got %d", padreSelects)
	}
	// La proyección final descarta el campo solo-captura (no es columna objetivo).
	if _, ok := out["comercio_sel"]; ok {
		t.Fatalf("la proyección final debe descartar el campo CaptureOnly: %+v", out)
	}
	for k := range out {
		switch k {
		case "valor_lista", "list_numero_h", "padr_numero", "depe_numero":
		default:
			t.Fatalf("clave inesperada tras la proyección final: %q", k)
		}
	}
}

// TestResolveFillStrategies_JumpZeroRows: 0 filas en el salto ⇒ ErrDeriveRelation.
func TestResolveFillStrategies_JumpZeroRows(t *testing.T) {
	drv := &recordedDriver{}
	drv.queryResponder = func(query string, args []driver.NamedValue) ([]string, [][]driver.Value, bool) {
		if strings.Contains(query, `FROM "gt_valores"`) {
			return []string{"codigo", "list_numero", "valo_numero", "valor_es"},
				[][]driver.Value{{"MP01", int64(10001), int64(55), "Tarjeta"}}, true
		}
		if strings.Contains(query, `FROM "gt_valores_padre"`) {
			return []string{"depe_numero", "padr_numero"}, [][]driver.Value{}, true // 0 filas
		}
		return nil, nil, false
	}
	tx, done := openFakeTx(t, drv)
	defer done()
	input := map[string]any{"__capture_comercio": int64(42), "valor_lista": int64(55)}
	_, err := resolveFillStrategies(context.Background(), tx, fsWL(), fsAction(), input)
	if err == nil || !errors.Is(err, ErrDeriveRelation) {
		t.Fatalf("se esperaba ErrDeriveRelation por 0 filas en el salto: %v", err)
	}
}

// TestResolveFillStrategies_JumpMultipleRows: >1 filas en el salto ⇒ ErrDeriveRelation.
func TestResolveFillStrategies_JumpMultipleRows(t *testing.T) {
	drv := &recordedDriver{}
	drv.queryResponder = func(query string, args []driver.NamedValue) ([]string, [][]driver.Value, bool) {
		if strings.Contains(query, `FROM "gt_valores"`) {
			return []string{"codigo", "list_numero", "valo_numero", "valor_es"},
				[][]driver.Value{{"MP01", int64(10001), int64(55), "Tarjeta"}}, true
		}
		if strings.Contains(query, `FROM "gt_valores_padre"`) {
			return []string{"depe_numero", "padr_numero"},
				[][]driver.Value{{int64(1), int64(2)}, {int64(3), int64(4)}}, true // >1
		}
		return nil, nil, false
	}
	tx, done := openFakeTx(t, drv)
	defer done()
	input := map[string]any{"__capture_comercio": int64(42), "valor_lista": int64(55)}
	_, err := resolveFillStrategies(context.Background(), tx, fsWL(), fsAction(), input)
	if err == nil || !errors.Is(err, ErrDeriveRelation) {
		t.Fatalf("se esperaba ErrDeriveRelation por >1 filas en el salto: %v", err)
	}
}

// TestResolveFillStrategies_CatalogZeroRows: 0 filas al releer el catálogo ⇒
// ErrDeriveRelation.
func TestResolveFillStrategies_CatalogZeroRows(t *testing.T) {
	drv := &recordedDriver{}
	drv.queryResponder = func(query string, args []driver.NamedValue) ([]string, [][]driver.Value, bool) {
		if strings.Contains(query, `FROM "gt_valores"`) {
			return []string{"codigo", "list_numero", "valo_numero", "valor_es"}, [][]driver.Value{}, true
		}
		return nil, nil, false
	}
	tx, done := openFakeTx(t, drv)
	defer done()
	input := map[string]any{"__capture_comercio": int64(42), "valor_lista": int64(55)}
	_, err := resolveFillStrategies(context.Background(), tx, fsWL(), fsAction(), input)
	if err == nil || !errors.Is(err, ErrDeriveRelation) {
		t.Fatalf("se esperaba ErrDeriveRelation por 0 filas al releer el catálogo: %v", err)
	}
}

// TestResolveFillStrategies_UnresolvedCaptureKey: un derive-from-option que
// referencia un captureKey inexistente ⇒ ErrDeriveRelation (sin SELECT).
func TestResolveFillStrategies_UnresolvedCaptureKey(t *testing.T) {
	drv := &recordedDriver{}
	tx, done := openFakeTx(t, drv)
	defer done()
	act := fsAction()
	for i := range act.InputFields {
		if act.InputFields[i].Name == "list_numero_h" {
			act.InputFields[i].Strategy.SourceCaptureKey = "inexistente"
		}
	}
	input := map[string]any{"__capture_comercio": int64(42), "valor_lista": int64(55)}
	_, err := resolveFillStrategies(context.Background(), tx, fsWL(), act, input)
	if err == nil || !errors.Is(err, ErrDeriveRelation) {
		t.Fatalf("se esperaba ErrDeriveRelation por captureKey sin resolver: %v", err)
	}
}

// TestResolveFillStrategies_NoStrategies_InputUntouched: retrocompat. Una acción
// sin estrategias no toca el input ni emite SELECT.
func TestResolveFillStrategies_NoStrategies_InputUntouched(t *testing.T) {
	drv := &recordedDriver{}
	tx, done := openFakeTx(t, drv)
	defer done()
	act := modules.Action{
		Kind: modules.ActionKindCreate, SQLOp: "INSERT", TargetTable: "gt_valores_hijo",
		InputFields: []modules.FieldSpec{{Name: "valor_lista", Source: modules.FieldSourceInput}},
	}
	input := map[string]any{"valor_lista": "directo"}
	out, err := resolveFillStrategies(context.Background(), tx, fsWL(), act, input)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if out["valor_lista"] != "directo" {
		t.Fatalf("sin estrategias el input debe quedar intacto: %+v", out)
	}
	if len(drv.queries) != 0 {
		t.Fatalf("sin estrategias no debe emitirse ningún SELECT: %+v", drv.queries)
	}
}

// TestResolveFillStrategies_NoReloadWhenLocalIsIDColumn: cuando LocalAttribute ==
// Catalog.IDColumn del comercio, NO se relee cl_comercios_sapp (el valor local es
// el id capturado). Solo debe haber SELECT a gt_valores (catálogo medio_pago) y a
// gt_valores_padre (salto), nunca a cl_comercios_sapp.
func TestResolveFillStrategies_NoReloadWhenLocalIsIDColumn(t *testing.T) {
	drv := &recordedDriver{}
	drv.queryResponder = func(query string, args []driver.NamedValue) ([]string, [][]driver.Value, bool) {
		switch {
		case strings.Contains(query, `FROM "gt_valores"`):
			return []string{"codigo", "list_numero", "valo_numero", "valor_es"},
				[][]driver.Value{{"MP01", int64(10001), int64(55), "Tarjeta"}}, true
		case strings.Contains(query, `FROM "gt_valores_padre"`):
			return []string{"depe_numero", "padr_numero"},
				[][]driver.Value{{int64(777), int64(999)}}, true
		}
		return nil, nil, false
	}
	tx, done := openFakeTx(t, drv)
	defer done()
	input := map[string]any{"__capture_comercio": int64(42), "valor_lista": int64(55)}
	if _, err := resolveFillStrategies(context.Background(), tx, fsWL(), fsAction(), input); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	for _, q := range drv.queries {
		if strings.Contains(q, `FROM "cl_comercios_sapp"`) {
			t.Fatalf("NO debe releerse cl_comercios_sapp cuando LocalAttribute==IDColumn: %q", q)
		}
	}
}
