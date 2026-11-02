package api

import (
	"context"
	"encoding/json"
	"errors"
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

// customersOTTPath es la ruta (relativa a la URL base del OTT) del recurso
// Customers. Según el contrato OTT los identificadores viajan por query/body,
// nunca en la URL: GET/POST/PUT /customers (sin DELETE).
const customersOTTPath = "/customers"

// customerCountryHeader es la cabecera con la que el BFF indica al backend OTT
// el país activo. El OTT la lee (X-Country) para delimitar los datos por país;
// el país no lo elige el front, se deriva de la configuración del BFF
// (OTT_ACTIVE_COUNTRY) — design.md "Manejo del país (country)".
const customerCountryHeader = "X-Country"

// CustomersHandler expone las operaciones de Customers del BFF (listar/detalle/
// crear/editar) traduciéndolas a las llamadas del backend OTT a través del
// OTTClient. Normaliza las respuestas OTT con ott.Normalize y aplica la
// validación server-side espejo (internal/validation) antes de proxyar las
// escrituras (Req. 6, 7, 8, 20.2, 20.3, 20.4).
type CustomersHandler struct {
	ott           *ott.Client
	activeCountry string
	// cache guarda el conjunto completo de customers por país. La lista pura y
	// el filtro por customer_id se resuelven sobre este conjunto en memoria,
	// reduciendo las llamadas a OTT (reemplaza el antiguo índice ad-hoc).
	cache *ottcache.Cache
}

// NewCustomersHandler construye un CustomersHandler con el OTTClient y el país
// activo del BFF (config.OTTActiveCountry). El país activo es el DEFAULT: cada
// petición puede elegir otro país vía la cabecera X-Country (resuelta por el
// middleware CountryResolver) y, de no hacerlo, se usa este default. El país
// efectivo determina las credenciales/JWT OTT usados y se propaga como X-Country.
func NewCustomersHandler(ottClient *ott.Client, activeCountry string, cache *ottcache.Cache) *CustomersHandler {
	return &CustomersHandler{
		ott:           ottClient,
		activeCountry: strings.ToLower(strings.TrimSpace(activeCountry)),
		cache:         cache,
	}
}

// effectiveCountry devuelve el país resuelto por el middleware CountryResolver
// para esta petición, o el país por defecto del handler si no está presente.
func (h *CustomersHandler) effectiveCountry(c *gin.Context) string {
	if country, ok := CountryFromContext(c); ok {
		return country
	}
	return h.activeCountry
}

// customerCreateBody es el cuerpo esperado por POST /customers. Replica los
// campos del contrato OTT (design.md "Customer — crear", Req. 7). Los punteros
// representan campos opcionales (ausentes cuando son nil).
type customerCreateBody struct {
	ExternalID        string  `json:"external_id"`
	FirstName         string  `json:"first_name"`
	LastName          string  `json:"last_name"`
	Email             string  `json:"email"`
	Phone             string  `json:"phone"`
	BirthDate         string  `json:"birth_date"`
	ExternalCreated   string  `json:"external_created"`
	SecondLastName    *string `json:"second_last_name,omitempty"`
	Gender            *string `json:"gender,omitempty"`
	Address           *string `json:"address,omitempty"`
	DeviceFingerprint *string `json:"device_fingerprint,omitempty"`
}

// toValidationInput proyecta el cuerpo de creación/edición hacia el tipo de
// entrada de la validación espejo (validation.CustomerInput).
func (b customerCreateBody) toValidationInput() validation.CustomerInput {
	return validation.CustomerInput{
		ExternalID:        b.ExternalID,
		FirstName:         b.FirstName,
		LastName:          b.LastName,
		Email:             b.Email,
		Phone:             b.Phone,
		BirthDate:         b.BirthDate,
		ExternalCreated:   b.ExternalCreated,
		SecondLastName:    b.SecondLastName,
		Gender:            b.Gender,
		Address:           b.Address,
		DeviceFingerprint: b.DeviceFingerprint,
	}
}

// customerEditBody es el cuerpo esperado por PUT /customers. Solo son editables
// first_name, last_name, phone, second_last_name, gender, address,
// device_fingerprint y birth_date (Req. 8.1). external_id y email NO son
// editables (Req. 8.2): se usan únicamente para localizar al cliente mediante la
// query, no en el cuerpo, y nunca se reenvían como campos a modificar.
type customerEditBody struct {
	FirstName         string  `json:"first_name"`
	LastName          string  `json:"last_name"`
	Phone             string  `json:"phone"`
	BirthDate         string  `json:"birth_date"`
	SecondLastName    *string `json:"second_last_name,omitempty"`
	Gender            *string `json:"gender,omitempty"`
	Address           *string `json:"address,omitempty"`
	DeviceFingerprint *string `json:"device_fingerprint,omitempty"`
}

// toValidationInput proyecta el cuerpo de edición hacia validation.CustomerInput.
// external_id y email se dejan vacíos a propósito: son inmutables y no se validan
// ni se reenvían como campos editables (Req. 8.2).
func (b customerEditBody) toValidationInput() validation.CustomerInput {
	return validation.CustomerInput{
		FirstName:         b.FirstName,
		LastName:          b.LastName,
		Phone:             b.Phone,
		BirthDate:         b.BirthDate,
		SecondLastName:    b.SecondLastName,
		Gender:            b.Gender,
		Address:           b.Address,
		DeviceFingerprint: b.DeviceFingerprint,
	}
}

// List maneja GET /customers. Si la query incluye `email` o `external_id`,
// delega en la consulta de detalle (Req. 6.2). En caso contrario, valida los
// parámetros de paginación y ordenamiento con la validación espejo (Req. 6.3,
// 6.4, 20.5), proxya el listado al OTT y normaliza la respuesta, tratando el 404
// como "sin resultados" (Req. 6.5, 6.6, 20.3, 20.4).
func (h *CustomersHandler) List(c *gin.Context) {
	email := strings.TrimSpace(c.Query("email"))
	externalID := strings.TrimSpace(c.Query("external_id"))
	customerID := strings.TrimSpace(c.Query("customer_id"))

	if email != "" || externalID != "" {
		h.detailByFilter(c, email, externalID)
		return
	}

	page, err := parseOptionalCustomerInt(c.Query("page"))
	if err != nil {
		writeError(c, http.StatusBadRequest, "E400", "page debe ser un entero válido")
		return
	}
	recordsPerPage, err := parseOptionalCustomerInt(c.Query("records_per_page"))
	if err != nil {
		writeError(c, http.StatusBadRequest, "E400", "records_per_page debe ser un entero válido")
		return
	}

	pagination, err := validation.NormalizePagination(page, recordsPerPage)
	if err != nil {
		writeError(c, http.StatusBadRequest, "E400", err.Error())
		return
	}

	ordering, err := validation.NormalizeOrdering(validation.ResourceCustomers, c.Query("order_by"), c.Query("order"))
	if err != nil {
		writeError(c, http.StatusBadRequest, "E400", err.Error())
		return
	}

	// Conjunto completo de customers del país, servido desde el caché (se carga
	// desde OTT recorriendo todas las páginas si no hay entrada vigente).
	country := h.effectiveCountry(c)
	records, err := h.cache.Get(c.Request.Context(), ottcache.ResourceCustomers, country, h.loadAll)
	if err != nil {
		writeError(c, http.StatusBadGateway, "E502", "No se pudo obtener el listado de clientes desde OTT")
		return
	}

	// Filtro propio por customer_id (OTT no lo soporta): se resuelve sobre el
	// conjunto cacheado. Se devuelve como LISTADO (página), coherente con un
	// filtro de tabla; el front que necesita el detalle toma el primer elemento.
	if customerID != "" {
		records = ottcache.FilterByField(records, "customer_id", customerID)
	}

	writeCachedPage(c, records, pagination, ordering)
}

// loadAll obtiene el conjunto completo de customers de un país recorriendo todas
// las páginas de OTT. Es el Loader que alimenta el caché.
func (h *CustomersHandler) loadAll(ctx context.Context, country string) ([]ottcache.Record, error) {
	return ottcache.LoadAllPages(ctx, func(ctx context.Context, page, perPage int) (ott.NormalizedResponse, error) {
		query := url.Values{}
		query.Set("page", strconv.Itoa(page))
		query.Set("records_per_page", strconv.Itoa(perPage))
		query.Set("order_by", "created_at")
		query.Set("order", "asc")

		headers := map[string]string{customerCountryHeader: country}
		resp, err := h.ott.Do(ctx, country, http.MethodGet, h.pathWithQuery(query), nil, headers)
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

// Refresh maneja POST /customers/refresh: fuerza recargar el conjunto completo
// de customers del país efectivo desde OTT y reemplaza el caché. Protegido por
// el permiso (customers, recargar) en el router.
func (h *CustomersHandler) Refresh(c *gin.Context) {
	country := h.effectiveCountry(c)
	count, err := h.cache.Refresh(c.Request.Context(), ottcache.ResourceCustomers, country, h.loadAll)
	if err != nil {
		writeError(c, http.StatusBadGateway, "E502", "No se pudo actualizar el caché de clientes desde OTT")
		return
	}
	writeOK(c, http.StatusOK, gin.H{
		"resource":    "customers",
		"country":     country,
		"count":       count,
		"refreshedAt": nowRFC3339(),
	}, "Caché de clientes actualizado")
}

// Detail maneja la consulta de detalle de un Customer localizándolo por `email`
// o `external_id` en la query (Req. 6.2). El contrato OTT no usa un id en la URL:
// el identificador viaja por query. Si no se proporciona ningún filtro, responde
// 400.
func (h *CustomersHandler) Detail(c *gin.Context) {
	email := strings.TrimSpace(c.Query("email"))
	externalID := strings.TrimSpace(c.Query("external_id"))

	if email == "" && externalID == "" {
		writeError(c, http.StatusBadRequest, "E400", "Se requiere email o external_id para consultar el detalle del cliente")
		return
	}

	h.detailByFilter(c, email, externalID)
}

// detailByFilter proxya al OTT la consulta de detalle de un Customer por los
// filtros `email`/`external_id` y normaliza la respuesta.
func (h *CustomersHandler) detailByFilter(c *gin.Context, email, externalID string) {
	query := url.Values{}
	if email != "" {
		query.Set("email", email)
	}
	if externalID != "" {
		query.Set("external_id", externalID)
	}

	h.proxy(c, http.MethodGet, h.pathWithQuery(query), nil)
}

// Create maneja POST /customers. Valida el cuerpo con la validación espejo
// (ValidateCustomerCreate, Req. 7.2-7.9); ante un fallo responde 400 con el
// detalle del campo. Si es válido, reenvía el cuerpo original al OTT y normaliza
// la respuesta (Req. 7.10, 7.11, 7.12, 20.2, 20.3).
func (h *CustomersHandler) Create(c *gin.Context) {
	var body customerCreateBody
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, http.StatusBadRequest, "E400", "Solicitud inválida")
		return
	}

	if err := validation.ValidateCustomerCreate(body.toValidationInput()); err != nil {
		writeCustomerValidationError(c, err)
		return
	}

	payload, err := json.Marshal(body)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "E500", "No se pudo construir la solicitud hacia OTT")
		return
	}

	h.proxyInvalidating(c, http.MethodPost, customersOTTPath, payload, true)
}

// Edit maneja PUT /customers. Localiza al cliente por `email`/`external_id` en la
// query (Req. 8.4) y valida el cuerpo editable con la validación espejo
// (ValidateCustomerEdit, Req. 8.3). external_id y email son inmutables: no se
// aceptan ni reenvían como campos del cuerpo (Req. 8.2). Si es válido, reenvía
// solo los campos editables al OTT y normaliza la respuesta.
func (h *CustomersHandler) Edit(c *gin.Context) {
	email := strings.TrimSpace(c.Query("email"))
	externalID := strings.TrimSpace(c.Query("external_id"))

	if email == "" && externalID == "" {
		writeError(c, http.StatusBadRequest, "E400", "Se requiere email o external_id para editar el cliente")
		return
	}

	var body customerEditBody
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, http.StatusBadRequest, "E400", "Solicitud inválida")
		return
	}

	if err := validation.ValidateCustomerEdit(body.toValidationInput()); err != nil {
		writeCustomerValidationError(c, err)
		return
	}

	payload, err := json.Marshal(body)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "E500", "No se pudo construir la solicitud hacia OTT")
		return
	}

	query := url.Values{}
	if email != "" {
		query.Set("email", email)
	}
	if externalID != "" {
		query.Set("external_id", externalID)
	}

	h.proxyInvalidating(c, http.MethodPut, h.pathWithQuery(query), payload, true)
}

// proxy ejecuta la llamada autenticada al OTT para el país activo del BFF,
// normaliza la respuesta y la escribe al cliente preservando el código HTTP del
// OTT (salvo el 404 de listado, que ott.Normalize convierte en un resultado
// vacío exitoso). Un fallo de red/sesión OTT se traduce en 502 (Req. 3.6, 20.3).
func (h *CustomersHandler) proxy(c *gin.Context, method, path string, body []byte) {
	h.proxyInvalidating(c, method, path, body, false)
}

// proxyInvalidating es como proxy pero, si invalidate es true y la respuesta OTT
// es exitosa (2xx), invalida el caché de customers del país efectivo para que la
// siguiente lectura refleje la escritura (Req. 5.1, 5.4).
func (h *CustomersHandler) proxyInvalidating(c *gin.Context, method, path string, body []byte, invalidate bool) {
	country := h.effectiveCountry(c)
	headers := map[string]string{customerCountryHeader: country}
	if len(body) > 0 {
		headers["Content-Type"] = "application/json"
	}

	resp, err := h.ott.Do(c.Request.Context(), country, method, path, body, headers)
	if err != nil {
		writeError(c, http.StatusBadGateway, "E502", "No se pudo establecer la sesión con el servicio OTT")
		return
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		writeError(c, http.StatusBadGateway, "E502", "No se pudo leer la respuesta del servicio OTT")
		return
	}

	normalized := ott.Normalize(resp.StatusCode, raw)

	// Invalidar solo ante escritura exitosa: así un error de OTT no borra el
	// caché vigente (Req. 5.4).
	if invalidate && resp.StatusCode >= 200 && resp.StatusCode < 300 && normalized.OK {
		h.cache.Invalidate(ottcache.ResourceCustomers, country)
	}

	c.JSON(customerResponseStatus(resp.StatusCode), normalized)
}

// pathWithQuery compone la ruta del recurso Customers con los parámetros de query
// codificados. Devuelve la ruta base cuando no hay parámetros.
func (h *CustomersHandler) pathWithQuery(query url.Values) string {
	if len(query) == 0 {
		return customersOTTPath
	}
	return customersOTTPath + "?" + query.Encode()
}

// writeCustomerValidationError traduce un error de validación espejo a una
// respuesta 400 con el detalle del campo cuando está disponible
// (validation.ValidationError). Es privado al handler de Customers para no
// colisionar con helpers análogos de otros handlers de recurso.
func writeCustomerValidationError(c *gin.Context, err error) {
	var ve *validation.ValidationError
	if errors.As(err, &ve) {
		writeError(c, http.StatusBadRequest, "E400", ve.Error())
		return
	}
	writeError(c, http.StatusBadRequest, "E400", err.Error())
}

// customerResponseStatus determina el código HTTP con el que el BFF responde al
// front a partir del código del OTT. El 404 de listado se expone como 200 porque
// ott.Normalize lo representa como "sin resultados" exitoso (Req. 6.5).
func customerResponseStatus(ottStatus int) int {
	if ottStatus == http.StatusNotFound {
		return http.StatusOK
	}
	return ottStatus
}

// parseOptionalCustomerInt interpreta un parámetro de query entero opcional. Una
// cadena vacía se trata como "no especificado" y devuelve 0 (que las funciones
// de normalización de paginación convierten en el valor por defecto). Un valor
// presente pero no numérico produce un error. Es privado al handler de Customers
// para no colisionar con helpers análogos de otros handlers de recurso.
func parseOptionalCustomerInt(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	return strconv.Atoi(raw)
}
