package api

import (
	"context"
	"errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"admin-backend/internal/auth"
	"admin-backend/internal/roles"
	"admin-backend/internal/users"
)

// AuthHandler agrupa los handlers HTTP de autenticación de sesión del admin:
// POST /auth/login (valida credenciales, calcula los permisos efectivos y emite
// el JWT de sesión) y GET /auth/me (devuelve la identidad del administrador
// autenticado junto con sus permisos efectivos a partir de los claims del
// token). Depende del repositorio de usuarios para localizar al administrador,
// del repositorio de roles para resolver sus permisos efectivos (unión de sus
// roles) y de la configuración del JWT de sesión (secreto y expiración).
type AuthHandler struct {
	users             users.Repository
	roles             roles.Repository
	jwtSecret         string
	jwtExpiresMinutes int
}

// NewAuthHandler construye un AuthHandler con sus dependencias: el repositorio
// de usuarios, el repositorio de roles (para resolver los permisos efectivos en
// el login), el secreto de firma del JWT de sesión (ADMIN_JWT_SECRET) y la
// expiración en minutos (ADMIN_JWT_EXPIRES_MINUTES).
func NewAuthHandler(usersRepo users.Repository, rolesRepo roles.Repository, jwtSecret string, jwtExpiresMinutes int) *AuthHandler {
	return &AuthHandler{
		users:             usersRepo,
		roles:             rolesRepo,
		jwtSecret:         jwtSecret,
		jwtExpiresMinutes: jwtExpiresMinutes,
	}
}

// loginRequest es el cuerpo esperado por POST /auth/login.
type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// publicUser es la vista pública de un usuario del panel que el BFF expone al
// admin-frontend: nunca incluye el hash de la contraseña (Req. 1.6, 4, 5).
type publicUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

// permissionDTO es la forma explícita de un permiso efectivo que el BFF expone
// al admin-frontend: la terna {resource, action, country}. Es más fácil de
// consumir por el front que la forma compacta del claim y mantiene coherencia
// entre la respuesta de /auth/login y /auth/me (Req. 7.2, 7.4, 12.5).
type permissionDTO struct {
	Resource string `json:"resource"`
	Action   string `json:"action"`
	Country  string `json:"country"`
}

// permissionsToDTO convierte un conjunto de permisos efectivos a la lista de
// ternas explícitas que se serializa en las respuestas. Devuelve siempre un
// slice no nil (vacío si no hay permisos) para que el front reciba `[]`.
func permissionsToDTO(perms auth.EffectivePermissions) []permissionDTO {
	list := perms.List()
	out := make([]permissionDTO, 0, len(list))
	for _, p := range list {
		out = append(out, permissionDTO{
			Resource: string(p.Resource),
			Action:   string(p.Action),
			Country:  p.Country,
		})
	}
	return out
}

// modulePermissionDTO es la forma explícita de un permiso de módulo dinámico
// (module-builder) que el BFF expone al front: {slug, action, country}. El
// front la usa para decidir la visibilidad de módulos y botones (Req. 11.3,
// 11.4). country "*" denota un módulo sin ámbito de país.
type modulePermissionDTO struct {
	Slug    string `json:"slug"`
	Action  string `json:"action"`
	Country string `json:"country"`
}

// modulePermissionsToDTO convierte un conjunto de permisos de módulo a la lista
// de ternas explícitas. Devuelve siempre un slice no nil.
func modulePermissionsToDTO(perms auth.EffectiveModulePermissions) []modulePermissionDTO {
	list := perms.List()
	out := make([]modulePermissionDTO, 0, len(list))
	for _, p := range list {
		out = append(out, modulePermissionDTO{
			Slug:    p.Slug,
			Action:  string(p.Action),
			Country: p.Country,
		})
	}
	return out
}

// loginData es la carga útil devuelta por un login exitoso: el JWT de sesión,
// la información pública del usuario autenticado y sus permisos efectivos. Los
// permisos también viajan dentro del token (claims `perms`/`modperms`), pero se
// exponen aquí de forma explícita para que el front pueda renderizar la UI sin
// decodificar el claim; ambas fuentes son coherentes (Req. 7.2, 11.3, 12.5).
type loginData struct {
	Token       string                `json:"token"`
	User        publicUser            `json:"user"`
	Roles       []string              `json:"roles"`
	Permissions []permissionDTO       `json:"permissions"`
	Modules     []modulePermissionDTO `json:"modules"`
}

// meData es la carga útil de GET /auth/me: la identidad del administrador
// autenticado y sus permisos efectivos (lista de ternas), tomados de los claims
// del JWT de sesión. El front (tarea 17) consume esta forma para pintar la UI
// según los permisos del usuario (Req. 7.2, 7.4).
type meData struct {
	ID          string                `json:"id"`
	Username    string                `json:"username"`
	Role        string                `json:"role"`
	Roles       []string              `json:"roles"`
	Permissions []permissionDTO       `json:"permissions"`
	Modules     []modulePermissionDTO `json:"modules"`
}

// Login maneja POST /auth/login. Lee {username, password}, localiza al usuario
// ACTIVO (un usuario inactivo se trata como credenciales inválidas, Req. 4.3),
// verifica la contraseña con bcrypt (auth.CheckPassword) y, si coincide,
// resuelve sus permisos efectivos (unión de sus roles) y emite un JWT de sesión
// que los embebe (auth.IssueSessionToken). Devuelve el token junto con la
// información pública del usuario y sus permisos efectivos (Req. 1.5, 1.6, 7.1,
// 7.2, 12.5).
//
// Ante credenciales inválidas responde 401 con un mensaje genérico, sin revelar
// si el username existe, si el usuario está inactivo o si la contraseña era
// incorrecta (Req. 1.7, 4.3).
func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "E400", "Solicitud inválida")
		return
	}

	if req.Username == "" || req.Password == "" {
		// Credenciales incompletas: se tratan como credenciales inválidas para
		// no distinguir casos ni filtrar información (Req. 1.7).
		writeInvalidCredentials(c)
		return
	}

	// Solo usuarios activos pueden autenticarse: un usuario inactivo devuelve
	// ErrNotFound y se trata como credenciales inválidas (Req. 4.3).
	user, err := h.users.FindActiveByUsername(c.Request.Context(), req.Username)
	if err != nil {
		if errors.Is(err, users.ErrNotFound) {
			// No se revela que el usuario no existe o está inactivo (Req. 1.7, 4.3).
			writeInvalidCredentials(c)
			return
		}
		writeError(c, http.StatusInternalServerError, "E500", "No se pudo procesar la autenticación")
		return
	}

	if err := auth.CheckPassword(user.PasswordHash, req.Password); err != nil {
		if errors.Is(err, auth.ErrPasswordMismatch) {
			writeInvalidCredentials(c)
			return
		}
		// Hash almacenado inválido u otro fallo de verificación.
		writeError(c, http.StatusInternalServerError, "E500", "No se pudo procesar la autenticación")
		return
	}

	// Permisos efectivos del usuario: unión (sin duplicados) de los permisos de
	// todos sus roles asignados (Req. 7.1).
	perms, err := h.roles.PermissionsForUser(c.Request.Context(), user.ID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "E500", "No se pudo procesar la autenticación")
		return
	}

	// Nombres de los roles asignados, para que la UI los muestre (header).
	roleNames := h.roleNamesForUser(c.Request.Context(), user.ID)

	// Separa los permisos estáticos (claim `perms`) de los de módulos dinámicos
	// (claim `modperms`): los recursos mod:<slug> y module-builder no caben en el
	// bitmask compacto y viajan aparte (Req. 11.3, 11.4).
	staticPerms, modulePerms := auth.SplitEffectivePermissions(perms)

	// El token embebe los permisos estáticos en `perms` y los de módulos en
	// `modperms`, más los nombres de rol para la UI.
	token, err := auth.IssueSessionTokenWithModules(*user, staticPerms, modulePerms, roleNames, h.jwtSecret, h.jwtExpiresMinutes)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "E500", "No se pudo emitir la sesión")
		return
	}

	writeOK(c, http.StatusOK, loginData{
		Token: token,
		User: publicUser{
			ID:       user.ID,
			Username: user.Username,
			Role:     user.Role,
		},
		Roles:       roleNames,
		Permissions: permissionsToDTO(perms),
		Modules:     modulePermissionsToDTO(modulePerms),
	}, "Autenticación exitosa")
}

// Me maneja GET /auth/me. Devuelve la identidad del administrador autenticado
// (id, username, role) y sus permisos efectivos a partir de los claims del JWT
// de sesión que el middleware de autenticación (tarea 4.1) depositó en el
// *gin.Context. Los permisos se toman del propio claim (claims.EffectivePermissions)
// y se serializan como lista de ternas {resource, action, country}, la misma
// forma que /auth/login, para que el front renderice la UI según ellos
// (Req. 4, 5, 7.2, 7.4). Si no hay claims en el contexto responde 401, ya que
// la ruta debió pasar por el middleware de sesión.
func (h *AuthHandler) Me(c *gin.Context) {
	claims, ok := GetSessionClaims(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "E401", "No autenticado")
		return
	}

	writeOK(c, http.StatusOK, meData{
		ID:          claims.UserID(),
		Username:    claims.Username,
		Role:        claims.Role,
		Roles:       claims.Roles,
		Permissions: permissionsToDTO(claims.EffectivePermissions()),
		Modules:     modulePermissionsToDTO(claims.EffectiveModulePermissions()),
	}, "")
}

// RefreshPermissions maneja POST /auth/refresh-permissions. Reemite el JWT de
// sesión del usuario autenticado con sus permisos ACTUALES (recalculados contra
// el repositorio de roles), de modo que un cambio de permisos —p. ej. al crear
// un módulo nuevo— se refleje sin obligar a cerrar sesión y volver a entrar. El
// usuario se identifica SIEMPRE por el claim de sesión (claims.UserID()), nunca
// por el cuerpo de la petición, para que no pueda reemitir el token de otro.
//
// Responde 401 si no hay sesión, si el usuario ya no existe o está inactivo (sin
// devolver token); 500 ante un fallo de infraestructura; y 200 con la misma
// forma que /auth/login (token + identidad + permisos) cuando tiene éxito
// (Req. 11.3, 11.4).
func (h *AuthHandler) RefreshPermissions(c *gin.Context) {
	claims, ok := GetSessionClaims(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "E401", "No autenticado")
		return
	}

	ctx := c.Request.Context()
	userID := claims.UserID()

	user, err := h.users.FindByID(ctx, userID)
	if err != nil {
		if errors.Is(err, users.ErrNotFound) {
			writeError(c, http.StatusUnauthorized, "E401", "Sesión no válida")
			return
		}
		log.Printf("auth: error buscando usuario %q para recargar permisos: %v", userID, err)
		writeError(c, http.StatusInternalServerError, "E500", "No se pudieron recargar los permisos")
		return
	}
	if !user.IsActive {
		writeError(c, http.StatusUnauthorized, "E401", "Sesión no válida")
		return
	}

	perms, err := h.roles.PermissionsForUser(ctx, userID)
	if err != nil {
		log.Printf("auth: error resolviendo permisos del usuario %q: %v", userID, err)
		writeError(c, http.StatusInternalServerError, "E500", "No se pudieron recargar los permisos")
		return
	}

	roleNames := h.roleNamesForUser(ctx, userID)
	staticPerms, modulePerms := auth.SplitEffectivePermissions(perms)

	token, err := auth.IssueSessionTokenWithModules(*user, staticPerms, modulePerms, roleNames, h.jwtSecret, h.jwtExpiresMinutes)
	if err != nil {
		log.Printf("auth: error emitiendo la sesión al recargar permisos del usuario %q: %v", userID, err)
		writeError(c, http.StatusInternalServerError, "E500", "No se pudo emitir la sesión")
		return
	}

	writeOK(c, http.StatusOK, loginData{
		Token: token,
		User: publicUser{
			ID:       user.ID,
			Username: user.Username,
			Role:     user.Role,
		},
		Roles:       roleNames,
		Permissions: permissionsToDTO(perms),
		Modules:     modulePermissionsToDTO(modulePerms),
	}, "Permisos recargados")
}

// roleNamesForUser devuelve los nombres de los roles asignados al usuario para
// mostrarlos en la UI. Tolera errores devolviendo nil (la UI simplemente no
// mostrará roles), sin afectar el login.
func (h *AuthHandler) roleNamesForUser(ctx context.Context, userID string) []string {
	refs, err := h.roles.RolesForUser(ctx, userID)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(refs))
	for _, ref := range refs {
		names = append(names, ref.Name)
	}
	return names
}

// writeInvalidCredentials responde 401 con el mensaje genérico de credenciales
// inválidas, sin distinguir usuario inexistente de contraseña incorrecta
// (Req. 1.7).
func writeInvalidCredentials(c *gin.Context) {
	writeError(c, http.StatusUnauthorized, "E401", "Credenciales incorrectas")
}
