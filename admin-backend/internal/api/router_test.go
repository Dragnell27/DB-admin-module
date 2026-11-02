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
	"admin-backend/internal/config"
	"admin-backend/internal/ott"
	"admin-backend/internal/users"
)

// Pruebas de integración de las rutas del BFF (tarea 9.2). Verifican el
// cableado del router (NewRouter): el healthcheck público, la protección por
// sesión (401 sin token), la protección por rol en las rutas de escritura (403
// para viewer), la validación de cuerpo antes del proxy OTT (400 con token
// admin), el login/me y las cabeceras CORS.
//
// Importante sobre OTT: los handlers de recurso usan un *ott.Client cuyo
// transporte interno NO es inyectable desde el paquete api (solo desde pruebas
// in-package de ott). Por eso estas pruebas se diseñan deliberadamente para
// cubrir únicamente comportamientos que se resuelven ANTES de cualquier llamada
// a OTT: 401 (sesión), 403 (rol) y 400 (validación de entrada). El éxito del
// proxy extremo a extremo (que requiere un doble de OTT) queda cubierto por la
// tarea 19.1.
//
// _Requirements: 5.3, 20.1, 20.2, 21.4_

const (
	routerTestSecret         = "router-test-session-secret-0123456789"
	routerTestExpiresMinutes = 60
	routerTestCORSOrigin     = "https://admin.example.com"
	// routerTestCountry es el país efectivo por defecto del router de prueba.
	// RequirePermission autoriza las ternas en el país efectivo, por lo que el
	// router debe resolver uno (si no, rechazaría de forma defensiva) y los
	// tokens de prueba portan permisos para este país.
	routerTestCountry = "gt"
)

// routerTestFakeRepo es un repositorio de usuarios en memoria para construir un
// AuthHandler real sin base de datos. Permite registrar usuarios por username.
type routerTestFakeRepo struct {
	byUsername map[string]*users.User
}

func newRouterTestFakeRepo() *routerTestFakeRepo {
	return &routerTestFakeRepo{byUsername: make(map[string]*users.User)}
}

func (r *routerTestFakeRepo) FindByUsername(_ context.Context, username string) (*users.User, error) {
	u, ok := r.byUsername[username]
	if !ok {
		return nil, users.ErrNotFound
	}
	cp := *u
	return &cp, nil
}

func (r *routerTestFakeRepo) Create(_ context.Context, user *users.User) error {
	if user == nil {
		return errors.New("usuario nil")
	}
	r.byUsername[user.Username] = user
	return nil
}

func (r *routerTestFakeRepo) ExistsByUsername(_ context.Context, username string) (bool, error) {
	_, ok := r.byUsername[username]
	return ok, nil
}

func (r *routerTestFakeRepo) Count(_ context.Context) (int64, error) {
	return int64(len(r.byUsername)), nil
}

// routerTestResponse refleja el envoltorio normalizado del BFF para decodificar
// y hacer aserciones sobre {ok, data, errorCode, message}.
type routerTestResponse struct {
	OK        bool            `json:"ok"`
	Data      json.RawMessage `json:"data"`
	ErrorCode string          `json:"errorCode"`
	Message   string          `json:"message"`
}

// routerTestEnv agrupa el router construido y las dependencias compartidas por
// las pruebas (el repositorio para registrar usuarios de login).
type routerTestEnv struct {
	router *gin.Engine
	repo   *routerTestFakeRepo
}

// routerTestNewEnv construye un router realista con un AuthHandler real (sobre
// un repo falso) y los handlers de recurso reales conectados a un *ott.Client
// mínimo. El OTT nunca se invoca en estas pruebas porque todos los casos se
// resuelven antes del proxy (401/403/400); el cliente existe solo para que el
// wiring sea el real.
func routerTestNewEnv(t *testing.T) *routerTestEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)

	repo := newRouterTestFakeRepo()
	authHandler := NewAuthHandler(repo, newFakeRolesRepo(), routerTestSecret, routerTestExpiresMinutes)

	// Cliente OTT mínimo: no se usará realmente en estas pruebas (ver nota de
	// cabecera). Se construye con el país activo del BFF para completar el
	// wiring de los handlers de recurso.
	const activeCountry = "gt"
	ottClient := ott.NewClient(&config.Config{
		OTTBaseURL:       "http://ott.invalid",
		OTTActiveCountry: activeCountry,
		OTTCredentialsByCountry: map[string]config.OTTCredentials{
			activeCountry: {ClientID: "test-client", ClientSecret: "test-secret"},
		},
	})

	deps := RouterDeps{
		SessionSecret:       routerTestSecret,
		CORSAllowedOrigin:   routerTestCORSOrigin,
		DefaultCountry:      activeCountry,
		ConfiguredCountries: []string{activeCountry},
		Auth:                authHandler,
		Customers:           NewCustomersHandler(ottClient, activeCountry, newTestCache()),
		Plans:               NewPlansHandler(ottClient, activeCountry, newTestCache()),
		Subscriptions:       NewSubscriptionsHandler(ottClient, activeCountry, newTestCache()),
		Cards:               NewCardsHandler(ottClient, activeCountry),
	}

	return &routerTestEnv{router: NewRouter(deps), repo: repo}
}

// routerTestToken emite un JWT de sesión válido para el rol dado, usando el
// mismo secreto con el que el router monta el middleware de sesión.
func routerTestToken(t *testing.T, role string) string {
	t.Helper()
	user := users.User{
		ID:       "00000000-0000-0000-0000-000000000001",
		Username: role + "-user",
		Role:     role,
	}
	// El token porta los permisos efectivos propios del rol en el país efectivo
	// del router (routerTestCountry): admin → todas las acciones; viewer → solo
	// lectura. Así RequirePermission autoriza/deniega según corresponde.
	perms := testPermsForRole(role, routerTestCountry)
	token, err := auth.IssueSessionToken(user, perms, nil, routerTestSecret, routerTestExpiresMinutes)
	if err != nil {
		t.Fatalf("no se pudo emitir el token de prueba (%s): %v", role, err)
	}
	return token
}

// routerTestDo ejecuta una petición contra el router. token vacío => sin
// cabecera Authorization. body vacío => sin cuerpo.
func routerTestDo(env *routerTestEnv, method, target, token, body string) *httptest.ResponseRecorder {
	var reader *strings.Reader
	if body != "" {
		reader = strings.NewReader(body)
	} else {
		reader = strings.NewReader("")
	}
	req := httptest.NewRequest(method, target, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, req)
	return rec
}

func routerTestDecode(t *testing.T, rec *httptest.ResponseRecorder) routerTestResponse {
	t.Helper()
	var resp routerTestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("no se pudo decodificar la respuesta: %v (cuerpo: %s)", err, rec.Body.String())
	}
	return resp
}

// --- Healthcheck público (Req. 21.4, 20.3) ---

func TestRouter_Health_PublicReturnsOKEnvelope(t *testing.T) {
	env := routerTestNewEnv(t)

	rec := routerTestDo(env, http.MethodGet, "/health", "", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("esperaba 200, obtuvo %d (cuerpo: %s)", rec.Code, rec.Body.String())
	}
	resp := routerTestDecode(t, rec)
	if !resp.OK {
		t.Fatalf("esperaba ok=true en el envoltorio normalizado (cuerpo: %s)", rec.Body.String())
	}
}

// --- Rutas protegidas sin token => 401 con envoltorio de error (Req. 20.1) ---

func TestRouter_ProtectedRoutesWithoutToken_Return401(t *testing.T) {
	env := routerTestNewEnv(t)

	cases := []struct {
		method string
		target string
	}{
		{http.MethodGet, "/auth/me"},
		{http.MethodGet, "/customers"},
		{http.MethodPost, "/customers"},
		{http.MethodPut, "/customers"},
		{http.MethodGet, "/plans"},
		{http.MethodPost, "/plans"},
		{http.MethodPut, "/plans"},
		{http.MethodDelete, "/plans"},
		{http.MethodGet, "/subscriptions"},
		{http.MethodPost, "/subscriptions"},
		{http.MethodPut, "/subscriptions/change"},
		{http.MethodPost, "/subscriptions/cancel"},
		{http.MethodGet, "/cards"},
		{http.MethodPost, "/cards"},
		{http.MethodDelete, "/cards"},
	}

	for _, tc := range cases {
		t.Run(tc.method+" "+tc.target, func(t *testing.T) {
			rec := routerTestDo(env, tc.method, tc.target, "", "")
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("esperaba 401, obtuvo %d (cuerpo: %s)", rec.Code, rec.Body.String())
			}
			resp := routerTestDecode(t, rec)
			if resp.OK {
				t.Error("esperaba ok=false en el envoltorio de error 401")
			}
			if resp.ErrorCode == "" {
				t.Error("esperaba un errorCode no vacío en el envoltorio de error 401")
			}
		})
	}
}

// --- Rutas de escritura con token VIEWER => 403 (RequireAdmin, Req. 5.3) ---
// RequireAdmin se evalúa después del middleware de sesión y antes del handler,
// por lo que no se realiza ninguna llamada a OTT.

func TestRouter_WriteRoutesWithViewerToken_Return403(t *testing.T) {
	env := routerTestNewEnv(t)
	viewer := routerTestToken(t, users.RoleViewer)

	cases := []struct {
		method string
		target string
	}{
		{http.MethodPost, "/customers"},
		{http.MethodPut, "/customers"},
		{http.MethodPost, "/plans"},
		{http.MethodPut, "/plans"},
		{http.MethodDelete, "/plans"},
		{http.MethodPost, "/subscriptions"},
		{http.MethodPut, "/subscriptions/change"},
		{http.MethodPost, "/subscriptions/cancel"},
		{http.MethodPost, "/cards"},
		{http.MethodDelete, "/cards"},
	}

	for _, tc := range cases {
		t.Run(tc.method+" "+tc.target, func(t *testing.T) {
			// Cuerpo válido-ish: no importa, RequireAdmin corta antes de validar.
			rec := routerTestDo(env, tc.method, tc.target, viewer, `{}`)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("esperaba 403, obtuvo %d (cuerpo: %s)", rec.Code, rec.Body.String())
			}
			resp := routerTestDecode(t, rec)
			if resp.OK {
				t.Error("esperaba ok=false en el envoltorio de error 403")
			}
		})
	}
}

// --- Lectura con token VIEWER: la sesión y el rol NO bloquean GET ---
// GET /auth/me no requiere admin; con un viewer válido debe responder 200 y la
// identidad tomada de los claims (sin tocar OTT).

func TestRouter_Me_WithViewerToken_ReturnsIdentity(t *testing.T) {
	env := routerTestNewEnv(t)
	viewer := routerTestToken(t, users.RoleViewer)

	rec := routerTestDo(env, http.MethodGet, "/auth/me", viewer, "")

	if rec.Code != http.StatusOK {
		t.Fatalf("esperaba 200, obtuvo %d (cuerpo: %s)", rec.Code, rec.Body.String())
	}
	resp := routerTestDecode(t, rec)
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
	if pu.Role != users.RoleViewer {
		t.Errorf("rol esperado %q, obtuvo %q", users.RoleViewer, pu.Role)
	}
	if pu.Username != users.RoleViewer+"-user" {
		t.Errorf("username esperado %q, obtuvo %q", users.RoleViewer+"-user", pu.Username)
	}
}

// --- Escritura con token ADMIN pero cuerpo inválido => 400 antes del proxy ---
// La validación espejo del BFF rechaza el cuerpo antes de llamar a OTT, de modo
// que estos casos no requieren un doble de OTT.

func TestRouter_WriteRoutesWithAdminToken_InvalidBody_Return400(t *testing.T) {
	env := routerTestNewEnv(t)
	admin := routerTestToken(t, users.RoleAdmin)

	cases := []struct {
		name   string
		method string
		target string
		body   string
	}{
		{
			name:   "POST /plans con cuerpo vacío",
			method: http.MethodPost,
			target: "/plans",
			body:   `{}`,
		},
		{
			name:   "POST /customers con cuerpo vacío (campos requeridos faltantes)",
			method: http.MethodPost,
			target: "/customers",
			body:   `{}`,
		},
		{
			name:   "POST /subscriptions con IDs no-UUID",
			method: http.MethodPost,
			target: "/subscriptions",
			body:   `{"plan_id":"no-es-uuid","customer_id":"tampoco","payment_method":"card","payment_method_token":"x"}`,
		},
		{
			name:   "POST /subscriptions/cancel con customer_id no-UUID",
			method: http.MethodPost,
			target: "/subscriptions/cancel",
			body:   `{"customer_id":"no-es-uuid"}`,
		},
		{
			name:   "POST /cards con cuerpo inválido",
			method: http.MethodPost,
			target: "/cards",
			body:   `{"customer_id":"no-es-uuid","card_number":"","expiration_date":"","card_token":"","payment_method_name":"","card_franchise":""}`,
		},
		{
			name:   "DELETE /cards sin card_id",
			method: http.MethodDelete,
			target: "/cards",
			body:   `{}`,
		},
		{
			name:   "DELETE /plans sin plan_id",
			method: http.MethodDelete,
			target: "/plans",
			body:   `{}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := routerTestDo(env, tc.method, tc.target, admin, tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("esperaba 400, obtuvo %d (cuerpo: %s)", rec.Code, rec.Body.String())
			}
			resp := routerTestDecode(t, rec)
			if resp.OK {
				t.Error("esperaba ok=false en el envoltorio de error 400")
			}
		})
	}
}

// --- Login: POST /auth/login público con repo falso (Req. 20.1) ---

func TestRouter_Login_GoodCredentials_Returns200AndToken(t *testing.T) {
	env := routerTestNewEnv(t)

	hash, err := auth.HashPassword("s3cret-pass", 4)
	if err != nil {
		t.Fatalf("no se pudo generar el hash de prueba: %v", err)
	}
	env.repo.byUsername["admin"] = &users.User{
		ID:           "11111111-1111-1111-1111-111111111111",
		Username:     "admin",
		PasswordHash: hash,
		Role:         users.RoleAdmin,
	}

	rec := routerTestDo(env, http.MethodPost, "/auth/login", "", `{"username":"admin","password":"s3cret-pass"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperaba 200, obtuvo %d (cuerpo: %s)", rec.Code, rec.Body.String())
	}
	resp := routerTestDecode(t, rec)
	if !resp.OK {
		t.Fatalf("esperaba ok=true (cuerpo: %s)", rec.Body.String())
	}

	var data struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("no se pudo decodificar data: %v", err)
	}
	if data.Token == "" {
		t.Fatal("esperaba un token no vacío en el login exitoso")
	}
	// El token emitido debe ser utilizable contra las rutas protegidas.
	if _, err := auth.ValidateSessionToken(data.Token, routerTestSecret); err != nil {
		t.Fatalf("el token emitido debería validar con el secreto del router: %v", err)
	}
}

func TestRouter_Login_BadCredentials_Returns401(t *testing.T) {
	env := routerTestNewEnv(t)

	hash, err := auth.HashPassword("correct-pass", 4)
	if err != nil {
		t.Fatalf("no se pudo generar el hash de prueba: %v", err)
	}
	env.repo.byUsername["admin"] = &users.User{
		ID:           "id-1",
		Username:     "admin",
		PasswordHash: hash,
		Role:         users.RoleAdmin,
	}

	rec := routerTestDo(env, http.MethodPost, "/auth/login", "", `{"username":"admin","password":"wrong-pass"}`)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("esperaba 401, obtuvo %d (cuerpo: %s)", rec.Code, rec.Body.String())
	}
	resp := routerTestDecode(t, rec)
	if resp.OK {
		t.Error("esperaba ok=false ante credenciales inválidas")
	}
}

// --- CORS (design.md Security → CORS) ---

func TestRouter_CORS_PreflightReturns204WithAllowOrigin(t *testing.T) {
	env := routerTestNewEnv(t)

	req := httptest.NewRequest(http.MethodOptions, "/plans", nil)
	req.Header.Set("Origin", routerTestCORSOrigin)
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("esperaba 204 en el preflight, obtuvo %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != routerTestCORSOrigin {
		t.Errorf("Access-Control-Allow-Origin esperado %q, obtuvo %q", routerTestCORSOrigin, got)
	}
	// El selector de país envía X-Country, por lo que debe estar permitido en el
	// preflight; de lo contrario el navegador bloquea la petición por CORS.
	if got := rec.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(got, "X-Country") {
		t.Errorf("Access-Control-Allow-Headers = %q; debe incluir X-Country", got)
	}
}

func TestRouter_CORS_NormalRequestIncludesCORSHeaders(t *testing.T) {
	env := routerTestNewEnv(t)

	// Una petición normal (sin token) a una ruta protegida igualmente debe
	// llevar las cabeceras CORS (el middleware CORS corre antes de la sesión).
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", routerTestCORSOrigin)
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != routerTestCORSOrigin {
		t.Errorf("Access-Control-Allow-Origin esperado %q, obtuvo %q", routerTestCORSOrigin, got)
	}
}
