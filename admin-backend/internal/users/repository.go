package users

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// ErrNotFound se devuelve cuando no existe un usuario que satisfaga la consulta.
var ErrNotFound = errors.New("users: usuario no encontrado")

// userRole mapea la tabla pivote `user_roles` (user_id, role_id). Se define
// localmente para no importar el paquete roles y evitar un ciclo.
type userRole struct {
	UserID string `gorm:"column:user_id;type:uuid;primaryKey"`
	RoleID string `gorm:"column:role_id;type:uuid;primaryKey"`
}

// TableName fija el nombre de la tabla pivote para GORM.
func (userRole) TableName() string {
	return "user_roles"
}

// userCountry mapea la tabla `user_countries` (user_id, country): el scope de
// países del usuario. Rediseño 000003: el país vive en el usuario, no en el rol.
type userCountry struct {
	UserID  string `gorm:"column:user_id;type:uuid;primaryKey"`
	Country string `gorm:"column:country;type:varchar(8);primaryKey"`
}

// TableName fija el nombre de la tabla para GORM.
func (userCountry) TableName() string {
	return "user_countries"
}

// Repository define las operaciones de persistencia sobre usuarios.
type Repository interface {
	// FindByUsername busca un usuario por su nombre de login, activo o no.
	FindByUsername(ctx context.Context, username string) (*User, error)
	// FindActiveByUsername busca un usuario por username solo si está activo.
	FindActiveByUsername(ctx context.Context, username string) (*User, error)
	// FindByID recupera un usuario por su id. Devuelve ErrNotFound si no existe.
	FindByID(ctx context.Context, id string) (*User, error)
	// List devuelve todos los usuarios registrados.
	List(ctx context.Context) ([]User, error)
	// Create inserta un nuevo usuario.
	Create(ctx context.Context, user *User) error
	// SetRoles reemplaza por completo las asignaciones de roles del usuario.
	SetRoles(ctx context.Context, userID string, roleIDs []string) error
	// SetCountries reemplaza por completo el scope de países del usuario.
	SetCountries(ctx context.Context, userID string, countries []string) error
	// CountriesForUser devuelve los países del scope del usuario (minúsculas).
	CountriesForUser(ctx context.Context, userID string) ([]string, error)
	// UpdateRolesStatusAndCountries actualiza el estado de activación y reemplaza
	// roles y países del usuario, todo en una única transacción (Req. 4.2, 4.4).
	UpdateRolesStatusAndCountries(ctx context.Context, userID string, roleIDs []string, isActive bool, countries []string) error
	// UpdatePasswordHash actualiza únicamente el hash de contraseña del usuario.
	UpdatePasswordHash(ctx context.Context, userID string, passwordHash string) error
	// ExistsByUsername indica si ya existe un usuario con ese username.
	ExistsByUsername(ctx context.Context, username string) (bool, error)
	// Count devuelve el número total de usuarios registrados.
	Count(ctx context.Context) (int64, error)
}

// gormRepository es la implementación de Repository respaldada por GORM.
type gormRepository struct {
	db *gorm.DB
}

// NewRepository crea un Repository que opera sobre la conexión GORM dada.
func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

// FindByUsername recupera el usuario cuyo username coincide, activo o no.
func (r *gormRepository) FindByUsername(ctx context.Context, username string) (*User, error) {
	var user User
	err := r.db.WithContext(ctx).
		Where("username = ?", username).
		First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("users: error buscando por username: %w", err)
	}
	return &user, nil
}

// FindActiveByUsername recupera el usuario activo cuyo username coincide.
func (r *gormRepository) FindActiveByUsername(ctx context.Context, username string) (*User, error) {
	var user User
	err := r.db.WithContext(ctx).
		Where("username = ? AND is_active = ?", username, true).
		First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("users: error buscando usuario activo por username: %w", err)
	}
	return &user, nil
}

// FindByID recupera el usuario por su identificador.
func (r *gormRepository) FindByID(ctx context.Context, id string) (*User, error) {
	var user User
	err := r.db.WithContext(ctx).
		Where("id = ?", id).
		First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("users: error buscando por id: %w", err)
	}
	return &user, nil
}

// List devuelve todos los usuarios ordenados por fecha de creación.
func (r *gormRepository) List(ctx context.Context) ([]User, error) {
	var list []User
	if err := r.db.WithContext(ctx).Order("created_at ASC").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("users: error listando usuarios: %w", err)
	}
	return list, nil
}

// Create inserta el usuario.
func (r *gormRepository) Create(ctx context.Context, user *User) error {
	if user == nil {
		return fmt.Errorf("users: el usuario a crear no puede ser nil")
	}
	if strings.TrimSpace(user.Username) == "" {
		return fmt.Errorf("users: el username no puede estar vacío")
	}
	if user.Role != "" && !IsValidRole(user.Role) {
		return fmt.Errorf("users: rol inválido %q (debe ser %q o %q)", user.Role, RoleAdmin, RoleViewer)
	}
	if err := r.db.WithContext(ctx).Create(user).Error; err != nil {
		return fmt.Errorf("users: error creando usuario: %w", err)
	}
	return nil
}

// SetRoles reemplaza por completo las filas de user_roles del usuario.
func (r *gormRepository) SetRoles(ctx context.Context, userID string, roleIDs []string) error {
	if strings.TrimSpace(userID) == "" {
		return fmt.Errorf("users: el id de usuario no puede estar vacío")
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return replaceUserRoles(tx, userID, roleIDs)
	})
	if err != nil {
		return fmt.Errorf("users: error asignando roles: %w", err)
	}
	return nil
}

// SetCountries reemplaza por completo el scope de países del usuario.
func (r *gormRepository) SetCountries(ctx context.Context, userID string, countries []string) error {
	if strings.TrimSpace(userID) == "" {
		return fmt.Errorf("users: el id de usuario no puede estar vacío")
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return replaceUserCountries(tx, userID, countries)
	})
	if err != nil {
		return fmt.Errorf("users: error asignando países: %w", err)
	}
	return nil
}

// CountriesForUser devuelve los países del scope del usuario, ordenados.
func (r *gormRepository) CountriesForUser(ctx context.Context, userID string) ([]string, error) {
	var countries []string
	if err := r.db.WithContext(ctx).
		Table("user_countries").
		Where("user_id = ?", userID).
		Order("country").
		Pluck("country", &countries).Error; err != nil {
		return nil, fmt.Errorf("users: error listando países del usuario: %w", err)
	}
	if countries == nil {
		countries = []string{}
	}
	return countries, nil
}

// UpdateRolesStatusAndCountries actualiza is_active y reemplaza roles y países
// del usuario en una única transacción atómica.
func (r *gormRepository) UpdateRolesStatusAndCountries(ctx context.Context, userID string, roleIDs []string, isActive bool, countries []string) error {
	if strings.TrimSpace(userID) == "" {
		return fmt.Errorf("users: el id de usuario no puede estar vacío")
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&User{}).Where("id = ?", userID).Update("is_active", isActive)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		if err := replaceUserRoles(tx, userID, roleIDs); err != nil {
			return err
		}
		return replaceUserCountries(tx, userID, countries)
	})
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return err
		}
		return fmt.Errorf("users: error actualizando roles, estado y países: %w", err)
	}
	return nil
}

// UpdatePasswordHash actualiza únicamente el hash de contraseña del usuario.
func (r *gormRepository) UpdatePasswordHash(ctx context.Context, userID string, passwordHash string) error {
	if strings.TrimSpace(userID) == "" {
		return fmt.Errorf("users: el id de usuario no puede estar vacío")
	}
	if passwordHash == "" {
		return fmt.Errorf("users: el hash de contraseña no puede estar vacío")
	}
	res := r.db.WithContext(ctx).Model(&User{}).Where("id = ?", userID).Update("password_hash", passwordHash)
	if res.Error != nil {
		return fmt.Errorf("users: error actualizando contraseña: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ExistsByUsername indica si hay al menos un usuario con ese username.
func (r *gormRepository) ExistsByUsername(ctx context.Context, username string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&User{}).
		Where("username = ?", username).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("users: error verificando existencia por username: %w", err)
	}
	return count > 0, nil
}

// Count devuelve el total de usuarios registrados.
func (r *gormRepository) Count(ctx context.Context) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&User{}).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("users: error contando usuarios: %w", err)
	}
	return count, nil
}

// replaceUserRoles borra las asignaciones actuales e inserta las de roleIDs.
func replaceUserRoles(tx *gorm.DB, userID string, roleIDs []string) error {
	if err := tx.Where("user_id = ?", userID).Delete(&userRole{}).Error; err != nil {
		return err
	}
	rows := make([]userRole, 0, len(roleIDs))
	seen := make(map[string]struct{}, len(roleIDs))
	for _, id := range roleIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		rows = append(rows, userRole{UserID: userID, RoleID: id})
	}
	if len(rows) == 0 {
		return nil
	}
	return tx.Create(&rows).Error
}

// replaceUserCountries borra el scope actual e inserta los países dados,
// normalizados a minúsculas, sin vacíos ni duplicados.
func replaceUserCountries(tx *gorm.DB, userID string, countries []string) error {
	if err := tx.Where("user_id = ?", userID).Delete(&userCountry{}).Error; err != nil {
		return err
	}
	rows := make([]userCountry, 0, len(countries))
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
		rows = append(rows, userCountry{UserID: userID, Country: c})
	}
	if len(rows) == 0 {
		return nil
	}
	return tx.Create(&rows).Error
}
