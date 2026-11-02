package middleware

// Este archivo implementa el middleware de autorización por rol (tarea 4.2).
// Se aplica sobre las rutas de escritura (POST/PUT/DELETE y cancelación) que el
// router monta tras el middleware de sesión (tarea 9.1) y exige que el
// administrador autenticado tenga el rol `admin`. Un `viewer` (o cualquier rol
// distinto de `admin`) recibe 403 con el envoltorio de error normalizado y la
// cadena se aborta antes de ejecutar la operación (Req. 5.3, 5.4).
//
// Reutiliza el envoltorio errorBody y el helper de identidad CurrentRole
// definidos en auth.go (mismo paquete). No redefine errorBody.

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"admin-backend/internal/users"
)

// RequireAdmin construye el middleware de autorización por rol para rutas de
// escritura.
//
// Debe montarse después del middleware de sesión (SessionAuth), que es quien
// deja el rol del administrador en el gin.Context. Lee el rol con CurrentRole:
// si no hay rol en el contexto (ruta sin sesión aplicada) o el rol no es
// `admin`, responde 403 con el envoltorio de error normalizado y aborta la
// petición sin ejecutar el handler. El 401 por ausencia o invalidez del token
// lo gestiona el middleware de sesión; aquí la identidad ya está autenticada y
// lo que falta es permiso suficiente, de ahí el 403 (Req. 5.3, 5.4).
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, ok := CurrentRole(c)
		if !ok || role != users.RoleAdmin {
			abortForbidden(c, "AUTH_FORBIDDEN", "No tienes permisos para realizar esta operación")
			return
		}
		c.Next()
	}
}

// abortForbidden responde 403 con el envoltorio de error normalizado y aborta
// la cadena de middlewares/handlers. Complementa a abortUnauthorized (401) de
// auth.go: este se usa cuando la identidad está autenticada pero su rol no
// autoriza la operación de escritura solicitada.
func abortForbidden(c *gin.Context, errorCode, message string) {
	c.AbortWithStatusJSON(http.StatusForbidden, errorBody{
		OK:        false,
		ErrorCode: errorCode,
		Message:   message,
	})
}
