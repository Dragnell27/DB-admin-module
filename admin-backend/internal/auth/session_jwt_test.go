package auth

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"admin-backend/internal/users"
)

const testSecret = "super-secret-signing-key"

func testUser() users.User {
	return users.User{
		ID:       "11111111-1111-1111-1111-111111111111",
		Username: "admin-user",
		Role:     users.RoleAdmin,
	}
}

func TestIssueSessionTokenEmbedsIdentityAndRole(t *testing.T) {
	user := testUser()

	token, err := IssueSessionToken(user, EffectivePermissions{}, nil, testSecret, 60)
	if err != nil {
		t.Fatalf("IssueSessionToken devolvió error: %v", err)
	}
	if token == "" {
		t.Fatal("el token emitido no debe estar vacío")
	}

	claims, err := ValidateSessionToken(token, testSecret)
	if err != nil {
		t.Fatalf("ValidateSessionToken devolvió error para un token recién emitido: %v", err)
	}

	if claims.UserID() != user.ID {
		t.Errorf("sub esperado %q, obtuvo %q", user.ID, claims.UserID())
	}
	if claims.Username != user.Username {
		t.Errorf("username esperado %q, obtuvo %q", user.Username, claims.Username)
	}
	if claims.Role != user.Role {
		t.Errorf("role esperado %q, obtuvo %q", user.Role, claims.Role)
	}
}

func TestIssueSessionTokenSetsExpiryFromMinutes(t *testing.T) {
	const minutes = 30
	before := time.Now()

	token, err := IssueSessionToken(testUser(), EffectivePermissions{}, nil, testSecret, minutes)
	if err != nil {
		t.Fatalf("IssueSessionToken devolvió error: %v", err)
	}

	claims, err := ValidateSessionToken(token, testSecret)
	if err != nil {
		t.Fatalf("ValidateSessionToken devolvió error: %v", err)
	}

	if claims.ExpiresAt == nil {
		t.Fatal("el claim exp no debe ser nil")
	}

	expExpected := before.Add(minutes * time.Minute)
	diff := claims.ExpiresAt.Time.Sub(expExpected)
	if diff < -2*time.Second || diff > 2*time.Second {
		t.Errorf("exp esperado ~%v, obtuvo %v (diff %v)", expExpected, claims.ExpiresAt.Time, diff)
	}
}

func TestIssueSessionTokenRejectsEmptySecret(t *testing.T) {
	if _, err := IssueSessionToken(testUser(), EffectivePermissions{}, nil, "", 60); !errors.Is(err, ErrEmptySecret) {
		t.Fatalf("se esperaba ErrEmptySecret, obtuvo: %v", err)
	}
}

func TestIssueSessionTokenRejectsNonPositiveExpiry(t *testing.T) {
	if _, err := IssueSessionToken(testUser(), EffectivePermissions{}, nil, testSecret, 0); err == nil {
		t.Fatal("se esperaba un error con expiración 0")
	}
	if _, err := IssueSessionToken(testUser(), EffectivePermissions{}, nil, testSecret, -5); err == nil {
		t.Fatal("se esperaba un error con expiración negativa")
	}
}

func TestValidateSessionTokenRejectsWrongSecret(t *testing.T) {
	token, err := IssueSessionToken(testUser(), EffectivePermissions{}, nil, testSecret, 60)
	if err != nil {
		t.Fatalf("IssueSessionToken devolvió error: %v", err)
	}

	if _, err := ValidateSessionToken(token, "a-different-secret"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("se esperaba ErrInvalidToken con secreto incorrecto, obtuvo: %v", err)
	}
}

func TestValidateSessionTokenRejectsExpiredToken(t *testing.T) {
	// Construye manualmente un token ya expirado para no depender de dormir.
	now := time.Now()
	claims := SessionClaims{
		Username: "admin-user",
		Role:     users.RoleAdmin,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "id",
			IssuedAt:  jwt.NewNumericDate(now.Add(-2 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(now.Add(-1 * time.Hour)),
		},
	}
	raw := jwt.NewWithClaims(sessionSigningMethod, claims)
	signed, err := raw.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("no se pudo firmar el token de prueba: %v", err)
	}

	if _, err := ValidateSessionToken(signed, testSecret); !errors.Is(err, ErrExpiredToken) {
		t.Fatalf("se esperaba ErrExpiredToken, obtuvo: %v", err)
	}
}

func TestValidateSessionTokenRejectsMalformedToken(t *testing.T) {
	if _, err := ValidateSessionToken("not.a.jwt", testSecret); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("se esperaba ErrInvalidToken para un token malformado, obtuvo: %v", err)
	}
}

func TestValidateSessionTokenRejectsEmptySecret(t *testing.T) {
	if _, err := ValidateSessionToken("whatever", ""); !errors.Is(err, ErrEmptySecret) {
		t.Fatalf("se esperaba ErrEmptySecret, obtuvo: %v", err)
	}
}

func TestValidateSessionTokenRejectsUnexpectedSigningMethod(t *testing.T) {
	// Un token firmado con "none" (alg distinto a HS256) debe rechazarse.
	claims := SessionClaims{
		Username: "admin-user",
		Role:     users.RoleAdmin,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "id",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	raw := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	signed, err := raw.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("no se pudo firmar el token none de prueba: %v", err)
	}

	if _, err := ValidateSessionToken(signed, testSecret); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("se esperaba ErrInvalidToken ante método de firma inesperado, obtuvo: %v", err)
	}
}

func TestIssueSessionTokenProducesThreeSegmentJWT(t *testing.T) {
	token, err := IssueSessionToken(testUser(), EffectivePermissions{}, nil, testSecret, 60)
	if err != nil {
		t.Fatalf("IssueSessionToken devolvió error: %v", err)
	}
	if parts := strings.Split(token, "."); len(parts) != 3 {
		t.Fatalf("un JWT debe tener 3 segmentos, obtuvo %d", len(parts))
	}
}
