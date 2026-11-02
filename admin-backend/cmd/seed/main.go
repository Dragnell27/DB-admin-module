// Command seed crea el usuario administrador inicial del panel de forma
// idempotente a partir de las variables de entorno SEED_ADMIN_USERNAME,
// SEED_ADMIN_PASSWORD y SEED_ADMIN_ROLE (Req. 22.2, 22.3).
//
// El seed puede ejecutarse de forma repetida sin efectos secundarios: si ya
// existe un usuario con el username indicado, no crea un duplicado ni devuelve
// error. La lógica central se factoriza en SeedAdmin para que pueda probarse
// con un repositorio en memoria sin depender de una base de datos real.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"admin-backend/internal/auth"
	"admin-backend/internal/config"
	"admin-backend/internal/db"
	"admin-backend/internal/roles"
	"admin-backend/internal/users"
)

// adminRoleName es el nombre canónico del rol de administración total que crea
// el seed. Se usa tanto para detectar su existencia (idempotencia) como para
// crearlo (Req. 12.2, 12.3).
const adminRoleName = "Administrador total"

// SeedRoleResult describe el desenlace del seed del rol de administración total,
// permitiendo al llamador (y a las pruebas) distinguir entre creación y omisión
// idempotente del rol.
type SeedRoleResult struct {
	// RoleCreated indica si se creó el rol "Administrador total". Es false
	// cuando el rol ya existía (caso idempotente, Req. 12.3).
	RoleCreated bool
	// RoleID es el id del rol de administración total (recién creado o existente).
	RoleID string
	// PermissionCount es el número de ternas (recurso × acción × país) que
	// contiene el conjunto generado para el rol.
	PermissionCount int
}

// SeedResult describe el desenlace de una ejecución del seed, permitiendo al
// llamador (y a las pruebas) distinguir entre creación y omisión idempotente.
type SeedResult struct {
	// Created indica si se creó un nuevo usuario administrador. Es false cuando
	// el usuario ya existía (caso idempotente, Req. 22.3).
	Created bool
	// Username es el nombre del usuario administrador objetivo del seed.
	Username string
}

// SeedAdmin crea el usuario administrador inicial de forma idempotente usando
// el repositorio dado. Si ya existe un usuario con ese username no hace nada y
// devuelve un SeedResult con Created=false (Req. 22.3). En caso contrario,
// hashea la contraseña con bcrypt (coste bcryptCost) y crea el usuario con el
// rol indicado (Req. 22.2).
//
// Se factoriza fuera de main para poder probar la idempotencia con un
// repositorio en memoria (ver tarea 10.2).
func SeedAdmin(ctx context.Context, repo users.Repository, username, password, role string, bcryptCost int) (SeedResult, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return SeedResult{}, errors.New("seed: SEED_ADMIN_USERNAME no puede estar vacío")
	}
	if password == "" {
		return SeedResult{}, errors.New("seed: SEED_ADMIN_PASSWORD no puede estar vacío")
	}
	if role == "" {
		role = users.RoleAdmin
	}
	if !users.IsValidRole(role) {
		return SeedResult{}, fmt.Errorf("seed: rol inválido %q (debe ser %q o %q)", role, users.RoleAdmin, users.RoleViewer)
	}

	// Idempotencia: si el usuario ya existe, completar sin crear un duplicado.
	exists, err := repo.ExistsByUsername(ctx, username)
	if err != nil {
		return SeedResult{}, fmt.Errorf("seed: no se pudo verificar la existencia del usuario %q: %w", username, err)
	}
	if exists {
		return SeedResult{Created: false, Username: username}, nil
	}

	hash, err := auth.HashPassword(password, bcryptCost)
	if err != nil {
		return SeedResult{}, fmt.Errorf("seed: no se pudo hashear la contraseña del administrador: %w", err)
	}

	user := &users.User{
		Username:     username,
		PasswordHash: hash,
		Role:         role,
	}
	if err := repo.Create(ctx, user); err != nil {
		return SeedResult{}, fmt.Errorf("seed: no se pudo crear el usuario administrador %q: %w", username, err)
	}

	return SeedResult{Created: true, Username: username}, nil
}

// buildAdminPermissions genera, de forma determinista, TODAS las ternas de
// permiso del rol de administración total: para cada país configurado, para
// cada recurso del catálogo (auth.AllResources) y para cada acción válida de
// ese recurso (auth.ActionsForResource), una terna Recurso × Acción × País
// (Req. 12.2, 2.2). Al basarse en el catálogo Go y en configuredCountries,
// cualquier país configurado queda cubierto sin cambios de código.
func buildAdminPermissions() []auth.Permission {
	resources := auth.AllResources()
	perms := make([]auth.Permission, 0, len(resources)*4)
	for _, resource := range resources {
		for _, action := range auth.ActionsForResource(resource) {
			perms = append(perms, auth.Permission{
				Resource: resource,
				Action:   action,
			})
		}
	}
	return perms
}

// SeedAdminRole crea de forma idempotente el rol "Administrador total" con
// todas las ternas (recurso × acción válida × país configurado) y lo asigna al
// usuario administrador sembrado (Req. 12.2, 12.3).
//
// Idempotencia: si ya existe un rol con el nombre "Administrador total" no se
// recrea; se reutiliza su id. Solo se crea cuando no existe. En ambos casos el
// rol se asigna al usuario admin mediante usersRepo.SetRoles, que reemplaza por
// completo las asignaciones del usuario: para el seed inicial esto garantiza
// que el admin acabe con exactamente el rol total y es seguro de repetir.
//
// Se factoriza fuera de main para poder probarla con repositorios en memoria
// (ver tarea 10.2 opcional).
func SeedAdminRole(ctx context.Context, rolesRepo roles.Repository, usersRepo users.Repository, adminUsername string, configuredCountries []string) (SeedRoleResult, error) {
	adminUsername = strings.TrimSpace(adminUsername)
	if adminUsername == "" {
		return SeedRoleResult{}, errors.New("seed: el username del administrador no puede estar vacío")
	}

	perms := buildAdminPermissions()

	// Localiza el rol existente o créalo con todas las ternas (idempotencia).
	var roleID string
	roleCreated := false
	existing, err := rolesRepo.FindByName(ctx, adminRoleName)
	switch {
	case err == nil:
		// El rol ya existe: se reutiliza su id, pero se RECONCILIAN sus permisos
		// al conjunto completo generado por buildAdminPermissions(). Así el seed
		// es idempotente también frente a cambios del catálogo: cuando se añade
		// un recurso a auth.AllResources() (p. ej. "module-builder") y se vuelve
		// a ejecutar el seed, el rol existente adquiere el permiso nuevo sin
		// necesidad de recrearlo (Req. 12.2, 12.3).
		roleID = existing.ID
		role := &roles.Role{ID: existing.ID, Name: adminRoleName, Permissions: perms}
		if uerr := rolesRepo.Update(ctx, role); uerr != nil {
			return SeedRoleResult{}, fmt.Errorf("seed: no se pudo reconciliar los permisos del rol %q: %w", adminRoleName, uerr)
		}
	case errors.Is(err, roles.ErrNotFound):
		// No existe: se crea con el conjunto completo de permisos (Req. 12.2).
		role := &roles.Role{Name: adminRoleName, Permissions: perms}
		if cerr := rolesRepo.Create(ctx, role); cerr != nil {
			return SeedRoleResult{}, fmt.Errorf("seed: no se pudo crear el rol %q: %w", adminRoleName, cerr)
		}
		roleID = role.ID
		roleCreated = true
	default:
		return SeedRoleResult{}, fmt.Errorf("seed: no se pudo verificar la existencia del rol %q: %w", adminRoleName, err)
	}

	// Localiza el usuario admin y asígnale el rol total.
	adminUser, err := usersRepo.FindByUsername(ctx, adminUsername)
	if err != nil {
		return SeedRoleResult{}, fmt.Errorf("seed: no se pudo localizar el usuario administrador %q: %w", adminUsername, err)
	}
	if err := usersRepo.SetRoles(ctx, adminUser.ID, []string{roleID}); err != nil {
		return SeedRoleResult{}, fmt.Errorf("seed: no se pudo asignar el rol %q al usuario %q: %w", adminRoleName, adminUsername, err)
	}

	// El admin opera en TODOS los países configurados (scope de usuario).
	if err := usersRepo.SetCountries(ctx, adminUser.ID, configuredCountries); err != nil {
		return SeedRoleResult{}, fmt.Errorf("seed: no se pudieron asignar los países al usuario %q: %w", adminUsername, err)
	}

	return SeedRoleResult{
		RoleCreated:     roleCreated,
		RoleID:          roleID,
		PermissionCount: len(perms),
	}, nil
}

// main mantiene una responsabilidad mínima: cargar la configuración, abrir la
// conexión a la base de datos, delegar en SeedAdmin y registrar el resultado.
func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("seed: error cargando la configuración: %v", err)
	}

	gormDB, err := db.Open(cfg)
	if err != nil {
		log.Fatalf("seed: error abriendo la conexión a la base de datos: %v", err)
	}
	defer func() {
		if cerr := db.Close(gormDB); cerr != nil {
			log.Printf("seed: advertencia al cerrar la conexión: %v", cerr)
		}
	}()

	repo := users.NewRepository(gormDB)

	result, err := SeedAdmin(
		context.Background(),
		repo,
		cfg.SeedAdminUsername,
		cfg.SeedAdminPassword,
		cfg.SeedAdminRole,
		cfg.BcryptCost,
	)
	if err != nil {
		log.Fatalf("seed: %v", err)
	}

	if result.Created {
		log.Printf("seed: usuario administrador %q creado correctamente", result.Username)
	} else {
		log.Printf("seed: el usuario administrador %q ya existía; no se realizaron cambios", result.Username)
	}

	// Tras garantizar el usuario admin, sembrar (idempotente) el rol
	// "Administrador total" con todas las ternas de los países configurados y
	// asignárselo (Req. 12.2, 12.3).
	rolesRepo := roles.NewRepository(gormDB)
	roleResult, err := SeedAdminRole(
		context.Background(),
		rolesRepo,
		repo,
		result.Username,
		cfg.ConfiguredCountries(),
	)
	if err != nil {
		log.Fatalf("seed: %v", err)
	}

	if roleResult.RoleCreated {
		log.Printf("seed: rol %q creado con %d permisos y asignado al usuario %q",
			adminRoleName, roleResult.PermissionCount, result.Username)
	} else {
		log.Printf("seed: el rol %q ya existía; reconciliado (%d permisos) y reasignado al usuario %q",
			adminRoleName, roleResult.PermissionCount, result.Username)
	}
}
