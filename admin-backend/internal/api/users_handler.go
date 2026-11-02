package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"admin-backend/internal/auth"
	"admin-backend/internal/roles"
	"admin-backend/internal/users"
)

// UsersHandler expone la administración RBAC de usuarios del panel: listar,
// detalle, crear (con contraseña generada), editar (roles + estado + países) y
// resetear contraseña. Ninguna respuesta incluye NUNCA el hash de contraseña.
//
// Modelo (rediseño 000003): el usuario tiene un scope de PAÍSES; sus permisos
// efectivos son el producto (acciones de sus roles × sus países). La edición y
// la auto-protección del último administrador razonan sobre ese producto.
type UsersHandler struct {
	users      users.Repository
	roles      roles.Repository
	bcryptCost int
}

// NewUsersHandler construye un UsersHandler.
func NewUsersHandler(usersRepo users.Repository, rolesRepo roles.Repository, bcryptCost int) *UsersHandler {
	return &UsersHandler{
		users:      usersRepo,
		roles:      rolesRepo,
		bcryptCost: bcryptCost,
	}
}

// userRoleView es la forma mínima de un rol en la vista pública de un usuario.
type userRoleView struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// userPublic es la vista pública de un Usuario_Admin: id, username, estado,
// roles asignados y países del scope. NUNCA incluye el hash.
type userPublic struct {
	ID        string         `json:"id"`
	Username  string         `json:"username"`
	IsActive  bool           `json:"isActive"`
	Roles     []userRoleView `json:"roles"`
	Countries []string       `json:"countries"`
}

// toUserPublic arma la vista pública de un usuario SIN exponer el hash.
func toUserPublic(u users.User, rolesView []userRoleView, countries []string) userPublic {
	if rolesView == nil {
		rolesView = []userRoleView{}
	}
	if countries == nil {
		countries = []string{}
	}
	return userPublic{
		ID:        u.ID,
		Username:  u.Username,
		IsActive:  u.IsActive,
		Roles:     rolesView,
		Countries: countries,
	}
}

// userCreateBody es el cuerpo de POST /users: username, roles y países. La
// contraseña NO viaja: la genera el servidor.
type userCreateBody struct {
	Username  string   `json:"username"`
	RoleIDs   []string `json:"roleIds"`
	Countries []string `json:"countries"`
}

// userEditBody es el cuerpo de PUT /users.
type userEditBody struct {
	ID        string   `json:"id"`
	RoleIDs   []string `json:"roleIds"`
	IsActive  bool     `json:"isActive"`
	Countries []string `json:"countries"`
}

// userResetPasswordBody es el cuerpo de POST /users/reset-password.
type userResetPasswordBody struct {
	ID string `json:"id"`
}

// userCreatedData es la carga útil de POST /users.
type userCreatedData struct {
	User              userPublic `json:"user"`
	GeneratedPassword string     `json:"generatedPassword"`
}

// userEditedData es la carga útil de PUT /users.
type userEditedData struct {
	User userPublic `json:"user"`
}

// userResetPasswordData es la carga útil de POST /users/reset-password.
type userResetPasswordData struct {
	GeneratedPassword string `json:"generatedPassword"`
}

// resolveUserView resuelve los roles (id+name) y países de un usuario para su
// vista pública. Tolera errores devolviendo slices vacíos no nil.
func (h *UsersHandler) resolveUserView(ctx context.Context, u users.User) userPublic {
	rolesView := []userRoleView{}
	if refs, err := h.roles.RolesForUser(ctx, u.ID); err == nil {
		for _, ref := range refs {
			rolesView = append(rolesView, userRoleView{ID: ref.ID, Name: ref.Name})
		}
	}
	countries, err := h.users.CountriesForUser(ctx, u.ID)
	if err != nil {
		countries = []string{}
	}
	return toUserPublic(u, rolesView, countries)
}

// List maneja GET /users.
func (h *UsersHandler) List(c *gin.Context) {
	ctx := c.Request.Context()
	list, err := h.users.List(ctx)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "E500", "No se pudieron listar los usuarios")
		return
	}

	out := make([]userPublic, 0, len(list))
	for _, u := range list {
		out = append(out, h.resolveUserView(ctx, u))
	}
	writeOK(c, http.StatusOK, out, "")
}

// Detail maneja GET /users/:id.
func (h *UsersHandler) Detail(c *gin.Context) {
	ctx := c.Request.Context()
	id := c.Param("id")

	user, err := h.users.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, users.ErrNotFound) {
			writeError(c, http.StatusNotFound, "E404", "Usuario no encontrado")
			return
		}
		writeError(c, http.StatusInternalServerError, "E500", "No se pudo obtener el usuario")
		return
	}

	writeOK(c, http.StatusOK, h.resolveUserView(ctx, *user), "")
}

// Create maneja POST /users. Exige username, al menos un rol y al menos un país.
func (h *UsersHandler) Create(c *gin.Context) {
	var body userCreateBody
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, http.StatusBadRequest, "E400", "Solicitud inválida")
		return
	}

	ctx := c.Request.Context()

	username := trimRequired(body.Username)
	if username == "" {
		writeError(c, http.StatusBadRequest, "E400", "El nombre de usuario es obligatorio")
		return
	}

	roleIDs := sanitizeStrings(body.RoleIDs)
	if len(roleIDs) == 0 {
		writeError(c, http.StatusBadRequest, "E400", "Se requiere al menos un rol para crear el usuario")
		return
	}

	countries := sanitizeCountries(body.Countries)
	if len(countries) == 0 {
		writeError(c, http.StatusBadRequest, "E400", "Se requiere al menos un país para crear el usuario")
		return
	}

	exists, err := h.users.ExistsByUsername(ctx, username)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "E500", "No se pudo verificar el nombre de usuario")
		return
	}
	if exists {
		writeError(c, http.StatusConflict, "USER_USERNAME_TAKEN", "Ya existe un usuario con ese nombre")
		return
	}

	plainPassword, err := auth.GeneratePassword()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "E500", "No se pudo generar la contraseña")
		return
	}

	hash, err := auth.HashPassword(plainPassword, h.bcryptCost)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "E500", "No se pudo proteger la contraseña")
		return
	}

	user := &users.User{
		Username:     username,
		PasswordHash: hash,
		IsActive:     true,
	}
	if err := h.users.Create(ctx, user); err != nil {
		writeError(c, http.StatusInternalServerError, "E500", "No se pudo crear el usuario")
		return
	}

	if err := h.users.SetRoles(ctx, user.ID, roleIDs); err != nil {
		writeError(c, http.StatusInternalServerError, "E500", "No se pudieron asignar los roles al usuario")
		return
	}

	if err := h.users.SetCountries(ctx, user.ID, countries); err != nil {
		writeError(c, http.StatusInternalServerError, "E500", "No se pudieron asignar los países al usuario")
		return
	}

	// Fija el id del usuario creado para la bitácora de auditoría.
	SetAuditObjectID(c, user.ID)

	writeOK(c, http.StatusCreated, userCreatedData{
		User:              h.resolveUserView(ctx, *user),
		GeneratedPassword: plainPassword,
	}, "Usuario creado correctamente")
}

// Edit maneja PUT /users. Reemplaza roles, estado y países de forma atómica,
// con la auto-protección del último administrador (Req. 14).
func (h *UsersHandler) Edit(c *gin.Context) {
	var body userEditBody
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, http.StatusBadRequest, "E400", "Solicitud inválida")
		return
	}

	id := trimRequired(body.ID)
	if id == "" {
		writeError(c, http.StatusBadRequest, "E400", "El id del usuario es obligatorio")
		return
	}

	ctx := c.Request.Context()

	current, err := h.users.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, users.ErrNotFound) {
			writeError(c, http.StatusNotFound, "E404", "Usuario no encontrado")
			return
		}
		writeError(c, http.StatusInternalServerError, "E500", "No se pudo obtener el usuario")
		return
	}

	roleIDs := sanitizeStrings(body.RoleIDs)
	countries := sanitizeCountries(body.Countries)

	// --- Auto-protección del último administrador (Req. 14) ---
	activeAdmins, err := h.roles.CountActiveAdmins(ctx)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "E500", "No se pudo evaluar la auto-protección del administrador")
		return
	}

	// ¿Es admin activo HOY? Sus permisos efectivos actuales (acciones × países
	// actuales) habilitan administración y está activo.
	currentPerms, err := h.roles.PermissionsForUser(ctx, id)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "E500", "No se pudieron resolver los permisos del usuario")
		return
	}
	targetIsActiveAdmin := current.IsActive && auth.IsAdminPermissions(currentPerms)

	// ¿Seguiría siendo admin activo tras el cambio? Permisos resultantes = unión
	// de acciones de los nuevos roles × países editados. Si no quedan países, el
	// conjunto es vacío y NO es admin.
	resultingPerms, err := h.resolveEffectiveForRolesAndCountries(ctx, roleIDs, countries)
	if err != nil {
		if errors.Is(err, roles.ErrNotFound) {
			writeError(c, http.StatusBadRequest, "E400", "Uno de los roles indicados no existe")
			return
		}
		writeError(c, http.StatusInternalServerError, "E500", "No se pudieron resolver los permisos de los roles")
		return
	}
	targetRemainsAdmin := body.IsActive && auth.IsAdminPermissions(resultingPerms)

	state := auth.AdminState{
		ActiveAdmins:        int(activeAdmins),
		TargetIsActiveAdmin: targetIsActiveAdmin,
	}
	op := auth.AdminOp{
		Kind:                    auth.OpEditUserRoles,
		TargetRemainsAdminAfter: targetRemainsAdmin,
		ResultingActiveAdmins:   -1,
	}
	if auth.WouldLeaveNoAdmin(state, op) {
		writeError(c, http.StatusConflict, "AUTH_LAST_ADMIN", "La operación dejaría al sistema sin ningún administrador activo")
		return
	}

	if err := h.users.UpdateRolesStatusAndCountries(ctx, id, roleIDs, body.IsActive, countries); err != nil {
		if errors.Is(err, users.ErrNotFound) {
			writeError(c, http.StatusNotFound, "E404", "Usuario no encontrado")
			return
		}
		writeError(c, http.StatusInternalServerError, "E500", "No se pudo actualizar el usuario")
		return
	}

	updated := *current
	updated.IsActive = body.IsActive
	writeOK(c, http.StatusOK, userEditedData{User: h.resolveUserView(ctx, updated)}, "Usuario actualizado correctamente")
}

// ResetPassword maneja POST /users/reset-password.
func (h *UsersHandler) ResetPassword(c *gin.Context) {
	var body userResetPasswordBody
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, http.StatusBadRequest, "E400", "Solicitud inválida")
		return
	}

	id := trimRequired(body.ID)
	if id == "" {
		writeError(c, http.StatusBadRequest, "E400", "El id del usuario es obligatorio")
		return
	}

	plainPassword, err := auth.GeneratePassword()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "E500", "No se pudo generar la contraseña")
		return
	}

	hash, err := auth.HashPassword(plainPassword, h.bcryptCost)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "E500", "No se pudo proteger la contraseña")
		return
	}

	if err := h.users.UpdatePasswordHash(c.Request.Context(), id, hash); err != nil {
		if errors.Is(err, users.ErrNotFound) {
			writeError(c, http.StatusNotFound, "E404", "Usuario no encontrado")
			return
		}
		writeError(c, http.StatusInternalServerError, "E500", "No se pudo restablecer la contraseña")
		return
	}

	writeOK(c, http.StatusOK, userResetPasswordData{GeneratedPassword: plainPassword}, "Contraseña restablecida correctamente")
}

// resolveEffectiveForRolesAndCountries calcula los permisos efectivos que
// resultarían de la unión de las acciones (recurso×acción) de los roles dados,
// expandidas por los países dados (producto cartesiano). Devuelve
// roles.ErrNotFound si algún rol no existe.
func (h *UsersHandler) resolveEffectiveForRolesAndCountries(ctx context.Context, roleIDs []string, countries []string) (auth.EffectivePermissions, error) {
	pairs := make([]auth.Permission, 0)
	for _, rid := range roleIDs {
		role, err := h.roles.FindByID(ctx, rid)
		if err != nil {
			return auth.EffectivePermissions{}, err
		}
		pairs = append(pairs, role.Permissions...)
	}

	expanded := make([]auth.Permission, 0, len(pairs)*len(countries))
	for _, p := range pairs {
		for _, country := range countries {
			expanded = append(expanded, auth.Permission{
				Resource: p.Resource,
				Action:   p.Action,
				Country:  country,
			})
		}
	}
	return auth.NewEffectivePermissions(expanded), nil
}

// trimRequired recorta espacios de un campo obligatorio de texto.
func trimRequired(s string) string {
	return strings.TrimSpace(s)
}

// sanitizeStrings recorta, descarta vacíos y deduplica conservando el orden.
func sanitizeStrings(ids []string) []string {
	out := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// sanitizeCountries normaliza países a minúsculas, descarta vacíos y deduplica.
func sanitizeCountries(countries []string) []string {
	out := make([]string, 0, len(countries))
	seen := make(map[string]struct{}, len(countries))
	for _, country := range countries {
		c := strings.ToLower(strings.TrimSpace(country))
		if c == "" {
			continue
		}
		if _, dup := seen[c]; dup {
			continue
		}
		seen[c] = struct{}{}
		out = append(out, c)
	}
	return out
}
