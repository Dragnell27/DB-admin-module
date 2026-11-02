// Package ott centraliza la integración del admin-backend (BFF) con las APIs
// OTT existentes del backend hub-services-ott. El OTTClient se autentica por
// país contra POST /api/v1/authenticate, obtiene el JWT OTT y lo cachea/reutiliza
// por país hasta su expiración (Req. 3.1, 3.2, 3.3, 3.4, 3.7).
package ott

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"admin-backend/internal/config"
)

// defaultTokenTTL es la vigencia aplicada a un JWT OTT cuando no se puede
// derivar su expiración desde el propio token. El OTT emite tokens RS256 con
// expiración aproximada de 24 horas; se usa un margen conservador por defecto.
const defaultTokenTTL = 23 * time.Hour

// expirySkew es el margen de seguridad que se resta a la expiración declarada
// del token para evitar usarlo justo cuando el OTT ya lo considera vencido.
const expirySkew = 60 * time.Second

// authEndpoint es la ruta (relativa a OTTBaseURL) del endpoint de autenticación
// del backend OTT.
const authEndpoint = "/authenticate"

// securityErrorCodePrefix identifica los errorCode de seguridad del backend OTT
// que indican que el JWT OTT presentado es inválido o ha expirado (p. ej.
// "S401SEC"). Ante un 401 con un errorCode de este tipo, el OTTClient debe
// reautenticarse y reintentar la llamada una sola vez (Req. 3.5).
const securityErrorCodePrefix = "S401"

// httpDoer abstrae el cliente HTTP para facilitar las pruebas sustituyendo la
// implementación real por un doble de prueba.
type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// clock abstrae la obtención del tiempo actual para facilitar pruebas
// deterministas del cache de tokens.
type clock func() time.Time

// tokenEntry representa un JWT OTT cacheado junto con el instante en que expira.
type tokenEntry struct {
	token     string
	expiresAt time.Time
}

// authResult modela el campo result de la respuesta de autenticación OTT:
// { "result": { "Authorization": "<JWT OTT>" } }.
type authResult struct {
	Authorization string `json:"Authorization"`
}

// authResponse modela la respuesta estándar del endpoint de autenticación OTT
// en su variante resultCode/resultMessage/errorCode/result.
type authResponse struct {
	ResultCode    string     `json:"resultCode"`
	ResultMessage string     `json:"resultMessage"`
	ErrorCode     string     `json:"errorCode"`
	Result        authResult `json:"result"`
}

// Client es el cliente HTTP hacia las APIs OTT. Mantiene un cache de JWT OTT por
// país protegido con mutex y reutiliza el token hasta su expiración (Req. 3.3).
//
// authBaseURL y baseURL pueden apuntar a hosts distintos: en CENAM la
// autenticación (POST /authenticate) vive en un host de pagos y los recursos
// (customers/plans/subscriptions/cards) en el host OTT. Cuando authBaseURL está
// vacío se usa baseURL para autenticar (ambos en el mismo host).
type Client struct {
	authBaseURL string
	baseURL     string
	credentials map[string]config.OTTCredentials
	httpClient  httpDoer
	now         clock

	mu     sync.Mutex
	tokens map[string]tokenEntry
}

// NewClient construye un OTTClient a partir de la configuración del BFF. Las
// credenciales por país y la URL base del OTT provienen exclusivamente de la
// configuración (nunca embebidas en el código).
func NewClient(cfg *config.Config) *Client {
	// La base de autenticación es independiente; si no se configura, se usa la
	// base de recursos (comportamiento previo).
	authBase := strings.TrimRight(cfg.OTTAuthBaseURL, "/")
	resourceBase := strings.TrimRight(cfg.OTTBaseURL, "/")
	if authBase == "" {
		authBase = resourceBase
	}
	return &Client{
		authBaseURL: authBase,
		baseURL:     resourceBase,
		credentials: cfg.OTTCredentialsByCountry,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		now:         time.Now,
		tokens:      make(map[string]tokenEntry),
	}
}

// Token devuelve un JWT OTT vigente para el país indicado. Si existe un token
// cacheado que aún no ha expirado, lo reutiliza sin autenticarse de nuevo
// (Req. 3.2, 3.3). En caso contrario, se autentica contra el OTT usando las
// credenciales del país (Req. 3.2, 3.7), cachea el resultado y lo devuelve.
func (c *Client) Token(ctx context.Context, country string) (string, error) {
	country = strings.ToLower(strings.TrimSpace(country))

	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, ok := c.tokens[country]; ok && c.now().Before(entry.expiresAt) {
		return entry.token, nil
	}

	entry, err := c.authenticate(ctx, country)
	if err != nil {
		return "", err
	}
	c.tokens[country] = entry
	return entry.token, nil
}

// invalidate elimina del cache el token del país indicado, forzando una nueva
// autenticación en la próxima llamada a Token. Se usa cuando el OTT rechaza el
// JWT OTT con un 401 de seguridad (Req. 3.5).
func (c *Client) invalidate(country string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.tokens, country)
}

// Do ejecuta una llamada autenticada hacia un endpoint OTT protegido para el
// país indicado. Adjunta el JWT OTT vigente como `Authorization: Bearer <JWT>`
// (Req. 3.4) reutilizando el token cacheado del país (Req. 3.3).
//
// Si el OTT responde 401 con un errorCode de seguridad (p. ej. S401SEC), el
// token cacheado del país se invalida, el cliente se reautentica y la llamada
// se reintenta UNA sola vez (Req. 3.5). Un segundo 401 de seguridad se propaga
// como error, evitando reintentos indefinidos. Si la reautenticación falla, se
// propaga un error claro indicando que la sesión OTT no pudo establecerse
// (Req. 3.6).
//
// path es relativo a la URL base del OTT. body puede ser nil. headers permite
// añadir cabeceras adicionales (p. ej. Content-Type); la cabecera Authorization
// la gestiona el propio cliente y no debe pasarse en headers.
//
// La respuesta devuelta tiene su Body ya rebobinable (buffer en memoria) para
// que el llamador (normalizador/handler) pueda leerlo íntegramente. El llamador
// es responsable de cerrar resp.Body.
func (c *Client) Do(ctx context.Context, country, method, path string, body []byte, headers map[string]string) (*http.Response, error) {
	country = strings.ToLower(strings.TrimSpace(country))

	resp, respBody, err := c.doOnce(ctx, country, method, path, body, headers)
	if err != nil {
		return nil, err
	}

	// Si no es un 401 de seguridad, se devuelve la respuesta tal cual.
	if !isSecurity401(resp.StatusCode, respBody) {
		return rewindResponse(resp, respBody), nil
	}

	// 401 de seguridad: invalidar el token del país y reautenticar una vez.
	resp.Body.Close()
	c.invalidate(country)

	if _, err := c.Token(ctx, country); err != nil {
		return nil, fmt.Errorf("ott: no se pudo establecer la sesión con el servicio OTT: %w", err)
	}

	// Reintento único con el token recién obtenido.
	retryResp, retryBody, err := c.doOnce(ctx, country, method, path, body, headers)
	if err != nil {
		return nil, err
	}

	// Un segundo 401 de seguridad se propaga como error (sin más reintentos).
	if isSecurity401(retryResp.StatusCode, retryBody) {
		retryResp.Body.Close()
		return nil, fmt.Errorf("ott: la reautenticación no resolvió el 401 de seguridad del servicio OTT para el país %q", country)
	}

	return rewindResponse(retryResp, retryBody), nil
}

// doOnce realiza una única llamada autenticada (sin reintentos): obtiene el
// token vigente del país, construye la petición con el Bearer y las cabeceras
// indicadas, la ejecuta y lee el cuerpo de la respuesta por completo. Devuelve
// la respuesta (con el Body original aún cerrado por leer) junto al cuerpo ya
// leído para permitir la inspección del errorCode sin perderlo.
func (c *Client) doOnce(ctx context.Context, country, method, path string, body []byte, headers map[string]string) (*http.Response, []byte, error) {
	token, err := c.Token(ctx, country)
	if err != nil {
		return nil, nil, fmt.Errorf("ott: no se pudo establecer la sesión con el servicio OTT: %w", err)
	}

	url := c.baseURL + ensureLeadingSlash(path)

	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return nil, nil, fmt.Errorf("ott: no se pudo crear la petición hacia OTT: %w", err)
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
	}
	if len(body) > 0 && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("ott: fallo de red al llamar al servicio OTT: %w", err)
	}

	respBody, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, nil, fmt.Errorf("ott: no se pudo leer la respuesta del servicio OTT: %w", err)
	}

	return resp, respBody, nil
}

// rewindResponse reemplaza el Body (ya consumido) de la respuesta por un lector
// rebobinable sobre el cuerpo leído, de modo que el llamador pueda leerlo de
// nuevo. Ajusta ContentLength al tamaño real del cuerpo.
func rewindResponse(resp *http.Response, body []byte) *http.Response {
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	return resp
}

// isSecurity401 determina si una respuesta corresponde a un 401 de seguridad del
// OTT: HTTP 401 cuyo errorCode (en cualquiera de las dos variantes de la
// respuesta estándar OTT) corresponde a un error de seguridad (p. ej. S401SEC).
func isSecurity401(statusCode int, body []byte) bool {
	if statusCode != http.StatusUnauthorized {
		return false
	}
	return isSecurityErrorCode(extractErrorCode(body))
}

// isSecurityErrorCode indica si el errorCode dado corresponde a un error de
// seguridad del OTT. Se considera de seguridad cualquier código que comience por
// "S401" (p. ej. "S401SEC"), de forma case-insensitive.
func isSecurityErrorCode(code string) bool {
	code = strings.ToUpper(strings.TrimSpace(code))
	return strings.HasPrefix(code, securityErrorCodePrefix)
}

// extractErrorCode obtiene el campo errorCode del cuerpo de una respuesta OTT,
// soportando ambas variantes de la respuesta estándar. En la variante
// resultCode/resultMessage/errorCode/result el campo es "errorCode"; en la
// variante status/http_code/message/data el error se transmite típicamente en
// "status" o "message". Devuelve cadena vacía si no se puede derivar.
func extractErrorCode(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var parsed struct {
		ErrorCode string `json:"errorCode"`
		Status    string `json:"status"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return ""
	}
	if parsed.ErrorCode != "" {
		return parsed.ErrorCode
	}
	return parsed.Status
}

// ensureLeadingSlash garantiza que el path comience con "/" para componerlo con
// la URL base del OTT sin duplicar ni omitir el separador.
func ensureLeadingSlash(path string) string {
	if path == "" {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		return "/" + path
	}
	return path
}

// authenticate ejecuta POST /authenticate contra el OTT con las credenciales del
// país y construye la entrada de cache con la expiración derivada del token.
// No adquiere el mutex: el llamador (Token) ya lo mantiene.
func (c *Client) authenticate(ctx context.Context, country string) (tokenEntry, error) {
	creds, ok := c.credentials[country]
	if !ok {
		return tokenEntry{}, fmt.Errorf("ott: país no soportado %q", country)
	}
	if creds.ClientID == "" || creds.ClientSecret == "" {
		return tokenEntry{}, fmt.Errorf("ott: credenciales no configuradas para el país %q", country)
	}

	payload, err := json.Marshal(map[string]string{
		"clientId":     creds.ClientID,
		"clientSecret": creds.ClientSecret,
	})
	if err != nil {
		return tokenEntry{}, fmt.Errorf("ott: no se pudo construir la petición de autenticación: %w", err)
	}

	url := c.authBaseURL + authEndpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return tokenEntry{}, fmt.Errorf("ott: no se pudo crear la petición de autenticación: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return tokenEntry{}, fmt.Errorf("ott: fallo de red al autenticar contra OTT: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return tokenEntry{}, fmt.Errorf("ott: no se pudo leer la respuesta de autenticación OTT: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return tokenEntry{}, fmt.Errorf("ott: autenticación rechazada por el backend OTT (status %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed authResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return tokenEntry{}, fmt.Errorf("ott: respuesta de autenticación OTT con formato inválido: %w", err)
	}

	token := strings.TrimSpace(parsed.Result.Authorization)
	if token == "" {
		return tokenEntry{}, fmt.Errorf("ott: la respuesta de autenticación OTT no contiene un JWT")
	}

	return tokenEntry{
		token:     token,
		expiresAt: c.tokenExpiry(token),
	}, nil
}

// tokenExpiry determina el instante de expiración de un JWT OTT. Intenta leer el
// claim "exp" del token (RS256) y le resta un pequeño margen de seguridad; si no
// puede derivarlo, aplica la vigencia por defecto desde el momento actual.
func (c *Client) tokenExpiry(token string) time.Time {
	if exp, ok := parseJWTExp(token); ok {
		candidate := exp.Add(-expirySkew)
		if candidate.After(c.now()) {
			return candidate
		}
	}
	return c.now().Add(defaultTokenTTL)
}

// parseJWTExp extrae el claim "exp" (segundos Unix) del payload de un JWT sin
// validar la firma (la firma la valida el propio OTT). Devuelve false si el
// token no tiene el formato esperado o carece de un "exp" numérico válido.
func parseJWTExp(token string) (time.Time, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return time.Time{}, false
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		// Algunos emisores incluyen padding; se intenta la variante con padding.
		payload, err = base64.URLEncoding.DecodeString(parts[1])
		if err != nil {
			return time.Time{}, false
		}
	}

	var claims struct {
		Exp *float64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return time.Time{}, false
	}
	if claims.Exp == nil {
		return time.Time{}, false
	}

	return time.Unix(int64(*claims.Exp), 0), true
}
