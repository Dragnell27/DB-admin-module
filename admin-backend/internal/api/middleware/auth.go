// Package middleware contiene los middlewares HTTP (Gin) del admin-backend:
// autenticación de sesión y, en archivos separados, autorización por rol.
//
// Este archivo implementa únicamente el middleware de autenticación de sesión
// (tarea 4.1). Valida el JWT de sesión del admin (presencia, firma y
// expiración) y, en caso de éxito, deja los claims (id de usuario, username y
// rol) disponibles en el gin.Context para los handlers y el middleware de rol
// (tarea 4.2). Si el token falta o es inválido, responde 401 con el envoltorio
// de error normalizado y aborta la cadena (Req. 2.2, 2.5).
package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"admin-backend/internal/auth"
)

// Claves de contexto bajo las que el middleware de sesión almacena la identidad
// del administrador autenticado. Los handlers y el middleware de rol (tarea
// 4.2) deben leer la identidad mediante los helpers de este archivo
// (CurrentUserID, CurrentUsername, CurrentRole) y no acceder a estas claves
// directamente, para mantener el acoplamiento aislado.
const (
	contextKeyUserID   = "auth.userID"
	contextKeyUsername = "auth.username"
	contextKeyRole     = "auth.role"

	// contextKeySessionClaims es la clave bajo la cual el middleware deja los
	// *auth.SessionClaims completos para los handlers que necesitan el objeto
	// de claims (p. ej. GET /auth/me). Su valor literal coincide a propósito
	// con api.ContextKeySessionClaims para que el helper api.GetSessionClaims
	// recupere los claims que este middleware deposita, sin que el paquete
	// middleware importe api (lo que crearía un ciclo de importación, ya que el
	// router de api importa middleware). Reconcilia así las dos convenciones de
	// identidad descritas en context.go (tarea 9.1).
	contextKeySessionClaims = "sessionClaims"

	// contextKeyPermissions es la clave bajo la cual el middleware de sesión
	// deja los permisos efectivos (auth.EffectivePermissions) decodificados del
	// claim `perms`. El middleware de autorización por permiso
	// (RequirePermission, tarea 11.1) los recupera mediante CurrentPermissions
	// para decidir, junto con el país efectivo, si la terna solicitada está
	// autorizada (Req. 8.2, 8.3).
	contextKeyPermissions = "auth.permissions"

	// contextKeyModulePermissions es la clave bajo la cual el middleware de
	// sesión deja los permisos de módulos dinámicos (auth.EffectiveModule
	// Permissions) decodificados del claim `modperms`. RequireModulePermission
	// (module-builder) los recupera con CurrentModulePermissions para autorizar
	// acciones sobre recursos dinámicos mod:<slug> (Req. 11.5, 11.6).
	contextKeyModulePermissions = "auth.modulePermissions"
)

// errorBody es el envoltorio de error que el BFF devuelve al admin-frontend.
// Replica la forma del resultado normalizado OTT (`ok`, `errorCode`,
// `message`) para que el interceptor del front trate de forma uniforme los
// errores del propio BFF y los propagados desde OTT (Req. 20.1). Un 401 del
// middleware desencadena el logout en el front (Req. 2.5).
type errorBody struct {
	OK        bool   `json:"ok"`
	ErrorCode string `json:"errorCode"`
	Message   string `json:"message"`
}

// SessionAuth construye el middleware de autenticación de sesión del admin.
//
// Extrae el token Bearer de la cabecera Authorization, lo valida con
// auth.ValidateSessionToken usando `secret` (ADMIN_JWT_SECRET) y, de ser
// válido, almacena en el gin.Context el id de usuario (claim `sub`), el
// username y el rol para los handlers posteriores. Ante ausencia de la
// cabecera, un esquema distinto de Bearer, o un token con firma inválida o
// expirado, responde 401 con el envoltorio de error normalizado y aborta la
// petición (Req. 2.2, 2.5).
func SessionAuth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := bearerToken(c.GetHeader("Authorization"))
		if !ok {
			abortUnauthorized(c, "AUTH_MISSING_TOKEN", "Falta el token de sesión o el formato de la cabecera Authorization es inválido")
			return
		}

		claims, err := auth.ValidateSessionToken(token, secret)
		if err != nil {
			abortUnauthorized(c, errorCodeFor(err), messageFor(err))
			return
		}

		c.Set(contextKeyUserID, claims.UserID())
		c.Set(contextKeyUsername, claims.Username)
		c.Set(contextKeyRole, claims.Role)
		// Depositar además el objeto de claims completo bajo la clave compartida
		// con api, para que GET /auth/me (api.GetSessionClaims) lo recupere.
		c.Set(contextKeySessionClaims, claims)
		// Decodificar los permisos efectivos del claim compacto `perms` y
		// dejarlos en el contexto para que RequirePermission (tarea 11.1)
		// autorice por recurso/acción/país sin volver a parsear el token
		// (Req. 8.2, 8.3).
		c.Set(contextKeyPermissions, claims.EffectivePermissions())
		// Permisos de módulos dinámicos (module-builder) del claim `modperms`,
		// para RequireModulePermission (Req. 11.5, 11.6).
		c.Set(contextKeyModulePermissions, claims.EffectiveModulePermissions())

		c.Next()
	}
}

// bearerToken extrae el token de un valor de cabecera Authorization con el
// esquema Bearer. Devuelve el token y true solo si la cabecera usa el esquema
// Bearer (sin distinguir mayúsculas) y el token resultante no está vacío.
func bearerToken(header string) (string, bool) {
	const prefix = "Bearer "
	header = strings.TrimSpace(header)
	if len(header) < len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return "", false
	}
	token := strings.TrimSpace(header[len(prefix):])
	if token == "" {
		return "", false
	}
	return token, true
}

// errorCodeFor mapea un error de validación del token a un código de error
// estable que el front puede inspeccionar.
func errorCodeFor(err error) string {
	switch {
	case err == auth.ErrExpiredToken:
		return "AUTH_TOKEN_EXPIRED"
	default:
		// ErrInvalidToken, ErrEmptySecret y cualquier otro fallo de validación
		// se exponen como token inválido: todos resultan en un 401 que fuerza
		// el relogin en el front.
		return "AUTH_INVALID_TOKEN"
	}
}

// messageFor devuelve un mensaje legible en español para el front según la
// causa del rechazo.
func messageFor(err error) string {
	switch {
	case err == auth.ErrExpiredToken:
		return "La sesión ha expirado"
	default:
		return "Token de sesión inválido"
	}
}

// abortUnauthorized responde 401 con el envoltorio de error normalizado y
// aborta la cadena de middlewares/handlers.
func abortUnauthorized(c *gin.Context, errorCode, message string) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, errorBody{
		OK:        false,
		ErrorCode: errorCode,
		Message:   message,
	})
}

// CurrentUserID devuelve el id del administrador autenticado almacenado por el
// middleware de sesión, y false si no hay identidad en el contexto (p. ej. en
// una ruta sin el middleware aplicado).
func CurrentUserID(c *gin.Context) (string, bool) {
	return stringFromContext(c, contextKeyUserID)
}

// CurrentUsername devuelve el username del administrador autenticado, y false
// si no está presente en el contexto.
func CurrentUsername(c *gin.Context) (string, bool) {
	return stringFromContext(c, contextKeyUsername)
}

// CurrentRole devuelve el rol del administrador autenticado, y false si no está
// presente en el contexto. El middleware de autorización por rol (tarea 4.2)
// debe leer el rol mediante este helper.
func CurrentRole(c *gin.Context) (string, bool) {
	return stringFromContext(c, contextKeyRole)
}

// CurrentClaims devuelve los *auth.SessionClaims completos depositados por el
// middleware de sesión, y false si no están presentes en el contexto. Comparte
// la clave con api.GetSessionClaims para que ambos helpers vean el mismo valor.
func CurrentClaims(c *gin.Context) (*auth.SessionClaims, bool) {
	v, exists := c.Get(contextKeySessionClaims)
	if !exists {
		return nil, false
	}
	claims, ok := v.(*auth.SessionClaims)
	if !ok || claims == nil {
		return nil, false
	}
	return claims, true
}

// CurrentPermissions devuelve los permisos efectivos del administrador
// autenticado que el middleware de sesión decodificó del claim `perms`, y false
// si no están presentes en el contexto (p. ej. una ruta que no pasó por
// SessionAuth). El middleware de autorización por permiso (RequirePermission,
// tarea 11.1) debe leer los permisos mediante este helper (Req. 8.2).
func CurrentPermissions(c *gin.Context) (auth.EffectivePermissions, bool) {
	v, exists := c.Get(contextKeyPermissions)
	if !exists {
		return auth.EffectivePermissions{}, false
	}
	perms, ok := v.(auth.EffectivePermissions)
	if !ok {
		return auth.EffectivePermissions{}, false
	}
	return perms, true
}

// CurrentModulePermissions devuelve los permisos de módulos dinámicos del
// administrador autenticado que el middleware de sesión decodificó del claim
// `modperms`, y false si no están presentes (ruta sin SessionAuth).
// RequireModulePermission los usa para autorizar acciones sobre mod:<slug>.
func CurrentModulePermissions(c *gin.Context) (auth.EffectiveModulePermissions, bool) {
	v, exists := c.Get(contextKeyModulePermissions)
	if !exists {
		return auth.EffectiveModulePermissions{}, false
	}
	perms, ok := v.(auth.EffectiveModulePermissions)
	if !ok {
		return auth.EffectiveModulePermissions{}, false
	}
	return perms, true
}

// stringFromContext lee un valor string de una clave del gin.Context de forma
// segura.
func stringFromContext(c *gin.Context, key string) (string, bool) {
	v, exists := c.Get(key)
	if !exists {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}
