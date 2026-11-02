package modules

import (
	"reflect"
	"testing"
)

// TestPermActionsForDefinition verifica el helper puro que calcula el conjunto
// de acciones de permiso RBAC de un módulo: siempre `ver` primero, el resto
// alfabético, deduplicado y descartando PermAction vacío.
func TestPermActionsForDefinition(t *testing.T) {
	t.Run("dedupe y orden estable (ver primero, resto alfabético)", func(t *testing.T) {
		def := Definition{
			Actions: []Action{
				{PermAction: "editar"},
				{PermAction: "crear"},
				{PermAction: "editar"}, // duplicado -> una sola vez
				{PermAction: "ver"},    // ver repetido -> ya está por defecto
				{PermAction: "custom:activar"},
			},
		}
		got := PermActionsForDefinition(def)
		want := []string{"ver", "crear", "custom:activar", "editar"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("esperaba %v, obtuvo %v", want, got)
		}
	})

	t.Run("ignora PermAction vacío", func(t *testing.T) {
		def := Definition{
			Actions: []Action{
				{PermAction: ""},
				{PermAction: "crear"},
				{PermAction: ""},
			},
		}
		got := PermActionsForDefinition(def)
		want := []string{"ver", "crear"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("esperaba %v, obtuvo %v", want, got)
		}
	})

	t.Run("sin acciones devuelve solo ver", func(t *testing.T) {
		got := PermActionsForDefinition(Definition{})
		want := []string{"ver"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("esperaba %v, obtuvo %v", want, got)
		}
	})
}

// TestSearchKindIsReadOnly verifica que el tipo 'search' (lectura con filtro,
// hermana de 'list') NO cuenta como escritura: cae en el default de IsWriteKind.
func TestSearchKindIsReadOnly(t *testing.T) {
	if IsWriteKind(ActionKindSearch) {
		t.Fatalf("ActionKindSearch no debe ser escritura; IsWriteKind devolvió true")
	}
}

// TestPermActionsForDefinition_Search verifica que una acción 'search' aporta su
// PermAction al catálogo RBAC como cualquier otra (deriva de PermAction, no de
// Kind): una definición con {Kind: search, PermAction: ver} sigue dando ["ver"]
// sin duplicar ni añadir filas.
func TestPermActionsForDefinition_Search(t *testing.T) {
	def := Definition{
		Actions: []Action{
			{Kind: ActionKindSearch, PermAction: "ver"},
		},
	}
	got := PermActionsForDefinition(def)
	want := []string{"ver"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("esperaba %v, obtuvo %v", want, got)
	}
}

// TestCustomActionLabels verifica el helper puro que mapea las acciones custom
// de un módulo a su etiqueta legible: determinismo por orden de definición,
// exclusión de las acciones fijas y descarte de labels vacíos.
func TestCustomActionLabels(t *testing.T) {
	t.Run("misma permAction custom: gana la primera en orden de definición", func(t *testing.T) {
		def := Definition{
			Actions: []Action{
				{PermAction: "custom:buscar", Label: "Buscar"},
				{PermAction: "custom:buscar", Label: "Buscar por filtros"},
			},
		}
		got := CustomActionLabels(def)
		want := map[string]string{"custom:buscar": "Buscar"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("esperaba %v, obtuvo %v", want, got)
		}
	})

	t.Run("las acciones fijas no aparecen en el mapa", func(t *testing.T) {
		def := Definition{
			Actions: []Action{
				{PermAction: "ver", Label: "Ver"},
				{PermAction: "crear", Label: "Crear"},
				{PermAction: "editar", Label: "Editar"},
				{PermAction: "eliminar", Label: "Eliminar"},
				{PermAction: "custom:activar", Label: "Activar"},
			},
		}
		got := CustomActionLabels(def)
		want := map[string]string{"custom:activar": "Activar"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("esperaba %v, obtuvo %v", want, got)
		}
	})

	t.Run("descarta acciones custom con Label vacío o solo espacios", func(t *testing.T) {
		def := Definition{
			Actions: []Action{
				{PermAction: "custom:activar", Label: ""},
				{PermAction: "custom:pausar", Label: "   "},
				{PermAction: "custom:buscar", Label: "Buscar"},
			},
		}
		got := CustomActionLabels(def)
		want := map[string]string{"custom:buscar": "Buscar"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("esperaba %v, obtuvo %v", want, got)
		}
	})

	t.Run("sin acciones custom devuelve mapa vacío no nil", func(t *testing.T) {
		got := CustomActionLabels(Definition{Actions: []Action{{PermAction: "ver", Label: "Ver"}}})
		if got == nil {
			t.Fatal("esperaba mapa no nil")
		}
		if len(got) != 0 {
			t.Fatalf("esperaba mapa vacío, obtuvo %v", got)
		}
	})
}

// TestUndeclaredActions verifica el helper puro que decide qué acciones de
// role_permissions deben purgarse al reducirse el conjunto declarado de un
// módulo (Parte C). Demuestra, a nivel de lógica, que tras quitar una acción de
// la definición esa dupla mod:<slug>/<acción> dejaría de conservarse para
// cualquier rol (el DELETE transaccional real contra Postgres lo valida el
// usuario al reiniciar; no hay driver de DB en los tests).
func TestUndeclaredActions(t *testing.T) {
	t.Run("purga la acción borrada del módulo", func(t *testing.T) {
		// La definición ya solo declara ["ver", "editar"]; role_permissions
		// todavía tiene ["ver", "editar", "eliminar"] -> se purga "eliminar".
		def := Definition{Actions: []Action{{PermAction: "editar"}}}
		declared := PermActionsForDefinition(def) // ["ver", "editar"]
		present := []string{"ver", "editar", "eliminar"}

		got := undeclaredActions(present, declared)
		want := []string{"eliminar"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("esperaba %v, obtuvo %v", want, got)
		}
	})

	t.Run("no purga nada cuando todo sigue declarado", func(t *testing.T) {
		def := Definition{Actions: []Action{{PermAction: "crear"}, {PermAction: "editar"}}}
		declared := PermActionsForDefinition(def) // ["ver", "crear", "editar"]
		present := []string{"ver", "crear", "editar"}

		got := undeclaredActions(present, declared)
		if len(got) != 0 {
			t.Fatalf("esperaba vacío, obtuvo %v", got)
		}
	})

	t.Run("purga varias acciones retiradas en orden de aparición", func(t *testing.T) {
		def := Definition{}                       // solo declara "ver"
		declared := PermActionsForDefinition(def) // ["ver"]
		present := []string{"ver", "crear", "eliminar", "editar"}

		got := undeclaredActions(present, declared)
		want := []string{"crear", "eliminar", "editar"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("esperaba %v, obtuvo %v", want, got)
		}
	})
}

// TestTargetCreatorRoleIDs verifica el helper puro que decide a qué roles
// (además del admin) se les conceden de forma ADITIVA las acciones del módulo:
// son los roles con permiso de crear módulos (module-builder/crear). Demuestra,
// a nivel de lógica de conjunto, que el admin se excluye (se procesa aparte por
// reemplazo), que las cadenas vacías se descartan, que no hay duplicados y que
// el orden es estable; el efecto transaccional real (INSERT ... ON CONFLICT)
// se valida contra Postgres, igual que la nota de TestUndeclaredActions.
func TestTargetCreatorRoleIDs(t *testing.T) {
	t.Run("excluye al admin presente en la lista", func(t *testing.T) {
		present := []string{"role-admin", "role-a", "role-b"}
		got := targetCreatorRoleIDs(present, "role-admin")
		want := []string{"role-a", "role-b"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("esperaba %v, obtuvo %v", want, got)
		}
	})

	t.Run("admin ausente de la lista: todos pasan", func(t *testing.T) {
		present := []string{"role-a", "role-b"}
		got := targetCreatorRoleIDs(present, "role-admin")
		want := []string{"role-a", "role-b"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("esperaba %v, obtuvo %v", want, got)
		}
	})

	t.Run("lista vacía devuelve vacío (degradación: solo admin)", func(t *testing.T) {
		got := targetCreatorRoleIDs(nil, "role-admin")
		if len(got) != 0 {
			t.Fatalf("esperaba vacío, obtuvo %v", got)
		}
	})

	t.Run("deduplica role_id repetidos", func(t *testing.T) {
		present := []string{"role-a", "role-b", "role-a", "role-b"}
		got := targetCreatorRoleIDs(present, "role-admin")
		want := []string{"role-a", "role-b"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("esperaba %v, obtuvo %v", want, got)
		}
	})

	t.Run("entrada desordenada produce salida ordenada ascendentemente", func(t *testing.T) {
		present := []string{"role-c", "role-a", "role-b"}
		got := targetCreatorRoleIDs(present, "role-admin")
		want := []string{"role-a", "role-b", "role-c"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("esperaba %v, obtuvo %v", want, got)
		}
	})

	t.Run("descarta cadenas vacías", func(t *testing.T) {
		present := []string{"", "role-a", "", "role-b"}
		got := targetCreatorRoleIDs(present, "role-admin")
		want := []string{"role-a", "role-b"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("esperaba %v, obtuvo %v", want, got)
		}
	})
}
