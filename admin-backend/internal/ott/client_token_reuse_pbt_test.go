// Feature: panel-admin-ott, Property 3: Reutilización del JWT OTT por país mientras sea vigente
//
// Prueba basada en propiedades (PBT) del cache de token por país del OTTClient.
//
// Validates: Requirements 3.2, 3.3
//
// Propiedad: *para toda* secuencia arbitraria de llamadas a Token(country)
// sobre los países soportados {gt,sv,ni,hn,cr}, mientras el reloj no avance más
// allá de la expiración de los tokens cacheados, el OTTClient se autentica
// contra POST /authenticate COMO MÁXIMO UNA VEZ por país distinto. Es decir, el
// token se obtiene una sola vez por país y se reutiliza en las llamadas
// siguientes sin volver a autenticarse, con independencia del orden o la
// repetición de las llamadas.
//
// Se usa la librería estándar `testing/quick` (mínimo 100 iteraciones). Se
// inyecta un `httpDoer` falso que cuenta las autenticaciones por país y devuelve
// un JWT con tres segmentos y un claim `exp` muy lejano en el futuro, junto con
// un reloj fijo, de modo que ningún token cacheado expire durante la secuencia.
package ott

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
	"testing/quick"
	"time"

	"admin-backend/internal/config"
)

// tokenReuseCountries es el conjunto de países soportados sobre el que se
// generan las secuencias de llamadas.
var tokenReuseCountries = []string{"gt", "sv", "ni", "hn", "cr"}

// tokenReuseFixedClock es un instante base determinista para el reloj inyectado.
var tokenReuseFixedClock = time.Date(2024, time.January, 1, 12, 0, 0, 0, time.UTC)

// tokenReuseCountingDoer es un httpDoer de prueba que atiende las peticiones a
// /authenticate contando cuántas veces se autentica cada país. Devuelve un JWT
// con `exp` muy lejano para que el token cacheado nunca expire con el reloj fijo.
type tokenReuseCountingDoer struct {
	mu     sync.Mutex
	counts map[string]int
	// expUnix es el claim exp (segundos Unix) incrustado en el JWT devuelto.
	expUnix int64
}

// Do implementa httpDoer. Interpreta el cuerpo de /authenticate para derivar el
// país a partir del clientId (con formato "<country>-client-id") e incrementa
// el contador de autenticaciones de ese país.
func (d *tokenReuseCountingDoer) Do(req *http.Request) (*http.Response, error) {
	if !strings.HasSuffix(req.URL.Path, authEndpoint) {
		return nil, fmt.Errorf("tokenReuse: endpoint inesperado %q", req.URL.Path)
	}

	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	var creds struct {
		ClientID     string `json:"clientId"`
		ClientSecret string `json:"clientSecret"`
	}
	if err := json.Unmarshal(bodyBytes, &creds); err != nil {
		return nil, err
	}

	country := strings.TrimSuffix(creds.ClientID, "-client-id")

	d.mu.Lock()
	d.counts[country]++
	d.mu.Unlock()

	token := tokenReuseSignedJWT(d.expUnix)
	respBody, _ := json.Marshal(authResponse{
		ResultCode: "0",
		Result:     authResult{Authorization: token},
	})

	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(string(respBody))),
		Header:     make(http.Header),
	}, nil
}

// countFor devuelve el número de autenticaciones registradas para un país.
func (d *tokenReuseCountingDoer) countFor(country string) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.counts[country]
}

// tokenReuseSignedJWT construye un JWT con tres segmentos (header.payload.sig)
// cuyo payload declara el claim exp indicado. La firma es un marcador: el
// OTTClient no valida la firma (eso lo hace el propio OTT).
func tokenReuseSignedJWT(expUnix int64) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	payloadJSON, _ := json.Marshal(map[string]interface{}{
		"exp": expUnix,
		"iss": "ott-test",
	})
	payload := base64.RawURLEncoding.EncodeToString(payloadJSON)
	sig := base64.RawURLEncoding.EncodeToString([]byte("tokenReuse-signature"))
	return header + "." + payload + "." + sig
}

// tokenReuseNewClient construye un Client de pruebas con el doble httpDoer y un
// reloj fijo inyectados directamente (ambos campos son accesibles dentro del
// paquete ott). Las credenciales por país usan clientId/clientSecret derivados
// del código de país para que el doble pueda identificarlos.
func tokenReuseNewClient(doer httpDoer, now clock) *Client {
	creds := make(map[string]config.OTTCredentials, len(tokenReuseCountries))
	for _, c := range tokenReuseCountries {
		creds[c] = config.OTTCredentials{
			ClientID:     c + "-client-id",
			ClientSecret: c + "-client-secret",
		}
	}
	return &Client{
		baseURL:     "https://ott.test/api/v1",
		credentials: creds,
		httpClient:  doer,
		now:         now,
		tokens:      make(map[string]tokenEntry),
	}
}

// tokenReuseCallSeq es una secuencia arbitraria de llamadas a Token: cada
// elemento es un índice dentro de tokenReuseCountries.
type tokenReuseCallSeq struct {
	indices []int
}

// Generate satisface quick.Generator produciendo secuencias de longitud y
// contenido variados, con repeticiones (para ejercitar la reutilización).
func (tokenReuseCallSeq) Generate(rnd *rand.Rand, size int) reflect.Value {
	// Al menos una llamada; hasta ~2x size para forzar repeticiones.
	n := rnd.Intn(size*2+1) + 1
	indices := make([]int, n)
	for i := range indices {
		indices[i] = rnd.Intn(len(tokenReuseCountries))
	}
	return reflect.ValueOf(tokenReuseCallSeq{indices: indices})
}

// TestTokenReusePerCountryAtMostOneAuth verifica la Property 3: para cualquier
// secuencia de llamadas a Token sobre los países soportados, con los tokens
// vigentes (reloj no avanzado), el cliente se autentica como máximo una vez por
// país distinto, reutilizando el token cacheado en el resto de llamadas.
func TestTokenReusePerCountryAtMostOneAuth(t *testing.T) {
	property := func(seq tokenReuseCallSeq) bool {
		// exp muy lejano en el futuro respecto al reloj fijo: ningún token
		// cacheado expira durante la secuencia.
		expUnix := tokenReuseFixedClock.Add(24 * time.Hour).Unix()
		doer := &tokenReuseCountingDoer{
			counts:  make(map[string]int),
			expUnix: expUnix,
		}
		fixedNow := func() time.Time { return tokenReuseFixedClock }
		client := tokenReuseNewClient(doer, fixedNow)

		ctx := context.Background()

		// Países efectivamente invocados en esta secuencia.
		invoked := make(map[string]bool)
		var firstToken string
		for _, idx := range seq.indices {
			country := tokenReuseCountries[idx]
			invoked[country] = true

			tok, err := client.Token(ctx, country)
			if err != nil {
				return false
			}
			// El token devuelto debe ser siempre el mismo (reutilizado).
			if firstToken == "" {
				firstToken = tok
			}
			if tok != tokenReuseSignedJWT(expUnix) {
				return false
			}
		}

		// Cada país invocado se autentica exactamente una vez; cualquier país no
		// invocado, cero. En ningún caso el contador supera 1.
		for _, country := range tokenReuseCountries {
			got := doer.countFor(country)
			if got > 1 {
				return false
			}
			if invoked[country] && got != 1 {
				return false
			}
			if !invoked[country] && got != 0 {
				return false
			}
		}
		return true
	}

	cfg := &quick.Config{MaxCount: pbtIterations}
	if err := quick.Check(property, cfg); err != nil {
		t.Fatalf("Property 3 violada: el JWT OTT debe reutilizarse por país (como máximo una autenticación por país vigente): %v", err)
	}
}

// TestTokenReuseKnownSequences complementa la generación aleatoria con casos
// deterministas que ejercitan la reutilización y el aislamiento por país.
func TestTokenReuseKnownSequences(t *testing.T) {
	cases := []struct {
		name      string
		calls     []string
		wantAuth  map[string]int
	}{
		{
			name:     "mismo país repetido reutiliza el token",
			calls:    []string{"gt", "gt", "gt", "gt"},
			wantAuth: map[string]int{"gt": 1},
		},
		{
			name:     "países distintos se autentican una vez cada uno",
			calls:    []string{"gt", "sv", "ni", "hn", "cr"},
			wantAuth: map[string]int{"gt": 1, "sv": 1, "ni": 1, "hn": 1, "cr": 1},
		},
		{
			name:     "intercalado no reautentica países ya vigentes",
			calls:    []string{"gt", "sv", "gt", "sv", "gt", "cr", "cr", "sv"},
			wantAuth: map[string]int{"gt": 1, "sv": 1, "cr": 1},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			expUnix := tokenReuseFixedClock.Add(24 * time.Hour).Unix()
			doer := &tokenReuseCountingDoer{
				counts:  make(map[string]int),
				expUnix: expUnix,
			}
			client := tokenReuseNewClient(doer, func() time.Time { return tokenReuseFixedClock })

			wantToken := tokenReuseSignedJWT(expUnix)
			for _, country := range tc.calls {
				tok, err := client.Token(context.Background(), country)
				if err != nil {
					t.Fatalf("Token(%q) devolvió error inesperado: %v", country, err)
				}
				if tok != wantToken {
					t.Errorf("Token(%q) = %q; se esperaba el JWT cacheado %q", country, tok, wantToken)
				}
			}

			for _, country := range tokenReuseCountries {
				want := tc.wantAuth[country]
				if got := doer.countFor(country); got != want {
					t.Errorf("autenticaciones de %q = %d; se esperaban %d", country, got, want)
				}
			}
		})
	}
}
