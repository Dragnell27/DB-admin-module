// Package ott contiene la integración del admin-backend (BFF) con las APIs OTT
// de hub-services-ott: el cliente HTTP, el cache de token por país y el
// normalizador de respuestas. Este archivo implementa únicamente el
// normalizador de respuesta estándar OTT (lógica pura del BFF).
package ott

import (
	"encoding/json"
	"net/http"
)

// NormalizedResponse es el envoltorio uniforme que el BFF expone al
// admin-frontend. Independiza al front de las dos variantes de la respuesta
// estándar OTT (`resultCode/resultMessage/errorCode/result` y
// `status/http_code/message/data`).
//
// Requisito 20.3: el BFF interpreta ambas variantes y expone un resultado
// normalizado. Requisito 20.4: expone los metadatos de paginación cuando
// existen.
type NormalizedResponse struct {
	// OK indica si la respuesta representa un resultado exitoso. Un 404 de
	// listado se normaliza como OK=true con una colección vacía (Req. 6.5,
	// 9.5, 13.5, 17.3).
	OK bool `json:"ok"`
	// Data es la carga útil normalizada. Para listados contiene la colección
	// (posiblemente vacía); para detalles/operaciones, el objeto devuelto por
	// OTT. Es nil cuando no hay datos.
	Data interface{} `json:"data"`
	// Pagination contiene los metadatos de paginación cuando la respuesta OTT
	// los incluye (o metadatos en cero para un 404 de listado). Es nil cuando
	// la respuesta no corresponde a un listado.
	Pagination *Pagination `json:"pagination,omitempty"`
	// ErrorCode es el código de error OTT (p. ej. `S401SEC`, `E500`) cuando la
	// respuesta representa un error. Vacío en respuestas exitosas.
	ErrorCode string `json:"errorCode,omitempty"`
	// Message es el mensaje legible devuelto por OTT (`resultMessage` o
	// `message`). Se preserva tanto en éxito como en error para que el front
	// pueda mostrarlo (Req. 7.12, 10.15, etc.).
	Message string `json:"message"`
}

// Pagination expone los metadatos de paginación de un listado OTT (Req. 20.4).
type Pagination struct {
	TotalRecords   int64 `json:"totalRecords"`
	Page           int   `json:"page"`
	RecordsPerPage int   `json:"recordsPerPage"`
	TotalPages     int   `json:"totalPages"`
}

// ottEnvelope modela las dos variantes de la respuesta estándar OTT en una
// sola estructura. Un cuerpo OTT bien formado llena los campos de una de las
// dos variantes; la decodificación JSON es tolerante y deja en cero los campos
// ausentes.
type ottEnvelope struct {
	// Variante A: resultCode/resultMessage/errorCode/result.
	ResultCode    string          `json:"resultCode"`
	ResultMessage string          `json:"resultMessage"`
	ErrorCode     string          `json:"errorCode"`
	Result        json.RawMessage `json:"result"`

	// Variante B: status/http_code/message/data.
	Status   string          `json:"status"`
	HTTPCode int             `json:"http_code"`
	Message  string          `json:"message"`
	Data     json.RawMessage `json:"data"`
}

// paginationFields captura los metadatos de paginación que OTT embebe dentro de
// `result`/`data` para los listados. Solo se consideran presentes si al menos
// uno de los campos aparece en el cuerpo (ver hasPagination).
type paginationFields struct {
	// Variante en minúscula (p. ej. Plans/Customers): totalRecords, page,
	// recordsPerPage, totalPages.
	TotalRecords   *int64 `json:"totalRecords"`
	Page           *int   `json:"page"`
	RecordsPerPage *int   `json:"recordsPerPage"`
	TotalPages     *int   `json:"totalPages"`

	// Variante en PascalCase/abreviada (p. ej. Subscriptions): Total, Page,
	// PerPage, TotalPages. El OTT no es consistente entre recursos, así que se
	// aceptan ambas y se prefiere la que venga informada.
	TotalAlt      *int64 `json:"Total"`
	PageAlt       *int   `json:"Page"`
	PerPageAlt    *int   `json:"PerPage"`
	TotalPagesAlt *int   `json:"TotalPages"`
}

func (p paginationFields) hasAny() bool {
	return p.TotalRecords != nil || p.Page != nil || p.RecordsPerPage != nil || p.TotalPages != nil ||
		p.TotalAlt != nil || p.PageAlt != nil || p.PerPageAlt != nil || p.TotalPagesAlt != nil
}

// pickInt64 devuelve el primer puntero no nil (preferencia por el primero).
func pickInt64(vals ...*int64) int64 {
	for _, v := range vals {
		if v != nil {
			return *v
		}
	}
	return 0
}

// pickInt devuelve el primer puntero no nil (preferencia por el primero).
func pickInt(vals ...*int) int {
	for _, v := range vals {
		if v != nil {
			return *v
		}
	}
	return 0
}

func (p paginationFields) toPagination() *Pagination {
	return &Pagination{
		TotalRecords:   pickInt64(p.TotalRecords, p.TotalAlt),
		Page:           pickInt(p.Page, p.PageAlt),
		RecordsPerPage: pickInt(p.RecordsPerPage, p.PerPageAlt),
		TotalPages:     pickInt(p.TotalPages, p.TotalPagesAlt),
	}
}

// Normalize convierte una respuesta OTT (código HTTP + cuerpo crudo) en el
// envoltorio uniforme NormalizedResponse.
//
// Reglas (Req. 20.3, 20.4, 6.5, 9.5, 13.5, 17.3):
//   - Un 404 se trata como "sin resultados": OK=true, Data=colección vacía y
//     metadatos de paginación en cero, sin error (no es un fallo).
//   - Para cualquier otro código, interpreta ambas variantes de la respuesta
//     estándar, preserva los datos y extrae los metadatos de paginación
//     presentes dentro de `result`/`data`.
//   - OK se determina por el código HTTP 2xx (y 404, tratado como vacío).
//
// `body` puede ser nil o vacío; en ese caso se produce un envoltorio con Data
// nil y OK derivado del código HTTP.
func Normalize(statusCode int, body []byte) NormalizedResponse {
	// 404 de listado: sin resultados, no es un error (Req. 6.5, 9.5, 13.5, 17.3).
	if statusCode == http.StatusNotFound {
		return NormalizedResponse{
			OK:         true,
			Data:       []interface{}{},
			Pagination: &Pagination{},
			Message:    messageFromBody(body),
		}
	}

	var env ottEnvelope
	if len(body) > 0 {
		// Un cuerpo malformado no debe hacer fallar la normalización: se trata
		// como ausencia de campos y OK se decide por el código HTTP.
		_ = json.Unmarshal(body, &env)
	}

	ok := statusCode >= 200 && statusCode < 300

	// Seleccionar la carga útil y los metadatos según la variante presente.
	payload := env.Result
	if len(payload) == 0 {
		payload = env.Data
	}

	message := env.ResultMessage
	if message == "" {
		message = env.Message
	}

	errorCode := env.ErrorCode
	if ok {
		// En respuestas exitosas no se propaga un código de error al front.
		errorCode = ""
	}

	norm := NormalizedResponse{
		OK:        ok,
		ErrorCode: errorCode,
		Message:   message,
	}

	norm.Data, norm.Pagination = extractDataAndPagination(payload)
	return norm
}

// extractDataAndPagination decodifica la carga útil de `result`/`data`.
// Si el cuerpo es un objeto que contiene metadatos de paginación, los extrae y
// devuelve el objeto completo como Data (preservando también la colección y
// cualquier otro campo). Si no hay metadatos de paginación, devuelve el valor
// tal cual (objeto, arreglo o escalar) sin paginación.
func extractDataAndPagination(payload json.RawMessage) (interface{}, *Pagination) {
	if len(payload) == 0 {
		return nil, nil
	}

	// Intentar leer metadatos de paginación (solo aplica si es un objeto JSON).
	var pf paginationFields
	if err := json.Unmarshal(payload, &pf); err == nil && pf.hasAny() {
		data := decodeGeneric(payload)
		return data, pf.toPagination()
	}

	return decodeGeneric(payload), nil
}

// decodeGeneric decodifica un json.RawMessage a un valor genérico de Go
// (map[string]interface{}, []interface{}, string, float64, bool o nil),
// preservando los datos para que el round-trip no pierda información.
func decodeGeneric(raw json.RawMessage) interface{} {
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	return v
}

// messageFromBody intenta extraer un mensaje legible del cuerpo (de cualquiera
// de las dos variantes) para adjuntarlo al resultado "sin resultados" de un 404.
// Devuelve cadena vacía si el cuerpo no es interpretable.
func messageFromBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var env ottEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return ""
	}
	if env.ResultMessage != "" {
		return env.ResultMessage
	}
	return env.Message
}
