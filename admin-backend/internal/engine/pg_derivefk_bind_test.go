package engine

import (
	"context"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"
)

// TestResolveFillStrategies_DeriveViaFK_BindsCapturedComeNumero congela el
// comportamiento CORRECTO del salto derive-via-fk (Bug #3): el SELECT del salto
// debe ser exactamente
// `SELECT ... FROM "gt_valores_padre" WHERE "valor_objeto" = $1 LIMIT 2`
// y ligar como $1 el come_numero CAPTURADO del comercio (storeColumn del campo
// con captureKey `comercio`), NO el codigo del medio, ni el valor_objeto de
// gt_valores_hijo (columna homónima), ni el id de otra opción. Previene
// regresiones de homónimo y de bind.
func TestResolveFillStrategies_DeriveViaFK_BindsCapturedComeNumero(t *testing.T) {
	drv := &recordedDriver{}

	var padreSQL string
	var padreArg any
	var padreSelects int
	drv.queryResponder = func(query string, args []driver.NamedValue) ([]string, [][]driver.Value, bool) {
		switch {
		case strings.Contains(query, `FROM "gt_valores"`):
			// Relectura del catálogo del medio de pago.
			return []string{"codigo", "list_numero", "valo_numero", "valor_es"},
				[][]driver.Value{{"MP01", int64(10001), int64(55), "Tarjeta"}}, true
		case strings.Contains(query, `FROM "gt_valores_padre"`):
			padreSelects++
			padreSQL = query
			if len(args) == 1 {
				padreArg = args[0].Value
			}
			// 1 fila padre ⇒ el salto resuelve (simula come_numero=1 existente).
			return []string{"depe_numero", "padr_numero"},
				[][]driver.Value{{int64(10000), int64(100000)}}, true
		}
		return nil, nil, false
	}

	tx, done := openFakeTx(t, drv)
	defer done()

	// El POST real llega con el come_numero capturado como string ("1").
	input := map[string]any{
		"__capture_comercio": "1",
		"valor_lista":        int64(55),
	}
	out, err := resolveFillStrategies(context.Background(), tx, fsWL(), fsAction(), input)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	// (a) SQL EXACTO del salto, calificado a gt_valores_padre (sin ambigüedad con
	// gt_valores_hijo.valor_objeto).
	wantSQL := `SELECT "depe_numero", "padr_numero" FROM "gt_valores_padre" WHERE "valor_objeto" = $1 LIMIT 2`
	if padreSQL != wantSQL {
		t.Fatalf("SQL del salto inesperado:\n got %s\nwant %s", padreSQL, wantSQL)
	}

	// (b) $1 ligado == come_numero capturado ("1"), NO otro valor.
	if padreArg != "1" {
		t.Fatalf("el salto debe ligar el come_numero capturado \"1\", ligó %#v", padreArg)
	}

	// (c) un único SELECT a gt_valores_padre (dedup de los dos derive-via-fk).
	if padreSelects != 1 {
		t.Fatalf("se esperaba 1 SELECT a gt_valores_padre, got %d", padreSelects)
	}

	// Resultado: las dos columnas derivadas quedan pobladas desde la fila padre.
	if out["padr_numero"] != int64(100000) || out["depe_numero"] != int64(10000) {
		t.Fatalf("derivaciones inesperadas: padr=%v depe=%v", out["padr_numero"], out["depe_numero"])
	}
}

// TestResolveFillStrategies_DeriveViaFK_ZeroRowsIsE422 confirma la otra cara: si
// el salto NO encuentra fila padre (0 filas), el motor aborta con
// ErrDeriveRelation (el 422 E422_DERIVE del handler). Esto documenta que, con el
// SQL/bind ya correctos, el E422 universal del informe es data-dependiente (0 ó
// >1 filas contra la BD real), no un defecto de construcción del SELECT.
func TestResolveFillStrategies_DeriveViaFK_ZeroRowsIsE422(t *testing.T) {
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

	input := map[string]any{"__capture_comercio": "1", "valor_lista": int64(55)}
	_, err := resolveFillStrategies(context.Background(), tx, fsWL(), fsAction(), input)
	if err == nil || !errors.Is(err, ErrDeriveRelation) {
		t.Fatalf("0 filas en el salto debe dar ErrDeriveRelation (E422_DERIVE): %v", err)
	}
}

// TestResolveFillStrategies_DeriveViaFK_RunsWithEmptyTargetTable documenta por
// qué el orden #1→#3 es obligatorio: con el estado post-reload ANTES del fix de
// Bug #1 (TargetTable==""), el camino multi-tabla IGUAL ejecuta el salto (fase 4)
// porque needsResolutionTx solo exige HasFillStrategies || Uniqueness, no
// TargetTable. La fase 6 proyecta entonces contra whitelist[""] (sin columnas),
// vaciando el out. Verificar #3 contra este estado razona sobre un runtime
// irreal; por eso se corrige #1 primero.
func TestResolveFillStrategies_DeriveViaFK_RunsWithEmptyTargetTable(t *testing.T) {
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
				[][]driver.Value{{int64(10000), int64(100000)}}, true
		}
		return nil, nil, false
	}
	tx, done := openFakeTx(t, drv)
	defer done()

	act := fsAction()
	act.TargetTable = "" // estado post-reload PRE-#1

	input := map[string]any{"__capture_comercio": "1", "valor_lista": int64(55)}
	out, err := resolveFillStrategies(context.Background(), tx, fsWL(), act, input)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	// El salto SÍ corrió aunque TargetTable=="" (needsResolutionTx no lo exige).
	if padreSelects != 1 {
		t.Fatalf("el salto debe ejecutarse aun con TargetTable vacío, SELECTs=%d", padreSelects)
	}
	// Fase 6 proyecta contra whitelist[""] (sin columnas) ⇒ out vacío. Esto es el
	// runtime irreal que motiva el orden #1→#3.
	if len(out) != 0 {
		t.Fatalf("con TargetTable vacío la proyección final deja out vacío, got %+v", out)
	}
}
