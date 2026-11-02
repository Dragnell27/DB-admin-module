package roles

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"admin-backend/internal/auth"
)

// Repository define las operaciones de persistencia sobre roles, sus permisos
// (duplas recurso×acción) y la relación usuario-rol (Req. 5.1, 6.1, 6.2, 6.4,
// 7.1, 14.1).
type Repository interface {
	// List devuelve todos los roles con sus permisos materializados.
	List(ctx context.Context) ([]Role, error)
	// FindByID busca un rol por id, con sus permisos. Devuelve ErrNotFound si
	// no existe.
	FindByID(ctx context.Context, id string) (*Role, error)
	// FindByName busca un rol por nombre, con sus permisos. Devuelve ErrNotFound
	// si no existe.
	FindByName(ctx context.Context, name string) (*Role, error)
	// Create inserta un rol y su conjunto de permisos en una única transacción.
	Create(ctx context.Context, role *Role) error
	// Update actualiza el nombre del rol y REEMPLAZA TOTALMENTE su conjunto de
	// permisos en una única transacción.
	Update(ctx context.Context, role *Role) error
	// Delete elimina el rol (y por cascade sus role_permissions). Falla con
	// ErrRoleAssigned si el rol está asignado a algún usuario (Req. 6.4).
	Delete(ctx context.Context, id string) error
	// AssignmentCount devuelve cuántos usuarios tienen asignado el rol.
	AssignmentCount(ctx context.Context, roleID string) (int64, error)
	// RolesForUser devuelve los roles (id + nombre) asignados al usuario, vía
	// user_roles ⋈ roles. Útil para precargar el formulario de edición.
	RolesForUser(ctx context.Context, userID string) ([]RoleRef, error)
	// PermissionsForUser devuelve los permisos EFECTIVOS del usuario como el
	// producto cartesiano de (acciones recurso×acción de sus roles) × (países
	// del usuario, tabla user_countries), sin duplicados (Req. 7.1).
	PermissionsForUser(ctx context.Context, userID string) (auth.EffectivePermissions, error)
	// CountActiveAdmins devuelve el número de usuarios activos cuyos permisos
	// efectivos habilitan la administración (Req. 14.1).
	CountActiveAdmins(ctx context.Context) (int64, error)
}

// gormRepository es la implementación de Repository respaldada por GORM.
type gormRepository struct {
	db *gorm.DB
}

// NewRepository crea un Repository que opera sobre la conexión GORM dada.
func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

// List devuelve todos los roles con sus permisos. Realiza una consulta de roles
// y una de permisos, uniéndolas en memoria para evitar N+1.
func (r *gormRepository) List(ctx context.Context) ([]Role, error) {
	var rows []roleRow
	if err := r.db.WithContext(ctx).Order("name").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("roles: error listando roles: %w", err)
	}

	roles := make([]Role, 0, len(rows))
	index := make(map[string]int, len(rows))
	for i, row := range rows {
		roles = append(roles, toDomain(row))
		index[row.ID] = i
	}

	if len(rows) == 0 {
		return roles, nil
	}

	var perms []RolePermission
	if err := r.db.WithContext(ctx).Find(&perms).Error; err != nil {
		return nil, fmt.Errorf("roles: error listando permisos: %w", err)
	}
	for _, p := range perms {
		if i, ok := index[p.RoleID]; ok {
			roles[i].Permissions = append(roles[i].Permissions, toPermission(p))
		}
	}
	return roles, nil
}

// FindByID recupera un rol por id junto con sus permisos.
func (r *gormRepository) FindByID(ctx context.Context, id string) (*Role, error) {
	var row roleRow
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("roles: error buscando rol por id: %w", err)
	}
	return r.loadWithPermissions(ctx, row)
}

// FindByName recupera un rol por nombre junto con sus permisos.
func (r *gormRepository) FindByName(ctx context.Context, name string) (*Role, error) {
	var row roleRow
	err := r.db.WithContext(ctx).Where("name = ?", name).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("roles: error buscando rol por nombre: %w", err)
	}
	return r.loadWithPermissions(ctx, row)
}

// loadWithPermissions completa el modelo de dominio de row cargando sus duplas
// de permisos desde role_permissions.
func (r *gormRepository) loadWithPermissions(ctx context.Context, row roleRow) (*Role, error) {
	role := toDomain(row)
	var perms []RolePermission
	if err := r.db.WithContext(ctx).Where("role_id = ?", row.ID).Find(&perms).Error; err != nil {
		return nil, fmt.Errorf("roles: error cargando permisos del rol: %w", err)
	}
	for _, p := range perms {
		role.Permissions = append(role.Permissions, toPermission(p))
	}
	return &role, nil
}

// Create inserta el rol y sus permisos en una transacción. Nombre único y no vacío.
func (r *gormRepository) Create(ctx context.Context, role *Role) error {
	if role == nil {
		return fmt.Errorf("roles: el rol a crear no puede ser nil")
	}
	name := strings.TrimSpace(role.Name)
	if name == "" {
		return ErrEmptyName
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&roleRow{}).Where("name = ?", name).Count(&count).Error; err != nil {
			return fmt.Errorf("roles: error verificando nombre: %w", err)
		}
		if count > 0 {
			return ErrNameTaken
		}

		row := roleRow{ID: role.ID, Name: name}
		if err := tx.Create(&row).Error; err != nil {
			return fmt.Errorf("roles: error creando rol: %w", err)
		}

		if err := insertPermissions(tx, row.ID, role.Permissions); err != nil {
			return err
		}

		role.ID = row.ID
		role.Name = row.Name
		role.CreatedAt = row.CreatedAt
		role.UpdatedAt = row.UpdatedAt
		return nil
	})
}

// Update actualiza el nombre y reemplaza totalmente el conjunto de permisos.
func (r *gormRepository) Update(ctx context.Context, role *Role) error {
	if role == nil {
		return fmt.Errorf("roles: el rol a actualizar no puede ser nil")
	}
	if strings.TrimSpace(role.ID) == "" {
		return fmt.Errorf("roles: el id del rol a actualizar no puede estar vacío")
	}
	name := strings.TrimSpace(role.Name)
	if name == "" {
		return ErrEmptyName
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row roleRow
		if err := tx.Where("id = ?", role.ID).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return fmt.Errorf("roles: error buscando rol a actualizar: %w", err)
		}

		var count int64
		if err := tx.Model(&roleRow{}).
			Where("name = ? AND id <> ?", name, role.ID).
			Count(&count).Error; err != nil {
			return fmt.Errorf("roles: error verificando nombre: %w", err)
		}
		if count > 0 {
			return ErrNameTaken
		}

		if err := tx.Model(&roleRow{}).Where("id = ?", role.ID).
			Update("name", name).Error; err != nil {
			return fmt.Errorf("roles: error actualizando nombre del rol: %w", err)
		}

		if err := tx.Where("role_id = ?", role.ID).
			Delete(&RolePermission{}).Error; err != nil {
			return fmt.Errorf("roles: error limpiando permisos previos: %w", err)
		}
		if err := insertPermissions(tx, role.ID, role.Permissions); err != nil {
			return err
		}

		role.Name = name
		return nil
	})
}

// Delete elimina el rol salvo que esté asignado a algún usuario (ErrRoleAssigned).
func (r *gormRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row roleRow
		if err := tx.Where("id = ?", id).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return fmt.Errorf("roles: error buscando rol a eliminar: %w", err)
		}

		var assigned int64
		if err := tx.Model(&UserRole{}).Where("role_id = ?", id).
			Count(&assigned).Error; err != nil {
			return fmt.Errorf("roles: error verificando asignaciones: %w", err)
		}
		if assigned > 0 {
			return ErrRoleAssigned
		}

		if err := tx.Where("id = ?", id).Delete(&roleRow{}).Error; err != nil {
			return fmt.Errorf("roles: error eliminando rol: %w", err)
		}
		return nil
	})
}

// AssignmentCount devuelve cuántos usuarios tienen asignado el rol.
func (r *gormRepository) AssignmentCount(ctx context.Context, roleID string) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&UserRole{}).
		Where("role_id = ?", roleID).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("roles: error contando asignaciones del rol: %w", err)
	}
	return count, nil
}

// RolesForUser devuelve los roles (id + nombre) asignados al usuario.
func (r *gormRepository) RolesForUser(ctx context.Context, userID string) ([]RoleRef, error) {
	var refs []RoleRef
	err := r.db.WithContext(ctx).
		Table("roles AS r").
		Select("r.id, r.name").
		Joins("JOIN user_roles AS ur ON ur.role_id = r.id").
		Where("ur.user_id = ?", userID).
		Order("r.name").
		Scan(&refs).Error
	if err != nil {
		return nil, fmt.Errorf("roles: error resolviendo roles del usuario: %w", err)
	}
	if refs == nil {
		refs = []RoleRef{}
	}
	return refs, nil
}

// rolePair es una fila intermedia (recurso, acción) de los roles de un usuario.
type rolePair struct {
	Resource string
	Action   string
}

// PermissionsForUser calcula los permisos efectivos del usuario como el producto
// cartesiano de las duplas (recurso, acción) de sus roles por los países de su
// scope (user_countries). Sin países el conjunto es vacío (Req. 7.1).
func (r *gormRepository) PermissionsForUser(ctx context.Context, userID string) (auth.EffectivePermissions, error) {
	// Duplas (recurso, acción) de la unión de los roles del usuario.
	var pairs []rolePair
	err := r.db.WithContext(ctx).
		Table("role_permissions AS rp").
		Select("DISTINCT rp.resource, rp.action").
		Joins("JOIN user_roles AS ur ON ur.role_id = rp.role_id").
		Where("ur.user_id = ?", userID).
		Scan(&pairs).Error
	if err != nil {
		return auth.EffectivePermissions{}, fmt.Errorf("roles: error resolviendo permisos del usuario: %w", err)
	}

	// Países del scope del usuario.
	var countries []string
	if err := r.db.WithContext(ctx).
		Table("user_countries").
		Where("user_id = ?", userID).
		Pluck("country", &countries).Error; err != nil {
		return auth.EffectivePermissions{}, fmt.Errorf("roles: error resolviendo países del usuario: %w", err)
	}

	// Producto cartesiano: cada dupla por cada país.
	list := make([]auth.Permission, 0, len(pairs)*len(countries))
	for _, p := range pairs {
		for _, country := range countries {
			list = append(list, auth.Permission{
				Resource: auth.Resource(p.Resource),
				Action:   auth.Action(p.Action),
				Country:  country,
			})
		}
	}
	return auth.NewEffectivePermissions(list), nil
}

// CountActiveAdmins cuenta los usuarios activos cuyos permisos efectivos
// habilitan la administración (acción sobre `users` y sobre `roles`).
func (r *gormRepository) CountActiveAdmins(ctx context.Context) (int64, error) {
	var userIDs []string
	if err := r.db.WithContext(ctx).
		Table("users").
		Where("is_active = ?", true).
		Pluck("id", &userIDs).Error; err != nil {
		return 0, fmt.Errorf("roles: error listando usuarios activos: %w", err)
	}

	var admins int64
	for _, id := range userIDs {
		perms, err := r.PermissionsForUser(ctx, id)
		if err != nil {
			return 0, err
		}
		if isAdminPermissions(perms) {
			admins++
		}
	}
	return admins, nil
}

// isAdminPermissions indica si un conjunto de permisos efectivos habilita la
// administración: concede al menos una acción sobre `users` y sobre `roles`.
func isAdminPermissions(e auth.EffectivePermissions) bool {
	var hasUsers, hasRoles bool
	for _, p := range e.List() {
		switch p.Resource {
		case auth.ResourceUsers:
			hasUsers = true
		case auth.ResourceRoles:
			hasRoles = true
		}
		if hasUsers && hasRoles {
			return true
		}
	}
	return false
}

// insertPermissions persiste las duplas (recurso, acción) de un rol, sin país.
// Deduplica por dupla. No valida el catálogo (responsabilidad del handler).
func insertPermissions(tx *gorm.DB, roleID string, perms []auth.Permission) error {
	if len(perms) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(perms))
	rows := make([]RolePermission, 0, len(perms))
	for _, p := range perms {
		key := string(p.Resource) + "|" + string(p.Action)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		rows = append(rows, RolePermission{
			RoleID:   roleID,
			Resource: string(p.Resource),
			Action:   string(p.Action),
		})
	}
	if len(rows) == 0 {
		return nil
	}
	if err := tx.Create(&rows).Error; err != nil {
		return fmt.Errorf("roles: error insertando permisos del rol: %w", err)
	}
	return nil
}

// toDomain convierte un roleRow al modelo de dominio Role, sin permisos.
func toDomain(row roleRow) Role {
	return Role{
		ID:        row.ID,
		Name:      row.Name,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

// toPermission convierte una fila de role_permissions al tipo compartido
// auth.Permission (Country vacío: los permisos de rol son duplas).
func toPermission(p RolePermission) auth.Permission {
	return auth.Permission{
		Resource: auth.Resource(p.Resource),
		Action:   auth.Action(p.Action),
	}
}
