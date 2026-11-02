package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"admin-backend/internal/auth"
	"admin-backend/internal/users"
)

const testSecret = "super-secret-signing-key"

func init() {
	gin.SetMode(gin.TestMode)
}

// newToken emite un JWT de sesión válido para un usuario con el rol dado.
func newToken(t *testing.T, id, username, role string, expiresMinutes int) string {
	t.Helper()
	token, err := auth.IssueSessionToken(
		users.User{ID: id, Username: username, Role: role},
		auth.EffectivePermissions{},
		nil,
		testSecret,
		expiresMinutes,
	)
	if err != nil {
		t.Fatalf("no se pudo emitir el token de prueba: %v", err)
	}
	return token
}

// expiredToken emite directamente un JWT HS256 ya expirado, firmado con
// testSecret, para ejercitar la rama de token expirado del middleware (la API
// pública IssueSessionToken rechaza expiraciones no positivas).
func expiredToken(t *testing.T) string {
	t.Helper()
	claims := auth.SessionClaims{
		Username: "u",
		Role:     users.RoleAdmin,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "id",
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("no se pudo firmar el token expirado: %v", err)
	}
	return signed
}

// buildRouter arma un router Gin con el middleware de sesión y un handler
// protegido que expone la identidad extraída del contexto.
func buildRouter() *gin.Engine {
	r := gin.New()
	r.GET("/protected", SessionAuth(testSecret), func(c *gin.Context) {
		id, _ := CurrentUserID(c)
		username, _ := CurrentUsername(c)
		role, _ := CurrentRole(c)
		c.JSON(http.StatusOK, gin.H{"id": id, "username": username, "role": role})
	})
	return r
}

func doRequest(r *gin.Engine, authorization string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func decodeError(t *testing.T, w *httptest.ResponseRecorder) errorBody {
	t.Helper()
	var body errorBody
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("no se pudo decodificar el cuerpo de error: %v (body=%q)", err, w.Body.String())
	}
	return body
}

func TestSessionAuthValidTokenPopulatesContext(t *testing.T) {
	r := buildRouter()
	token := newToken(t, "user-id-123", "admin-user", users.RoleAdmin, 60)

	w := doRequest(r, "Bearer "+token)

	if w.Code != http.StatusOK {
		t.Fatalf("se esperaba 200, se obtuvo %d (body=%q)", w.Code, w.Body.String())
	}

	var got map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("no se pudo decodificar la respuesta: %v", err)
	}
	if got["id"] != "user-id-123" {
		t.Errorf("id en contexto = %q, se esperaba %q", got["id"], "user-id-123")
	}
	if got["username"] != "admin-user" {
		t.Errorf("username en contexto = %q, se esperaba %q", got["username"], "admin-user")
	}
	if got["role"] != users.RoleAdmin {
		t.Errorf("role en contexto = %q, se esperaba %q", got["role"], users.RoleAdmin)
	}
}

func TestSessionAuthAcceptsCaseInsensitiveScheme(t *testing.T) {
	r := buildRouter()
	token := newToken(t, "id", "u", users.RoleViewer, 60)

	w := doRequest(r, "bearer "+token)

	if w.Code != http.StatusOK {
		t.Fatalf("se esperaba 200 con esquema en minúsculas, se obtuvo %d", w.Code)
	}
}

func TestSessionAuthMissingHeaderReturns401(t *testing.T) {
	r := buildRouter()

	w := doRequest(r, "")

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("se esperaba 401 sin cabecera, se obtuvo %d", w.Code)
	}
	body := decodeError(t, w)
	if body.OK {
		t.Error("el envoltorio de error debe tener ok=false")
	}
	if body.ErrorCode != "AUTH_MISSING_TOKEN" {
		t.Errorf("errorCode = %q, se esperaba AUTH_MISSING_TOKEN", body.ErrorCode)
	}
}

func TestSessionAuthWrongSchemeReturns401(t *testing.T) {
	r := buildRouter()
	token := newToken(t, "id", "u", users.RoleAdmin, 60)

	w := doRequest(r, "Basic "+token)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("se esperaba 401 con esquema no Bearer, se obtuvo %d", w.Code)
	}
	if ec := decodeError(t, w).ErrorCode; ec != "AUTH_MISSING_TOKEN" {
		t.Errorf("errorCode = %q, se esperaba AUTH_MISSING_TOKEN", ec)
	}
}

func TestSessionAuthEmptyBearerTokenReturns401(t *testing.T) {
	r := buildRouter()

	w := doRequest(r, "Bearer    ")

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("se esperaba 401 con token Bearer vacío, se obtuvo %d", w.Code)
	}
}

func TestSessionAuthInvalidSignatureReturns401(t *testing.T) {
	r := buildRouter()
	// Token firmado con otro secreto: firma inválida bajo testSecret.
	bad, err := auth.IssueSessionToken(
		users.User{ID: "id", Username: "u", Role: users.RoleAdmin},
		auth.EffectivePermissions{},
		nil,
		"otro-secreto-distinto",
		60,
	)
	if err != nil {
		t.Fatalf("no se pudo emitir el token de prueba: %v", err)
	}

	w := doRequest(r, "Bearer "+bad)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("se esperaba 401 con firma inválida, se obtuvo %d", w.Code)
	}
	if ec := decodeError(t, w).ErrorCode; ec != "AUTH_INVALID_TOKEN" {
		t.Errorf("errorCode = %q, se esperaba AUTH_INVALID_TOKEN", ec)
	}
}

func TestSessionAuthMalformedTokenReturns401(t *testing.T) {
	r := buildRouter()

	w := doRequest(r, "Bearer not-a-jwt")

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("se esperaba 401 con token malformado, se obtuvo %d", w.Code)
	}
	if ec := decodeError(t, w).ErrorCode; ec != "AUTH_INVALID_TOKEN" {
		t.Errorf("errorCode = %q, se esperaba AUTH_INVALID_TOKEN", ec)
	}
}

func TestSessionAuthExpiredTokenReturns401(t *testing.T) {
	r := buildRouter()
	expired := expiredToken(t)

	w := doRequest(r, "Bearer "+expired)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("se esperaba 401 con token expirado, se obtuvo %d", w.Code)
	}
	if ec := decodeError(t, w).ErrorCode; ec != "AUTH_TOKEN_EXPIRED" {
		t.Errorf("errorCode = %q, se esperaba AUTH_TOKEN_EXPIRED", ec)
	}
}

func TestContextHelpersReturnFalseWhenAbsent(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	if _, ok := CurrentUserID(c); ok {
		t.Error("CurrentUserID debe devolver false cuando no hay identidad en el contexto")
	}
	if _, ok := CurrentUsername(c); ok {
		t.Error("CurrentUsername debe devolver false cuando no hay identidad en el contexto")
	}
	if _, ok := CurrentRole(c); ok {
		t.Error("CurrentRole debe devolver false cuando no hay identidad en el contexto")
	}
}
