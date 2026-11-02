package auth

import (
	"errors"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestHashPasswordProducesVerifiableHash(t *testing.T) {
	const plain = "s3cr3t-passphrase"

	hash, err := HashPassword(plain, 10)
	if err != nil {
		t.Fatalf("HashPassword devolvió error: %v", err)
	}
	if hash == plain {
		t.Fatal("el hash no debe ser igual al texto plano")
	}
	if hash == "" {
		t.Fatal("el hash no debe estar vacío")
	}

	if err := CheckPassword(hash, plain); err != nil {
		t.Fatalf("CheckPassword debió aceptar la contraseña correcta, obtuvo: %v", err)
	}
}

func TestHashPasswordUsesConfiguredCost(t *testing.T) {
	const plain = "another-password"
	const cost = 11

	hash, err := HashPassword(plain, cost)
	if err != nil {
		t.Fatalf("HashPassword devolvió error: %v", err)
	}

	got, err := bcrypt.Cost([]byte(hash))
	if err != nil {
		t.Fatalf("bcrypt.Cost devolvió error: %v", err)
	}
	if got != cost {
		t.Fatalf("coste esperado %d, obtuvo %d", cost, got)
	}
}

func TestHashPasswordRejectsEmpty(t *testing.T) {
	if _, err := HashPassword("", 12); !errors.Is(err, ErrEmptyPassword) {
		t.Fatalf("se esperaba ErrEmptyPassword, obtuvo: %v", err)
	}
}

func TestHashPasswordNormalizesInvalidCost(t *testing.T) {
	hash, err := HashPassword("pw", 0)
	if err != nil {
		t.Fatalf("HashPassword con coste inválido devolvió error: %v", err)
	}

	got, err := bcrypt.Cost([]byte(hash))
	if err != nil {
		t.Fatalf("bcrypt.Cost devolvió error: %v", err)
	}
	if got != bcrypt.DefaultCost {
		t.Fatalf("coste esperado DefaultCost (%d), obtuvo %d", bcrypt.DefaultCost, got)
	}
}

func TestCheckPasswordRejectsWrongPassword(t *testing.T) {
	hash, err := HashPassword("correct-password", 10)
	if err != nil {
		t.Fatalf("HashPassword devolvió error: %v", err)
	}

	if err := CheckPassword(hash, "wrong-password"); !errors.Is(err, ErrPasswordMismatch) {
		t.Fatalf("se esperaba ErrPasswordMismatch, obtuvo: %v", err)
	}
}

func TestCheckPasswordRejectsInvalidHash(t *testing.T) {
	err := CheckPassword("not-a-bcrypt-hash", "anything")
	if err == nil {
		t.Fatal("se esperaba un error ante un hash inválido")
	}
	if errors.Is(err, ErrPasswordMismatch) {
		t.Fatal("un hash malformado no debe reportarse como simple mismatch")
	}
}
