package api

import (
	"context"
	"encoding/json"
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

// Prueba de integración del selector de país extremo a extremo: con DOS países
// configurados (gt y ni), una petición autenticada con la cabecera X-Country
// debe autenticar y consultar al OTT con las credenciales del país solicitado,
// y el BFF debe adjuntar ese país (Bearer del JWT de ese país + query country).
//
// Levanta un OTT simulado que distingue credenciales por país y registra, por
// cada llamada a /plans, el JWT presentado para poder verificar qué país se usó.
// Helpers con prefijo `mcTest*` para no colisionar con otros archivos de prueba.

const (
	mcTestSessionSecret  = "multicountry-session-secret-0123456789abc"
	mcTestExpiresMinutes = 60
)

// mcTestCredsByCountry asocia a cada país unas credenciales OTT y el JWT que el
// stub devuelve tras autenticar con ellas.
var mcTestCredsByCountry = map[string]struct {
	clientID     string
	clientSecret string
	jwt          string
}{
	"gt": {clientID: "gt-client", clientSecret: "gt-secret", jwt: "ott-jwt-gt"},
	"ni": {clientID: "ni-client", clientSecret: "ni-secret", jwt: "ott-jwt-ni"},
}

// mcTestStub es un OTT simulado multi-país.
type mcTestStub struct {
	server *httptest.Server

	mu             sync.Mutex
	planBearerSeen []string // JWT presentado en cada llamada a /plans, en orden
}

func newMCTestStub(t *testing.T) *mcTestStub {
	t.Helper()
	stub := &mcTestStub{}
	mux := http.NewServeMux()

	mux.HandleFunc("/authenticate", func(w http.ResponseWriter, r *http.Request) {
		var creds struct {
			ClientID     string `json:"clientId"`
			ClientSecret string `json:"clientSecret"`
		}
		_ = json.NewDecoder(r.Body).Decode(&creds)

		// Encontrar el país cuyas credenciales coinciden y devolver su JWT.
		for _, cc := range mcTestCredsByCountry {
			if creds.ClientID == cc.clientID && creds.ClientSecret == cc.clientSecret {
				mcTestWriteJSON(w, http.StatusOK, map[string]any{
					"resultCode":    "200",
					"resultMessage": "OK",
					"result":        map[string]any{"Authorization": cc.jwt},
				})
				return
			}
		}
		mcTestWriteJSON(w, http.StatusUnauthorized, map[string]any{
			"resultCode": "error", "errorCode": "S401SEC", "resultMessage": "credenciales inválidas",
		})
	})

	mux.HandleFunc("/plans", func(w http.ResponseWriter, r *http.Request) {
		bearer := ""
		if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
			bearer = strings.TrimPrefix(h, "Bearer ")
		}
		stub.mu.Lock()
		stub.planBearerSeen = append(stub.planBearerSeen, bearer)
		stub.mu.Unlock()

		mcTestWriteJSON(w, http.StatusOK, map[string]any{
			"resultCode":    "200",
			"resultMessage": "OK",
			"result": map[string]any{
				"items":          []map[string]any{{"plan_id": "p1"}},
				"totalRecords":   1,
				"page":           1,
				"recordsPerPage": 20,
				"totalPages":     1,
			},
		})
	})

	stub.server = httptest.NewServer(mux)
	t.Cleanup(stub.server.Close)
	return stub
}

func (s *mcTestStub) bearers() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.planBearerSeen))
	copy(out, s.planBearerSeen)
	return out
}

func mcTestWriteJSON(w http.ResponseWriter, status int, payload map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// mcTestRepo es un repositorio de usuarios en memoria.
type mcTestRepo struct{ byUsername map[string]*users.User }

func (r *mcTestRepo) FindByUsername(_ context.Context, username string) (*users.User, error) {
	u, ok := r.byUsername[username]
	if !ok {
		return nil, users.ErrNotFound
	}
	cp := *u
	return &cp, nil
}
func (r *mcTestRepo) Create(_ context.Context, u *users.User) error {
	r.byUsername[u.Username] = u
	return nil
}
func (r *mcTestRepo) ExistsByUsername(_ context.Context, username string) (bool, error) {
	_, ok := r.byUsername[username]
	return ok, nil
}
func (r *mcTestRepo) Count(_ context.Context) (int64, error) { return int64(len(r.byUsername)), nil }

// Métodos adicionales para satisfacer la interfaz users.Repository (ampliada en
// la tarea 9.1). Esta prueba solo usa el login, por lo que basta con delegar
// FindActiveByUsername en FindByUsername y dejar el resto como no-ops.
func (r *mcTestRepo) FindActiveByUsername(ctx context.Context, username string) (*users.User, error) {
	return r.FindByUsername(ctx, username)
}
func (r *mcTestRepo) FindByID(_ context.Context, id string) (*users.User, error) {
	for _, u := range r.byUsername {
		if u.ID == id {
			cp := *u
			return &cp, nil
		}
	}
	return nil, users.ErrNotFound
}
func (r *mcTestRepo) List(context.Context) ([]users.User, error)       { return nil, nil }
func (r *mcTestRepo) SetRoles(context.Context, string, []string) error { return nil }

func (r *mcTestRepo) SetCountries(context.Context, string, []string) error { return nil }

func (r *mcTestRepo) CountriesForUser(context.Context, string) ([]string, error) { return nil, nil }
func (r *mcTestRepo) UpdateRolesStatusAndCountries(context.Context, string, []string, bool, []string) error {
	return nil
}
func (r *mcTestRepo) UpdatePasswordHash(context.Context, string, string) error { return nil }

var _ users.Repository = (*mcTestRepo)(nil)

// mcTestBuildRouter arma el router real con el OTT simulado multi-país y gt/ni
// configurados; gt es el país activo por defecto.
func mcTestBuildRouter(t *testing.T, stub *mcTestStub) (*gin.Engine, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		OTTBaseURL:       stub.server.URL,
		OTTActiveCountry: "gt",
		OTTCredentialsByCountry: map[string]config.OTTCredentials{
			"gt": {ClientID: mcTestCredsByCountry["gt"].clientID, ClientSecret: mcTestCredsByCountry["gt"].clientSecret},
			"ni": {ClientID: mcTestCredsByCountry["ni"].clientID, ClientSecret: mcTestCredsByCountry["ni"].clientSecret},
		},
	}
	ottClient := ott.NewClient(cfg)

	hash, err := auth.HashPassword("admin-pass", 4)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	repo := &mcTestRepo{byUsername: map[string]*users.User{
		"admin": {ID: "id-admin", Username: "admin", PasswordHash: hash, Role: users.RoleAdmin},
	}}

	configured := cfg.ConfiguredCountries()
	deps := RouterDeps{
		SessionSecret:       mcTestSessionSecret,
		DefaultCountry:      cfg.OTTActiveCountry,
		ConfiguredCountries: configured,
		Auth:                NewAuthHandler(repo, newFakeRolesRepo(), mcTestSessionSecret, mcTestExpiresMinutes),
		Meta:                NewMetaHandler(configured, cfg.OTTActiveCountry),
		Customers:           NewCustomersHandler(ottClient, cfg.OTTActiveCountry, newTestCache()),
		Plans:               NewPlansHandler(ottClient, cfg.OTTActiveCountry, newTestCache()),
		Subscriptions:       NewSubscriptionsHandler(ottClient, cfg.OTTActiveCountry, newTestCache()),
		Cards:               NewCardsHandler(ottClient, cfg.OTTActiveCountry),
	}

	// El token del admin porta todas las acciones de todos los recursos en los
	// países configurados (gt y ni), de modo que RequirePermission autorice las
	// lecturas de /plans y /meta/countries en el país efectivo de cada petición.
	adminPerms := testAllPermissions("gt", "ni")
	token, err := auth.IssueSessionToken(users.User{ID: "id-admin", Username: "admin", Role: users.RoleAdmin}, adminPerms, nil, mcTestSessionSecret, mcTestExpiresMinutes)
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	return NewRouter(deps), token
}

func mcTestGetPlans(r *gin.Engine, token, country string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/plans", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	if country != "" {
		req.Header.Set(CountryHeader, country)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestIntegration_MultiCountry_RoutesByXCountryHeader(t *testing.T) {
	stub := newMCTestStub(t)
	router, token := mcTestBuildRouter(t, stub)

	// Sin cabecera → país por defecto gt → JWT de gt.
	if rec := mcTestGetPlans(router, token, ""); rec.Code != http.StatusOK {
		t.Fatalf("GET /plans (default): esperaba 200, obtuvo %d (%s)", rec.Code, rec.Body.String())
	}
	// Con X-Country: ni → JWT de ni.
	if rec := mcTestGetPlans(router, token, "ni"); rec.Code != http.StatusOK {
		t.Fatalf("GET /plans (ni): esperaba 200, obtuvo %d (%s)", rec.Code, rec.Body.String())
	}
	// Con X-Country: gt (explícito) → JWT de gt.
	if rec := mcTestGetPlans(router, token, "GT"); rec.Code != http.StatusOK {
		t.Fatalf("GET /plans (GT): esperaba 200, obtuvo %d (%s)", rec.Code, rec.Body.String())
	}

	// El listado de /plans ahora se sirve desde el caché del BFF por (recurso,
	// país). La 1ª petición (gt) y la 2ª (ni) son fallos de caché y cargan desde
	// OTT con el JWT de su país; la 3ª (gt de nuevo) es un ACIERTO de caché y NO
	// vuelve a llamar a OTT. Por eso se esperan 2 llamadas, con bearers [gt, ni],
	// lo que sigue verificando el enrutamiento por país.
	bearers := stub.bearers()
	if len(bearers) != 2 {
		t.Fatalf("esperaba 2 llamadas a /plans (gt y ni; el 2º gt sale del caché), obtuvo %d", len(bearers))
	}
	if bearers[0] != mcTestCredsByCountry["gt"].jwt {
		t.Errorf("llamada 1 (default gt): JWT = %q, se esperaba el de gt", bearers[0])
	}
	if bearers[1] != mcTestCredsByCountry["ni"].jwt {
		t.Errorf("llamada 2 (ni): JWT = %q, se esperaba el de ni", bearers[1])
	}
}

func TestIntegration_MultiCountry_UnconfiguredCountryIsRejected(t *testing.T) {
	stub := newMCTestStub(t)
	router, token := mcTestBuildRouter(t, stub)

	// 'sv' no tiene credenciales configuradas → 400 antes de tocar el OTT.
	rec := mcTestGetPlans(router, token, "sv")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("GET /plans (sv no configurado): esperaba 400, obtuvo %d (%s)", rec.Code, rec.Body.String())
	}
	if len(stub.bearers()) != 0 {
		t.Errorf("no debió llegar ninguna llamada a /plans para un país no configurado")
	}
}

func TestIntegration_MetaCountriesEndpoint(t *testing.T) {
	stub := newMCTestStub(t)
	router, token := mcTestBuildRouter(t, stub)

	req := httptest.NewRequest(http.MethodGet, "/meta/countries", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /meta/countries: esperaba 200, obtuvo %d (%s)", rec.Code, rec.Body.String())
	}
	var resp struct {
		OK   bool `json:"ok"`
		Data struct {
			Countries []string `json:"countries"`
			Active    string   `json:"active"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.OK || resp.Data.Active != "gt" {
		t.Fatalf("respuesta inesperada: %s", rec.Body.String())
	}
	if len(resp.Data.Countries) != 2 || resp.Data.Countries[0] != "gt" || resp.Data.Countries[1] != "ni" {
		t.Errorf("countries = %v, se esperaba [gt ni]", resp.Data.Countries)
	}
}
