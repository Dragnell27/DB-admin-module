package api

import (
	"context"

	"admin-backend/internal/auth"
	"admin-backend/internal/roles"
	"admin-backend/internal/users"
)

// Este archivo centraliza los dobles de prueba (fakes) compartidos por las
// pruebas del paquete api tras la tarea 13.1: el AuthHandler ahora depende
// también de roles.Repository para resolver los permisos efectivos en el login.
//
// Se provee un fake de roles.Repository configurable por usuario y se amplían
// los fakes de users.Repository de los distintos archivos de prueba
// (auth_handler_test.go, router_test.go, integration_test.go) con los métodos
// que exige la interfaz completa (añadidos en la tarea 9.1), implementados de
// forma mínima en memoria. El objetivo es que el paquete compile y las pruebas
// pasen con la nueva firma de NewAuthHandler.

// fakeRolesRepo es un repositorio de roles en memoria para las pruebas. Permite
// fijar los permisos efectivos que devolverá PermissionsForUser por id de
// usuario y, opcionalmente, forzar un error de infraestructura.
type fakeRolesRepo struct {
	permsByUser map[string]auth.EffectivePermissions
	permsErr    error
}

// newFakeRolesRepo crea un fake de roles sin permisos configurados: por defecto
// PermissionsForUser devuelve un conjunto vacío para cualquier usuario, lo que
// basta para las pruebas de login/sesión que no verifican autorización fina.
func newFakeRolesRepo() *fakeRolesRepo {
	return &fakeRolesRepo{permsByUser: make(map[string]auth.EffectivePermissions)}
}

// setPerms asocia un conjunto de permisos efectivos al usuario dado, de modo
// que PermissionsForUser(userID) los devuelva. Útil para que las pruebas de
// integración construyan, vía login, tokens que porten los permisos que las
// rutas (RequirePermission) exigen.
func (r *fakeRolesRepo) setPerms(userID string, perms auth.EffectivePermissions) {
	r.permsByUser[userID] = perms
}

// testAllPermissions construye el conjunto de permisos efectivos que concede
// TODAS las acciones de TODOS los recursos del catálogo en cada país indicado.
// Representa a un administrador total y es la base de los tokens de prueba con
// rol admin.
func testAllPermissions(countries ...string) auth.EffectivePermissions {
	var list []auth.Permission
	for _, country := range countries {
		for _, res := range auth.AllResources() {
			for _, act := range auth.ActionsForResource(res) {
				list = append(list, auth.Permission{Resource: res, Action: act, Country: country})
			}
		}
	}
	return auth.NewEffectivePermissions(list)
}

// testViewerPermissions construye el conjunto de permisos de un "viewer":
// únicamente la acción `ver` sobre cada recurso del catálogo en cada país
// indicado. Permite las lecturas (GET) pero no las escrituras.
func testViewerPermissions(countries ...string) auth.EffectivePermissions {
	var list []auth.Permission
	for _, country := range countries {
		for _, res := range auth.AllResources() {
			list = append(list, auth.Permission{Resource: res, Action: auth.ActionVer, Country: country})
		}
	}
	return auth.NewEffectivePermissions(list)
}

// testPermsForRole devuelve los permisos efectivos que corresponden al rol
// legacy dado en los países indicados: admin → todas las acciones; cualquier
// otro (viewer) → solo lectura. Reproduce, para las pruebas, la relación
// rol→permisos que en producción resuelve roles.Repository.
func testPermsForRole(role string, countries ...string) auth.EffectivePermissions {
	if role == users.RoleAdmin {
		return testAllPermissions(countries...)
	}
	return testViewerPermissions(countries...)
}

func (r *fakeRolesRepo) List(context.Context) ([]roles.Role, error) { return nil, nil }

func (r *fakeRolesRepo) FindByID(context.Context, string) (*roles.Role, error) {
	return nil, roles.ErrNotFound
}

func (r *fakeRolesRepo) FindByName(context.Context, string) (*roles.Role, error) {
	return nil, roles.ErrNotFound
}

func (r *fakeRolesRepo) Create(context.Context, *roles.Role) error { return nil }

func (r *fakeRolesRepo) Update(context.Context, *roles.Role) error { return nil }

func (r *fakeRolesRepo) Delete(context.Context, string) error { return nil }

func (r *fakeRolesRepo) AssignmentCount(context.Context, string) (int64, error) { return 0, nil }

func (r *fakeRolesRepo) PermissionsForUser(_ context.Context, userID string) (auth.EffectivePermissions, error) {
	if r.permsErr != nil {
		return auth.EffectivePermissions{}, r.permsErr
	}
	return r.permsByUser[userID], nil
}

func (r *fakeRolesRepo) RolesForUser(context.Context, string) ([]roles.RoleRef, error) {
	return nil, nil
}

func (r *fakeRolesRepo) CountActiveAdmins(context.Context) (int64, error) { return 0, nil }

// Verificación en tiempo de compilación de que el fake satisface la interfaz.
var _ roles.Repository = (*fakeRolesRepo)(nil)

// --- Métodos adicionales de los fakes de users.Repository ---
//
// Los fakes de usuarios definidos en los archivos de prueba (authTestFakeRepo,
// routerTestFakeRepo, integFakeRepo) solo implementaban el subconjunto que el
// login original necesitaba. La interfaz users.Repository creció en la tarea
// 9.1; aquí se completan los métodos faltantes con una implementación mínima en
// memoria para que los fakes sigan satisfaciendo la interfaz. En particular,
// FindActiveByUsername —que el login ahora usa— reutiliza FindByUsername, ya
// que estos fakes solo registran usuarios activos.

// --- authTestFakeRepo (auth_handler_test.go) ---

func (r *authTestFakeRepo) FindActiveByUsername(ctx context.Context, username string) (*users.User, error) {
	return r.FindByUsername(ctx, username)
}

func (r *authTestFakeRepo) FindByID(_ context.Context, id string) (*users.User, error) {
	for _, u := range r.byUsername {
		if u.ID == id {
			cp := *u
			return &cp, nil
		}
	}
	return nil, users.ErrNotFound
}

func (r *authTestFakeRepo) List(context.Context) ([]users.User, error) { return nil, nil }

func (r *authTestFakeRepo) SetRoles(context.Context, string, []string) error { return nil }

func (r *authTestFakeRepo) SetCountries(context.Context, string, []string) error { return nil }

func (r *authTestFakeRepo) CountriesForUser(context.Context, string) ([]string, error) {
	return nil, nil
}

func (r *authTestFakeRepo) UpdateRolesStatusAndCountries(context.Context, string, []string, bool, []string) error {
	return nil
}

func (r *authTestFakeRepo) UpdatePasswordHash(context.Context, string, string) error { return nil }

var _ users.Repository = (*authTestFakeRepo)(nil)

// --- routerTestFakeRepo (router_test.go) ---

func (r *routerTestFakeRepo) FindActiveByUsername(ctx context.Context, username string) (*users.User, error) {
	return r.FindByUsername(ctx, username)
}

func (r *routerTestFakeRepo) FindByID(_ context.Context, id string) (*users.User, error) {
	for _, u := range r.byUsername {
		if u.ID == id {
			cp := *u
			return &cp, nil
		}
	}
	return nil, users.ErrNotFound
}

func (r *routerTestFakeRepo) List(context.Context) ([]users.User, error) { return nil, nil }

func (r *routerTestFakeRepo) SetRoles(context.Context, string, []string) error { return nil }

func (r *routerTestFakeRepo) SetCountries(context.Context, string, []string) error { return nil }

func (r *routerTestFakeRepo) CountriesForUser(context.Context, string) ([]string, error) {
	return nil, nil
}

func (r *routerTestFakeRepo) UpdateRolesStatusAndCountries(context.Context, string, []string, bool, []string) error {
	return nil
}

func (r *routerTestFakeRepo) UpdatePasswordHash(context.Context, string, string) error { return nil }

var _ users.Repository = (*routerTestFakeRepo)(nil)

// --- integFakeRepo (integration_test.go) ---

func (r *integFakeRepo) FindActiveByUsername(ctx context.Context, username string) (*users.User, error) {
	return r.FindByUsername(ctx, username)
}

func (r *integFakeRepo) FindByID(_ context.Context, id string) (*users.User, error) {
	for _, u := range r.byUsername {
		if u.ID == id {
			cp := *u
			return &cp, nil
		}
	}
	return nil, users.ErrNotFound
}

func (r *integFakeRepo) List(context.Context) ([]users.User, error) { return nil, nil }

func (r *integFakeRepo) SetRoles(context.Context, string, []string) error { return nil }

func (r *integFakeRepo) SetCountries(context.Context, string, []string) error { return nil }

func (r *integFakeRepo) CountriesForUser(context.Context, string) ([]string, error) { return nil, nil }

func (r *integFakeRepo) UpdateRolesStatusAndCountries(context.Context, string, []string, bool, []string) error {
	return nil
}

func (r *integFakeRepo) UpdatePasswordHash(context.Context, string, string) error { return nil }

var _ users.Repository = (*integFakeRepo)(nil)
