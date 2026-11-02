package api

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"admin-backend/internal/auth"
	"admin-backend/internal/modules"
	"admin-backend/internal/roles"
)

// RolesHandler expone la administración RBAC de roles del panel. Un permiso de
// rol es la dupla Recurso × Acción (sin país): el país es scope del usuario. El
// handler valida el catálogo (recurso/acción) con auth.ValidateRolePermission
// antes de persistir; el repositorio solo guarda (Req. 5.1, 5.2, 5.4, 6.1, 6.3,
// 6.4, 6.5).
type RolesHandler struct {
	roles roles.Repository
	// modules resuelve las definiciones de módulo para validar los permisos
	// dinámicos mod:<slug> y poblar el catálogo de módulos de la UI de roles.
	// Puede ser nil cuando el constructor de módulos está desactivado: en ese
	// caso cualquier permiso mod:<slug> se rechaza con 400 y el catálogo devuelve
	// una lista vacía.
	modules modules.Repository
}

// NewRolesHandler construye un RolesHandler con el repositorio de roles y el de
// módulos (puede ser nil si el constructor de módulos está desactivado).
func NewRolesHandler(rolesRepo roles.Repository, modulesRepo modules.Repository) *RolesHandler {
	return &RolesHandler{roles: rolesRepo, modules: modulesRepo}
}

// rolePermissionDTO es la forma explícita de un permiso de ROL: la dupla
// {resource, action}, sin país.
type rolePermissionDTO struct {
	Resource string `json:"resource"`
	Action   string `json:"action"`
}

// rolePublic es la vista pública de un rol: id, nombre y permisos (duplas).
type rolePublic struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	Permissions []rolePermissionDTO `json:"permissions"`
}

// roleCreateBody es el cuerpo de POST /roles.
type roleCreateBody struct {
	Name        string              `json:"name"`
	Permissions []rolePermissionDTO `json:"permissions"`
}

// roleEditBody es el cuerpo de PUT /roles.
type roleEditBody struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	Permissions []rolePermissionDTO `json:"permissions"`
}

// roleDeleteBody es el cuerpo opcional de DELETE /roles cuando el id no viaja en
// la ruta.
type roleDeleteBody struct {
	ID string `json:"id"`
}

// roleData es la carga útil de POST /roles y PUT /roles.
type roleData struct {
	Role rolePublic `json:"role"`
}

// rolePermissionsFromDTO mapea la lista de duplas del cuerpo HTTP al tipo de
// dominio []auth.Permission (Country vacío). No valida el catálogo.
func rolePermissionsFromDTO(dtos []rolePermissionDTO) []auth.Permission {
	out := make([]auth.Permission, 0, len(dtos))
	for _, d := range dtos {
		out = append(out, auth.Permission{
			Resource: auth.Resource(d.Resource),
			Action:   auth.Action(d.Action),
		})
	}
	return out
}

// rolePermissionsToDTO mapea los permisos de dominio de un rol a su forma de
// salida (duplas). Siempre devuelve un slice no nil.
func rolePermissionsToDTO(perms []auth.Permission) []rolePermissionDTO {
	out := make([]rolePermissionDTO, 0, len(perms))
	for _, p := range perms {
		out = append(out, rolePermissionDTO{
			Resource: string(p.Resource),
			Action:   string(p.Action),
		})
	}
	return out
}

// toRolePublic arma la vista pública de un rol.
func toRolePublic(r roles.Role) rolePublic {
	return rolePublic{
		ID:          r.ID,
		Name:        r.Name,
		Permissions: rolePermissionsToDTO(r.Permissions),
	}
}

// permValidationError es un error tipado de validación de permisos de rol que
// transporta el estado HTTP, el código y el mensaje ya resueltos. Permite que
// validatePermissions distinga un rechazo de catálogo (400, mensaje al cliente)
// de un fallo de infraestructura al consultar el módulo (500, mensaje genérico
// sin filtrar el detalle crudo). El llamador lo recupera con errors.As.
type permValidationError struct {
	status  int
	code    string
	message string
	// err es el error de infraestructura subyacente (solo en status 500); se
	// registra en el log del servidor y NUNCA se expone al cliente.
	err error
}

// Error satisface la interfaz error devolviendo el mensaje orientado al cliente.
func (e permValidationError) Error() string {
	return e.message
}

// validatePermissions valida cada dupla del cuerpo. Para recursos dinámicos
// (mod:<slug>) comprueba que el módulo exista y que la acción esté declarada por
// su Definición; para recursos del catálogo cerrado delega en
// auth.ValidateRolePermission. Devuelve el conjunto de dominio ya mapeado o un
// permValidationError con el estado/código/mensaje correspondiente.
//
// Degradación segura de permisos COLGANTES (R3.4, caso acción-no-declarada): si
// un recurso dinámico bien formado apunta a un módulo que EXISTE pero cuya
// acción ya NO está declarada por su Definición (p. ej. porque se borró esa
// acción del módulo), la dupla se considera colgante y se OMITE del conjunto
// persistido en vez de rechazar el guardado con 400. Así el rol se puede
// guardar para quedar SIN el permiso obsoleto. Se sigue rechazando con 400 lo
// genuinamente inválido: un módulo inexistente (ErrNotFound), un mod:<slug> mal
// formado (que auth.IsDynamicResource considera NO dinámico y cae al catálogo
// cerrado) o una dupla fija inválida del catálogo cerrado.
func (h *RolesHandler) validatePermissions(ctx context.Context, dtos []rolePermissionDTO) ([]auth.Permission, error) {
	perms := rolePermissionsFromDTO(dtos)
	// kept acumula solo las duplas conservadas; las colgantes (módulo existente
	// con acción no declarada) se descartan silenciosamente para que Create/Edit
	// persistan el conjunto limpio y toRolePublic lo refleje en la respuesta.
	kept := make([]auth.Permission, 0, len(perms))
	for _, p := range perms {
		if auth.IsDynamicResource(p.Resource) {
			slug, _ := auth.SlugFromDynamicResource(p.Resource)

			if h.modules == nil {
				return nil, permValidationError{
					status:  http.StatusBadRequest,
					code:    "E400",
					message: "El módulo " + slug + " no existe",
				}
			}

			def, err := h.modules.FindBySlug(ctx, slug)
			if err != nil {
				if errors.Is(err, modules.ErrNotFound) {
					return nil, permValidationError{
						status:  http.StatusBadRequest,
						code:    "E400",
						message: "El módulo " + slug + " no existe",
					}
				}
				return nil, permValidationError{
					status:  http.StatusInternalServerError,
					code:    "E500",
					message: "No se pudo validar los permisos de módulo",
					err:     err,
				}
			}

			declaredStr := modules.PermActionsForDefinition(*def)
			declared := make([]auth.Action, 0, len(declaredStr))
			for _, a := range declaredStr {
				declared = append(declared, auth.Action(a))
			}
			if err := auth.ValidateDynamicModuleAction(p.Resource, p.Action, declared); err != nil {
				// El recurso ya es dinámico bien formado y el módulo existe, por
				// lo que ValidateDynamicModuleAction solo puede devolver
				// ErrInvalidAction: la acción ya no está declarada (colgante). Se
				// descarta la dupla del conjunto persistido y se continúa, sin
				// bloquear el guardado.
				if errors.Is(err, auth.ErrInvalidAction) {
					continue
				}
				// Cualquier otro error (p. ej. recurso mal formado) se trata como
				// estructuralmente inválido y se rechaza.
				return nil, permValidationError{
					status:  http.StatusBadRequest,
					code:    "E400",
					message: "La acción " + string(p.Action) + " no está declarada por el módulo " + slug,
				}
			}
			kept = append(kept, p)
			continue
		}

		if err := auth.ValidateRolePermission(p.Resource, p.Action); err != nil {
			return nil, permValidationError{
				status:  http.StatusBadRequest,
				code:    "E400",
				message: err.Error(),
			}
		}
		kept = append(kept, p)
	}
	return kept, nil
}

// writePermValidationError traduce el error de validatePermissions a una
// respuesta HTTP: un permValidationError usa su estado/código/mensaje (y
// registra en el log el err real cuando es un 500); cualquier otro error cae a
// un 400 genérico con su texto.
func writePermValidationError(c *gin.Context, err error) {
	var pe permValidationError
	if errors.As(err, &pe) {
		if pe.status == http.StatusInternalServerError && pe.err != nil {
			log.Printf("roles: error validando permisos de módulo: %v", pe.err)
		}
		writeError(c, pe.status, pe.code, pe.message)
		return
	}
	writeError(c, http.StatusBadRequest, "E400", err.Error())
}

// List maneja GET /roles.
func (h *RolesHandler) List(c *gin.Context) {
	list, err := h.roles.List(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "E500", "No se pudieron listar los roles")
		return
	}

	out := make([]rolePublic, 0, len(list))
	for _, r := range list {
		out = append(out, toRolePublic(r))
	}
	writeOK(c, http.StatusOK, out, "")
}

// Detail maneja GET /roles/:id.
func (h *RolesHandler) Detail(c *gin.Context) {
	id := c.Param("id")

	role, err := h.roles.FindByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, roles.ErrNotFound) {
			writeError(c, http.StatusNotFound, "E404", "Rol no encontrado")
			return
		}
		writeError(c, http.StatusInternalServerError, "E500", "No se pudo obtener el rol")
		return
	}

	writeOK(c, http.StatusOK, toRolePublic(*role), "")
}

// Create maneja POST /roles.
func (h *RolesHandler) Create(c *gin.Context) {
	var body roleCreateBody
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, http.StatusBadRequest, "E400", "Solicitud inválida")
		return
	}

	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeError(c, http.StatusBadRequest, "E400", "El nombre del rol es obligatorio")
		return
	}

	perms, err := h.validatePermissions(c.Request.Context(), body.Permissions)
	if err != nil {
		writePermValidationError(c, err)
		return
	}

	role := &roles.Role{
		Name:        name,
		Permissions: perms,
	}
	if err := h.roles.Create(c.Request.Context(), role); err != nil {
		switch {
		case errors.Is(err, roles.ErrNameTaken):
			writeError(c, http.StatusConflict, "ROLE_NAME_TAKEN", "Ya existe un rol con ese nombre")
		case errors.Is(err, roles.ErrEmptyName):
			writeError(c, http.StatusBadRequest, "E400", "El nombre del rol es obligatorio")
		default:
			writeError(c, http.StatusInternalServerError, "E500", "No se pudo crear el rol")
		}
		return
	}

	// Fija el id del rol creado para la bitácora de auditoría.
	SetAuditObjectID(c, role.ID)

	writeOK(c, http.StatusCreated, roleData{Role: toRolePublic(*role)}, "Rol creado correctamente")
}

// Edit maneja PUT /roles.
func (h *RolesHandler) Edit(c *gin.Context) {
	var body roleEditBody
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, http.StatusBadRequest, "E400", "Solicitud inválida")
		return
	}

	id := strings.TrimSpace(body.ID)
	if id == "" {
		writeError(c, http.StatusBadRequest, "E400", "El id del rol es obligatorio")
		return
	}

	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeError(c, http.StatusBadRequest, "E400", "El nombre del rol es obligatorio")
		return
	}

	perms, err := h.validatePermissions(c.Request.Context(), body.Permissions)
	if err != nil {
		writePermValidationError(c, err)
		return
	}

	role := &roles.Role{
		ID:          id,
		Name:        name,
		Permissions: perms,
	}
	if err := h.roles.Update(c.Request.Context(), role); err != nil {
		switch {
		case errors.Is(err, roles.ErrNotFound):
			writeError(c, http.StatusNotFound, "E404", "Rol no encontrado")
		case errors.Is(err, roles.ErrNameTaken):
			writeError(c, http.StatusConflict, "ROLE_NAME_TAKEN", "Ya existe un rol con ese nombre")
		case errors.Is(err, roles.ErrEmptyName):
			writeError(c, http.StatusBadRequest, "E400", "El nombre del rol es obligatorio")
		default:
			writeError(c, http.StatusInternalServerError, "E500", "No se pudo actualizar el rol")
		}
		return
	}

	writeOK(c, http.StatusOK, roleData{Role: toRolePublic(*role)}, "Rol actualizado correctamente")
}

// Delete maneja DELETE /roles/:id y DELETE /roles con cuerpo {id}.
func (h *RolesHandler) Delete(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		var body roleDeleteBody
		if err := c.ShouldBindJSON(&body); err == nil {
			id = strings.TrimSpace(body.ID)
		}
	}
	if id == "" {
		writeError(c, http.StatusBadRequest, "E400", "El id del rol es obligatorio")
		return
	}

	// Fija el id del rol para la bitácora (viene del path :id, no del cuerpo).
	SetAuditObjectID(c, id)

	if err := h.roles.Delete(c.Request.Context(), id); err != nil {
		switch {
		case errors.Is(err, roles.ErrRoleAssigned):
			writeError(c, http.StatusConflict, "ROLE_ASSIGNED", "El rol está asignado a usuarios y no puede eliminarse")
		case errors.Is(err, roles.ErrNotFound):
			writeError(c, http.StatusNotFound, "E404", "Rol no encontrado")
		default:
			writeError(c, http.StatusInternalServerError, "E500", "No se pudo eliminar el rol")
		}
		return
	}

	writeOK(c, http.StatusOK, gin.H{"id": id}, "Rol eliminado correctamente")
}

// moduleCatalogItemDTO es un módulo del catálogo que consume la UI de roles para
// pintar los recursos dinámicos mod:<slug> junto a los fijos: su slug, su nombre
// legible y las acciones de permiso que declara (ver + las propias), ya
// deduplicadas y en orden estable (ver primero).
type moduleCatalogItemDTO struct {
	Slug    string   `json:"slug"`
	Name    string   `json:"name"`
	Actions []string `json:"actions"`
	// ActionLabels mapea las acciones de permiso PERSONALIZADAS (clave
	// "custom:<x>") a su etiqueta legible, para que la UI de roles rotule la
	// columna con el nombre real de la acción en vez de capitalizar la clave. Es
	// RETROCOMPATIBLE (omitempty): un cliente que no lo lea sigue usando Actions.
	// Las acciones fijas no aparecen aquí.
	ActionLabels map[string]string `json:"actionLabels,omitempty"`
}

// ModuleCatalog maneja GET /roles/module-catalog: devuelve el catálogo de
// módulos del constructor (slug, nombre y acciones declaradas) para que la UI de
// roles muestre los recursos dinámicos mod:<slug> además de los fijos. Si el
// constructor de módulos está desactivado (h.modules == nil) devuelve una lista
// vacía; si listar falla responde 500 genérico (el detalle solo va al log).
func (h *RolesHandler) ModuleCatalog(c *gin.Context) {
	if h.modules == nil {
		writeOK(c, http.StatusOK, gin.H{"items": []moduleCatalogItemDTO{}}, "")
		return
	}

	defs, err := h.modules.List(c.Request.Context())
	if err != nil {
		log.Printf("roles: error listando módulos para el catálogo: %v", err)
		writeError(c, http.StatusInternalServerError, "E500", "No se pudieron listar los módulos")
		return
	}

	items := make([]moduleCatalogItemDTO, 0, len(defs))
	for _, def := range defs {
		items = append(items, moduleCatalogItemDTO{
			Slug:         def.Slug,
			Name:         def.Name,
			Actions:      modules.PermActionsForDefinition(def),
			ActionLabels: modules.CustomActionLabels(def),
		})
	}
	writeOK(c, http.StatusOK, gin.H{"items": items}, "")
}
