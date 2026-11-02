package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"admin-backend/internal/ott"
	"admin-backend/internal/validation"
)

// CardsHandler agrupa los handlers HTTP del recurso Cards del admin-backend
// (BFF). Mapea las operaciones del panel a los endpoints OTT `GET/POST/DELETE
// /cards` (sin edición), aplicando validación server-side espejo antes de
// proxyar la llamada y normalizando la respuesta OTT hacia el envoltorio
// uniforme del BFF (Req. 17, 18, 19, 20.2, 20.3).
//
// Según el contrato OTT (hub-services-ott), el recurso Cards no usa parámetros
// en la URL: `customer_id`/`email` y `card_id` viajan por query o cuerpo.
type CardsHandler struct {
	ott           *ott.Client
	activeCountry string
}

// NewCardsHandler construye un CardsHandler con el cliente OTT (que gestiona el
// JWT OTT por país y la reautenticación) y el país activo del BFF
// (config.OTTActiveCountry), usado como país de las llamadas OTT.
func NewCardsHandler(client *ott.Client, activeCountry string) *CardsHandler {
	return &CardsHandler{
		ott:           client,
		activeCountry: strings.ToLower(strings.TrimSpace(activeCountry)),
	}
}

// effectiveCountry devuelve el país resuelto por el middleware CountryResolver
// para esta petición, o el país por defecto del handler si no está presente.
func (h *CardsHandler) effectiveCountry(c *gin.Context) string {
	if country, ok := CountryFromContext(c); ok {
		return country
	}
	return h.activeCountry
}

// cardCreateRequest es el cuerpo esperado por POST /cards. Replica los campos
// del contrato OTT de creación de tarjeta (Req. 18.1) para validarlos de forma
// espejo antes de proxyar la llamada.
type cardCreateRequest struct {
	CustomerID        string `json:"customer_id"`
	CardNumber        string `json:"card_number"`
	ExpirationDate    string `json:"expiration_date"`
	CardToken         string `json:"card_token"`
	PaymentMethodName string `json:"payment_method_name"`
	CardFranchise     string `json:"card_franchise"`
}

// ListByCustomer maneja GET /cards. Lista las tarjetas de un cliente localizado
// por `customer_id` o `email` en la query (Req. 17.1). Exige al menos uno de los
// dos parámetros (Req. 17.1, 17.2); si se envía `customer_id`, se valida que sea
// un UUID antes de llamar a OTT (Property 12, Req. 18.2 aplicado al filtro).
//
// Proxya a `GET /cards` reenviando los parámetros de filtro y normaliza la
// respuesta. Un 404 de listado se normaliza como "sin resultados" por el
// normalizador (Req. 17.3).
func (h *CardsHandler) ListByCustomer(c *gin.Context) {
	customerID := strings.TrimSpace(c.Query("customer_id"))
	email := strings.TrimSpace(c.Query("email"))

	if customerID == "" && email == "" {
		writeError(c, http.StatusBadRequest, "E400", "Debe indicar customer_id o email para consultar las tarjetas")
		return
	}

	if customerID != "" && !validation.IsUUID(customerID) {
		writeError(c, http.StatusBadRequest, "E400", "customer_id no es un UUID válido")
		return
	}

	query := url.Values{}
	if customerID != "" {
		query.Set("customer_id", customerID)
	}
	if email != "" {
		query.Set("email", email)
	}

	path := "/cards"
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}

	h.proxy(c, http.MethodGet, path, nil)
}

// Create maneja POST /cards. Valida el cuerpo con la validación espejo de Card
// (ValidateCardCreate: customer_id/card_token UUID, expiration_date MM/YY futura
// y límites de longitud; Req. 18.2-18.7) y, si es válido, proxya la creación a
// `POST /cards` reenviando el cuerpo original (Req. 18.8). La respuesta OTT se
// normaliza (Req. 20.3).
func (h *CardsHandler) Create(c *gin.Context) {
	var req cardCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "E400", "Solicitud inválida")
		return
	}

	if err := validation.ValidateCardCreate(validation.CardInput{
		CustomerID:        req.CustomerID,
		CardNumber:        req.CardNumber,
		ExpirationDate:    req.ExpirationDate,
		CardToken:         req.CardToken,
		PaymentMethodName: req.PaymentMethodName,
		CardFranchise:     req.CardFranchise,
	}); err != nil {
		writeError(c, http.StatusBadRequest, "E400", err.Error())
		return
	}

	body, err := json.Marshal(req)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "E500", "No se pudo construir la petición")
		return
	}

	h.proxy(c, http.MethodPost, "/cards", body)
}

// cardDeleteRequest es el cuerpo aceptado por DELETE /cards cuando el `card_id`
// se envía en el cuerpo en lugar de la query (Req. 19.2).
type cardDeleteRequest struct {
	CardID string `json:"card_id"`
}

// Delete maneja DELETE /cards. Identifica la tarjeta por `card_id` tomado de la
// query o del cuerpo (Req. 19.2). Exige el identificador y valida que sea un
// UUID antes de proxyar; en caso contrario responde 400. Proxya a
// `DELETE /cards?card_id=<id>` y normaliza la respuesta (Req. 20.3).
func (h *CardsHandler) Delete(c *gin.Context) {
	cardID := strings.TrimSpace(c.Query("card_id"))
	if cardID == "" {
		var req cardDeleteRequest
		// El cuerpo es opcional: si no se puede decodificar, se mantiene vacío
		// y se trata como identificador ausente.
		if err := c.ShouldBindJSON(&req); err == nil {
			cardID = strings.TrimSpace(req.CardID)
		}
	}

	if cardID == "" {
		writeError(c, http.StatusBadRequest, "E400", "card_id es requerido")
		return
	}
	if !validation.IsUUID(cardID) {
		writeError(c, http.StatusBadRequest, "E400", "card_id no es un UUID válido")
		return
	}

	query := url.Values{}
	query.Set("card_id", cardID)

	h.proxy(c, http.MethodDelete, "/cards?"+query.Encode(), nil)
}

// proxy ejecuta la llamada OTT para el país activo del BFF, lee el cuerpo de la
// respuesta, lo normaliza al envoltorio uniforme y lo escribe al cliente
// conservando el código HTTP de OTT. Centraliza el manejo de errores de
// transporte y el cierre del cuerpo para los tres handlers de Cards.
func (h *CardsHandler) proxy(c *gin.Context, method, path string, body []byte) {
	resp, err := h.ott.Do(c.Request.Context(), h.effectiveCountry(c), method, path, body, nil)
	if err != nil {
		writeError(c, http.StatusBadGateway, "E502", "No se pudo comunicar con el servicio OTT")
		return
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		writeError(c, http.StatusBadGateway, "E502", "No se pudo leer la respuesta del servicio OTT")
		return
	}

	normalized := ott.Normalize(resp.StatusCode, raw)

	// El OTT devuelve las tarjetas de un cliente sin metadatos de paginación y
	// con la colección anidada (p. ej. `data: { customer_id, data: [ ... ] }`).
	// Para que el front cuente registros/páginas correctamente, derivamos la
	// paginación del tamaño de la colección cuando el normalizador no la trae:
	// una sola página con todas las tarjetas del cliente.
	if normalized.OK && normalized.Pagination == nil {
		if n, ok := cardsCollectionLen(normalized.Data); ok {
			normalized.Pagination = &ott.Pagination{
				TotalRecords:   int64(n),
				Page:           1,
				RecordsPerPage: n,
				TotalPages:     1,
			}
		}
	}

	c.JSON(resp.StatusCode, normalized)
}

// cardsCollectionLen busca la colección de tarjetas dentro del Data normalizado
// y devuelve su longitud. Soporta tanto un arreglo directo como un objeto que
// contiene el arreglo (incluido el anidamiento de Cards `{ customer_id, data:
// [ ... ] }`). Devuelve (0, false) si no encuentra ninguna colección.
func cardsCollectionLen(data interface{}) (int, bool) {
	switch t := data.(type) {
	case []interface{}:
		return len(t), true
	case map[string]interface{}:
		// Preferir una clave `data` si contiene el arreglo (forma de Cards).
		if arr, ok := t["data"].([]interface{}); ok {
			return len(arr), true
		}
		for _, v := range t {
			if arr, ok := v.([]interface{}); ok {
				return len(arr), true
			}
		}
	}
	return 0, false
}
