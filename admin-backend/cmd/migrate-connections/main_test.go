package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"admin-backend/internal/connections"
	"admin-backend/internal/secrets"
)

func testSealer(t *testing.T) *secrets.Sealer {
	t.Helper()
	key := bytes.Repeat([]byte{0x33}, secrets.KeySize)
	s, err := secrets.New(key)
	if err != nil {
		t.Fatalf("sealer: %v", err)
	}
	return s
}

// sealLegacy sella un bundle legado {dsn: ...}.
func sealLegacy(t *testing.T, sealer *secrets.Sealer, dsn string) []byte {
	t.Helper()
	plain, _ := json.Marshal(legacySecrets{DSN: dsn})
	enc, err := sealer.Seal(plain)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	return enc
}

func TestMigrateDataSource_Decomposable(t *testing.T) {
	sealer := testSealer(t)
	enc := sealLegacy(t, sealer, "postgres://svc:s3cr3t@10.0.0.5:5432/recaudo_gt?sslmode=require")
	derived, err := migrateDataSource(sealer, enc)
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	// Todos los campos deben ser FIJOS.
	for _, f := range derived.Fields {
		if f.Kind != connections.FieldFixed {
			t.Errorf("campo %q no es FIJO", f.Key)
		}
	}
	if derived.Fixed["dbname"] != "recaudo_gt" {
		t.Errorf("dbname = %q", derived.Fixed["dbname"])
	}
	if derived.Fixed["password"] != "s3cr3t" {
		t.Errorf("password = %q", derived.Fixed["password"])
	}
}

func TestMigrateDataSource_Undecomposable(t *testing.T) {
	sealer := testSealer(t)
	// DSN sin dbname -> migración manual pendiente, sin pánico.
	enc := sealLegacy(t, sealer, "postgres://svc@10.0.0.5:5432/")
	_, err := migrateDataSource(sealer, enc)
	if err == nil {
		t.Fatal("se esperaba error (manual pendiente) por dbname ausente")
	}
	if !strings.Contains(err.Error(), "manual") {
		t.Errorf("mensaje inesperado: %v", err)
	}
}

func TestMigrateDataSource_RoundTripWithExtraParams(t *testing.T) {
	sealer := testSealer(t)
	enc := sealLegacy(t, sealer, "postgres://u:p@h:5432/db?connect_timeout=10&application_name=admin")
	derived, err := migrateDataSource(sealer, enc)
	if err != nil {
		t.Fatalf("debería verificar y migrar: %v", err)
	}
	if derived.Fixed["connect_timeout"] != "10" || derived.Fixed["application_name"] != "admin" {
		t.Errorf("parámetros extra no derivados: %+v", derived.Fixed)
	}
}

func TestMigrateDataSource_BadCipher(t *testing.T) {
	sealer := testSealer(t)
	_, err := migrateDataSource(sealer, []byte("no es un bundle sellado"))
	if err == nil {
		t.Fatal("se esperaba error de descifrado")
	}
}

func TestLegacyEquivalent_NormalizedComponents(t *testing.T) {
	cases := []struct {
		name string
		a, b string
		want bool
	}{
		{"puerto implícito vs explícito", "postgres://u:p@h/db", "postgres://u:p@h:5432/db", true},
		{"host mayúsculas", "postgres://u:p@HOST/db", "postgres://u:p@host/db", true},
		{"sslmode ausente vs prefer", "postgres://u:p@h/db", "postgres://u:p@h/db?sslmode=prefer", true},
		{"orden de params", "postgres://u:p@h/db?a=1&b=2", "postgres://u:p@h/db?b=2&a=1", true},
		{"user distinto", "postgres://u1:p@h/db", "postgres://u2:p@h/db", false},
		{"param extra en un lado", "postgres://u:p@h/db?a=1", "postgres://u:p@h/db", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := connections.LegacyEquivalent(c.a, c.b); got != c.want {
				t.Errorf("LegacyEquivalent(%q,%q) = %v, want %v", c.a, c.b, got, c.want)
			}
		})
	}
}
