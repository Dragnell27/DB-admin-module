// Package api contiene la capa HTTP del admin-backend (BFF): handlers Gin,
// middlewares (autenticación de sesión y rol) y el cableado de rutas.
//
// Este archivo define el contrato compartido para transportar los claims del
// JWT de sesión del admin a través del *gin.Context*: la clave de contexto y
// los accesores. El middleware de autenticación de sesión (tarea 4.1) deposita
// los claims validados con SetSessionClaims; los handlers (p. ej. /auth/me) y
// el middleware de rol (tarea 4.2) los recuperan con GetSessionClaims. Mantener
// una única fuente de verdad evita que cada componente redefina la clave.
package api

import (
	"github.com/gin-gonic/gin"

	"admin-backend/internal/auth"
)

// ContextKeySessionClaims es la clave bajo la cual se almacenan los
// *auth.SessionClaims validados en el *gin.Context. El middleware de rol y los
// handlers deben leerla mediante GetSessionClaims en lugar de referirse a la
// cadena directamente.
//
// Reconciliación del seam de identidad (tarea 9.1): el middleware de
// autenticación de sesión (internal/api/middleware) deposita los claims bajo
// este MISMO valor literal ("sessionClaims") sin importar el paquete api —lo
// que evitaría un ciclo de importación, pues el router de api importa
// middleware—. Así, cuando SessionAuth se ejecuta en el router real, GET
// /auth/me (que usa GetSessionClaims) recupera los claims que el middleware
// dejó. Mantener ambas constantes sincronizadas es parte del contrato.
const ContextKeySessionClaims = "sessionClaims"

// SetSessionClaims guarda los claims de sesión validados en el contexto Gin.
// Lo usa el middleware de autenticación de sesión (tarea 4.1) tras verificar la
// firma y la expiración del JWT de sesión del admin.
func SetSessionClaims(c *gin.Context, claims *auth.SessionClaims) {
	c.Set(ContextKeySessionClaims, claims)
}

// GetSessionClaims recupera los claims de sesión previamente depositados por el
// middleware de autenticación. Devuelve (claims, true) si están presentes y son
// del tipo esperado; (nil, false) en caso contrario (p. ej. si la ruta no pasó
// por el middleware de sesión).
func GetSessionClaims(c *gin.Context) (*auth.SessionClaims, bool) {
	v, exists := c.Get(ContextKeySessionClaims)
	if !exists {
		return nil, false
	}
	claims, ok := v.(*auth.SessionClaims)
	if !ok || claims == nil {
		return nil, false
	}
	return claims, true
}
