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

// SubscriptionsHandler agrupa los handlers HTTP del recurso Subscriptions del
// admin-backend (BFF). Cada método valida la entrada con la validación espejo
// del BFF (internal/validation) y, si procede, proxya la petición al backend
// OTT (hub-services-ott) reutilizando el OTTClient (token por país, cache y
// reautenticación) y devolviendo la respuesta normalizada al admin-frontend.
//
// Mapeo a OTT (design.md "Recursos (mapeo a OTT)", Req. 13-16, 20.2):
//   - GET    /subscriptions        → GET  /subscriptions   (lista/detalle/filtros)
//   - POST   /subscriptions        → POST /subscriptions   (crear)
//   - PUT    /subscriptions/change → PUT  /subscriptions/change (cambiar)
//   - POST   /subscriptions/cancel → POST /subscriptions/cancel (cancelar)
//
// Las rutas de escritura (Create/Change/Cancel) las protege el middleware de
// rol en el wiring del router (tarea 9.1); este handler no reimplementa el
// control de acceso.
type SubscriptionsHandler struct {
	ott           *ott.Client
	activeCountry string
	// cache guarda el conjunto completo de subscriptions por país; el listado y
	// los filtros por customer_id/plan_id se resuelven en memoria sobre él.
	cache *ottcache.Cache
}

// NewSubscriptionsHandler construye un SubscriptionsHandler con el OTTClient y
// el país activo del BFF (config.OTTActiveCountry). Todas las llamadas a OTT se
// hacen en nombre de ese país (el front no elige país; design.md "Manejo del
// país").
func NewSubscriptionsHandler(ottClient *ott.Client, activeCountry string, cache *ottcache.Cache) *SubscriptionsHandler {
	return &SubscriptionsHandler{
		ott:           ottClient,
		activeCountry: strings.ToLower(strings.TrimSpace(activeCountry)),
		cache:         cache,
	}
}

// effectiveCountry devuelve el país resuelto por el middleware CountryResolver
// para esta petición, o el país por defecto del handler si no está presente.
func (h *SubscriptionsHandler) effectiveCountry(c *gin.Context) string {
	if country, ok := CountryFromContext(c); ok {
		return country
	}
	return h.activeCountry
}

// subscriptionCreateRequest es el cuerpo esperado por POST /subscriptions
// (design.md subscriptionCreateSchema, Req. 14). Se reenvía tal cual a OTT tras
// validarlo con la validación espejo del BFF.
type subscriptionCreateRequest struct {
	PlanID             string   `json:"plan_id"`
	CustomerID         string   `json:"customer_id"`
	PaymentMethod      string   `json:"payment_method"`
	PaymentMethodToken string   `json:"payment_method_token"`
	Amount             *float64 `json:"amount,omitempty"`
	StartDate          *string  `json:"start_date,omitempty"`
}

// subscriptionChangeRequest es el cuerpo esperado por PUT /subscriptions/change
// (Req. 15). Todos los campos son opcionales; se validan cuando están presentes.
// subscription_id/customer_id identifican la suscripción a cambiar según el
// contrato OTT y se reenvían tal cual.
type subscriptionChangeRequest struct {
	SubscriptionID     *string  `json:"subscription_id,omitempty"`
	PlanID             *string  `json:"plan_id,omitempty"`
	CustomerID         *string  `json:"customer_id,omitempty"`
	PaymentMethod      *string  `json:"payment_method,omitempty"`
	PaymentMethodToken *string  `json:"payment_method_token,omitempty"`
	Amount             *float64 `json:"amount,omitempty"`
	StartDate          *string  `json:"start_date,omitempty"`
}

// subscriptionCancelRequest es el cuerpo esperado por POST /subscriptions/cancel
// (Req. 16): customer_id (UUID requerido) y cancel_reason (opcional). También
// admite subscription_id si el front lo envía, reenviándolo a OTT.
type subscriptionCancelRequest struct {
	SubscriptionID *string `json:"subscription_id,omitempty"`
	CustomerID     string  `json:"customer_id"`
	CancelReason   *string `json:"cancel_reason,omitempty"`
}

// List maneja GET /subscriptions. Soporta tres modos según los parámetros de
// query (contrato OTT, Req. 13):
//   - Detalle: `subscription_id` presente → se reenvía a OTT para obtener una
//     suscripción concreta.
//   - Filtro: `customer_id` o `plan_id` presentes → lista filtrada paginada.
//   - Lista: sin identificadores → lista paginada completa.
//
// En todos los casos valida la paginación (page > 0, records_per_page ≤ 100) y
// el ordenamiento (campos permitidos del recurso) con la validación espejo del
// BFF (Req. 6.3, 6.4 aplicadas a subscriptions vía 13.4) y propaga la respuesta
// normalizada. Un 404 de listado se normaliza como "sin resultados" (Req. 13.5)
// por el normalizador compartido.
func (h *SubscriptionsHandler) List(c *gin.Context) {
	page, err := subscriptionsParseOptionalInt(c.Query("page"))
	if err != nil {
		writeError(c, http.StatusBadRequest, "E400", "page debe ser un número entero")
		return
	}
	recordsPerPage, err := subscriptionsParseOptionalInt(c.Query("records_per_page"))
	if err != nil {
		writeError(c, http.StatusBadRequest, "E400", "records_per_page debe ser un número entero")
		return
	}

	pagination, err := validation.NormalizePagination(page, recordsPerPage)
	if err != nil {
		writeError(c, http.StatusBadRequest, "E400", err.Error())
		return
	}

	ordering, err := validation.NormalizeOrdering(
		validation.ResourceSubscriptions,
		c.Query("order_by"),
		c.Query("order"),
	)
	if err != nil {
		writeError(c, http.StatusBadRequest, "E400", err.Error())
		return
	}

	// Detalle por subscription_id: se resuelve en vivo contra OTT (objeto
	// puntual), sin pasar por el caché de listado.
	if subscriptionID := strings.TrimSpace(c.Query("subscription_id")); subscriptionID != "" {
		q := url.Values{}
		q.Set("page", strconv.Itoa(pagination.Page))
		q.Set("records_per_page", strconv.Itoa(pagination.RecordsPerPage))
		q.Set("order_by", ordering.OrderBy)
		q.Set("order", ordering.Order)
		q.Set("subscription_id", subscriptionID)
		h.proxy(c, http.MethodGet, "/subscriptions?"+q.Encode(), nil)
		return
	}

	// Listado (con o sin filtros customer_id/plan_id): servir desde el caché.
	country := h.effectiveCountry(c)
	records, err := h.cache.Get(c.Request.Context(), ottcache.ResourceSubscriptions, country, h.loadAll)
	if err != nil {
		writeError(c, http.StatusBadGateway, "E502", "No se pudo obtener el listado de suscripciones desde OTT")
		return
	}

	if customerID := strings.TrimSpace(c.Query("customer_id")); customerID != "" {
		records = ottcache.FilterByField(records, "customer_id", customerID)
	}
	if planID := strings.TrimSpace(c.Query("plan_id")); planID != "" {
		records = ottcache.FilterByField(records, "plan_id", planID)
	}

	writeCachedPage(c, records, pagination, ordering)
}

// loadAll obtiene el conjunto completo de subscriptions de un país recorriendo
// todas las páginas de OTT. Es el Loader del caché.
func (h *SubscriptionsHandler) loadAll(ctx context.Context, country string) ([]ottcache.Record, error) {
	return ottcache.LoadAllPages(ctx, func(ctx context.Context, page, perPage int) (ott.NormalizedResponse, error) {
		q := url.Values{}
		q.Set("page", strconv.Itoa(page))
		q.Set("records_per_page", strconv.Itoa(perPage))
		q.Set("order_by", "created_at")
		q.Set("order", "asc")

		headers := map[string]string{}
		resp, err := h.ott.Do(ctx, country, http.MethodGet, "/subscriptions?"+q.Encode(), nil, headers)
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

// Refresh maneja POST /subscriptions/refresh: fuerza recargar el conjunto
// completo del país efectivo. Protegido por (subscriptions, recargar).
func (h *SubscriptionsHandler) Refresh(c *gin.Context) {
	country := h.effectiveCountry(c)
	count, err := h.cache.Refresh(c.Request.Context(), ottcache.ResourceSubscriptions, country, h.loadAll)
	if err != nil {
		writeError(c, http.StatusBadGateway, "E502", "No se pudo actualizar el caché de suscripciones desde OTT")
		return
	}
	writeOK(c, http.StatusOK, gin.H{
		"resource":    "subscriptions",
		"country":     country,
		"count":       count,
		"refreshedAt": nowRFC3339(),
	}, "Caché de suscripciones actualizado")
}

// Detail maneja la obtención de una suscripción concreta por `id` tomado de la
// ruta BFF. Reenvía a OTT `GET /subscriptions?subscription_id=<id>` (el contrato
// OTT recibe el identificador por query/body, no por URL). Valida que el
// identificador sea un UUID antes de llamar a OTT (Req. 13, Property 12).
func (h *SubscriptionsHandler) Detail(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		id = strings.TrimSpace(c.Query("subscription_id"))
	}
	if id == "" {
		writeError(c, http.StatusBadRequest, "E400", "subscription_id es requerido")
		return
	}
	if !validation.IsUUID(id) {
		writeError(c, http.StatusBadRequest, "E400", validation.ErrInvalidUUID.Error())
		return
	}

	q := url.Values{}
	q.Set("subscription_id", id)
	h.proxy(c, http.MethodGet, "/subscriptions?"+q.Encode(), nil)
}

// Create maneja POST /subscriptions. Decodifica y valida el cuerpo con
// ValidateSubscriptionCreate (UUID de plan_id/customer_id/payment_method_token,
// payment_method requerido, amount/start_date opcionales; Req. 14.2-14.7,
// Property 12). Ante un fallo de validación responde 400 con writeError; si es
// válido, reenvía el cuerpo original a `POST /subscriptions` de OTT.
func (h *SubscriptionsHandler) Create(c *gin.Context) {
	raw, ok := subscriptionsReadBody(c)
	if !ok {
		return
	}

	var req subscriptionCreateRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		writeError(c, http.StatusBadRequest, "E400", "Solicitud inválida")
		return
	}

	in := validation.SubscriptionInput{
		PlanID:             req.PlanID,
		CustomerID:         req.CustomerID,
		PaymentMethod:      req.PaymentMethod,
		PaymentMethodToken: req.PaymentMethodToken,
		Amount:             req.Amount,
		StartDate:          req.StartDate,
	}
	if err := validation.ValidateSubscriptionCreate(in); err != nil {
		writeError(c, http.StatusBadRequest, "E400", err.Error())
		return
	}

	h.proxyInvalidating(c, http.MethodPost, "/subscriptions", raw, true)
}

// Change maneja PUT /subscriptions/change. Decodifica y valida el cuerpo con
// ValidateSubscriptionChange (todos los campos opcionales; UUID/formato cuando
// están presentes; Req. 15.2, 15.3). Ante un fallo responde 400.
//
// Contrato OTT (verificado contra QA): el endpoint de cambio espera el
// `subscription_id` en la QUERY STRING, no en el cuerpo. Si llega un
// `subscription_id` en el body (como lo envía el front), se mueve a la query y
// se reenvía el resto de campos en el cuerpo (sin `subscription_id`). Enviar el
// `subscription_id` en el body hace que el gateway del OTT rechace la petición
// con `S400JSON` ("The JSON format is invalid").
func (h *SubscriptionsHandler) Change(c *gin.Context) {
	raw, ok := subscriptionsReadBody(c)
	if !ok {
		return
	}

	var req subscriptionChangeRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		writeError(c, http.StatusBadRequest, "E400", "Solicitud inválida")
		return
	}

	in := validation.SubscriptionChangeInput{
		PlanID:             req.PlanID,
		CustomerID:         req.CustomerID,
		PaymentMethodToken: req.PaymentMethodToken,
		Amount:             req.Amount,
		StartDate:          req.StartDate,
	}
	if err := validation.ValidateSubscriptionChange(in); err != nil {
		writeError(c, http.StatusBadRequest, "E400", err.Error())
		return
	}

	// `subscription_id` viaja por query (contrato OTT). Se toma del body o, si
	// no viene ahí, del query param que el front podría enviar.
	subscriptionID := ""
	if req.SubscriptionID != nil {
		subscriptionID = strings.TrimSpace(*req.SubscriptionID)
	}
	if subscriptionID == "" {
		subscriptionID = strings.TrimSpace(c.Query("subscription_id"))
	}

	// Reconstruir el cuerpo SIN `subscription_id` para no enviarlo en el body
	// (el gateway OTT lo rechaza). Se preservan los demás campos presentes.
	bodyReq := req
	bodyReq.SubscriptionID = nil
	outBody, err := json.Marshal(bodyReq)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "E500", "No se pudo construir la solicitud hacia OTT")
		return
	}

	path := "/subscriptions/change"
	if subscriptionID != "" {
		q := url.Values{}
		q.Set("subscription_id", subscriptionID)
		path += "?" + q.Encode()
	}

	h.proxyInvalidating(c, http.MethodPut, path, outBody, true)
}

// Cancel maneja POST /subscriptions/cancel. Decodifica y valida el cuerpo con
// ValidateSubscriptionCancel (customer_id UUID requerido, cancel_reason opcional
// máx 255; Req. 16.2, 16.3). Ante un fallo responde 400; si es válido, reenvía
// el cuerpo original a `POST /subscriptions/cancel` de OTT.
func (h *SubscriptionsHandler) Cancel(c *gin.Context) {
	raw, ok := subscriptionsReadBody(c)
	if !ok {
		return
	}

	var req subscriptionCancelRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		writeError(c, http.StatusBadRequest, "E400", "Solicitud inválida")
		return
	}

	in := validation.SubscriptionCancelInput{
		CustomerID:   req.CustomerID,
		CancelReason: req.CancelReason,
	}
	if err := validation.ValidateSubscriptionCancel(in); err != nil {
		writeError(c, http.StatusBadRequest, "E400", err.Error())
		return
	}

	h.proxyInvalidating(c, http.MethodPost, "/subscriptions/cancel", raw, true)
}

// proxy ejecuta la llamada al backend OTT para el país activo del BFF y escribe
// la respuesta normalizada al cliente. Centraliza el patrón común de los
// métodos del handler: delega en el OTTClient (token/cache/reautenticación),
// lee el cuerpo de la respuesta y lo normaliza con ott.Normalize para exponer el
// envoltorio uniforme del BFF (Req. 20.3, 20.4). Un fallo de red/sesión con OTT
// se traduce a un 502 normalizado.
func (h *SubscriptionsHandler) proxy(c *gin.Context, method, path string, body []byte) {
	h.proxyInvalidating(c, method, path, body, false)
}

// proxyInvalidating es como proxy pero, si invalidate es true y la respuesta OTT
// es exitosa, invalida el caché de subscriptions del país efectivo (Req. 5.1,
// 5.4).
func (h *SubscriptionsHandler) proxyInvalidating(c *gin.Context, method, path string, body []byte, invalidate bool) {
	country := h.effectiveCountry(c)
	headers := map[string]string{}
	if len(body) > 0 {
		headers["Content-Type"] = "application/json"
	}

	resp, err := h.ott.Do(c.Request.Context(), country, method, path, body, headers)
	if err != nil {
		writeError(c, http.StatusBadGateway, "E502", "No se pudo contactar con el servicio OTT")
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
		h.cache.Invalidate(ottcache.ResourceSubscriptions, country)
	}

	c.JSON(subscriptionsStatusForNormalized(resp.StatusCode, normalized), normalized)
}

// subscriptionsStatusForNormalized determina el código HTTP con que el BFF
// responde al front a partir del código OTT y del resultado normalizado. Un 404
// de listado se expone como 200 ("sin resultados", Req. 13.5); el resto preserva
// el código OTT.
func subscriptionsStatusForNormalized(ottStatus int, normalized ott.NormalizedResponse) int {
	if ottStatus == http.StatusNotFound && normalized.OK {
		return http.StatusOK
	}
	return ottStatus
}

// subscriptionsReadBody lee el cuerpo crudo de la petición. Devuelve
// (nil, false) y escribe un 400 normalizado si el cuerpo no se puede leer; en
// ese caso el llamador debe abortar. Un cuerpo vacío es válido (devuelve
// ([]byte vacío, true)) para endpoints cuyos campos son todos opcionales
// (p. ej. change).
func subscriptionsReadBody(c *gin.Context) ([]byte, bool) {
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		writeError(c, http.StatusBadRequest, "E400", "No se pudo leer la solicitud")
		return nil, false
	}
	return raw, true
}

// subscriptionsParseOptionalInt convierte un parámetro de query opcional a
// entero. Una cadena vacía se interpreta como "no especificado" y devuelve 0
// (para que la normalización de paginación aplique los valores por defecto). Un
// valor no numérico produce un error.
func subscriptionsParseOptionalInt(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	return strconv.Atoi(s)
}
