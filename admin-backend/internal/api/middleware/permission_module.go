package middleware

import (
	"github.com/gin-gonic/gin"

	"admin-backend/internal/auth"
)

// ModuleActionLoader resuelve, para un slug de módulo, las acciones de permiso
// que ese módulo declara (p. ej. "ver", "crear", "custom:activar"). Lo inyecta
// el router desde el repositorio de módulos; se expresa como función para que el
// paquete middleware NO importe internal/modules ni internal/api (lo que crearía
// ciclos de importación). Devuelve (acciones, true) si el módulo existe; (nil,
// false) si no existe o no se puede resolver.
type ModuleActionLoader func(ctx *gin.Context, slug string) ([]auth.Action, bool)

// RequireModulePermission construye el middleware de autorización para una
// acción de un módulo dinámico (module-builder). Autoriza ANTES de ejecutar la
// operación contra el origen (Req. 13.4):
//
//  1. Recupera los permisos de módulo del contexto (los deja SessionAuth al
//     decodificar `modperms`). Si no hay, 401 (ruta sin sesión).
//  2. Recupera el país efectivo (contextKeyEffectiveCountry, el mismo literal
//     que usa RequirePermission para no acoplar middleware↔api).
//  3. Carga las acciones declaradas del módulo con el loader inyectado y valida
//     que `action` pertenezca a ellas; si el módulo no existe o la acción no
//     está declarada, 403.
//  4. Evalúa auth.HasModulePermission(modperms, slug, action, país); si falla,
//     403 AUTH_FORBIDDEN (misma semántica y helper que RequirePermission).
//
// El slug se toma del parámetro de ruta `:slug`.
func RequireModulePermission(loader ModuleActionLoader, action auth.Action) gin.HandlerFunc {
	return func(c *gin.Context) {
		slug := c.Param("slug")

		modperms, ok := CurrentModulePermissions(c)
		if !ok {
			abortUnauthorized(c, "AUTH_MISSING_TOKEN", "Falta el token de sesión o el formato de la cabecera Authorization es inválido")
			return
		}

		country, ok := effectiveCountry(c)
		if !ok || country == "" {
			abortForbidden(c, "AUTH_FORBIDDEN", "No tienes permisos para realizar esta operación")
			return
		}

		// El módulo debe existir y declarar la acción solicitada.
		if loader != nil {
			declared, found := loader(c, slug)
			if !found || !containsAction(declared, action) {
				abortForbidden(c, "AUTH_FORBIDDEN", "No tienes permisos para realizar esta operación")
				return
			}
		}

		if !auth.HasModulePermission(modperms, slug, action, country) {
			abortForbidden(c, "AUTH_FORBIDDEN", "No tienes permisos para realizar esta operación")
			return
		}

		c.Next()
	}
}

// RequireModuleDefsPermission autoriza el CRUD de definiciones de módulo (el
// recurso estático module-builder), cuyos permisos se transportan en `modperms`
// bajo el slug reservado defsSlug. A diferencia de RequireModulePermission, NO
// lee el slug del path (las rutas de definiciones llevan el slug del módulo
// concreto en `:slug`, que no debe confundirse con el recurso de definiciones):
// usa defsSlug fijo. Las acciones válidas del recurso de definiciones son las 4
// del CRUD.
func RequireModuleDefsPermission(defsSlug string, action auth.Action) gin.HandlerFunc {
	return func(c *gin.Context) {
		modperms, ok := CurrentModulePermissions(c)
		if !ok {
			abortUnauthorized(c, "AUTH_MISSING_TOKEN", "Falta el token de sesión o el formato de la cabecera Authorization es inválido")
			return
		}
		country, ok := effectiveCountry(c)
		if !ok || country == "" {
			abortForbidden(c, "AUTH_FORBIDDEN", "No tienes permisos para realizar esta operación")
			return
		}
		if !auth.HasModulePermission(modperms, defsSlug, action, country) {
			abortForbidden(c, "AUTH_FORBIDDEN", "No tienes permisos para realizar esta operación")
			return
		}
		c.Next()
	}
}

// containsAction indica si action pertenece al conjunto declarado.
func containsAction(declared []auth.Action, action auth.Action) bool {
	for _, a := range declared {
		if a == action {
			return true
		}
	}
	return false
}
