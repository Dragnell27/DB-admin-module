package middleware

// Este archivo implementa el middleware de autorización por permiso
// (RequirePermission, tarea 11.1), que sustituye a RequireAdmin en las rutas de
// recurso del router (la sustitución la realiza la tarea 12.1). Exige que los
// permisos efectivos del administrador autenticado incluyan la terna
// (recurso, acción, país efectivo) de la operación solicitada; en caso
// contrario responde 403 con el envoltorio de error normalizado y aborta la
// cadena antes de ejecutar el handler (Req. 8.1, 8.2, 8.3, 8.4, 8.7).
//
// Debe montarse después de SessionAuth (que deja los permisos efectivos en el
// contexto, tarea 11.1 PARTE A) y de CountryResolver (que deja el país efectivo
// en el contexto). Reutiliza abortUnauthorized (401) y abortForbidden (403)
// definidos en auth.go/role.go (mismo paquete) y el envoltorio errorBody.

import (
	"github.com/gin-gonic/gin"

	"admin-backend/internal/auth"
)

// contextKeyEffectiveCountry es la clave bajo la cual el middleware de país del
// paquete api (api.CountryResolver) deja el país efectivo ya validado y
// normalizado en el gin.Context.
//
// El literal se DUPLICA aquí a propósito: su contraparte canónica es
// api.contextKeyCountry (= "effectiveCountry"), pero el paquete middleware NO
// puede importar api porque el router de api importa middleware y eso crearía
// un ciclo de importación. Al compartir el mismo valor literal, RequirePermission
// recupera el país que CountryResolver depositó sin acoplar los paquetes.
// Cualquier cambio del literal en api.country.go debe reflejarse aquí.
const contextKeyEffectiveCountry = "effectiveCountry"

// RequirePermission construye el middleware de autorización por permiso para una
// ruta concreta, parametrizado por el (recurso, acción) que la operación
// requiere.
//
// Secuencia de evaluación (Req. 8.2, 8.3, 8.4, 8.6, 8.7):
//
//  1. Recupera los permisos efectivos del contexto con CurrentPermissions. Si
//     no hay (la ruta no pasó por SessionAuth), responde 401 con
//     abortUnauthorized. En la práctica SessionAuth corre antes y este caso no
//     ocurre, pero se comprueba de forma defensiva.
//  2. Recupera el país efectivo del contexto (lo deja api.CountryResolver bajo
//     contextKeyEffectiveCountry). Si no hay país o está vacío, responde 403 de
//     forma defensiva: sin país no es posible autorizar una terna.
//  3. Evalúa auth.HasPermission(perms, resource, action, country). Si la terna
//     está autorizada continúa la cadena (c.Next()); si no, responde 403 con
//     abortForbidden y el código estable "AUTH_FORBIDDEN".
func RequirePermission(resource auth.Resource, action auth.Action) gin.HandlerFunc {
	return func(c *gin.Context) {
		perms, ok := CurrentPermissions(c)
		if !ok {
			// La ruta no pasó por SessionAuth: no hay identidad autenticada.
			abortUnauthorized(c, "AUTH_MISSING_TOKEN", "Falta el token de sesión o el formato de la cabecera Authorization es inválido")
			return
		}

		country, ok := effectiveCountry(c)
		if !ok || country == "" {
			// Sin país efectivo no se puede autorizar la terna; se rechaza con
			// 403 de forma defensiva (en la práctica CountryResolver siempre lo
			// deja antes de llegar aquí).
			abortForbidden(c, "AUTH_FORBIDDEN", "No tienes permisos para realizar esta operación")
			return
		}

		if !auth.HasPermission(perms, resource, action, country) {
			abortForbidden(c, "AUTH_FORBIDDEN", "No tienes permisos para realizar esta operación")
			return
		}

		c.Next()
	}
}

// effectiveCountry recupera el país efectivo que api.CountryResolver dejó en el
// contexto bajo contextKeyEffectiveCountry. Devuelve (país, true) si está
// presente como string; (vacío, false) en caso contrario.
func effectiveCountry(c *gin.Context) (string, bool) {
	v, ok := c.Get(contextKeyEffectiveCountry)
	if !ok {
		return "", false
	}
	country, ok := v.(string)
	if !ok {
		return "", false
	}
	return country, true
}
