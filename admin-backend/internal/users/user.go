// Package users define el modelo de dominio del administrador del panel y el
// repositorio de acceso a datos sobre la tabla `users` (PostgreSQL / GORM).
//
// El modelo refleja el esquema creado por la migración
// 000001_create_users (id UUID PK con gen_random_uuid(), username único,
// password_hash, role con CHECK IN ('admin','viewer'), created_at/updated_at
// TIMESTAMPTZ). El repositorio expone las operaciones que necesitan el flujo de
// autenticación de sesión (login) y el seed del administrador inicial
// (Req. 1.6, 22.2, 22.3).
package users

import "time"

// Roles soportados para un usuario del panel (Req. 5). Coinciden con el
// CHECK de la columna `role` en la migración.
const (
	RoleAdmin  = "admin"
	RoleViewer = "viewer"
)

// User es el modelo GORM que mapea la tabla `users` del admin-backend.
//
// El `id` lo genera el servidor de base de datos mediante gen_random_uuid()
// (ver migración), por lo que no se asigna desde la aplicación salvo que se
// desee un valor explícito. `PasswordHash` almacena únicamente el hash bcrypt;
// nunca la contraseña en texto plano (ver Security en design.md).
//
// Evolución RBAC (migración 000002): se añade `IsActive` (is_active) para poder
// desactivar usuarios sin borrarlos (Req. 4.2, 4.3) y el `Role` legacy se
// relaja a nullable —ya no es obligatorio— porque los usuarios nuevos del RBAC
// obtienen sus permisos vía la tabla `user_roles`, no por esta columna
// (Req. 15.2, 15.4).
type User struct {
	ID           string `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Username     string `gorm:"column:username;type:varchar(100);uniqueIndex:idx_users_username;not null" json:"username"`
	PasswordHash string `gorm:"column:password_hash;type:varchar(255);not null" json:"-"`
	// Role es el rol legacy (admin/viewer). Tras la migración 000002 es
	// nullable: se conserva solo por convivencia y puede quedar vacío en los
	// usuarios creados por el flujo RBAC.
	Role string `gorm:"column:role;type:varchar(20)" json:"role,omitempty"`
	// IsActive indica si el usuario puede autenticarse. Por defecto true en la
	// base de datos; un usuario inactivo no debe poder iniciar sesión (Req. 4.3).
	IsActive  bool      `gorm:"column:is_active;not null;default:true" json:"is_active"`
	CreatedAt time.Time `gorm:"column:created_at;type:timestamptz;autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;type:timestamptz;autoUpdateTime" json:"updated_at"`
}

// TableName fija el nombre de la tabla para GORM, evitando depender de la
// pluralización automática.
func (User) TableName() string {
	return "users"
}

// IsValidRole indica si r es uno de los roles legacy permitidos del panel.
//
// Nota (RBAC): ya NO se usa para bloquear la creación de usuarios —los usuarios
// del flujo RBAC se crean sin role legacy— sino solo como utilidad de
// validación opcional cuando el role sí viene informado.
func IsValidRole(r string) bool {
	return r == RoleAdmin || r == RoleViewer
}
