// Pruebas unitarias de la lógica del seed del administrador inicial.
//
// Cubren la tarea 10.2: verificar que SeedAdmin es idempotente (ejecutarlo
// varias veces no crea duplicados) además de los casos de borde relevantes de
// la implementación (username/contraseña vacíos, rol inválido y rol vacío que
// adopta "admin" por defecto). Se usa un repositorio en memoria que implementa
// users.Repository para evitar dependencias de base de datos.
//
// _Requirements: 22.3_
package main

import (
	"context"
	"errors"
	"testing"

	"admin-backend/internal/auth"
	"admin-backend/internal/roles"
	"admin-backend/internal/users"
)

// seedTestBcryptCost usa el coste bcrypt mínimo para mantener las pruebas
// rápidas sin dejar de ejercitar el hashing real (nunca texto plano).
const seedTestBcryptCost = 4

// seedTestRepo es un repositorio de usuarios en memoria que implementa
// users.Repository usando un mapa indexado por username. Permite probar la
// idempotencia del seed sin una base de datos real.
type seedTestRepo struct {
	byUsername map[string]*users.User
	// createCalls cuenta cuántas veces se invocó Create, para distinguir una
	// omisión idempotente de una creación efectiva.
	createCalls int
}

// seedTestNewRepo construye un repositorio en memoria vacío.
func seedTestNewRepo() *seedTestRepo {
	return &seedTestRepo{byUsername: make(map[string]*users.User)}
}

func (r *seedTestRepo) FindByUsername(_ context.Context, username string) (*users.User, error) {
	u, ok := r.byUsername[username]
	if !ok {
		return nil, users.ErrNotFound
	}
	return u, nil
}

func (r *seedTestRepo) Create(_ context.Context, user *users.User) error {
	if user == nil {
		return errors.New("seedTestRepo: usuario nil")
	}
	if _, exists := r.byUsername[user.Username]; exists {
		// Reproduce la restricción de unicidad: no debería alcanzarse si el
		// seed es idempotente.
		return errors.New("seedTestRepo: username duplicado")
	}
	r.createCalls++
	// Copia defensiva para no exponer el puntero del llamador.
	stored := *user
	if stored.ID == "" {
		stored.ID = "seed-test-id"
	}
	r.byUsername[stored.Username] = &stored
	return nil
}

func (r *seedTestRepo) ExistsByUsername(_ context.Context, username string) (bool, error) {
	_, ok := r.byUsername[username]
	return ok, nil
}

func (r *seedTestRepo) Count(_ context.Context) (int64, error) {
	return int64(len(r.byUsername)), nil
}

// seedTestCount es un helper que recupera el conteo del repo o falla la prueba.
func seedTestCount(t *testing.T, repo *seedTestRepo) int64 {
	t.Helper()
	n, err := repo.Count(context.Background())
	if err != nil {
		t.Fatalf("Count devolvió error inesperado: %v", err)
	}
	return n
}

// TestSeedAdmin_PrimeraEjecucionCreaElAdmin verifica que la primera ejecución
// crea el administrador, que queda registrado con el username y rol dados, y
// que el hash almacenado verifica contra la contraseña del seed sin almacenar
// texto plano.
func TestSeedAdmin_PrimeraEjecucionCreaElAdmin(t *testing.T) {
	ctx := context.Background()
	repo := seedTestNewRepo()

	const (
		username = "admin"
		password = "S3gura-y-larga!"
		role     = users.RoleAdmin
	)

	res, err := SeedAdmin(ctx, repo, username, password, role, seedTestBcryptCost)
	if err != nil {
		t.Fatalf("SeedAdmin devolvió error inesperado: %v", err)
	}
	if !res.Created {
		t.Fatalf("se esperaba Created=true en la primera ejecución, got false")
	}
	if res.Username != username {
		t.Fatalf("Username = %q, se esperaba %q", res.Username, username)
	}

	if got := seedTestCount(t, repo); got != 1 {
		t.Fatalf("Count = %d tras la primera ejecución, se esperaba 1", got)
	}

	stored, err := repo.FindByUsername(ctx, username)
	if err != nil {
		t.Fatalf("no se encontró el usuario creado: %v", err)
	}
	if stored.Username != username {
		t.Fatalf("usuario almacenado con username %q, se esperaba %q", stored.Username, username)
	}
	if stored.Role != role {
		t.Fatalf("usuario almacenado con rol %q, se esperaba %q", stored.Role, role)
	}

	// El hash nunca debe ser la contraseña en texto plano y debe verificar.
	if stored.PasswordHash == "" {
		t.Fatalf("PasswordHash está vacío; se esperaba un hash bcrypt")
	}
	if stored.PasswordHash == password {
		t.Fatalf("PasswordHash contiene la contraseña en texto plano; debe almacenarse hasheada")
	}
	if err := auth.CheckPassword(stored.PasswordHash, password); err != nil {
		t.Fatalf("el hash almacenado no verifica contra la contraseña del seed: %v", err)
	}
}

// TestSeedAdmin_SegundaEjecucionEsIdempotente verifica que una segunda
// ejecución con el mismo username no crea un duplicado: devuelve Created=false,
// sin error, y el conteo permanece en 1 (Req. 22.3).
func TestSeedAdmin_SegundaEjecucionEsIdempotente(t *testing.T) {
	ctx := context.Background()
	repo := seedTestNewRepo()

	const (
		username = "admin"
		password = "S3gura-y-larga!"
		role     = users.RoleAdmin
	)

	first, err := SeedAdmin(ctx, repo, username, password, role, seedTestBcryptCost)
	if err != nil {
		t.Fatalf("primera ejecución devolvió error inesperado: %v", err)
	}
	if !first.Created {
		t.Fatalf("primera ejecución: se esperaba Created=true")
	}

	second, err := SeedAdmin(ctx, repo, username, password, role, seedTestBcryptCost)
	if err != nil {
		t.Fatalf("segunda ejecución devolvió error inesperado: %v", err)
	}
	if second.Created {
		t.Fatalf("segunda ejecución: se esperaba Created=false (idempotente)")
	}
	if second.Username != username {
		t.Fatalf("segunda ejecución: Username = %q, se esperaba %q", second.Username, username)
	}

	if got := seedTestCount(t, repo); got != 1 {
		t.Fatalf("Count = %d tras la segunda ejecución, se esperaba 1 (sin duplicados)", got)
	}
	if repo.createCalls != 1 {
		t.Fatalf("Create se invocó %d veces, se esperaba 1 (solo en la creación inicial)", repo.createCalls)
	}
}

// TestSeedAdmin_EjecucionesRepetidasSonSeguras verifica que ejecutar el seed
// repetidamente (varias veces) sigue siendo seguro: nunca duplica ni falla.
func TestSeedAdmin_EjecucionesRepetidasSonSeguras(t *testing.T) {
	ctx := context.Background()
	repo := seedTestNewRepo()

	const (
		username = "admin"
		password = "otra-clave-segura-123"
	)

	for i := 0; i < 5; i++ {
		res, err := SeedAdmin(ctx, repo, username, password, users.RoleAdmin, seedTestBcryptCost)
		if err != nil {
			t.Fatalf("ejecución %d devolvió error inesperado: %v", i+1, err)
		}
		wantCreated := i == 0
		if res.Created != wantCreated {
			t.Fatalf("ejecución %d: Created = %v, se esperaba %v", i+1, res.Created, wantCreated)
		}
	}

	if got := seedTestCount(t, repo); got != 1 {
		t.Fatalf("Count = %d tras 5 ejecuciones, se esperaba 1", got)
	}
	if repo.createCalls != 1 {
		t.Fatalf("Create se invocó %d veces tras 5 ejecuciones, se esperaba 1", repo.createCalls)
	}
}

// TestSeedAdmin_RolVacioAdoptaAdminPorDefecto verifica que un rol vacío hace
// que el usuario se cree con rol "admin" por defecto.
func TestSeedAdmin_RolVacioAdoptaAdminPorDefecto(t *testing.T) {
	ctx := context.Background()
	repo := seedTestNewRepo()

	res, err := SeedAdmin(ctx, repo, "admin", "clave-segura-xyz", "", seedTestBcryptCost)
	if err != nil {
		t.Fatalf("SeedAdmin devolvió error inesperado: %v", err)
	}
	if !res.Created {
		t.Fatalf("se esperaba Created=true")
	}

	stored, err := repo.FindByUsername(ctx, "admin")
	if err != nil {
		t.Fatalf("no se encontró el usuario creado: %v", err)
	}
	if stored.Role != users.RoleAdmin {
		t.Fatalf("rol = %q con rol vacío de entrada, se esperaba %q por defecto", stored.Role, users.RoleAdmin)
	}
}

// TestSeedAdmin_EntradasInvalidas verifica los casos de borde que deben
// devolver error sin crear ningún usuario: username vacío (incluyendo solo
// espacios), contraseña vacía y rol inválido.
func TestSeedAdmin_EntradasInvalidas(t *testing.T) {
	cases := []struct {
		name     string
		username string
		password string
		role     string
	}{
		{name: "username vacío", username: "", password: "clave-segura", role: users.RoleAdmin},
		{name: "username solo espacios", username: "   ", password: "clave-segura", role: users.RoleAdmin},
		{name: "contraseña vacía", username: "admin", password: "", role: users.RoleAdmin},
		{name: "rol inválido", username: "admin", password: "clave-segura", role: "superadmin"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			repo := seedTestNewRepo()

			res, err := SeedAdmin(ctx, repo, tc.username, tc.password, tc.role, seedTestBcryptCost)
			if err == nil {
				t.Fatalf("se esperaba un error para %s, got nil (res=%+v)", tc.name, res)
			}
			if res.Created {
				t.Fatalf("no debió crearse ningún usuario para %s", tc.name)
			}
			if got := seedTestCount(t, repo); got != 0 {
				t.Fatalf("Count = %d para %s, se esperaba 0 (ninguna creación)", got, tc.name)
			}
		})
	}
}

// Métodos adicionales para satisfacer la interfaz users.Repository, ampliada en
// la tarea 9.1. El seed solo usa FindByUsername/Create/ExistsByUsername, así que
// estos métodos tienen una implementación mínima en memoria suficiente para que
// *seedTestRepo siga siendo un users.Repository válido.

func (r *seedTestRepo) FindActiveByUsername(ctx context.Context, username string) (*users.User, error) {
	return r.FindByUsername(ctx, username)
}

func (r *seedTestRepo) FindByID(_ context.Context, id string) (*users.User, error) {
	for _, u := range r.byUsername {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, users.ErrNotFound
}

func (r *seedTestRepo) List(context.Context) ([]users.User, error) { return nil, nil }

func (r *seedTestRepo) SetRoles(context.Context, string, []string) error { return nil }

func (r *seedTestRepo) SetCountries(context.Context, string, []string) error { return nil }

func (r *seedTestRepo) CountriesForUser(context.Context, string) ([]string, error) { return nil, nil }

func (r *seedTestRepo) UpdateRolesStatusAndCountries(context.Context, string, []string, bool, []string) error {
	return nil
}

func (r *seedTestRepo) UpdatePasswordHash(context.Context, string, string) error { return nil }

var _ users.Repository = (*seedTestRepo)(nil)

// seedTestRolesRepo es un repositorio de roles en memoria que implementa
// roles.Repository con un mapa indexado por id. Solo materializa el
// comportamiento que ejercita SeedAdminRole (FindByName/Create/Update); el
// resto de la interfaz tiene una implementación mínima suficiente para que
// *seedTestRolesRepo siga siendo un roles.Repository válido.
type seedTestRolesRepo struct {
	byID map[string]*roles.Role
	// updateCalls cuenta cuántas veces se invocó Update, para distinguir la
	// reconciliación de un rol existente de su creación.
	updateCalls int
}

// seedTestNewRolesRepo construye un repositorio de roles en memoria vacío.
func seedTestNewRolesRepo() *seedTestRolesRepo {
	return &seedTestRolesRepo{byID: make(map[string]*roles.Role)}
}

func (r *seedTestRolesRepo) FindByName(_ context.Context, name string) (*roles.Role, error) {
	for _, role := range r.byID {
		if role.Name == name {
			stored := *role
			return &stored, nil
		}
	}
	return nil, roles.ErrNotFound
}

func (r *seedTestRolesRepo) FindByID(_ context.Context, id string) (*roles.Role, error) {
	role, ok := r.byID[id]
	if !ok {
		return nil, roles.ErrNotFound
	}
	stored := *role
	return &stored, nil
}

func (r *seedTestRolesRepo) Create(_ context.Context, role *roles.Role) error {
	if role == nil {
		return errors.New("seedTestRolesRepo: rol nil")
	}
	if role.ID == "" {
		role.ID = "seed-test-role-id"
	}
	stored := *role
	r.byID[stored.ID] = &stored
	return nil
}

func (r *seedTestRolesRepo) Update(_ context.Context, role *roles.Role) error {
	if role == nil {
		return errors.New("seedTestRolesRepo: rol nil")
	}
	if _, ok := r.byID[role.ID]; !ok {
		return roles.ErrNotFound
	}
	r.updateCalls++
	stored := *role
	r.byID[stored.ID] = &stored
	return nil
}

func (r *seedTestRolesRepo) List(context.Context) ([]roles.Role, error) { return nil, nil }

func (r *seedTestRolesRepo) Delete(context.Context, string) error { return nil }

func (r *seedTestRolesRepo) AssignmentCount(context.Context, string) (int64, error) {
	return 0, nil
}

func (r *seedTestRolesRepo) RolesForUser(context.Context, string) ([]roles.RoleRef, error) {
	return nil, nil
}

func (r *seedTestRolesRepo) PermissionsForUser(context.Context, string) (auth.EffectivePermissions, error) {
	return auth.EffectivePermissions{}, nil
}

func (r *seedTestRolesRepo) CountActiveAdmins(context.Context) (int64, error) { return 0, nil }

var _ roles.Repository = (*seedTestRolesRepo)(nil)

// hasModuleBuilderPermission indica si el conjunto de permisos contiene al menos
// una terna cuyo recurso es auth.ResourceModuleBuilder.
func hasModuleBuilderPermission(perms []auth.Permission) bool {
	for _, p := range perms {
		if p.Resource == auth.ResourceModuleBuilder {
			return true
		}
	}
	return false
}

// TestSeedAdminRole_ReconciliaRolExistenteConCatalogoCompleto verifica que
// cuando el rol "Administrador total" YA existe con un conjunto de permisos
// REDUCIDO (sin "module-builder"), ejecutar SeedAdminRole reconcilia sus
// permisos al conjunto completo de buildAdminPermissions(), que incluye el
// recurso auth.ResourceModuleBuilder. Esto cubre el caso de haber añadido un
// recurso al catálogo (auth.AllResources) después del primer seed del rol.
func TestSeedAdminRole_ReconciliaRolExistenteConCatalogoCompleto(t *testing.T) {
	ctx := context.Background()

	// Prepara un usuario admin existente (requisito de SeedAdminRole).
	usersRepo := seedTestNewRepo()
	const adminUsername = "admin"
	if err := usersRepo.Create(ctx, &users.User{Username: adminUsername, PasswordHash: "x", Role: users.RoleAdmin}); err != nil {
		t.Fatalf("no se pudo preparar el usuario admin: %v", err)
	}

	// Rol preexistente con permisos reducidos: deliberadamente SIN module-builder.
	rolesRepo := seedTestNewRolesRepo()
	reduced := []auth.Permission{
		{Resource: auth.ResourceUsers, Action: auth.ActionVer},
		{Resource: auth.ResourceRoles, Action: auth.ActionVer},
	}
	if hasModuleBuilderPermission(reduced) {
		t.Fatalf("el conjunto reducido no debía contener module-builder")
	}
	existing := &roles.Role{ID: "rol-existente", Name: adminRoleName, Permissions: reduced}
	if err := rolesRepo.Create(ctx, existing); err != nil {
		t.Fatalf("no se pudo preparar el rol existente: %v", err)
	}

	res, err := SeedAdminRole(ctx, rolesRepo, usersRepo, adminUsername, []string{"GT"})
	if err != nil {
		t.Fatalf("SeedAdminRole devolvió error inesperado: %v", err)
	}
	if res.RoleCreated {
		t.Fatalf("se esperaba RoleCreated=false (el rol ya existía)")
	}
	if res.RoleID != existing.ID {
		t.Fatalf("RoleID = %q, se esperaba reutilizar %q", res.RoleID, existing.ID)
	}
	if rolesRepo.updateCalls != 1 {
		t.Fatalf("Update se invocó %d veces, se esperaba 1 (reconciliación)", rolesRepo.updateCalls)
	}

	// El rol reconciliado debe tener exactamente el conjunto completo generado
	// e INCLUIR el permiso de module-builder.
	stored, err := rolesRepo.FindByID(ctx, existing.ID)
	if err != nil {
		t.Fatalf("no se encontró el rol reconciliado: %v", err)
	}
	full := buildAdminPermissions()
	if len(stored.Permissions) != len(full) {
		t.Fatalf("el rol tiene %d permisos tras reconciliar, se esperaban %d (catálogo completo)",
			len(stored.Permissions), len(full))
	}
	if res.PermissionCount != len(full) {
		t.Fatalf("PermissionCount = %d, se esperaba %d", res.PermissionCount, len(full))
	}
	if !hasModuleBuilderPermission(stored.Permissions) {
		t.Fatalf("tras reconciliar, el rol debe incluir un permiso con Resource == %q", auth.ResourceModuleBuilder)
	}
}

// TestSeedAdminRole_CreaRolNuevoConCatalogoCompleto verifica que cuando el rol
// NO existe, SeedAdminRole lo crea con el conjunto completo (incluido
// module-builder) sin invocar Update.
func TestSeedAdminRole_CreaRolNuevoConCatalogoCompleto(t *testing.T) {
	ctx := context.Background()

	usersRepo := seedTestNewRepo()
	const adminUsername = "admin"
	if err := usersRepo.Create(ctx, &users.User{Username: adminUsername, PasswordHash: "x", Role: users.RoleAdmin}); err != nil {
		t.Fatalf("no se pudo preparar el usuario admin: %v", err)
	}

	rolesRepo := seedTestNewRolesRepo()

	res, err := SeedAdminRole(ctx, rolesRepo, usersRepo, adminUsername, []string{"GT"})
	if err != nil {
		t.Fatalf("SeedAdminRole devolvió error inesperado: %v", err)
	}
	if !res.RoleCreated {
		t.Fatalf("se esperaba RoleCreated=true (el rol no existía)")
	}
	if rolesRepo.updateCalls != 0 {
		t.Fatalf("Update se invocó %d veces en la creación, se esperaba 0", rolesRepo.updateCalls)
	}

	stored, err := rolesRepo.FindByID(ctx, res.RoleID)
	if err != nil {
		t.Fatalf("no se encontró el rol creado: %v", err)
	}
	if !hasModuleBuilderPermission(stored.Permissions) {
		t.Fatalf("el rol creado debe incluir un permiso con Resource == %q", auth.ResourceModuleBuilder)
	}
}
