package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"admin-backend/internal/config"
)

// Pruebas del soporte multi-país del BFF (selector de país):
//   - CountryResolver resuelve el país efectivo por petición (cabecera
//     X-Country), con respaldo al país por defecto, y rechaza con 400 un país
//     no configurado.
//   - GET /meta/countries expone los países con credenciales y el país activo.
//   - config.ConfiguredCountries devuelve el subconjunto con credenciales.
//
// Se usan helpers con prefijo `ccTest*` para no colisionar con router_test.go /
// integration_test.go del mismo paquete.

func init() { gin.SetMode(gin.TestMode) }

// ccTestNormalized refleja el envoltorio normalizado del BFF.
type ccTestNormalized struct {
	OK        bool            `json:"ok"`
	Data      json.RawMessage `json:"data"`
	ErrorCode string          `json:"errorCode"`
	Message   string          `json:"message"`
}

func ccTestDecode(t *testing.T, rec *httptest.ResponseRecorder) ccTestNormalized {
	t.Helper()
	var resp ccTestNormalized
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("no se pudo decodificar la respuesta: %v (cuerpo: %s)", err, rec.Body.String())
	}
	return resp
}

// ccTestRouterWithResolver arma un router mínimo con el middleware de país y una
// ruta que devuelve el país efectivo resuelto, para observar su comportamiento.
func ccTestRouterWithResolver(defaultCountry string, configured []string) *gin.Engine {
	r := gin.New()
	grp := r.Group("/")
	grp.Use(CountryResolver(defaultCountry, configured))
	grp.GET("/echo-country", func(c *gin.Context) {
		country, ok := CountryFromContext(c)
		writeOK(c, http.StatusOK, gin.H{"country": country, "present": ok}, "")
	})
	return r
}

func ccTestDoCountry(r *gin.Engine, headerValue string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/echo-country", nil)
	if headerValue != "" {
		req.Header.Set(CountryHeader, headerValue)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// echoedCountry decodifica la respuesta de /echo-country.
func ccTestEchoedCountry(t *testing.T, rec *httptest.ResponseRecorder) (string, bool) {
	t.Helper()
	resp := ccTestDecode(t, rec)
	var data struct {
		Country string `json:"country"`
		Present bool   `json:"present"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("no se pudo decodificar data: %v", err)
	}
	return data.Country, data.Present
}

func TestCountryResolver_NoHeaderUsesDefault(t *testing.T) {
	r := ccTestRouterWithResolver("gt", []string{"gt", "ni"})

	rec := ccTestDoCountry(r, "")

	if rec.Code != http.StatusOK {
		t.Fatalf("esperaba 200, obtuvo %d (cuerpo: %s)", rec.Code, rec.Body.String())
	}
	country, present := ccTestEchoedCountry(t, rec)
	if !present {
		t.Fatal("esperaba que el país estuviera presente en el contexto")
	}
	if country != "gt" {
		t.Errorf("país efectivo = %q, se esperaba el default %q", country, "gt")
	}
}

func TestCountryResolver_ValidHeaderOverridesDefault(t *testing.T) {
	r := ccTestRouterWithResolver("gt", []string{"gt", "ni"})

	// Mayúsculas y espacios para verificar normalización.
	rec := ccTestDoCountry(r, "  NI ")

	if rec.Code != http.StatusOK {
		t.Fatalf("esperaba 200, obtuvo %d (cuerpo: %s)", rec.Code, rec.Body.String())
	}
	country, _ := ccTestEchoedCountry(t, rec)
	if country != "ni" {
		t.Errorf("país efectivo = %q, se esperaba %q (normalizado desde ' NI ')", country, "ni")
	}
}

func TestCountryResolver_UnconfiguredCountryReturns400(t *testing.T) {
	r := ccTestRouterWithResolver("gt", []string{"gt", "ni"})

	// 'sv' es un país soportado por el BFF pero SIN credenciales configuradas.
	rec := ccTestDoCountry(r, "sv")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("esperaba 400 para país no configurado, obtuvo %d (cuerpo: %s)", rec.Code, rec.Body.String())
	}
	resp := ccTestDecode(t, rec)
	if resp.OK {
		t.Error("esperaba ok=false en el envoltorio de error")
	}
	if resp.ErrorCode != "E400_COUNTRY" {
		t.Errorf("errorCode = %q, se esperaba E400_COUNTRY", resp.ErrorCode)
	}
}

func TestCountryResolver_GarbageCountryReturns400(t *testing.T) {
	r := ccTestRouterWithResolver("gt", []string{"gt", "ni"})

	rec := ccTestDoCountry(r, "xx")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("esperaba 400 para país inválido, obtuvo %d", rec.Code)
	}
}

func TestCountryFromContext_AbsentReturnsFalse(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	if _, ok := CountryFromContext(c); ok {
		t.Error("CountryFromContext debe devolver false cuando no hay país en el contexto")
	}
}

// --- GET /meta/countries ---

func TestMetaHandler_CountriesReturnsConfiguredAndActive(t *testing.T) {
	h := NewMetaHandler([]string{"gt", "ni"}, "gt")

	r := gin.New()
	r.GET("/meta/countries", h.Countries)

	req := httptest.NewRequest(http.MethodGet, "/meta/countries", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperaba 200, obtuvo %d (cuerpo: %s)", rec.Code, rec.Body.String())
	}
	resp := ccTestDecode(t, rec)
	if !resp.OK {
		t.Fatalf("esperaba ok=true (cuerpo: %s)", rec.Body.String())
	}

	var data struct {
		Countries []string `json:"countries"`
		Active    string   `json:"active"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("no se pudo decodificar data: %v", err)
	}
	if data.Active != "gt" {
		t.Errorf("active = %q, se esperaba %q", data.Active, "gt")
	}
	if len(data.Countries) != 2 || data.Countries[0] != "gt" || data.Countries[1] != "ni" {
		t.Errorf("countries = %v, se esperaba [gt ni]", data.Countries)
	}
}

func TestMetaHandler_EmptyConfiguredReturnsEmptyArrayNotNull(t *testing.T) {
	h := NewMetaHandler(nil, "gt")

	r := gin.New()
	r.GET("/meta/countries", h.Countries)

	req := httptest.NewRequest(http.MethodGet, "/meta/countries", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	// El JSON debe contener "countries":[] (no null) para que el front lo maneje.
	if body := rec.Body.String(); !jsonHasEmptyCountries(body) {
		t.Errorf("se esperaba countries:[] no-null en la respuesta, cuerpo: %s", body)
	}
}

// jsonHasEmptyCountries confirma que el cuerpo contiene una lista vacía (no null).
func jsonHasEmptyCountries(body string) bool {
	var resp struct {
		Data struct {
			Countries *[]string `json:"countries"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		return false
	}
	return resp.Data.Countries != nil && len(*resp.Data.Countries) == 0
}

// --- config.ConfiguredCountries ---

func TestConfiguredCountries_OnlyCountriesWithCredentials(t *testing.T) {
	cfg := &config.Config{
		OTTActiveCountry: "gt",
		OTTCredentialsByCountry: map[string]config.OTTCredentials{
			"gt": {ClientID: "gt-id", ClientSecret: "gt-secret"},
			"ni": {ClientID: "ni-id", ClientSecret: "ni-secret"},
			"sv": {ClientID: "", ClientSecret: ""},           // sin credenciales
			"hn": {ClientID: "hn-id", ClientSecret: ""},      // incompletas
			"cr": {ClientID: "", ClientSecret: "cr-secret"},  // incompletas
		},
	}

	got := cfg.ConfiguredCountries()

	// Orden determinista según SupportedCountries {gt,sv,ni,hn,cr}: gt, ni.
	if len(got) != 2 || got[0] != "gt" || got[1] != "ni" {
		t.Errorf("ConfiguredCountries() = %v, se esperaba [gt ni]", got)
	}
}

func TestConfiguredCountries_NoneConfigured(t *testing.T) {
	cfg := &config.Config{
		OTTActiveCountry:        "gt",
		OTTCredentialsByCountry: map[string]config.OTTCredentials{},
	}
	if got := cfg.ConfiguredCountries(); len(got) != 0 {
		t.Errorf("ConfiguredCountries() = %v, se esperaba vacío", got)
	}
}
