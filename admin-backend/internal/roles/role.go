// Package roles define el modelo de dominio de un rol RBAC y el repositorio de
// acceso a datos sobre las tablas `roles`, `role_permissions` y `user_roles`
// (PostgreSQL / GORM).
//
// Modelo (rediseño 000003): un rol agrupa permisos expresados como la dupla
// Recurso × Acción (ver auth.Permission con Country vacío). El país YA NO vive
// en el rol: el scope de países es del usuario (tabla `user_countries`). Los
// permisos efectivos de un usuario se calculan como el producto cartesiano
// (acciones de sus roles × sus países) en PermissionsForUser (Req. 7.1).
//
// Este paquete solo persiste: NO valida el catálogo recurso/acción. Esa
// validación la realiza el handler con auth.ValidateRolePermission antes de
// llamar al repositorio.
package roles

import (
	"errors"
	"time"

	"admin-backend/internal/auth"
)

// Errores de negocio exportados del repositorio.
var (
	// ErrNotFound se devuelve cuando no existe un rol que satisfaga la consulta.
	ErrNotFound = errors.New("roles: rol no encontrado")
	// ErrNameTaken se devuelve cuando se intenta crear o renombrar un rol con un
	// nombre que ya usa otro rol (nombre único, Req. 5.3, 6.5).
	ErrNameTaken = errors.New("roles: el nombre de rol ya está en uso")
	// ErrRoleAssigned se devuelve al intentar eliminar un rol que todavía está
	// asignado a uno o más usuarios (Req. 6.4).
	ErrRoleAssigned = errors.New("roles: el rol está asignado a usuarios y no puede eliminarse")
	// ErrEmptyName se devuelve cuando el nombre del rol está vacío.
	ErrEmptyName = errors.New("roles: el nombre del rol no puede estar vacío")
)

// Role es el modelo de dominio (representación pública) de un rol. Los permisos
// se materializan desde `role_permissions` y se exponen como el tipo compartido
// auth.Permission con el campo Country vacío (los permisos de rol son duplas
// Recurso × Acción); no son una columna de la tabla `roles`.
type Role struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	Permissions []auth.Permission `json:"permissions"`
}

// RoleRef es la forma mínima de un rol (id + nombre) para resolver los roles
// asignados a un usuario sin arrastrar sus permisos.
type RoleRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// roleRow es el modelo GORM que mapea la tabla `roles`. El id lo genera la base
// de datos con gen_random_uuid() si no se asigna explícitamente.
type roleRow struct {
	ID        string    `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	Name      string    `gorm:"column:name;type:varchar(100);uniqueIndex;not null"`
	CreatedAt time.Time `gorm:"column:created_at;type:timestamptz;autoCreateTime"`
	UpdatedAt time.Time `gorm:"column:updated_at;type:timestamptz;autoUpdateTime"`
}

// TableName fija el nombre de la tabla para GORM.
func (roleRow) TableName() string {
	return "roles"
}

// RolePermission es el modelo GORM que mapea la tabla `role_permissions`. Cada
// fila es una dupla (resource, action) perteneciente a un rol. La unicidad de
// la dupla por rol la garantiza una restricción de la base de datos.
type RolePermission struct {
	ID       string `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	RoleID   string `gorm:"column:role_id;type:uuid;not null"`
	Resource string `gorm:"column:resource;type:varchar(32);not null"`
	Action   string `gorm:"column:action;type:varchar(32);not null"`
}

// TableName fija el nombre de la tabla para GORM.
func (RolePermission) TableName() string {
	return "role_permissions"
}

// UserRole es el modelo GORM que mapea la tabla de unión `user_roles`
// (relación muchos-a-muchos con PK compuesta user_id + role_id).
type UserRole struct {
	UserID string `gorm:"column:user_id;type:uuid;primaryKey"`
	RoleID string `gorm:"column:role_id;type:uuid;primaryKey"`
}

// TableName fija el nombre de la tabla para GORM.
func (UserRole) TableName() string {
	return "user_roles"
}
