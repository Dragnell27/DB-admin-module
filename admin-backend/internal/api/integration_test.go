package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"

	"admin-backend/internal/auth"
	"admin-backend/internal/config"
	"admin-backend/internal/ott"
	"admin-backend/internal/users"
)

// Prueba de integración extremo a extremo del admin-backend (BFF) — tarea 19.1.
//
// A diferencia de las pruebas de cableado del router (tarea 9.2, router_test.go),
// que deliberadamente NO ejercen el proxy a OTT, esta prueba recorre la ruta
// completa: login → JWT de sesión → operaciones CRUD de Plans a través del
// router real → *ott.Client real → un backend OTT SIMULADO con httptest.Server.
// Así se cubre el camino de proxy completo: autenticación por país contra el
// OTT, cache/reutilización del JWT OTT, adjunción del `Authorization: Bearer`,
// y la normalización de las respuestas estándar OTT hacia el envoltorio
// uniforme del BFF {ok, data, pagination?, errorCode?, message}.
//
// Enfoque (b): se levanta un httptest.Server que imita el backend OTT. Como
// `ott.NewClient` usa un *http.Client real, apuntar OTT_BASE_URL al servidor de
// prueba basta para que el cliente lo invoque de verdad — sin inyectar
// transportes ni modificar código de producción.
//
// _Requirements: 1.5, 1.8, 2.2, 2.4, 9.1, 10.13, 11.3, 12.2, 20.1, 20.2, 20.3_

const (
	integSessionSecret   = "integration-session-secret-0123456789abc"
	integExpiresMinutes  = 60
	integCORSOrigin      = "https://admin.example.com"
	integActiveCountry   = "gt"
	integOTTClientID     = "gt-client-id"
	integOTTClientSecret = "gt-client-secret"
	// integOTTJWT es el JWT OTT que el backend simulado entrega en /authenticate.
	// No necesita ser un JWT real: el cliente lo cachea tal cual y lo reenvía
	// como Bearer. Al no tener formato "a.b.c" válido, el cliente aplica el TTL
	// por defecto (~23h), por lo que se reutiliza durante toda la prueba.
	integOTTJWT = "ott-jwt-for-gt-country"
)

// integRecordedRequest captura lo esencial de una petición recibida por el OTT
// simulado para poder hacer aserciones posteriores (método, ruta, query y el
// token Bearer presentado).
type integRecordedRequest struct {
	Method      string
	Path        string
	RawQuery    string
	BearerToken string
	Body        string
}

// integOTTStub es un backend OTT simulado sobre httptest.Server. Atiende el
// endpoint de autenticación y las operaciones de Plans, respondiendo con la
// variante estándar OTT `resultCode/resultMessage/errorCode/result`. Registra
// todas las peticiones (con un mutex) para verificar el camino de proxy.
type integOTTStub struct {
	server *httptest.Server

	mu       sync.Mutex
	requests []integRecordedRequest
	authHits int
}

// newIntegOTTStub arranca el backend OTT simulado y devuelve el stub. El
// llamador debe cerrar el servidor con Close (vía t.Cleanup).
func newIntegOTTStub(t *testing.T) *integOTTStub {
	t.Helper()
	stub := &integOTTStub{}
	mux := http.NewServeMux()

	// Autenticación por país: el cliente OTT llama POST {baseURL}/authenticate.
	mux.HandleFunc("/authenticate", func(w http.ResponseWriter, r *http.Request) {
		stub.mu.Lock()
		stub.authHits++
		stub.mu.Unlock()

		var creds struct {
			ClientID     string `json:"clientId"`
			ClientSecret string `json:"clientSecret"`
		}
		_ = json.NewDecoder(r.Body).Decode(&creds)
		if creds.ClientID != integOTTClientID || creds.ClientSecret != integOTTClientSecret {
			integWriteOTTError(w, http.StatusUnauthorized, "S401SEC", "credenciales OTT inválidas")
			return
		}
		integWriteOTTJSON(w, http.StatusOK, map[string]any{
			"resultCode":    "200",
			"resultMessage": "OK",
			"result":        map[string]any{"Authorization": integOTTJWT},
		})
	})

	// Recurso Plans: lista/detalle (GET), crear (POST), editar (PUT), eliminar
	// (DELETE). Todas exigen el Bearer del JWT OTT.
	mux.HandleFunc("/plans", stub.handlePlans)

	stub.server = httptest.NewServer(mux)
	t.Cleanup(stub.server.Close)
	return stub
}

// handlePlans implementa el comportamiento del recurso Plans del OTT simulado.
func (s *integOTTStub) handlePlans(w http.ResponseWriter, r *http.Request) {
	s.record(r)

	// Toda llamada protegida debe venir con el Bearer del JWT OTT. Si falta,
	// se simula un 401 de seguridad del OTT.
	if integBearer(r) != integOTTJWT {
		integWriteOTTError(w, http.StatusUnauthorized, "S401SEC", "token OTT ausente o inválido")
		return
	}

	switch r.Method {
	case http.MethodGet:
		// Detalle por plan_id o listado paginado.
		if planID := r.URL.Query().Get("plan_id"); planID != "" {
			integWriteOTTJSON(w, http.StatusOK, map[string]any{
				"resultCode":    "200",
				"resultMessage": "OK",
				"result": map[string]any{
					"plan_id": planID,
					"name":    "Plan de prueba",
					"country": integActiveCountry,
				},
			})
			return
		}
		integWriteOTTJSON(w, http.StatusOK, map[string]any{
			"resultCode":    "200",
			"resultMessage": "OK",
			"result": map[string]any{
				"items": []map[string]any{
					{"plan_id": "11111111-1111-1111-1111-111111111111", "name": "Plan A", "country": integActiveCountry},
					{"plan_id": "22222222-2222-2222-2222-222222222222", "name": "Plan B", "country": integActiveCountry},
				},
				"totalRecords":   2,
				"page":           1,
				"recordsPerPage": 20,
				"totalPages":     1,
			},
		})
	case http.MethodPost:
		integWriteOTTJSON(w, http.StatusOK, map[string]any{
			"resultCode":    "200",
			"resultMessage": "Plan creado",
			"result": map[string]any{
				"plan_id": "33333333-3333-3333-3333-333333333333",
				"name":    "Plan nuevo",
				"country": integActiveCountry,
			},
		})
	case http.MethodPut:
		integWriteOTTJSON(w, http.StatusOK, map[string]any{
			"resultCode":    "200",
			"resultMessage": "Plan actualizado",
			"result": map[string]any{
				"plan_id": r.URL.Query().Get("plan_id"),
				"name":    "Plan editado",
			},
		})
	case http.MethodDelete:
		integWriteOTTJSON(w, http.StatusOK, map[string]any{
			"resultCode":    "200",
			"resultMessage": "Plan eliminado",
			"result":        map[string]any{"plan_id": r.URL.Query().Get("plan_id")},
		})
	default:
		integWriteOTTError(w, http.StatusMethodNotAllowed, "E405", "método no soportado")
	}
}

// record guarda una petición recibida (leyendo el cuerpo) para aserciones.
func (s *integOTTStub) record(r *http.Request) {
	body := ""
	if r.Body != nil {
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
	}
	s.mu.Lock()
	s.requests = append(s.requests, integRecordedRequest{
		Method:      r.Method,
		Path:        r.URL.Path,
		RawQuery:    r.URL.RawQuery,
		BearerToken: integBearer(r),
		Body:        body,
	})
	s.mu.Unlock()
}

// authCount devuelve cuántas veces se llamó al endpoint de autenticación OTT.
func (s *integOTTStub) authCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.authHits
}

// recordedPlanRequests devuelve una copia de las peticiones registradas a /plans.
func (s *integOTTStub) recordedPlanRequests() []integRecordedRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]integRecordedRequest, len(s.requests))
	copy(out, s.requests)
	return out
}

// integBearer extrae el token del encabezado Authorization: Bearer <token>.
func integBearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if strings.HasPrefix(h, prefix) {
		return strings.TrimSpace(strings.TrimPrefix(h, prefix))
	}
	return ""
}

// integWriteOTTJSON escribe una respuesta JSON con el código HTTP indicado.
func integWriteOTTJSON(w http.ResponseWriter, status int, payload map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// integWriteOTTError escribe un error con la variante estándar OTT.
func integWriteOTTError(w http.ResponseWriter, status int, errorCode, message string) {
	integWriteOTTJSON(w, status, map[string]any{
		"resultCode":    "error",
		"resultMessage": message,
		"errorCode":     errorCode,
	})
}

// integFakeRepo es un repositorio de usuarios en memoria para construir un
// AuthHandler real sin base de datos.
type integFakeRepo struct {
	byUsername map[string]*users.User
}

func newIntegFakeRepo() *integFakeRepo {
	return &integFakeRepo{byUsername: make(map[string]*users.User)}
}

func (r *integFakeRepo) FindByUsername(_ context.Context, username string) (*users.User, error) {
	u, ok := r.byUsername[username]
	if !ok {
		return nil, users.ErrNotFound
	}
	cp := *u
	return &cp, nil
}

func (r *integFakeRepo) Create(_ context.Context, user *users.User) error {
	if user == nil {
		return errors.New("usuario nil")
	}
	r.byUsername[user.Username] = user
	return nil
}

func (r *integFakeRepo) ExistsByUsername(_ context.Context, username string) (bool, error) {
	_, ok := r.byUsername[username]
	return ok, nil
}

func (r *integFakeRepo) Count(_ context.Context) (int64, error) {
	return int64(len(r.byUsername)), nil
}

// integResponse refleja el envoltorio normalizado del BFF para decodificar y
// hacer aserciones sobre {ok, data, pagination?, errorCode?, message}.
type integResponse struct {
	OK         bool            `json:"ok"`
	Data       json.RawMessage `json:"data"`
	Pagination *ott.Pagination `json:"pagination,omitempty"`
	ErrorCode  string          `json:"errorCode"`
	Message    string          `json:"message"`
}

// integEnv agrupa el router real y el OTT simulado para la prueba.
type integEnv struct {
	router *gin.Engine
	repo   *integFakeRepo
	roles  *fakeRolesRepo
	stub   *integOTTStub
}

// newIntegEnv construye el entorno completo: OTT simulado, *ott.Client real
// apuntando a él, AuthHandler real sobre un repo falso y los handlers de recurso
// reales, todo cableado por el NewRouter real.
func newIntegEnv(t *testing.T) *integEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)

	stub := newIntegOTTStub(t)

	cfg := &config.Config{
		OTTBaseURL:       stub.server.URL,
		OTTActiveCountry: integActiveCountry,
		OTTCredentialsByCountry: map[string]config.OTTCredentials{
			integActiveCountry: {ClientID: integOTTClientID, ClientSecret: integOTTClientSecret},
		},
	}
	ottClient := ott.NewClient(cfg)

	repo := newIntegFakeRepo()
	rolesRepo := newFakeRolesRepo()
	authHandler := NewAuthHandler(repo, rolesRepo, integSessionSecret, integExpiresMinutes)

	deps := RouterDeps{
		SessionSecret:       integSessionSecret,
		CORSAllowedOrigin:   integCORSOrigin,
		DefaultCountry:      integActiveCountry,
		ConfiguredCountries: []string{integActiveCountry},
		Auth:                authHandler,
		Customers:           NewCustomersHandler(ottClient, integActiveCountry, newTestCache()),
		Plans:               NewPlansHandler(ottClient, integActiveCountry, newTestCache()),
		Subscriptions:       NewSubscriptionsHandler(ottClient, integActiveCountry, newTestCache()),
		Cards:               NewCardsHandler(ottClient, integActiveCountry),
	}

	return &integEnv{router: NewRouter(deps), repo: repo, roles: rolesRepo, stub: stub}
}

// integSeedUser registra un usuario del panel con la contraseña indicada,
// hasheada con bcrypt (coste bajo para pruebas), tal como lo haría el seed real.
func (e *integEnv) integSeedUser(t *testing.T, username, password, role string) {
	t.Helper()
	hash, err := auth.HashPassword(password, 4)
	if err != nil {
		t.Fatalf("no se pudo generar el hash de prueba: %v", err)
	}
	id := "00000000-0000-0000-0000-0000000000" + username[:2]
	e.repo.byUsername[username] = &users.User{
		ID:           id,
		Username:     username,
		PasswordHash: hash,
		Role:         role,
	}
	// Registra los permisos efectivos del usuario según su rol en el país
	// activo, de modo que el login emita un token que porte esos permisos y las
	// rutas protegidas por RequirePermission autoricen correctamente.
	e.roles.setPerms(id, testPermsForRole(role, integActiveCountry))
}

// integDo ejecuta una petición contra el router real. token vacío => sin
// Authorization; body vacío => sin cuerpo.
func (e *integEnv) integDo(method, target, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	return rec
}

// integDecode decodifica el envoltorio normalizado del BFF.
func integDecode(t *testing.T, rec *httptest.ResponseRecorder) integResponse {
	t.Helper()
	var resp integResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("no se pudo decodificar la respuesta: %v (cuerpo: %s)", err, rec.Body.String())
	}
	return resp
}

// integLogin ejecuta POST /auth/login y devuelve el JWT de sesión emitido.
func (e *integEnv) integLogin(t *testing.T, username, password string) string {
	t.Helper()
	rec := e.integDo(http.MethodPost, "/auth/login", "", `{"username":"`+username+`","password":"`+password+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("login: esperaba 200, obtuvo %d (cuerpo: %s)", rec.Code, rec.Body.String())
	}
	resp := integDecode(t, rec)
	if !resp.OK {
		t.Fatalf("login: esperaba ok=true (cuerpo: %s)", rec.Body.String())
	}
	var data struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("login: no se pudo decodificar data: %v", err)
	}
	if data.Token == "" {
		t.Fatal("login: esperaba un token no vacío")
	}
	return data.Token
}

// TestIntegration_LoginThenPlansCRUD recorre el flujo completo extremo a
// extremo: login válido → JWT de sesión → list/create/edit/delete de Plans a
// través del BFF con el OTT simulado, verificando la normalización de las
// respuestas y que el proxy adjuntó el JWT OTT. Finalmente comprueba que el
// token de sesión deja de ser utilizable tras un "logout" del lado del cliente
// (el BFF es stateless: logout = descartar el token) usando un token inválido.
//
// _Requirements: 1.5, 1.8, 2.2, 9.1, 10.13, 11.3, 12.2, 20.1, 20.2, 20.3_
func TestIntegration_LoginThenPlansCRUD(t *testing.T) {
	env := newIntegEnv(t)
	env.integSeedUser(t, "admin", "s3cret-pass", users.RoleAdmin)

	// 1) Login válido → JWT de sesión (Req. 1.5, 1.8, 2.2).
	token := env.integLogin(t, "admin", "s3cret-pass")

	// El token de sesión emitido debe validar con el secreto del router.
	if _, err := auth.ValidateSessionToken(token, integSessionSecret); err != nil {
		t.Fatalf("el token de sesión emitido debería validar: %v", err)
	}

	// 2) GET /plans (listado) → el stub devuelve una lista paginada que el BFF
	// normaliza a {ok, data, pagination} (Req. 9.1, 20.2, 20.3).
	rec := env.integDo(http.MethodGet, "/plans?page=1&records_per_page=20", token, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /plans: esperaba 200, obtuvo %d (cuerpo: %s)", rec.Code, rec.Body.String())
	}
	listResp := integDecode(t, rec)
	if !listResp.OK {
		t.Fatalf("GET /plans: esperaba ok=true (cuerpo: %s)", rec.Body.String())
	}
	if listResp.Pagination == nil {
		t.Fatal("GET /plans: esperaba metadatos de paginación normalizados")
	}
	if listResp.Pagination.TotalRecords != 2 || listResp.Pagination.Page != 1 {
		t.Errorf("GET /plans: paginación inesperada: %+v", listResp.Pagination)
	}

	// 3) POST /plans (crear) con un plan válido para gt/GTQ (Req. 10.13, 20.2).
	createBody := `{
		"name": "Plan nuevo",
		"external_id": "ext-plan-001",
		"country": "gt",
		"amount": 99.9,
		"currency": "GTQ",
		"frequency": 1,
		"period_type": "mes"
	}`
	rec = env.integDo(http.MethodPost, "/plans", token, createBody)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /plans: esperaba 200, obtuvo %d (cuerpo: %s)", rec.Code, rec.Body.String())
	}
	createResp := integDecode(t, rec)
	if !createResp.OK {
		t.Fatalf("POST /plans: esperaba ok=true (cuerpo: %s)", rec.Body.String())
	}
	if createResp.Message != "Plan creado" {
		t.Errorf("POST /plans: mensaje OTT no propagado, obtuvo %q", createResp.Message)
	}

	// 4) PUT /plans (editar) por plan_id (Req. 11.3).
	editBody := `{"name": "Plan editado"}`
	rec = env.integDo(http.MethodPut, "/plans?plan_id=33333333-3333-3333-3333-333333333333", token, editBody)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT /plans: esperaba 200, obtuvo %d (cuerpo: %s)", rec.Code, rec.Body.String())
	}
	editResp := integDecode(t, rec)
	if !editResp.OK {
		t.Fatalf("PUT /plans: esperaba ok=true (cuerpo: %s)", rec.Body.String())
	}

	// 5) DELETE /plans por plan_id (Req. 12.2).
	rec = env.integDo(http.MethodDelete, "/plans?plan_id=33333333-3333-3333-3333-333333333333", token, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE /plans: esperaba 200, obtuvo %d (cuerpo: %s)", rec.Code, rec.Body.String())
	}
	delResp := integDecode(t, rec)
	if !delResp.OK {
		t.Fatalf("DELETE /plans: esperaba ok=true (cuerpo: %s)", rec.Body.String())
	}

	// --- Aserciones sobre el camino de proxy hacia el OTT ---

	// El OTT se autenticó UNA sola vez: el JWT OTT se cachea y reutiliza por
	// país durante las 4 operaciones CRUD (Req. 3.2, 3.3).
	if got := env.stub.authCount(); got != 1 {
		t.Errorf("esperaba exactamente 1 autenticación contra el OTT (token reutilizado), obtuvo %d", got)
	}

	// Todas las llamadas a /plans deben haber llevado el JWT OTT como Bearer y
	// el query param country=gt que el BFF inyecta (Req. 10.13 y enrutamiento).
	planReqs := env.stub.recordedPlanRequests()
	if len(planReqs) != 4 {
		t.Fatalf("esperaba 4 peticiones a /plans (list, create, edit, delete), obtuvo %d", len(planReqs))
	}
	methodsSeen := map[string]bool{}
	for _, pr := range planReqs {
		if pr.BearerToken != integOTTJWT {
			t.Errorf("%s /plans: esperaba el JWT OTT como Bearer, obtuvo %q", pr.Method, pr.BearerToken)
		}
		if !strings.Contains(pr.RawQuery, "country=gt") {
			t.Errorf("%s /plans: esperaba country=gt en la query, obtuvo %q", pr.Method, pr.RawQuery)
		}
		methodsSeen[pr.Method] = true
	}
	for _, m := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete} {
		if !methodsSeen[m] {
			t.Errorf("esperaba una petición %s a /plans en el flujo CRUD", m)
		}
	}

	// 6) "Logout" del cliente (descartar el token): un token manipulado/ inválido
	// deja de dar acceso a las rutas protegidas (Req. 2.4, 2.5).
	rec = env.integDo(http.MethodGet, "/plans", token+"tampered", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("tras invalidar el token: esperaba 401, obtuvo %d (cuerpo: %s)", rec.Code, rec.Body.String())
	}
}

// TestIntegration_ViewerCannotCreatePlanEndToEnd verifica que, extremo a
// extremo, un token de sesión con rol viewer recibe 403 al intentar crear un
// plan (POST /plans), sin que la petición llegue siquiera al OTT (RequireAdmin
// corta antes del proxy). Esto confirma el control de acceso por rol en el
// camino real del router (Req. 5.3, 20.1).
func TestIntegration_ViewerCannotCreatePlanEndToEnd(t *testing.T) {
	env := newIntegEnv(t)
	env.integSeedUser(t, "viewer", "viewer-pass", users.RoleViewer)

	token := env.integLogin(t, "viewer", "viewer-pass")

	createBody := `{
		"name": "Plan nuevo",
		"external_id": "ext-plan-001",
		"country": "gt",
		"amount": 99.9,
		"currency": "GTQ",
		"frequency": 1,
		"period_type": "mes"
	}`
	rec := env.integDo(http.MethodPost, "/plans", token, createBody)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("POST /plans (viewer): esperaba 403, obtuvo %d (cuerpo: %s)", rec.Code, rec.Body.String())
	}
	resp := integDecode(t, rec)
	if resp.OK {
		t.Error("POST /plans (viewer): esperaba ok=false en el envoltorio de error 403")
	}

	// El viewer SÍ puede leer: GET /plans debe pasar y llegar al OTT simulado.
	rec = env.integDo(http.MethodGet, "/plans", token, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /plans (viewer): esperaba 200, obtuvo %d (cuerpo: %s)", rec.Code, rec.Body.String())
	}

	// Confirmar que ninguna escritura llegó al OTT: solo la lectura (GET).
	for _, pr := range env.stub.recordedPlanRequests() {
		if pr.Method != http.MethodGet {
			t.Errorf("el viewer no debería haber alcanzado el OTT con %s /plans", pr.Method)
		}
	}
}
