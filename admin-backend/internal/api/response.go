package api

import (
	"github.com/gin-gonic/gin"

	"admin-backend/internal/ott"
)

// writeOK escribe una respuesta exitosa usando el envoltorio uniforme del BFF
// (ott.NormalizedResponse): { ok: true, data, message }. Mantiene un único
// formato de respuesta tanto para los recursos proxyados a OTT como para los
// endpoints propios del BFF (login/me), de modo que el front consuma siempre la
// misma forma (Req. 20.3).
func writeOK(c *gin.Context, statusCode int, data interface{}, message string) {
	c.JSON(statusCode, ott.NormalizedResponse{
		OK:      true,
		Data:    data,
		Message: message,
	})
}

// writeError escribe una respuesta de error con la forma normalizada del BFF
// ({ ok: false, errorCode, message }), de modo que el front maneje los errores
// del BFF con el mismo contrato que los errores normalizados de OTT.
func writeError(c *gin.Context, statusCode int, errorCode, message string) {
	// Deja el errorCode y el mensaje disponibles para el middleware de auditoría
	// (si la ruta lo tiene montado); es inofensivo en rutas sin auditoría.
	setAuditError(c, errorCode)
	setAuditMessage(c, message)
	c.JSON(statusCode, ott.NormalizedResponse{
		OK:        false,
		ErrorCode: errorCode,
		Message:   message,
	})
}
