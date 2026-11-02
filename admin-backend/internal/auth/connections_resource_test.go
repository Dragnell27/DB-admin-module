package auth

import "testing"

// TestAllResourcesIncludeConnections verifica que el recurso `connections`
// pertenece al catálogo cerrado (Req. 5).
func TestAllResourcesIncludeConnections(t *testing.T) {
	found := false
	for _, r := range AllResources() {
		if r == ResourceConnections {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("AllResources() debería incluir ResourceConnections")
	}
}

// TestActionsForConnections verifica las 4 acciones del CRUD de plantillas
// (Req. 5.2).
func TestActionsForConnections(t *testing.T) {
	actions := ActionsForResource(ResourceConnections)
	want := map[Action]bool{ActionVer: false, ActionCrear: false, ActionEditar: false, ActionEliminar: false}
	if len(actions) != len(want) {
		t.Fatalf("se esperaban %d acciones, got %d: %v", len(want), len(actions), actions)
	}
	for _, a := range actions {
		if _, ok := want[a]; !ok {
			t.Errorf("acción inesperada %q", a)
		}
		want[a] = true
	}
	for a, seen := range want {
		if !seen {
			t.Errorf("falta la acción %q", a)
		}
	}
}

// TestSplitEffectivePermissions_RoutesConnections verifica que un permiso
// `connections` se enruta a modperms bajo el slug reservado __connections__,
// conservando el país (paridad con module-builder → __defs__).
func TestSplitEffectivePermissions_RoutesConnections(t *testing.T) {
	eff := NewEffectivePermissions([]Permission{
		{Resource: ResourceConnections, Action: ActionCrear, Country: "gt"},
		{Resource: ResourceModuleBuilder, Action: ActionVer, Country: "gt"},
		{Resource: ResourceCustomers, Action: ActionVer, Country: "gt"},
	})
	static, modperms := SplitEffectivePermissions(eff)

	// connections NO debe quedar en el conjunto estático.
	for _, p := range static.List() {
		if p.Resource == ResourceConnections {
			t.Fatal("connections no debería permanecer en el conjunto estático")
		}
	}
	// connections debe evaluarse bajo __connections__ con su país.
	if !HasModulePermission(modperms, ConnectionsSlug, ActionCrear, "gt") {
		t.Fatal("connections debería autorizarse bajo __connections__ en gt")
	}
	// El país se sella: otro país no autoriza.
	if HasModulePermission(modperms, ConnectionsSlug, ActionCrear, "mx") {
		t.Fatal("connections no debería autorizarse en un país distinto")
	}
	// Paridad: module-builder sigue bajo __defs__.
	if !HasModulePermission(modperms, ModuleDefsSlug, ActionVer, "gt") {
		t.Fatal("module-builder debería autorizarse bajo __defs__ (paridad)")
	}
}

// TestConnectionsCountrySeal_ParityWithModuleBuilder fija por prueba que
// `connections` y `module-builder` se sellan igual por país: el permiso concedido
// en un país no autoriza en otro (misma semántica que el guard reutilizado).
func TestConnectionsCountrySeal_ParityWithModuleBuilder(t *testing.T) {
	eff := NewEffectivePermissions([]Permission{
		{Resource: ResourceConnections, Action: ActionEditar, Country: "gt"},
		{Resource: ResourceModuleBuilder, Action: ActionEditar, Country: "gt"},
	})
	_, modperms := SplitEffectivePermissions(eff)

	for _, slug := range []string{ConnectionsSlug, ModuleDefsSlug} {
		if !HasModulePermission(modperms, slug, ActionEditar, "gt") {
			t.Errorf("%s: debería autorizar en gt", slug)
		}
		if HasModulePermission(modperms, slug, ActionEditar, "") {
			t.Errorf("%s: país vacío no debería autorizar", slug)
		}
		if HasModulePermission(modperms, slug, ActionEditar, "mx") {
			t.Errorf("%s: país distinto no debería autorizar", slug)
		}
	}
}
