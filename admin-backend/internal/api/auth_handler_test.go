package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"admin-backend/internal/auth"
	"admin-backend/internal/users"
)

// Pruebas unitarias del handler de autenticación de sesión (tarea 3.4):
// POST /auth/login y GET /auth/me. Validan el login correcto/incorrecto con
// bcrypt y el uso del JWT de sesión emitido/validado, sin duplicar las pruebas
// de bajo nivel ya cubiertas en internal/auth (session_jwt_test.go,
// password_test.go). _Requirements: 1.5, 1.6, 1.7, 2.2_

const (
	authTestSecret         = "test-session-secret-0123456789"
	authTestExpiresMinutes = 60
)

// authTestFakeRepo es un repositorio de usuarios en memoria para las pruebas
// del handler. Permite registrar usuarios por username y forzar un error de
// infraestructura (distinto de users.ErrNotFound) en FindByUsername.
type authTestFakeRepo struct {
	byUsername map[string]*users.User
	findErr    error
}

func newAuthTestFakeRepo() *authTestFakeRepo {
	return &authTestFakeRepo{byUsername: make(map[string]*users.User)}
}

func (r *authTestFakeRepo) FindByUsername(_ context.Context, username string) (*users.User, error) {
	if r.findErr != nil {
		return nil, r.findErr
	}
	u, ok := r.byUsername[username]
	if !ok {
		return nil, users.ErrNotFound
	}
	// Devuelve una copia para evitar mutaciones accidentales desde el handler.
	cp := *u
	return &cp, nil
}

func (r *authTestFakeRepo) Create(_ context.Context, user *users.User) error {
	if user == nil {
		return errors.New("usuario nil")
	}
	r.byUsername[user.Username] = user
	return nil
}

func (r *authTestFakeRepo) ExistsByUsername(_ context.Context, username string) (bool, error) {
	_, ok := r.byUsername[username]
	return ok, nil
}

func (r *authTestFakeRepo) Count(_ context.Context) (int64, error) {
	return int64(len(r.byUsername)), nil
}

// authTestNewUser crea un usuario con un hash bcrypt válido para la contraseña
// dada, listo para almacenarse en el repositorio falso.
func authTestNewUser(t *testing.T, id, username, password, role string) *users.User {
	t.Helper()
	hash, err := auth.HashPassword(password, 4) // coste bajo para acelerar las pruebas
	if err != nil {
		t.Fatalf("no se pudo generar el hash de la contraseña de prueba: %v", err)
	}
	return &users.User{
		ID:           id,
		Username:     username,
		PasswordHash: hash,
		Role:         role,
	}
}

// authTestResponse refleja el envoltorio normalizado del BFF para decodificar
// las respuestas en las aserciones.
type authTestResponse struct {
	OK        bool            `json:"ok"`
	Data      json.RawMessage `json:"data"`
	ErrorCode string          `json:"errorCode"`
	Message   string          `json:"message"`
}

type authTestLoginData struct {
	Token string `json:"token"`
	User  struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		Role     string `json:"role"`
	} `json:"user"`
}

// authTestDoLogin construye el router con el handler y ejecuta POST /auth/login
// con el cuerpo dado, devolviendo el recorder de la respuesta.
func authTestDoLogin(repo users.Repository, body string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	h := NewAuthHandler(repo, newFakeRolesRepo(), authTestSecret, authTestExpiresMinutes)

	router := gin.New()
	router.POST("/auth/login", h.Login)

	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func authTestDecode(t *testing.T, rec *httptest.ResponseRecorder) authTestResponse {
	t.Helper()
	var resp authTestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("no se pudo decodificar la respuesta: %v (cuerpo: %s)", err, rec.Body.String())
	}
	return resp
}

func TestLogin_Success_ReturnsTokenAndPublicUser(t *testing.T) {
	repo := newAuthTestFakeRepo()
	user := authTestNewUser(t, "11111111-1111-1111-1111-111111111111", "admin", "s3cret-pass", users.RoleAdmin)
	repo.byUsername[user.Username] = user

	rec := authTestDoLogin(repo, `{"username":"admin","password":"s3cret-pass"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperaba 200, obtuvo %d (cuerpo: %s)", rec.Code, rec.Body.String())
	}

	resp := authTestDecode(t, rec)
	if !resp.OK {
		t.Fatalf("esperaba ok=true, obtuvo ok=false (cuerpo: %s)", rec.Body.String())
	}

	var data authTestLoginData
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("no se pudo decodificar data: %v", err)
	}

	if data.Token == "" {
		t.Fatal("esperaba un token no vacío")
	}
	if data.User.ID != user.ID {
		t.Errorf("id esperado %q, obtuvo %q", user.ID, data.User.ID)
	}
	if data.User.Username != "admin" {
		t.Errorf("username esperado %q, obtuvo %q", "admin", data.User.Username)
	}
	if data.User.Role != users.RoleAdmin {
		t.Errorf("rol esperado %q, obtuvo %q", users.RoleAdmin, data.User.Role)
	}

	// El hash de la contraseña nunca debe filtrarse en la respuesta.
	if strings.Contains(rec.Body.String(), user.PasswordHash) {
		t.Error("la respuesta no debe contener el hash de la contraseña")
	}
	if strings.Contains(strings.ToLower(rec.Body.String()), "password_hash") {
		t.Error("la respuesta no debe exponer el campo password_hash")
	}

	// El token emitido debe validarse con el mismo secreto y portar los claims
	// correctos (sub/username/role).
	claims, err := auth.ValidateSessionToken(data.Token, authTestSecret)
	if err != nil {
		t.Fatalf("el token emitido debería ser válido, error: %v", err)
	}
	if claims.UserID() != user.ID {
		t.Errorf("sub esperado %q, obtuvo %q", user.ID, claims.UserID())
	}
	if claims.Username != "admin" {
		t.Errorf("claim username esperado %q, obtuvo %q", "admin", claims.Username)
	}
	if claims.Role != users.RoleAdmin {
		t.Errorf("claim role esperado %q, obtuvo %q", users.RoleAdmin, claims.Role)
	}
}

func TestLogin_WrongPassword_Returns401Generic(t *testing.T) {
	repo := newAuthTestFakeRepo()
	user := authTestNewUser(t, "id-1", "admin", "correct-pass", users.RoleAdmin)
	repo.byUsername[user.Username] = user

	rec := authTestDoLogin(repo, `{"username":"admin","password":"wrong-pass"}`)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("esperaba 401, obtuvo %d (cuerpo: %s)", rec.Code, rec.Body.String())
	}
	resp := authTestDecode(t, rec)
	if resp.OK {
		t.Error("esperaba ok=false")
	}
	if resp.Message != "Credenciales incorrectas" {
		t.Errorf("mensaje genérico esperado, obtuvo %q", resp.Message)
	}
}

func TestLogin_UnknownUsername_SameGenericMessage(t *testing.T) {
	repo := newAuthTestFakeRepo()
	// Repositorio sin el usuario solicitado -> FindByUsername devuelve ErrNotFound.

	rec := authTestDoLogin(repo, `{"username":"nadie","password":"cualquiera"}`)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("esperaba 401, obtuvo %d (cuerpo: %s)", rec.Code, rec.Body.String())
	}
	resp := authTestDecode(t, rec)
	if resp.OK {
		t.Error("esperaba ok=false")
	}
	// El mensaje debe ser idéntico al de contraseña incorrecta para no permitir
	// enumeración de usuarios (Req. 1.7).
	if resp.Message != "Credenciales incorrectas" {
		t.Errorf("se esperaba el mismo mensaje genérico que para contraseña incorrecta, obtuvo %q", resp.Message)
	}
}

func TestLogin_EmptyCredentials_Returns401(t *testing.T) {
	cases := map[string]string{
		"username vacío": `{"username":"","password":"algo"}`,
		"password vacío": `{"username":"admin","password":""}`,
		"ambos vacíos":   `{"username":"","password":""}`,
	}

	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			repo := newAuthTestFakeRepo()
			user := authTestNewUser(t, "id-1", "admin", "correct-pass", users.RoleAdmin)
			repo.byUsername[user.Username] = user

			rec := authTestDoLogin(repo, body)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("esperaba 401, obtuvo %d (cuerpo: %s)", rec.Code, rec.Body.String())
			}
			resp := authTestDecode(t, rec)
			if resp.OK {
				t.Error("esperaba ok=false")
			}
			if resp.Message != "Credenciales incorrectas" {
				t.Errorf("mensaje genérico esperado, obtuvo %q", resp.Message)
			}
		})
	}
}

func TestLogin_MalformedJSON_Returns400(t *testing.T) {
	repo := newAuthTestFakeRepo()

	rec := authTestDoLogin(repo, `{"username":"admin", "password":`) // JSON incompleto

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("esperaba 400, obtuvo %d (cuerpo: %s)", rec.Code, rec.Body.String())
	}
	resp := authTestDecode(t, rec)
	if resp.OK {
		t.Error("esperaba ok=false")
	}
}

func TestLogin_RepositoryInfraError_Returns500(t *testing.T) {
	repo := newAuthTestFakeRepo()
	repo.findErr = errors.New("fallo de conexión a la base de datos") // no es ErrNotFound

	rec := authTestDoLogin(repo, `{"username":"admin","password":"s3cret-pass"}`)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("esperaba 500, obtuvo %d (cuerpo: %s)", rec.Code, rec.Body.String())
	}
	resp := authTestDecode(t, rec)
	if resp.OK {
		t.Error("esperaba ok=false")
	}
}

func TestMe_WithClaims_ReturnsIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newAuthTestFakeRepo()
	h := NewAuthHandler(repo, newFakeRolesRepo(), authTestSecret, authTestExpiresMinutes)

	router := gin.New()
	// Middleware de prueba que simula al middleware de sesión depositando los
	// claims validados en el contexto.
	router.GET("/auth/me", func(c *gin.Context) {
		claims := &auth.SessionClaims{Username: "admin", Role: users.RoleAdmin}
		claims.Subject = "id-42"
		SetSessionClaims(c, claims)
		c.Next()
	}, h.Me)

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperaba 200, obtuvo %d (cuerpo: %s)", rec.Code, rec.Body.String())
	}
	resp := authTestDecode(t, rec)
	if !resp.OK {
		t.Fatalf("esperaba ok=true (cuerpo: %s)", rec.Body.String())
	}

	var pu struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		Role     string `json:"role"`
	}
	if err := json.Unmarshal(resp.Data, &pu); err != nil {
		t.Fatalf("no se pudo decodificar data: %v", err)
	}
	if pu.ID != "id-42" {
		t.Errorf("id esperado %q, obtuvo %q", "id-42", pu.ID)
	}
	if pu.Username != "admin" {
		t.Errorf("username esperado %q, obtuvo %q", "admin", pu.Username)
	}
	if pu.Role != users.RoleAdmin {
		t.Errorf("rol esperado %q, obtuvo %q", users.RoleAdmin, pu.Role)
	}
}

// --- RefreshPermissions (POST /auth/refresh-permissions) -----------------

// refreshTestRouter monta POST /auth/refresh-permissions con un middleware que
// deposita los claims dados (nil => no deposita claims, simulando sesión
// ausente).
func refreshTestRouter(h *AuthHandler, claims *auth.SessionClaims) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/auth/refresh-permissions", func(c *gin.Context) {
		if claims != nil {
			SetSessionClaims(c, claims)
		}
		c.Next()
	}, h.RefreshPermissions)
	return router
}

func refreshClaimsFor(userID string) *auth.SessionClaims {
	claims := &auth.SessionClaims{Username: "admin", Role: users.RoleAdmin}
	claims.Subject = userID
	return claims
}

func TestRefreshPermissions_Success_ReturnsTokenAndPermissions(t *testing.T) {
	repo := newAuthTestFakeRepo()
	user := authTestNewUser(t, "id-7", "admin", "pass", users.RoleAdmin)
	user.IsActive = true
	repo.byUsername[user.Username] = user

	rolesRepo := newFakeRolesRepo()
	rolesRepo.setPerms(user.ID, testAllPermissions("gt"))

	h := NewAuthHandler(repo, rolesRepo, authTestSecret, authTestExpiresMinutes)
	router := refreshTestRouter(h, refreshClaimsFor(user.ID))

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh-permissions", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperaba 200, obtuvo %d (cuerpo: %s)", rec.Code, rec.Body.String())
	}
	resp := authTestDecode(t, rec)
	if !resp.OK {
		t.Fatalf("esperaba ok=true (cuerpo: %s)", rec.Body.String())
	}

	var data struct {
		Token       string            `json:"token"`
		Permissions []json.RawMessage `json:"permissions"`
		Modules     []json.RawMessage `json:"modules"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("no se pudo decodificar data: %v", err)
	}
	if data.Token == "" {
		t.Fatal("esperaba un token no vacío")
	}
	claims, err := auth.ValidateSessionToken(data.Token, authTestSecret)
	if err != nil {
		t.Fatalf("el token reemitido debería ser válido, error: %v", err)
	}
	if claims.UserID() != user.ID {
		t.Errorf("sub esperado %q, obtuvo %q", user.ID, claims.UserID())
	}
	if len(data.Permissions) == 0 {
		t.Error("esperaba permisos coherentes con PermissionsForUser")
	}
}

func TestRefreshPermissions_NoClaims_Returns401(t *testing.T) {
	repo := newAuthTestFakeRepo()
	h := NewAuthHandler(repo, newFakeRolesRepo(), authTestSecret, authTestExpiresMinutes)
	router := refreshTestRouter(h, nil)

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh-permissions", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("esperaba 401, obtuvo %d (cuerpo: %s)", rec.Code, rec.Body.String())
	}
}

func TestRefreshPermissions_InactiveUser_Returns401NoToken(t *testing.T) {
	repo := newAuthTestFakeRepo()
	user := authTestNewUser(t, "id-8", "admin", "pass", users.RoleAdmin)
	user.IsActive = false
	repo.byUsername[user.Username] = user

	h := NewAuthHandler(repo, newFakeRolesRepo(), authTestSecret, authTestExpiresMinutes)
	router := refreshTestRouter(h, refreshClaimsFor(user.ID))

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh-permissions", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("esperaba 401, obtuvo %d (cuerpo: %s)", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "\"token\"") {
		t.Error("una sesión inválida no debe devolver token")
	}
}

func TestRefreshPermissions_UnknownUser_Returns401(t *testing.T) {
	repo := newAuthTestFakeRepo() // sin el usuario -> FindByID devuelve ErrNotFound
	h := NewAuthHandler(repo, newFakeRolesRepo(), authTestSecret, authTestExpiresMinutes)
	router := refreshTestRouter(h, refreshClaimsFor("id-inexistente"))

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh-permissions", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("esperaba 401, obtuvo %d (cuerpo: %s)", rec.Code, rec.Body.String())
	}
}

func TestRefreshPermissions_PermissionsError_Returns500(t *testing.T) {
	repo := newAuthTestFakeRepo()
	user := authTestNewUser(t, "id-9", "admin", "pass", users.RoleAdmin)
	user.IsActive = true
	repo.byUsername[user.Username] = user

	rolesRepo := newFakeRolesRepo()
	rolesRepo.permsErr = errors.New("fallo resolviendo permisos")

	h := NewAuthHandler(repo, rolesRepo, authTestSecret, authTestExpiresMinutes)
	router := refreshTestRouter(h, refreshClaimsFor(user.ID))

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh-permissions", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("esperaba 500, obtuvo %d (cuerpo: %s)", rec.Code, rec.Body.String())
	}
}

func TestMe_WithoutClaims_Returns401(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newAuthTestFakeRepo()
	h := NewAuthHandler(repo, newFakeRolesRepo(), authTestSecret, authTestExpiresMinutes)

	router := gin.New()
	// Sin middleware que deposite claims: el handler debe responder 401.
	router.GET("/auth/me", h.Me)

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("esperaba 401, obtuvo %d (cuerpo: %s)", rec.Code, rec.Body.String())
	}
	resp := authTestDecode(t, rec)
	if resp.OK {
		t.Error("esperaba ok=false")
	}
}
