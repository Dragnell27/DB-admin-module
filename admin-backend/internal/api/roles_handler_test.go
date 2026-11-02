package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"admin-backend/internal/modules"
)

// Pruebas del RolesHandler centradas en FEAT-001: la aceptación/rechazo de
// permisos de recurso dinámico mod:<slug> en Create/Edit y el catálogo de
// módulos GET /roles/module-catalog. Reutiliza fakeRolesRepo de
// auth_test_fakes_test.go e introduce un fake de modules.Repository en memoria.

// fakeModulesRepo es un repositorio de módulos en memoria para las pruebas del
// RolesHandler. Permite registrar definiciones por slug y forzar un error de
// infraestructura (distinto de modules.ErrNotFound) tanto en FindBySlug como en
// List, para ejercitar el camino 500.
type fakeModulesRepo struct {
	bySlug  map[string]modules.Definition
	order   []string // orden de inserción = orden que devuelve List (ya ordenado por el llamador)
	findErr error
	listErr error
}

func newFakeModulesRepo() *fakeModulesRepo {
	return &fakeModulesRepo{bySlug: make(map[string]modules.Definition)}
}

// add registra una definición con el slug, nombre y acciones de permiso dados.
func (r *fakeModulesRepo) add(slug, name string, permActions ...string) {
	actions := make([]modules.Action, 0, len(permActions))
	for _, pa := range permActions {
		actions = append(actions, modules.Action{PermAction: pa})
	}
	r.bySlug[slug] = modules.Definition{Slug: slug, Name: name, Actions: actions}
	r.order = append(r.order, slug)
}

// addWithActions registra una definición con acciones completas (PermAction +
// Label por acción, en orden de definición). Complementa a add para los casos
// del catálogo que verifican actionLabels sin romper los tests existentes.
func (r *fakeModulesRepo) addWithActions(slug, name string, actions ...modules.Action) {
	r.bySlug[slug] = modules.Definition{Slug: slug, Name: name, Actions: actions}
	r.order = append(r.order, slug)
}

func (r *fakeModulesRepo) List(context.Context) ([]modules.Definition, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	out := make([]modules.Definition, 0, len(r.order))
	for _, slug := range r.order {
		out = append(out, r.bySlug[slug])
	}
	return out, nil
}

func (r *fakeModulesRepo) FindBySlug(_ context.Context, slug string) (*modules.Definition, error) {
	if r.findErr != nil {
		return nil, r.findErr
	}
	def, ok := r.bySlug[slug]
	if !ok {
		return nil, modules.ErrNotFound
	}
	cp := def
	return &cp, nil
}

func (r *fakeModulesRepo) Create(context.Context, *modules.Definition, modules.Secrets) error {
	return nil
}

func (r *fakeModulesRepo) Update(context.Context, *modules.Definition, modules.Secrets) error {
	return nil
}

func (r *fakeModulesRepo) Delete(context.Context, string) error { return nil }

var _ modules.Repository = (*fakeModulesRepo)(nil)

// rolesTestResponse refleja el envoltorio normalizado del BFF.
type rolesTestResponse struct {
	OK        bool            `json:"ok"`
	Data      json.RawMessage `json:"data"`
	ErrorCode string          `json:"errorCode"`
	Message   string          `json:"message"`
}

func rolesTestDecode(t *testing.T, rec *httptest.ResponseRecorder) rolesTestResponse {
	t.Helper()
	var resp rolesTestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("no se pudo decodificar la respuesta: %v (cuerpo: %s)", err, rec.Body.String())
	}
	return resp
}

// rolesTestDoCreate monta POST /roles con el handler dado y ejecuta la petición.
func rolesTestDoCreate(h *RolesHandler, body string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/roles", h.Create)
	req := httptest.NewRequest(http.MethodPost, "/roles", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// rolesTestDoCatalog monta GET /roles/module-catalog y ejecuta la petición.
func rolesTestDoCatalog(h *RolesHandler) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/roles/module-catalog", h.ModuleCatalog)
	req := httptest.NewRequest(http.MethodGet, "/roles/module-catalog", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestRolesCreate_AcceptsDynamicModulePermission(t *testing.T) {
	mods := newFakeModulesRepo()
	mods.add("suscripciones", "Suscripciones", "editar", "custom:activar")
	h := NewRolesHandler(newFakeRolesRepo(), mods)

	rec := rolesTestDoCreate(h, `{"name":"Operador","permissions":[{"resource":"mod:suscripciones","action":"custom:activar"}]}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("esperaba 201, obtuvo %d (cuerpo: %s)", rec.Code, rec.Body.String())
	}
	resp := rolesTestDecode(t, rec)
	if !resp.OK {
		t.Fatalf("esperaba ok=true (cuerpo: %s)", rec.Body.String())
	}
}

func TestRolesCreate_AcceptsImplicitVerOnModule(t *testing.T) {
	mods := newFakeModulesRepo()
	mods.add("suscripciones", "Suscripciones", "editar")
	h := NewRolesHandler(newFakeRolesRepo(), mods)

	// `ver` siempre está declarado aunque ninguna Action lo liste.
	rec := rolesTestDoCreate(h, `{"name":"Lector","permissions":[{"resource":"mod:suscripciones","action":"ver"}]}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("esperaba 201, obtuvo %d (cuerpo: %s)", rec.Code, rec.Body.String())
	}
}

// TestRolesCreate_DropsDanglingModuleAction fija la degradación segura (R3.4,
// caso acción-no-declarada): al guardar un rol que porta una dupla colgante
// (módulo existente + acción ya no declarada) el guardado NO falla con 400; la
// dupla colgante se descarta silenciosamente del conjunto persistido, mientras
// que una dupla válida del mismo módulo SÍ se conserva.
func TestRolesCreate_DropsDanglingModuleAction(t *testing.T) {
	mods := newFakeModulesRepo()
	mods.add("suscripciones", "Suscripciones", "editar")
	h := NewRolesHandler(newFakeRolesRepo(), mods)

	// `eliminar` ya no está declarada por el módulo (colgante) -> se descarta.
	// `ver` sí está declarada (implícita) -> se conserva.
	rec := rolesTestDoCreate(h, `{"name":"X","permissions":[{"resource":"mod:suscripciones","action":"ver"},{"resource":"mod:suscripciones","action":"eliminar"}]}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("esperaba 201, obtuvo %d (cuerpo: %s)", rec.Code, rec.Body.String())
	}
	resp := rolesTestDecode(t, rec)
	if !resp.OK {
		t.Fatalf("esperaba ok=true (cuerpo: %s)", rec.Body.String())
	}

	// La respuesta refleja (vía toRolePublic) exactamente lo que
	// validatePermissions dejó pasar: debe contener la dupla válida y NO la
	// colgante.
	var data struct {
		Role struct {
			Permissions []rolePermissionDTO `json:"permissions"`
		} `json:"role"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("no se pudo decodificar data: %v", err)
	}
	var hasVer, hasEliminar bool
	for _, p := range data.Role.Permissions {
		if p.Resource == "mod:suscripciones" && p.Action == "ver" {
			hasVer = true
		}
		if p.Resource == "mod:suscripciones" && p.Action == "eliminar" {
			hasEliminar = true
		}
	}
	if !hasVer {
		t.Errorf("la dupla válida mod:suscripciones/ver debía conservarse, permisos: %v", data.Role.Permissions)
	}
	if hasEliminar {
		t.Errorf("la dupla colgante mod:suscripciones/eliminar debía descartarse, permisos: %v", data.Role.Permissions)
	}
}

func TestRolesCreate_RejectsUnknownModuleSlug(t *testing.T) {
	mods := newFakeModulesRepo() // sin módulos registrados
	h := NewRolesHandler(newFakeRolesRepo(), mods)

	rec := rolesTestDoCreate(h, `{"name":"X","permissions":[{"resource":"mod:inexistente","action":"ver"}]}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("esperaba 400, obtuvo %d (cuerpo: %s)", rec.Code, rec.Body.String())
	}
	resp := rolesTestDecode(t, rec)
	if !strings.Contains(resp.Message, "no existe") {
		t.Errorf("mensaje de módulo inexistente esperado, obtuvo %q", resp.Message)
	}
}

func TestRolesCreate_MalformedSlugFallsToClosedCatalog(t *testing.T) {
	mods := newFakeModulesRepo()
	h := NewRolesHandler(newFakeRolesRepo(), mods)

	// mod:Mal_Slug no es un recurso dinámico bien formado -> se valida contra el
	// catálogo cerrado, que no lo reconoce -> 400 recurso inválido.
	rec := rolesTestDoCreate(h, `{"name":"X","permissions":[{"resource":"mod:Mal_Slug","action":"ver"}]}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("esperaba 400, obtuvo %d (cuerpo: %s)", rec.Code, rec.Body.String())
	}
}

func TestRolesCreate_InfraErrorOnModuleLookupReturns500(t *testing.T) {
	mods := newFakeModulesRepo()
	mods.add("suscripciones", "Suscripciones", "editar")
	mods.findErr = errors.New("fallo de conexión a la base de datos")
	h := NewRolesHandler(newFakeRolesRepo(), mods)

	rec := rolesTestDoCreate(h, `{"name":"X","permissions":[{"resource":"mod:suscripciones","action":"ver"}]}`)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("esperaba 500, obtuvo %d (cuerpo: %s)", rec.Code, rec.Body.String())
	}
	resp := rolesTestDecode(t, rec)
	if resp.ErrorCode != "E500" {
		t.Errorf("código E500 esperado, obtuvo %q", resp.ErrorCode)
	}
	// El detalle crudo del error de infraestructura NO debe filtrarse al cliente.
	if strings.Contains(rec.Body.String(), "conexión a la base de datos") {
		t.Error("el error de infraestructura no debe exponerse al cliente")
	}
}

func TestRolesCreate_ClosedCatalogRegression(t *testing.T) {
	mods := newFakeModulesRepo()
	h := NewRolesHandler(newFakeRolesRepo(), mods)

	// Dupla fija inválida (recurso del catálogo cerrado con acción no válida).
	rec := rolesTestDoCreate(h, `{"name":"X","permissions":[{"resource":"audit","action":"eliminar"}]}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("esperaba 400, obtuvo %d (cuerpo: %s)", rec.Code, rec.Body.String())
	}
}

func TestRolesCreate_ClosedCatalogValidPermissionAccepted(t *testing.T) {
	mods := newFakeModulesRepo()
	h := NewRolesHandler(newFakeRolesRepo(), mods)

	rec := rolesTestDoCreate(h, `{"name":"X","permissions":[{"resource":"roles","action":"ver"}]}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("esperaba 201, obtuvo %d (cuerpo: %s)", rec.Code, rec.Body.String())
	}
}

// --- ModuleCatalog -------------------------------------------------------

type catalogItem struct {
	Slug         string            `json:"slug"`
	Name         string            `json:"name"`
	Actions      []string          `json:"actions"`
	ActionLabels map[string]string `json:"actionLabels"`
}

type catalogData struct {
	Items []catalogItem `json:"items"`
}

func TestModuleCatalog_ListsModulesWithDedupedActions(t *testing.T) {
	mods := newFakeModulesRepo()
	// List ya devuelve ordenado por slug (lo garantiza el repositorio real); el
	// fake lo respeta por el orden de inserción.
	mods.add("alfa", "Alfa", "editar", "crear", "editar")
	mods.add("beta", "Beta", "custom:activar")
	h := NewRolesHandler(newFakeRolesRepo(), mods)

	rec := rolesTestDoCatalog(h)
	if rec.Code != http.StatusOK {
		t.Fatalf("esperaba 200, obtuvo %d (cuerpo: %s)", rec.Code, rec.Body.String())
	}
	resp := rolesTestDecode(t, rec)
	var data catalogData
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("no se pudo decodificar data: %v", err)
	}
	if len(data.Items) != 2 {
		t.Fatalf("esperaba 2 items, obtuvo %d", len(data.Items))
	}
	if data.Items[0].Slug != "alfa" || data.Items[1].Slug != "beta" {
		t.Errorf("orden por slug esperado [alfa, beta], obtuvo [%s, %s]", data.Items[0].Slug, data.Items[1].Slug)
	}
	// Acciones de alfa: ver primero, resto alfabético, dedupe de editar.
	wantAlfa := []string{"ver", "crear", "editar"}
	if strings.Join(data.Items[0].Actions, ",") != strings.Join(wantAlfa, ",") {
		t.Errorf("acciones de alfa esperadas %v, obtuvo %v", wantAlfa, data.Items[0].Actions)
	}
	if data.Items[0].Actions[0] != "ver" {
		t.Errorf("la primera acción debe ser ver, obtuvo %q", data.Items[0].Actions[0])
	}
}

// TestModuleCatalog_ExposesCustomActionLabels verifica que el DTO del catálogo
// expone actionLabels con el label legible de las acciones custom (el de la
// primera acción en orden de definición cuando dos comparten la misma clave) y
// que las acciones fijas NO aparecen en el mapa.
func TestModuleCatalog_ExposesCustomActionLabels(t *testing.T) {
	mods := newFakeModulesRepo()
	mods.addWithActions("suscripciones", "Suscripciones",
		modules.Action{PermAction: "editar", Label: "Editar"},
		modules.Action{PermAction: "custom:buscar", Label: "Buscar"},
		modules.Action{PermAction: "custom:buscar", Label: "Buscar por filtros"},
	)
	h := NewRolesHandler(newFakeRolesRepo(), mods)

	rec := rolesTestDoCatalog(h)
	if rec.Code != http.StatusOK {
		t.Fatalf("esperaba 200, obtuvo %d (cuerpo: %s)", rec.Code, rec.Body.String())
	}
	resp := rolesTestDecode(t, rec)
	var data catalogData
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("no se pudo decodificar data: %v", err)
	}
	if len(data.Items) != 1 {
		t.Fatalf("esperaba 1 item, obtuvo %d", len(data.Items))
	}
	labels := data.Items[0].ActionLabels
	// Gana el label de la primera acción custom:buscar en orden de definición.
	if labels["custom:buscar"] != "Buscar" {
		t.Errorf("esperaba label 'Buscar' para custom:buscar, obtuvo %q", labels["custom:buscar"])
	}
	// Las acciones fijas no están en el mapa.
	if _, ok := labels["editar"]; ok {
		t.Errorf("la acción fija 'editar' no debe aparecer en actionLabels: %v", labels)
	}
	if _, ok := labels["ver"]; ok {
		t.Errorf("la acción fija 'ver' no debe aparecer en actionLabels: %v", labels)
	}
	// El array actions sigue presente e intacto (ver primero).
	if len(data.Items[0].Actions) == 0 || data.Items[0].Actions[0] != "ver" {
		t.Errorf("actions debe seguir presente con ver primero, obtuvo %v", data.Items[0].Actions)
	}
}

func TestModuleCatalog_NilModulesReturnsEmptyItems(t *testing.T) {
	h := NewRolesHandler(newFakeRolesRepo(), nil)

	rec := rolesTestDoCatalog(h)
	if rec.Code != http.StatusOK {
		t.Fatalf("esperaba 200, obtuvo %d (cuerpo: %s)", rec.Code, rec.Body.String())
	}
	resp := rolesTestDecode(t, rec)
	var data catalogData
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("no se pudo decodificar data: %v", err)
	}
	if len(data.Items) != 0 {
		t.Errorf("esperaba items vacío, obtuvo %d", len(data.Items))
	}
}

func TestModuleCatalog_ListErrorReturns500(t *testing.T) {
	mods := newFakeModulesRepo()
	mods.listErr = errors.New("fallo listando")
	h := NewRolesHandler(newFakeRolesRepo(), mods)

	rec := rolesTestDoCatalog(h)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("esperaba 500, obtuvo %d (cuerpo: %s)", rec.Code, rec.Body.String())
	}
}
