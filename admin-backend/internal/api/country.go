package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// CountryHeader es la cabecera HTTP con la que el admin-frontend indica, por
// petición, el país OTT sobre el que quiere operar (selector de país). Si la
// cabecera no viene, el BFF usa el país activo por defecto (OTT_ACTIVE_COUNTRY).
const CountryHeader = "X-Country"

// contextKeyCountry es la clave bajo la cual el middleware de país deja el país
// efectivo (ya validado y normalizado) para que los handlers de recurso lo
// consuman mediante CountryFromContext.
const contextKeyCountry = "effectiveCountry"

// CountryResolver construye el middleware que resuelve el país efectivo de cada
// petición y lo deja en el gin.Context.
//
// Reglas:
//   - Lee la cabecera X-Country (en minúsculas y sin espacios).
//   - Si viene vacía, usa defaultCountry (OTT_ACTIVE_COUNTRY).
//   - Si viene con un valor que NO pertenece al conjunto de países configurados
//     (con credenciales OTT), responde 400 con el envoltorio normalizado y
//     aborta: el front no debe poder pedir un país sin credenciales.
//
// configured es el conjunto de países que tienen credenciales OTT (ver
// config.ConfiguredCountries); se inyecta para no depender de variables globales.
func CountryResolver(defaultCountry string, configured []string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(configured))
	for _, cc := range configured {
		allowed[strings.ToLower(strings.TrimSpace(cc))] = struct{}{}
	}
	def := strings.ToLower(strings.TrimSpace(defaultCountry))

	return func(c *gin.Context) {
		requested := strings.ToLower(strings.TrimSpace(c.GetHeader(CountryHeader)))

		country := def
		if requested != "" {
			if _, ok := allowed[requested]; !ok {
				writeError(
					c,
					http.StatusBadRequest,
					"E400_COUNTRY",
					"El país solicitado no está disponible o no tiene credenciales configuradas",
				)
				c.Abort()
				return
			}
			country = requested
		}

		SetCountry(c, country)
		c.Next()
	}
}

// SetCountry guarda el país efectivo en el contexto Gin.
func SetCountry(c *gin.Context, country string) {
	c.Set(contextKeyCountry, country)
}

// CountryFromContext recupera el país efectivo depositado por CountryResolver.
// Devuelve (país, true) si está presente; (vacío, false) en caso contrario
// (p. ej. una ruta que no pasó por el middleware), lo que permite a los
// handlers caer a su país por defecto sin romperse.
func CountryFromContext(c *gin.Context) (string, bool) {
	v, exists := c.Get(contextKeyCountry)
	if !exists {
		return "", false
	}
	country, ok := v.(string)
	if !ok || country == "" {
		return "", false
	}
	return country, true
}
