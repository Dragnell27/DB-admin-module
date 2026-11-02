package modules

import (
	"bytes"
	"encoding/json"
	"testing"

	"admin-backend/internal/secrets"
)

// testSealer construye un Sealer con una clave de 32 bytes de prueba.
func testSealer(t *testing.T) *secrets.Sealer {
	t.Helper()
	key := bytes.Repeat([]byte{0x22}, secrets.KeySize)
	s, err := secrets.New(key)
	if err != nil {
		t.Fatalf("no se pudo construir el Sealer de prueba: %v", err)
	}
	return s
}

// sealVariables sella un bundle de valores variables, para construir un
// prevEnc de prueba.
func sealVariables(t *testing.T, sealer *secrets.Sealer, vars map[string]string) []byte {
	t.Helper()
	plain, err := json.Marshal(Secrets{VariableValues: vars})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	enc, err := sealer.Seal(plain)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	return enc
}

// openVariables descifra un bundle sellado y devuelve el mapa de variables.
func openVariables(t *testing.T, sealer *secrets.Sealer, enc []byte) map[string]string {
	t.Helper()
	if len(enc) == 0 {
		return map[string]string{}
	}
	plain, err := sealer.Open(enc)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	var b Secrets
	if err := json.Unmarshal(plain, &b); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if b.VariableValues == nil {
		return map[string]string{}
	}
	return b.VariableValues
}

func TestMergeVariableValues_OneOfThreeRewritten(t *testing.T) {
	sealer := testSealer(t)
	keys := []string{"a", "b", "c"}
	prev := sealVariables(t, sealer, map[string]string{"a": "1", "b": "2", "c": "3"})

	// Solo reescribe "b"; "a" y "c" se conservan.
	sec := Secrets{VariableValues: map[string]string{"b": "22"}}
	sealed, err := mergeVariableValues(sealer, prev, sec, keys)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	got := openVariables(t, sealer, sealed)
	if got["a"] != "1" || got["b"] != "22" || got["c"] != "3" {
		t.Errorf("merge inesperado: %+v", got)
	}
}

func TestMergeVariableValues_KeyRemovedFromStructureDropped(t *testing.T) {
	sealer := testSealer(t)
	// prev tiene "a","b" pero la estructura ya solo declara "a": "b" se descarta.
	prev := sealVariables(t, sealer, map[string]string{"a": "1", "b": "2"})
	sealed, err := mergeVariableValues(sealer, prev, Secrets{}, []string{"a"})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	got := openVariables(t, sealer, sealed)
	if _, ok := got["b"]; ok {
		t.Error("la clave fuera de la estructura debía descartarse")
	}
	if got["a"] != "1" {
		t.Errorf("a debía conservarse: %+v", got)
	}
}

func TestMergeVariableValues_EmptyPrevAllNew(t *testing.T) {
	sealer := testSealer(t)
	sec := Secrets{VariableValues: map[string]string{"a": "1"}}
	sealed, err := mergeVariableValues(sealer, nil, sec, []string{"a"})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	got := openVariables(t, sealer, sealed)
	if got["a"] != "1" {
		t.Errorf("a debía estar: %+v", got)
	}
}

func TestMergeVariableValues_AbsentOrEmptyKeepsPrev(t *testing.T) {
	sealer := testSealer(t)
	prev := sealVariables(t, sealer, map[string]string{"a": "1"})

	// variableValues ausente ({}) -> bundle previo íntegro.
	sealed, err := mergeVariableValues(sealer, prev, Secrets{}, []string{"a"})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	got := openVariables(t, sealer, sealed)
	if got["a"] != "1" {
		t.Errorf("bundle previo debía conservarse íntegro: %+v", got)
	}

	// variableValues: {} explícito -> igual.
	sealed2, err := mergeVariableValues(sealer, prev, Secrets{VariableValues: map[string]string{}}, []string{"a"})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	got2 := openVariables(t, sealer, sealed2)
	if got2["a"] != "1" {
		t.Errorf("bundle previo debía conservarse con {}: %+v", got2)
	}
}

func TestMergeVariableValues_NoKeysProducesNil(t *testing.T) {
	sealer := testSealer(t)
	// Plantilla sin variables (mapeo 1:1 de migración): merge vacío -> nil.
	sealed, err := mergeVariableValues(sealer, nil, Secrets{}, nil)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if sealed != nil {
		t.Errorf("sin claves VARIABLE el bundle debería ser nil, got %d bytes", len(sealed))
	}
}

func TestSecrets_DSNNotSerializedFromClient(t *testing.T) {
	// El DSN efectivo (campo con tag "-") nunca se serializa en el bundle que se
	// persiste ni se acepta del cliente.
	b, err := json.Marshal(Secrets{VariableValues: map[string]string{"a": "1"}, DSN: "postgres://x/y"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if bytes.Contains(b, []byte("postgres://")) || bytes.Contains(b, []byte("dsn")) {
		t.Errorf("el DSN no debería serializarse: %s", b)
	}
}

func TestSecrets_IsEmpty(t *testing.T) {
	if !(Secrets{}).isEmpty() {
		t.Error("Secrets{} debería ser vacío")
	}
	if !(Secrets{VariableValues: map[string]string{}}).isEmpty() {
		t.Error("variableValues {} debería ser vacío")
	}
	if (Secrets{VariableValues: map[string]string{"a": "1"}}).isEmpty() {
		t.Error("variableValues con claves no debería ser vacío")
	}
}
