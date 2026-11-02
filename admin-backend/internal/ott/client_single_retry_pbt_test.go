// Feature: panel-admin-ott, Property 4: Reautenticación con reintento único ante 401 de seguridad
//
// Prueba basada en propiedades (PBT) del OTTClient.
//
// Validates: Requirements 3.5, 3.6
//
// Propiedad: *para toda* llamada OTT que responde 401 con un `errorCode` de
// seguridad (prefijo "S401", p. ej. "S401SEC"), el OTTClient invalida el token
// cacheado del país, se reautentica y reintenta la llamada A LO SUMO una vez:
//
//	(a) Si el reintento tiene éxito, Do devuelve esa respuesta exitosa, se
//	    realizan exactamente 2 llamadas proxied (no-auth) y ocurre exactamente
//	    una autenticación adicional (/authenticate) debida a la invalidación.
//
//	(b) Si el reintento vuelve a responder 401 de seguridad, Do propaga un
//	    error y NO existe un tercer intento proxied (reintento único acotado,
//	    sin bucles infinitos).
//
// Se usa la librería estándar `testing/quick` (mínimo 100 iteraciones) con un
// doble de prueba `singleRetryFakeDoer` que distingue las llamadas a
// /authenticate de las llamadas proxied y cuenta cada una. Los helpers se
// prefijan con `singleRetry` para no colisionar con otros ficheros de prueba.
package ott

import (
	"context"
	"encoding/json"
	"io"
	"math/rand"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"testing/quick"
	"time"

	"admin-backend/internal/config"
)

// singleRetryIterations es el número mínimo de iteraciones exigido por las
// convenciones de PBT del proyecto (mínimo 100).
const singleRetryIterations = 200

// singleRetrySupportedCountries son los países soportados por el OTT.
var singleRetrySupportedCountries = []string{"gt", "sv", "ni", "hn", "cr"}

// singleRetrySecurityCodes enumera errorCode de seguridad válidos (prefijo
// "S401") que deben disparar la reautenticación y el reintento único.
var singleRetrySecurityCodes = []string{"S401SEC", "S401", "s401sec", "S401TOKEN", "S401EXP"}

// singleRetryFakeDoer es un httpDoer scripted que distingue las llamadas a
// /authenticate de las llamadas proxied y cuenta cada tipo. Para las llamadas
// proxied devuelve respuestas tomadas de `proxiedResponses` en orden; si se
// agotan, falla la prueba marcando un intento proxied excedente.
type singleRetryFakeDoer struct {
	t *testing.T

	// proxiedResponses define, en orden, las respuestas que el doble devuelve
	// para cada llamada proxied (no-auth). Cada entrada produce una
	// *http.Response con el status y cuerpo indicados.
	proxiedResponses []singleRetryResp

	authCount    int // número de llamadas a /authenticate
	proxiedCount int // número de llamadas proxied (no-auth)
	overLimit    bool // true si se solicitó una respuesta proxied más allá del guion
}

// singleRetryResp describe una respuesta scripted: status HTTP y errorCode de
// seguridad opcional (vacío => respuesta normal/exitosa).
type singleRetryResp struct {
	status       int
	securityCode string // si no está vacío, se emite como errorCode de seguridad con status 401
}

// singleRetryAuthToken es un JWT OTT de prueba sin claim exp (fuerza el TTL por
// defecto, lo que mantiene el token vigente durante toda la prueba). No se
// valida la firma en el cliente.
const singleRetryAuthToken = "eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJvdHQifQ.c2ln"

func (d *singleRetryFakeDoer) Do(req *http.Request) (*http.Response, error) {
	if strings.HasSuffix(req.URL.Path, authEndpoint) {
		d.authCount++
		body, _ := json.Marshal(authResponse{
			ResultCode: "0",
			Result:     authResult{Authorization: singleRetryAuthToken},
		})
		return singleRetryHTTPResponse(http.StatusOK, body), nil
	}

	// Llamada proxied (no-auth).
	idx := d.proxiedCount
	d.proxiedCount++
	if idx >= len(d.proxiedResponses) {
		// Un intento proxied más allá del guion significa que se violó el
		// reintento único (p. ej. un tercer intento). Se marca y se devuelve
		// algo inocuo para no entrar en pánico.
		d.overLimit = true
		return singleRetryHTTPResponse(http.StatusOK, []byte(`{"resultCode":"0"}`)), nil
	}

	spec := d.proxiedResponses[idx]
	if spec.securityCode != "" {
		body, _ := json.Marshal(map[string]string{"errorCode": spec.securityCode})
		return singleRetryHTTPResponse(http.StatusUnauthorized, body), nil
	}
	return singleRetryHTTPResponse(spec.status, []byte(`{"resultCode":"0","resultMessage":"ok"}`)), nil
}

// singleRetryHTTPResponse construye una *http.Response con el status y cuerpo
// indicados y un Body legible.
func singleRetryHTTPResponse(status int, body []byte) *http.Response {
	return &http.Response{
		StatusCode:    status,
		Body:          io.NopCloser(strings.NewReader(string(body))),
		Header:        make(http.Header),
		ContentLength: int64(len(body)),
	}
}

// singleRetryNewClient construye un Client in-package con el doble inyectado y
// credenciales válidas para todos los países soportados. El reloj es fijo para
// mantener los tokens vigentes de forma determinista.
func singleRetryNewClient(doer httpDoer) *Client {
	creds := make(map[string]config.OTTCredentials, len(singleRetrySupportedCountries))
	for _, c := range singleRetrySupportedCountries {
		creds[c] = config.OTTCredentials{ClientID: "id-" + c, ClientSecret: "secret-" + c}
	}
	fixed := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	return &Client{
		baseURL:     "http://ott.test/api/v1",
		credentials: creds,
		httpClient:  doer,
		now:         func() time.Time { return fixed },
		tokens:      make(map[string]tokenEntry),
	}
}

// singleRetryScenario es la entrada generada para la propiedad: país, método,
// path, código de seguridad del primer 401 y si el reintento también falla.
type singleRetryScenario struct {
	country      string
	method       string
	path         string
	securityCode string
	retryFails   bool
}

// Generate satisface quick.Generator para singleRetryScenario.
func (singleRetryScenario) Generate(rnd *rand.Rand, _ int) reflect.Value {
	methods := []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete}
	paths := []string{"/customers", "/plans", "/subscriptions", "/cards", "customers/list", "/plans/detail"}
	return reflect.ValueOf(singleRetryScenario{
		country:      singleRetrySupportedCountries[rnd.Intn(len(singleRetrySupportedCountries))],
		method:       methods[rnd.Intn(len(methods))],
		path:         paths[rnd.Intn(len(paths))],
		securityCode: singleRetrySecurityCodes[rnd.Intn(len(singleRetrySecurityCodes))],
		retryFails:   rnd.Intn(2) == 0,
	})
}

// TestSingleRetryOnSecurity401 verifica la Property 4: ante un primer 401 de
// seguridad, el cliente reautentica y reintenta exactamente una vez.
func TestSingleRetryOnSecurity401(t *testing.T) {
	property := func(s singleRetryScenario) bool {
		var proxied []singleRetryResp
		// Primer intento proxied: 401 de seguridad.
		proxied = append(proxied, singleRetryResp{securityCode: s.securityCode})
		// Segundo intento proxied (reintento): éxito o nuevo 401 de seguridad.
		if s.retryFails {
			proxied = append(proxied, singleRetryResp{securityCode: s.securityCode})
		} else {
			proxied = append(proxied, singleRetryResp{status: http.StatusOK})
		}

		doer := &singleRetryFakeDoer{t: t, proxiedResponses: proxied}
		client := singleRetryNewClient(doer)

		// Prime el cache: una autenticación inicial. A partir de aquí, cualquier
		// autenticación adicional se debe a la invalidación por el 401.
		if _, err := client.Token(context.Background(), s.country); err != nil {
			return false
		}
		authAfterPrime := doer.authCount // debe ser 1

		resp, err := client.Do(context.Background(), s.country, s.method, s.path, nil, nil)

		// En ningún caso debe haberse realizado un tercer intento proxied.
		if doer.overLimit {
			return false
		}
		// Exactamente 2 llamadas proxied: intento original + un único reintento.
		if doer.proxiedCount != 2 {
			return false
		}
		// Exactamente una reautenticación adicional (invalidación -> re-auth).
		if doer.authCount != authAfterPrime+1 {
			return false
		}

		if s.retryFails {
			// (b) El segundo 401 de seguridad se propaga como error.
			if err == nil {
				return false
			}
			if resp != nil {
				return false
			}
		} else {
			// (a) El reintento tuvo éxito: Do devuelve esa respuesta exitosa.
			if err != nil {
				return false
			}
			if resp == nil {
				return false
			}
			if resp.StatusCode != http.StatusOK {
				resp.Body.Close()
				return false
			}
			resp.Body.Close()
		}

		return true
	}

	cfg := &quick.Config{MaxCount: singleRetryIterations}
	if err := quick.Check(property, cfg); err != nil {
		t.Fatalf("Property 4 violada: reautenticación con reintento único ante 401 de seguridad: %v", err)
	}
}

// TestSingleRetryKnownScenarios cubre explícitamente los dos casos frontera
// (reintento exitoso y reintento fallido) de forma determinista, complementando
// la generación aleatoria.
func TestSingleRetryKnownScenarios(t *testing.T) {
	t.Run("retry succeeds returns success with exactly two proxied calls", func(t *testing.T) {
		doer := &singleRetryFakeDoer{
			t: t,
			proxiedResponses: []singleRetryResp{
				{securityCode: "S401SEC"},
				{status: http.StatusOK},
			},
		}
		client := singleRetryNewClient(doer)
		if _, err := client.Token(context.Background(), "gt"); err != nil {
			t.Fatalf("Token inicial falló: %v", err)
		}

		resp, err := client.Do(context.Background(), "gt", http.MethodGet, "/customers", nil, nil)
		if err != nil {
			t.Fatalf("Do devolvió error inesperado: %v", err)
		}
		if resp == nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("se esperaba respuesta 200 tras el reintento exitoso; resp=%v", resp)
		}
		resp.Body.Close()

		if doer.proxiedCount != 2 {
			t.Errorf("llamadas proxied = %d; se esperaban 2", doer.proxiedCount)
		}
		if doer.authCount != 2 {
			t.Errorf("autenticaciones = %d; se esperaban 2 (inicial + re-auth)", doer.authCount)
		}
		if doer.overLimit {
			t.Errorf("se detectó un tercer intento proxied; el reintento no es único")
		}
	})

	t.Run("second security 401 propagates error without a third attempt", func(t *testing.T) {
		doer := &singleRetryFakeDoer{
			t: t,
			proxiedResponses: []singleRetryResp{
				{securityCode: "S401SEC"},
				{securityCode: "S401SEC"},
			},
		}
		client := singleRetryNewClient(doer)
		if _, err := client.Token(context.Background(), "sv"); err != nil {
			t.Fatalf("Token inicial falló: %v", err)
		}

		resp, err := client.Do(context.Background(), "sv", http.MethodPost, "/plans", []byte(`{}`), nil)
		if err == nil {
			t.Fatalf("se esperaba un error ante el segundo 401 de seguridad")
		}
		if resp != nil {
			t.Errorf("se esperaba resp nil al propagar el error; resp=%v", resp)
		}
		if doer.proxiedCount != 2 {
			t.Errorf("llamadas proxied = %d; se esperaban exactamente 2 (sin tercer intento)", doer.proxiedCount)
		}
		if doer.authCount != 2 {
			t.Errorf("autenticaciones = %d; se esperaban 2 (inicial + re-auth)", doer.authCount)
		}
		if doer.overLimit {
			t.Errorf("se detectó un tercer intento proxied; el reintento no está acotado")
		}
	})
}
