package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"admin-backend/internal/ott"
	"admin-backend/internal/ottcache"
	"admin-backend/internal/validation"
)

// PlansHandler agrupa los handlers HTTP del recurso Plans del admin-backend
// (BFF). Cada operación valida los parámetros/el cuerpo con las reglas espejo
// (internal/validation) y, si son válidos, proxya la llamada al backend OTT a
// través del *ott.Client, devolviendo el resultado normalizado al
// admin-frontend (Req. 9, 10, 11, 12, 20.2, 20.3).
//
// Mapeo de rutas BFF → OTT (design.md "Recursos (mapeo a OTT)"):
//   - GET    /plans  → GET /plans    (lista paginada o detalle por plan_id)
//   - POST   /plans  → POST /plans   (crear; valida country/moneda vs país activo)
//   - PUT    /plans  → PUT /plans    (editar por plan_id)
//   - DELETE /plans  → DELETE /plans (plan_id por query o body)
type PlansHandler struct {
	ott           *ott.Client
	activeCountry string
	// cache guarda el conjunto completo de plans por país; el listado puro se
	// sirve desde aquí y se refresca por TTL/botón, reduciendo llamadas a OTT.
	cache *ottcache.Cache
}

// NewPlansHandler construye un PlansHandler con el cliente OTT y el país activo
// del BFF (config.OTTActiveCountry). El país activo se usa como `tokenCountry`
// en las validaciones de coherencia moneda↔país (Req. 10.5, 10.7) y para
// seleccionar el token/credenciales OTT del país al proxyar.
func NewPlansHandler(ottClient *ott.Client, activeCountry string, cache *ottcache.Cache) *PlansHandler {
	return &PlansHandler{
		ott:           ottClient,
		activeCountry: strings.ToLower(strings.TrimSpace(activeCountry)),
		cache:         cache,
	}
}

// effectiveCountry devuelve el país resuelto por el middleware CountryResolver
// para esta petición, o el país por defecto del handler si no está presente. Es
// el país que se usa como `tokenCountry` en la validación moneda↔país y como
// país de la llamada OTT (credenciales/JWT y enrutamiento).
func (h *PlansHandler) effectiveCountry(c *gin.Context) string {
	if country, ok := CountryFromContext(c); ok {
		return country
	}
	return h.activeCountry
}

// plansOTTPath es la ruta del recurso Plans en el backend OTT (relativa a la
// URL base del OTT).
const plansOTTPath = "/plans"

// planInputBody modela el cuerpo JSON de creación/edición de Plan tal como lo
// envía el admin-frontend y lo espera el contrato OTT (plan_handler.go del OTT).
// Los punteros representan campos opcionales/emparejados: nil significa ausente,
// lo que permite distinguir "no enviado" de "enviado en cero" en la edición y en
// los grupos condicionales (monto variable, frequency_test, oneshot).
type planInputBody struct {
	PlanID         string  `json:"plan_id,omitempty"`
	Name           string  `json:"name"`
	ExternalID     string  `json:"external_id"`
	Country        string  `json:"country"`
	Amount         float64 `json:"amount"`
	Currency       string  `json:"currency"`
	Frequency      int     `json:"frequency"`
	PeriodType     string  `json:"period_type"`
	VariableAmount bool    `json:"variable_amount"`

	MinimumAmount *float64 `json:"minimum_amount,omitempty"`
	MaximumAmount *float64 `json:"maximum_amount,omitempty"`

	FrequencyTest  *int    `json:"frequency_test,omitempty"`
	TestTypePeriod *string `json:"test_type_period,omitempty"`

	OneshotAmount    *float64 `json:"oneshot_amount,omitempty"`
	OneshotFrequency *int     `json:"oneshot_frequency,omitempty"`
	OneshotPeriod    *string  `json:"oneshot_period,omitempty"`

	// Campos adicionales del contrato OTT que el BFF transporta tal cual sin
	// validarlos aquí (no tienen reglas espejo definidas en el diseño).
	MaxRetrys           *int    `json:"max_retrys,omitempty"`
	CanDuplicate        *bool   `json:"can_duplicate,omitempty"`
	CanSubscribe        *bool   `json:"can_subscribe,omitempty"`
	MaxUserSubscription *int    `json:"max_user_subscription,omitempty"`
	Status              *string `json:"status,omitempty"`
}

// toValidationInput traduce el cuerpo recibido al PlanInput que consumen las
// reglas espejo (ValidatePlanCreate/ValidatePlanEdit). Solo transporta los
// campos sujetos a validación.
func (b planInputBody) toValidationInput() validation.PlanInput {
	return validation.PlanInput{
		Name:             b.Name,
		ExternalID:       b.ExternalID,
		Country:          b.Country,
		Amount:           b.Amount,
		Currency:         b.Currency,
		Frequency:        b.Frequency,
		PeriodType:       b.PeriodType,
		VariableAmount:   b.VariableAmount,
		MinimumAmount:    b.MinimumAmount,
		MaximumAmount:    b.MaximumAmount,
		FrequencyTest:    b.FrequencyTest,
		TestTypePeriod:   b.TestTypePeriod,
		OneshotAmount:    b.OneshotAmount,
		OneshotFrequency: b.OneshotFrequency,
		OneshotPeriod:    b.OneshotPeriod,
	}
}

// List maneja GET /plans. Si viene `plan_id` en la query, delega el detalle de
// ese plan; en caso contrario valida/normaliza la paginación y el ordenamiento
// (Req. 9.3, 9.4) y proxya el listado al OTT. Un 404 del OTT en el listado se
// normaliza como "sin resultados" (Req. 9.5) por el normalizador.
func (h *PlansHandler) List(c *gin.Context) {
	// Listado: validar paginación y ordenamiento con las reglas espejo.
	page := plansAtoiDefault(c.Query("page"), 0)
	recordsPerPage := plansAtoiDefault(c.Query("records_per_page"), 0)

	pagination, err := validation.NormalizePagination(page, recordsPerPage)
	if err != nil {
		writeError(c, http.StatusBadRequest, "E400", err.Error())
		return
	}

	ordering, err := validation.NormalizeOrdering(validation.ResourcePlans, c.Query("order_by"), c.Query("order"))
	if err != nil {
		writeError(c, http.StatusBadRequest, "E400", err.Error())
		return
	}

	// Listado puro: servir desde el caché (conjunto completo por país), paginando
	// y ordenando en memoria.
	country := h.effectiveCountry(c)
	records, err := h.cache.Get(c.Request.Context(), ottcache.ResourcePlans, country, h.loadAll)
	if err != nil {
		writeError(c, http.StatusBadGateway, "E502", "No se pudo obtener el listado de planes desde OTT")
		return
	}

	// Filtros propios sobre el conjunto cacheado (OTT no filtra el listado de
	// planes). `plan_id` coincide por id del plan (que OTT entrega como `id` o
	// `plan_id`); `external_id` es coincidencia exacta; `name` es subcadena.
	if planID := strings.TrimSpace(c.Query("plan_id")); planID != "" {
		byID := ottcache.FilterByField(records, "id", planID)
		byPlanID := ottcache.FilterByField(records, "plan_id", planID)
		// Unir ambos resultados sin duplicar (un registro usa uno u otro campo).
		if len(byID) > 0 {
			records = byID
		} else {
			records = byPlanID
		}
	}
	if externalID := strings.TrimSpace(c.Query("external_id")); externalID != "" {
		records = ottcache.FilterByField(records, "external_id", externalID)
	}
	if name := strings.TrimSpace(c.Query("name")); name != "" {
		records = ottcache.FilterByFieldContains(records, "name", name)
	}

	writeCachedPage(c, records, pagination, ordering)
}

// loadAll obtiene el conjunto completo de plans de un país recorriendo todas las
// páginas de OTT. Es el Loader del caché.
func (h *PlansHandler) loadAll(ctx context.Context, country string) ([]ottcache.Record, error) {
	return ottcache.LoadAllPages(ctx, func(ctx context.Context, page, perPage int) (ott.NormalizedResponse, error) {
		q := url.Values{}
		q.Set("page", strconv.Itoa(page))
		q.Set("records_per_page", strconv.Itoa(perPage))
		q.Set("order_by", "created_at")
		q.Set("order", "asc")
		path := plansEnsureCountryQuery(plansParamString(q), country)

		headers := map[string]string{"X-Country": country}
		resp, err := h.ott.Do(ctx, country, http.MethodGet, path, nil, headers)
		if err != nil {
			return ott.NormalizedResponse{}, err
		}
		defer resp.Body.Close()
		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			return ott.NormalizedResponse{}, err
		}
		return ott.Normalize(resp.StatusCode, raw), nil
	})
}

// Refresh maneja POST /plans/refresh: fuerza recargar el conjunto completo de
// plans del país efectivo. Protegido por (plans, recargar) en el router.
func (h *PlansHandler) Refresh(c *gin.Context) {
	country := h.effectiveCountry(c)
	count, err := h.cache.Refresh(c.Request.Context(), ottcache.ResourcePlans, country, h.loadAll)
	if err != nil {
		writeError(c, http.StatusBadGateway, "E502", "No se pudo actualizar el caché de planes desde OTT")
		return
	}
	writeOK(c, http.StatusOK, gin.H{
		"resource":    "plans",
		"country":     country,
		"count":       count,
		"refreshedAt": nowRFC3339(),
	}, "Caché de planes actualizado")
}

// Detail maneja la consulta de un Plan concreto por `plan_id`. Acepta el
// identificador desde el parámetro de ruta `plan_id` (si la ruta lo define) o
// desde la query, y lo traslada al OTT como query param `plan_id`
// (el contrato OTT identifica el plan por query/body, no por URL) (Req. 9.2).
func (h *PlansHandler) Detail(c *gin.Context) {
	planID := strings.TrimSpace(c.Param("plan_id"))
	if planID == "" {
		planID = strings.TrimSpace(c.Query("plan_id"))
	}
	if planID == "" {
		writeError(c, http.StatusBadRequest, "E400", "plan_id es requerido")
		return
	}

	query := url.Values{}
	query.Set("plan_id", planID)
	h.proxy(c, http.MethodGet, plansParamString(query), nil)
}

// Create maneja POST /plans. Decodifica el cuerpo, valida con
// ValidatePlanCreate usando el país activo del BFF como `tokenCountry`
// (coherencia country↔moneda y reglas de grupos condicionales, Req. 10.2-10.12)
// y, si es válido, proxya la creación al OTT (Req. 10.13). El resultado o error
// del OTT se devuelve normalizado (Req. 10.14, 10.15).
func (h *PlansHandler) Create(c *gin.Context) {
	body, raw, ok := h.decodeBody(c)
	if !ok {
		return
	}

	if err := validation.ValidatePlanCreate(h.effectiveCountry(c), body.toValidationInput()); err != nil {
		writeError(c, http.StatusBadRequest, "E400", err.Error())
		return
	}

	h.proxyInvalidating(c, http.MethodPost, plansOTTPath, raw, true)
}

// Edit maneja PUT /plans. Identifica el plan por `plan_id` (query o cuerpo),
// valida los campos presentes con ValidatePlanEdit (todos opcionales pero, si
// vienen, deben cumplir formato/rango/emparejamiento y coherencia país↔moneda,
// Req. 11.2) y proxya la edición al OTT identificando el plan por plan_id
// (Req. 11.3). El OTT acepta `plan_id` por query o body; el BFF lo adjunta como
// query param para no depender de que venga en el cuerpo.
func (h *PlansHandler) Edit(c *gin.Context) {
	body, raw, ok := h.decodeBody(c)
	if !ok {
		return
	}

	planID := strings.TrimSpace(c.Query("plan_id"))
	if planID == "" {
		planID = strings.TrimSpace(body.PlanID)
	}
	if planID == "" {
		writeError(c, http.StatusBadRequest, "E400", "plan_id es requerido")
		return
	}

	if err := validation.ValidatePlanEdit(h.effectiveCountry(c), body.toValidationInput()); err != nil {
		writeError(c, http.StatusBadRequest, "E400", err.Error())
		return
	}

	query := url.Values{}
	query.Set("plan_id", planID)
	h.proxyInvalidating(c, http.MethodPut, plansOTTPath+plansParamSuffix(query), raw, true)
}

// Delete maneja DELETE /plans. Identifica el plan por `plan_id` (query o
// cuerpo) y proxya la eliminación al OTT (Req. 12.2), devolviendo el resultado
// o error normalizado (Req. 12.4, 12.5).
func (h *PlansHandler) Delete(c *gin.Context) {
	planID := strings.TrimSpace(c.Query("plan_id"))
	if planID == "" {
		// El plan_id puede venir en el cuerpo según el contrato OTT.
		if raw := plansReadBody(c); len(raw) > 0 {
			var body struct {
				PlanID string `json:"plan_id"`
			}
			if err := json.Unmarshal(raw, &body); err == nil {
				planID = strings.TrimSpace(body.PlanID)
			}
		}
	}
	if planID == "" {
		writeError(c, http.StatusBadRequest, "E400", "plan_id es requerido")
		return
	}

	query := url.Values{}
	query.Set("plan_id", planID)
	h.proxyInvalidating(c, http.MethodDelete, plansOTTPath+plansParamSuffix(query), nil, true)
}

// decodeBody lee y decodifica el cuerpo JSON de la petición en un planInputBody,
// devolviendo también el cuerpo crudo (para reenviarlo íntegro al OTT) y un
// indicador de éxito. Ante un cuerpo ausente o malformado responde 400 y
// devuelve ok=false.
func (h *PlansHandler) decodeBody(c *gin.Context) (planInputBody, []byte, bool) {
	raw := plansReadBody(c)
	if len(raw) == 0 {
		writeError(c, http.StatusBadRequest, "E400", "El cuerpo de la solicitud es requerido")
		return planInputBody{}, nil, false
	}

	var body planInputBody
	if err := json.Unmarshal(raw, &body); err != nil {
		writeError(c, http.StatusBadRequest, "E400", "Solicitud inválida")
		return planInputBody{}, nil, false
	}
	return body, raw, true
}

// proxy ejecuta la llamada autenticada al backend OTT para el país activo del
// BFF y escribe la respuesta normalizada al cliente. Adjunta el país activo
// tanto en la cabecera `X-Country` como en el query param `country` para que el
// OTT enrute la petición al servicio del país correcto (handlers.go del OTT).
// Un fallo de comunicación con el OTT se traduce en un 502 normalizado.
func (h *PlansHandler) proxy(c *gin.Context, method, path string, body []byte) {
	h.proxyInvalidating(c, method, path, body, false)
}

// proxyInvalidating es como proxy pero, si invalidate es true y la respuesta OTT
// es exitosa, invalida el caché de plans del país efectivo (Req. 5.1, 5.4).
func (h *PlansHandler) proxyInvalidating(c *gin.Context, method, path string, body []byte, invalidate bool) {
	country := h.effectiveCountry(c)
	path = plansEnsureCountryQuery(path, country)

	headers := map[string]string{
		"X-Country": country,
	}
	if len(body) > 0 {
		headers["Content-Type"] = "application/json"
	}

	resp, err := h.ott.Do(c.Request.Context(), country, method, path, body, headers)
	if err != nil {
		writeError(c, http.StatusBadGateway, "E502", "No se pudo comunicar con el servicio OTT")
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		writeError(c, http.StatusBadGateway, "E502", "No se pudo leer la respuesta del servicio OTT")
		return
	}

	normalized := ott.Normalize(resp.StatusCode, respBody)

	if invalidate && resp.StatusCode >= 200 && resp.StatusCode < 300 && normalized.OK {
		h.cache.Invalidate(ottcache.ResourcePlans, country)
	}

	c.JSON(plansStatusFor(resp.StatusCode, normalized.OK), normalized)
}

// plansReadBody lee el cuerpo de la petición por completo y lo devuelve. No
// falla ante cuerpo ausente: devuelve un slice vacío.
func plansReadBody(c *gin.Context) []byte {
	if c.Request == nil || c.Request.Body == nil {
		return nil
	}
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return nil
	}
	return raw
}

// plansAtoiDefault convierte una cadena a entero; si está vacía o es inválida,
// devuelve el valor por defecto indicado. Permite que la normalización de
// paginación interprete la ausencia (0) como "usar el valor por defecto".
func plansAtoiDefault(s string, def int) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

// plansParamString compone la ruta del recurso Plans con los parámetros de
// consulta codificados (incluyendo el separador '?').
func plansParamString(query url.Values) string {
	return plansOTTPath + plansParamSuffix(query)
}

// plansParamSuffix devuelve el sufijo de query (`?a=b&...`) para los valores
// dados, o cadena vacía si no hay parámetros.
func plansParamSuffix(query url.Values) string {
	if len(query) == 0 {
		return ""
	}
	return "?" + query.Encode()
}

// plansEnsureCountryQuery garantiza que la ruta incluya el query param
// `country` con el país activo, agregándolo si aún no está presente. El OTT
// enruta por país a partir de la cabecera o este parámetro.
func plansEnsureCountryQuery(path, country string) string {
	if country == "" {
		return path
	}
	if strings.Contains(path, "country=") {
		return path
	}
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return path + sep + "country=" + url.QueryEscape(country)
}

// plansStatusFor determina el código HTTP con que el BFF responde al front a
// partir del código del OTT y del resultado normalizado. Un listado vacío
// (404 del OTT normalizado como OK) se expone como 200, de modo que el front lo
// trate como "sin resultados" y no como error (Req. 9.5). En cualquier otro
// caso se preserva el código del OTT.
func plansStatusFor(ottStatus int, ok bool) int {
	if ottStatus == http.StatusNotFound && ok {
		return http.StatusOK
	}
	return ottStatus
}
