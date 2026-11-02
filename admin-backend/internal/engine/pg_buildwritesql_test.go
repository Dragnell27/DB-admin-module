package engine

import (
	"errors"
	"testing"

	"admin-backend/internal/modules"
)

// whitelist común de las pruebas de buildWriteSQL.
var writeWhitelist = map[string][]string{
	"planes": {"id", "pais", "monto", "estado", "creado"},
}

func pkCol(field string) modules.ResolvedColumn {
	return modules.ResolvedColumn{Field: field, IsPrimaryKey: true, Visible: true}
}
func editCol(field string) modules.ResolvedColumn {
	return modules.ResolvedColumn{Field: field, Editable: true, Visible: true}
}

// Verifica la derivación de WHERE/SET desde columnas en UPDATE/DELETE, el
// fallback heredado isKey, la exclusión de la PK del SET y los errores.
func TestBuildWriteSQL(t *testing.T) {
	t.Run("UPDATE con PK de columna: WHERE=PK, SET=editables del input", func(t *testing.T) {
		cols := []modules.ResolvedColumn{pkCol("id"), editCol("monto"), editCol("estado")}
		act := modules.Action{SQLOp: "UPDATE"}
		input := map[string]any{"id": 7, "monto": 100, "estado": "ok"}

		q, args, err := buildWriteSQL(writeWhitelist, "planes", cols, act, input)
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		want := `UPDATE "planes" SET "estado" = $1, "monto" = $2 WHERE "id" = $3`
		if q != want {
			t.Fatalf("SQL\n  got:  %s\n  want: %s", q, want)
		}
		if len(args) != 3 || args[0] != "ok" || args[1] != 100 || args[2] != 7 {
			t.Fatalf("args inesperados: %v", args)
		}
	})

	t.Run("UPDATE PK compuesta: WHERE con AND", func(t *testing.T) {
		cols := []modules.ResolvedColumn{pkCol("id"), pkCol("pais"), editCol("monto")}
		act := modules.Action{SQLOp: "UPDATE"}
		input := map[string]any{"id": 1, "pais": "gt", "monto": 5}

		q, _, err := buildWriteSQL(writeWhitelist, "planes", cols, act, input)
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		want := `UPDATE "planes" SET "monto" = $1 WHERE "id" = $2 AND "pais" = $3`
		if q != want {
			t.Fatalf("SQL\n  got:  %s\n  want: %s", q, want)
		}
	})

	t.Run("UPDATE: la PK nunca entra en el SET aunque venga en el input", func(t *testing.T) {
		cols := []modules.ResolvedColumn{pkCol("id"), editCol("monto")}
		act := modules.Action{SQLOp: "UPDATE"}
		input := map[string]any{"id": 7, "monto": 100}

		q, _, err := buildWriteSQL(writeWhitelist, "planes", cols, act, input)
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		want := `UPDATE "planes" SET "monto" = $1 WHERE "id" = $2`
		if q != want {
			t.Fatalf("SQL\n  got:  %s\n  want: %s", q, want)
		}
	})

	t.Run("UPDATE con PK de columna y sin editables con valor ⇒ sin columnas a modificar (no legacy)", func(t *testing.T) {
		cols := []modules.ResolvedColumn{pkCol("id"), editCol("monto")}
		act := modules.Action{SQLOp: "UPDATE", InputFields: []modules.FieldSpec{{Name: "estado", IsKey: true}}}
		input := map[string]any{"id": 7} // monto no viene

		_, _, err := buildWriteSQL(writeWhitelist, "planes", cols, act, input)
		if err == nil || err.Error() != "engine: UPDATE sin columnas a modificar" {
			t.Fatalf("se esperaba 'UPDATE sin columnas a modificar', got %v", err)
		}
	})

	t.Run("UPDATE legacy (sin PK de columna) usa isKey y SET=input no-clave", func(t *testing.T) {
		var cols []modules.ResolvedColumn // definición antigua: sin PK de columna
		act := modules.Action{SQLOp: "UPDATE", InputFields: []modules.FieldSpec{
			{Name: "id", IsKey: true},
			{Name: "monto"},
			{Name: "estado"},
		}}
		input := map[string]any{"id": 7, "monto": 100, "estado": "ok"}

		q, _, err := buildWriteSQL(writeWhitelist, "planes", cols, act, input)
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		want := `UPDATE "planes" SET "estado" = $1, "monto" = $2 WHERE "id" = $3`
		if q != want {
			t.Fatalf("SQL\n  got:  %s\n  want: %s", q, want)
		}
	})

	t.Run("UPDATE sin PK ni fallback ⇒ error de llave primaria", func(t *testing.T) {
		var cols []modules.ResolvedColumn
		act := modules.Action{SQLOp: "UPDATE", InputFields: []modules.FieldSpec{{Name: "monto"}}}
		input := map[string]any{"monto": 100}

		_, _, err := buildWriteSQL(writeWhitelist, "planes", cols, act, input)
		if err == nil || err.Error() != "engine: UPDATE requiere al menos una columna llave primaria" {
			t.Fatalf("se esperaba error de llave primaria, got %v", err)
		}
	})

	t.Run("DELETE con PK de columna: WHERE=PK", func(t *testing.T) {
		cols := []modules.ResolvedColumn{pkCol("id")}
		act := modules.Action{SQLOp: "DELETE"}
		input := map[string]any{"id": 9}

		q, args, err := buildWriteSQL(writeWhitelist, "planes", cols, act, input)
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		want := `DELETE FROM "planes" WHERE "id" = $1`
		if q != want {
			t.Fatalf("SQL\n  got:  %s\n  want: %s", q, want)
		}
		if len(args) != 1 || args[0] != 9 {
			t.Fatalf("args inesperados: %v", args)
		}
	})

	t.Run("columna de SET editable fuera de la lista blanca ⇒ ErrWhitelist", func(t *testing.T) {
		cols := []modules.ResolvedColumn{pkCol("id"), editCol("secreto")}
		act := modules.Action{SQLOp: "UPDATE"}
		input := map[string]any{"id": 1, "secreto": "x"}

		_, _, err := buildWriteSQL(writeWhitelist, "planes", cols, act, input)
		if err == nil || !errors.Is(err, ErrWhitelist) {
			t.Fatalf("se esperaba ErrWhitelist, got %v", err)
		}
	})

	t.Run("columna PK fuera de la lista blanca ⇒ ErrWhitelist", func(t *testing.T) {
		cols := []modules.ResolvedColumn{pkCol("secreto")}
		act := modules.Action{SQLOp: "DELETE"}
		input := map[string]any{"secreto": "x"}

		_, _, err := buildWriteSQL(writeWhitelist, "planes", cols, act, input)
		if err == nil || !errors.Is(err, ErrWhitelist) {
			t.Fatalf("se esperaba ErrWhitelist, got %v", err)
		}
	})

	t.Run("UPDATE especializado: el SET lo acota la acción, no todas las editables", func(t *testing.T) {
		// La acción solo toca "estado"; aunque el input traiga "monto" (editable),
		// no debe entrar en el SET.
		cols := []modules.ResolvedColumn{pkCol("id"), editCol("monto"), editCol("estado")}
		act := modules.Action{SQLOp: "UPDATE", Kind: modules.ActionKindEdit, InputFields: []modules.FieldSpec{
			{Name: "estado", Source: modules.FieldSourceInput},
		}}
		input := map[string]any{"id": 7, "monto": 100, "estado": "ok"}

		q, args, err := buildWriteSQL(writeWhitelist, "planes", cols, act, input)
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		want := `UPDATE "planes" SET "estado" = $1 WHERE "id" = $2`
		if q != want {
			t.Fatalf("SQL\n  got:  %s\n  want: %s", q, want)
		}
		if len(args) != 2 || args[0] != "ok" || args[1] != 7 {
			t.Fatalf("args inesperados: %v", args)
		}
	})
}

// Verifica el flujo "Inactivar": una acción cuyo único objetivo es una columna
// constante produce un SET con esa columna tras materializar el valor constante,
// sin que el usuario la teclee.
func TestInactivarConstante(t *testing.T) {
	cols := []modules.ResolvedColumn{pkCol("id"), editCol("estado")}
	act := modules.Action{SQLOp: "UPDATE", Kind: modules.ActionKindEdit, InputFields: []modules.FieldSpec{
		{Name: "estado", Source: modules.FieldSourceConstant, ConstValue: "2"},
	}}
	// El input solo trae la PK (de la fila); "estado" NO viene del usuario.
	input := map[string]any{"id": 7}
	input = applyConstantFields(act, input)

	q, args, err := buildWriteSQL(writeWhitelist, "planes", cols, act, input)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	want := `UPDATE "planes" SET "estado" = $1 WHERE "id" = $2`
	if q != want {
		t.Fatalf("SQL\n  got:  %s\n  want: %s", q, want)
	}
	if len(args) != 2 || args[0] != "2" || args[1] != 7 {
		t.Fatalf("args inesperados: %v", args)
	}
}

// Verifica applyConstantFields: fija los constant ausentes, no muta el input
// recibido y no pisa un valor ya presente.
func TestApplyConstantFields(t *testing.T) {
	act := modules.Action{InputFields: []modules.FieldSpec{
		{Name: "estado", Source: modules.FieldSourceConstant, ConstValue: "2"},
		{Name: "pais", Source: modules.FieldSourceConstant, ConstValue: "gt"},
		{Name: "monto", Source: modules.FieldSourceInput},
	}}
	input := map[string]any{"id": 7, "pais": "sv"} // pais ya presente

	out := applyConstantFields(act, input)

	if out["estado"] != "2" {
		t.Fatalf("estado constante no materializado: %v", out["estado"])
	}
	if out["pais"] != "sv" {
		t.Fatalf("no debe pisar un valor presente: %v", out["pais"])
	}
	if _, ok := out["monto"]; ok {
		t.Fatalf("un campo input no debe materializarse: %v", out["monto"])
	}
	// No muta el input original.
	if _, ok := input["estado"]; ok {
		t.Fatalf("applyConstantFields mutó el input recibido")
	}
}
