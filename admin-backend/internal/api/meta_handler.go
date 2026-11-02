package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// MetaHandler expone metadatos de configuración que el admin-frontend necesita
// para pintar su UI, en particular la lista de países disponibles (con
// credenciales OTT) y el país activo por defecto, para el selector de país.
type MetaHandler struct {
	// configuredCountries es el conjunto de países con credenciales OTT, en
	// orden determinista (config.ConfiguredCountries()).
	configuredCountries []string
	// activeCountry es el país por defecto del BFF (OTT_ACTIVE_COUNTRY).
	activeCountry string
}

// NewMetaHandler construye el MetaHandler con la lista de países configurados y
// el país activo por defecto.
func NewMetaHandler(configuredCountries []string, activeCountry string) *MetaHandler {
	// Copia defensiva para no exponer el slice interno de la configuración.
	cc := make([]string, len(configuredCountries))
	copy(cc, configuredCountries)
	return &MetaHandler{
		configuredCountries: cc,
		activeCountry:       activeCountry,
	}
}

// countriesResponse es la carga útil de GET /meta/countries.
type countriesResponse struct {
	// Countries son los países que el BFF puede consultar (tienen credenciales).
	Countries []string `json:"countries"`
	// Active es el país por defecto cuando la petición no envía X-Country.
	Active string `json:"active"`
}

// Countries maneja GET /meta/countries. Devuelve, en el envoltorio normalizado,
// los países configurados y el país activo por defecto. No requiere rol admin:
// cualquier sesión válida (incluido viewer) puede leerlo para poblar el selector.
func (h *MetaHandler) Countries(c *gin.Context) {
	// Nunca devolver null en el JSON: si no hay países configurados, exponer una
	// lista vacía para que el front la maneje con naturalidad.
	countries := h.configuredCountries
	if countries == nil {
		countries = []string{}
	}
	writeOK(c, http.StatusOK, countriesResponse{
		Countries: countries,
		Active:    h.activeCountry,
	}, "")
}
