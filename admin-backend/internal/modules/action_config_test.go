package modules

import (
	"reflect"
	"testing"
)

// TestEncodeDecodeActionConfig_RoundTrip verifica que targetTable, uniqueness
// (columnas + mensaje) y deleteTopTable sobreviven encode→decode exactos (Bug
// #1: la config extendida de la acción create se PERSISTE de extremo a extremo).
func TestEncodeDecodeActionConfig_RoundTrip(t *testing.T) {
	act := Action{
		Kind:        ActionKindCreate,
		TargetTable: "gt_valores_hijo",
		Uniqueness: &UniquenessCheck{
			Columns: []string{"padr_numero", "valor_lista", "valor_objeto"},
			Message: "Ya existe este medio de pago para el comercio",
		},
	}

	targetTable, uniq, deleteTop := decodeActionConfig(encodeActionConfig(act))

	if targetTable != "gt_valores_hijo" {
		t.Fatalf("targetTable: got %q want %q", targetTable, "gt_valores_hijo")
	}
	if deleteTop != "" {
		t.Fatalf("deleteTopTable: got %q want vacío", deleteTop)
	}
	if uniq == nil {
		t.Fatalf("uniqueness: got nil, se esperaba la combinación")
	}
	if !reflect.DeepEqual(uniq.Columns, act.Uniqueness.Columns) {
		t.Fatalf("uniqueness.columns: got %v want %v", uniq.Columns, act.Uniqueness.Columns)
	}
	if uniq.Message != act.Uniqueness.Message {
		t.Fatalf("uniqueness.message: got %q want %q", uniq.Message, act.Uniqueness.Message)
	}
}

// TestEncodeActionConfig_EmptyIsBackCompat garantiza retrocompatibilidad byte a
// byte: una acción sin config extendida produce exactamente "{}", y decodificar
// "{}", nil o []byte vacío devuelve ceros (sin panic). Así las filas antiguas
// se comportan igual que hoy.
func TestEncodeActionConfig_EmptyIsBackCompat(t *testing.T) {
	empty := Action{Kind: ActionKindCreate}
	if got := string(encodeActionConfig(empty)); got != "{}" {
		t.Fatalf("encode acción vacía: got %q want %q", got, "{}")
	}

	for _, in := range [][]byte{[]byte("{}"), nil, {}} {
		targetTable, uniq, deleteTop := decodeActionConfig(in)
		if targetTable != "" || uniq != nil || deleteTop != "" {
			t.Fatalf("decode %q: got (%q,%v,%q) want ceros", string(in), targetTable, uniq, deleteTop)
		}
	}

	// JSON inválido es tolerado (sin panic) y devuelve ceros.
	targetTable, uniq, deleteTop := decodeActionConfig([]byte("{no-json"))
	if targetTable != "" || uniq != nil || deleteTop != "" {
		t.Fatalf("decode JSON inválido: got (%q,%v,%q) want ceros", targetTable, uniq, deleteTop)
	}
}

// TestEncodeActionConfig_DeleteTopTable confirma que deleteTopTable (acción
// delete multi-tabla) también hace round-trip.
func TestEncodeActionConfig_DeleteTopTable(t *testing.T) {
	act := Action{Kind: ActionKindDelete, DeleteTopTable: "gt_valores_padre"}
	targetTable, uniq, deleteTop := decodeActionConfig(encodeActionConfig(act))
	if targetTable != "" || uniq != nil {
		t.Fatalf("delete: got targetTable=%q uniq=%v want vacíos", targetTable, uniq)
	}
	if deleteTop != "gt_valores_padre" {
		t.Fatalf("deleteTopTable: got %q want %q", deleteTop, "gt_valores_padre")
	}
}

// TestActionRowRoundTrip_PreservesConfig cubre el round-trip a nivel de
// repositorio sin BD: una Action create con TargetTable y Uniqueness se
// serializa a actionRow.Config (como en insertActions) y se relee con
// toActionDomain conservando los valores intactos.
func TestActionRowRoundTrip_PreservesConfig(t *testing.T) {
	act := Action{
		ActionKey:   "activar",
		Kind:        ActionKindCreate,
		TargetTable: "gt_valores_hijo",
		Uniqueness: &UniquenessCheck{
			Columns: []string{"padr_numero", "valor_lista", "valor_objeto"},
			Message: "Ya existe este medio de pago para el comercio",
		},
	}

	// Simula la escritura de insertActions y la relectura de toActionDomain.
	row := actionRow{
		ActionKey: act.ActionKey,
		Kind:      act.Kind,
		Config:    encodeActionConfig(act),
	}
	got := toActionDomain(row)

	if got.TargetTable != act.TargetTable {
		t.Fatalf("TargetTable releída: got %q want %q", got.TargetTable, act.TargetTable)
	}
	if got.Uniqueness == nil {
		t.Fatalf("Uniqueness releída: got nil")
	}
	if !reflect.DeepEqual(got.Uniqueness.Columns, act.Uniqueness.Columns) {
		t.Fatalf("Uniqueness.Columns releídas: got %v want %v", got.Uniqueness.Columns, act.Uniqueness.Columns)
	}
	if got.Uniqueness.Message != act.Uniqueness.Message {
		t.Fatalf("Uniqueness.Message releído: got %q want %q", got.Uniqueness.Message, act.Uniqueness.Message)
	}
}
