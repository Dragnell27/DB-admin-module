package modules

import (
	"fmt"
	"sort"
	"strings"

	"gorm.io/gorm"

	"admin-backend/internal/auth"
)

// RBACRegistrar registra y limpia el recurso dinámico de un módulo en el modelo
// de permisos (Req. 11.1, 11.2, 2.5).
//
// En este diseño los permisos viven en la tabla `role_permissions` como duplas
// (resource, action) de texto libre, por lo que el recurso dinámico mod:<slug>
// "existe" en cuanto un rol tiene al menos un permiso sobre él: no hay un
// catálogo de recursos aparte que haya que poblar. Por eso RegisterModule es
// idempotente por construcción (no inserta nada; validar el módulo basta) y la
// operación con efecto real es UnregisterModule, que elimina de role_permissions
// todas las filas que referencian el recurso del módulo eliminado.
type RBACRegistrar struct {
	db *gorm.DB
}

// NewRBACRegistrar construye el registrador sobre la conexión GORM dada.
func NewRBACRegistrar(db *gorm.DB) *RBACRegistrar {
	return &RBACRegistrar{db: db}
}

// adminRoleName es el nombre del rol de administración total (espejo del seed).
// Al registrar un módulo, sus acciones se conceden a este rol para que el
// administrador pueda ver y operar el módulo recién creado sin tener que editar
// roles a mano (cierra el ciclo crear→usar). Otros roles reciben el permiso
// editándolos explícitamente desde la gestión de roles.
const adminRoleName = "Administrador total"

// PermActionsForDefinition devuelve el conjunto de acciones de permiso RBAC que
// un módulo expone: siempre `ver` (listar/visibilidad) más la acción de permiso
// no vacía de cada acción declarada (def.Actions[].PermAction). El resultado se
// deduplica y se devuelve en un orden ESTABLE: `ver` primero y el resto en orden
// alfabético, de modo que tanto el registro RBAC (RegisterModule) como el
// catálogo de módulos de la UI (ModuleCatalog) y la validación de permisos
// dinámicos partan del mismo conjunto determinista.
func PermActionsForDefinition(def Definition) []string {
	seen := map[string]struct{}{string(auth.ActionVer): {}}
	rest := make([]string, 0, len(def.Actions))
	for _, a := range def.Actions {
		if a.PermAction == "" {
			continue
		}
		if _, ok := seen[a.PermAction]; ok {
			continue
		}
		seen[a.PermAction] = struct{}{}
		rest = append(rest, a.PermAction)
	}
	sort.Strings(rest)
	out := make([]string, 0, len(rest)+1)
	out = append(out, string(auth.ActionVer))
	out = append(out, rest...)
	return out
}

// CustomActionLabels devuelve el mapa de etiquetas legibles de las acciones
// PERSONALIZADAS de un módulo: por cada acción cuya PermAction empieza por
// "custom:" y cuyo Label (sin espacios al borde) no está vacío, asocia
// permAction -> Label. Las acciones de permiso fijas (ver/crear/editar/eliminar)
// NO entran: su etiqueta la resuelve la UI con su catálogo cerrado.
//
// Eleccion determinista del label representativo: se recorre def.Actions en
// ORDEN DE DEFINICIÓN y, cuando varias acciones comparten la misma permAction
// custom (p. ej. dos botones distintos que reusan la clave "buscar" para
// agrupar permisos), GANA la PRIMERA en orden de definición; las siguientes con
// la misma clave se ignoran. Así, aunque una acción personalizada pueda llamarse
// de cualquier forma, la matriz de permisos de Roles rotula la columna custom:*
// con un nombre real y estable en vez de la clave capitalizada.
//
// Función pura y sin estado externo: devuelve siempre un mapa no nil (vacío si
// el módulo no declara acciones custom con label).
func CustomActionLabels(def Definition) map[string]string {
	labels := make(map[string]string)
	for _, a := range def.Actions {
		if !strings.HasPrefix(a.PermAction, "custom:") {
			continue
		}
		label := strings.TrimSpace(a.Label)
		if label == "" {
			continue
		}
		if _, ok := labels[a.PermAction]; ok {
			// La primera acción en orden de definición que declara la clave gana.
			continue
		}
		labels[a.PermAction] = label
	}
	return labels
}

// undeclaredActions calcula, dado el conjunto de acciones actualmente presentes
// en role_permissions para un recurso (present) y el conjunto de acciones
// DECLARADAS por la definición vigente (declared, p. ej. el resultado de
// PermActionsForDefinition), las acciones que ya NO están declaradas y por tanto
// deben purgarse. Es una función pura y determinista: devuelve las acciones
// colgantes en el mismo orden en que aparecen en present. Se extrae para poder
// probar sin base de datos la decisión de qué acciones elimina RegisterModule
// cuando el conjunto de acciones de un módulo se reduce.
func undeclaredActions(present []string, declared []string) []string {
	declaredSet := make(map[string]struct{}, len(declared))
	for _, a := range declared {
		declaredSet[a] = struct{}{}
	}
	out := make([]string, 0)
	for _, a := range present {
		if _, ok := declaredSet[a]; !ok {
			out = append(out, a)
		}
	}
	return out
}

// targetCreatorRoleIDs calcula, dada la lista de role_id que tienen permiso de
// crear módulos (present, resultado de la consulta
// role_permissions(resource="module-builder", action="crear")) y el id del rol
// admin (adminRoleID), el conjunto de role_id que deben recibir las acciones del
// módulo de forma ADITIVA. Es una función pura y determinista:
//   - EXCLUYE al admin (adminRoleID): el admin se procesa aparte por reemplazo,
//     así que incluirlo aquí lo concedería dos veces.
//   - DESCARTA cadenas vacías (defensa ante filas mal formadas).
//   - DEDUPLICA role_id repetidos.
//   - ORDENA el resultado ascendentemente (sort.Strings), de modo que el orden
//     de los INSERT sea estable e independiente del orden de present.
//
// Se extrae para poder probar sin base de datos la decisión de "a qué roles
// conceder", igual que undeclaredActions.
func targetCreatorRoleIDs(present []string, adminRoleID string) []string {
	seen := make(map[string]struct{}, len(present))
	out := make([]string, 0, len(present))
	for _, roleID := range present {
		if roleID == "" || roleID == adminRoleID {
			continue
		}
		if _, ok := seen[roleID]; ok {
			continue
		}
		seen[roleID] = struct{}{}
		out = append(out, roleID)
	}
	sort.Strings(out)
	return out
}

// RegisterModule registra el recurso dinámico del módulo en el RBAC: concede al
// rol de administración total (`adminRoleName`) la acción de permiso de cada
// acción del módulo sobre el recurso mod:<slug>, además de `ver` (necesaria para
// que el módulo aparezca en el sidebar y se pueda listar). Adicionalmente,
// concede esas mismas acciones de forma ADITIVA a TODOS los roles que tengan
// permiso de crear módulos (role_permissions con resource="module-builder" y
// action="crear"), de modo que quien puede crear un módulo pueda usarlo de
// inmediato sin asignarse los permisos a mano. El conjunto de roles se deriva
// del PERMISO, no de la identidad del creador. Es IDEMPOTENTE: recrear o
// reeditar el mismo módulo no duplica filas (reemplazo para el admin; upsert
// ON CONFLICT DO NOTHING por la clave estable (role_id, resource, action) para
// los demás roles creadores); además limpia primero las acciones que ya no
// existan en la definición para TODOS los roles. Devuelve error si el slug es
// inválido (Req. 11.1, 11.2).
func (r *RBACRegistrar) RegisterModule(def Definition) error {
	if !auth.IsValidSlug(def.Slug) {
		return fmt.Errorf("modules: slug inválido para registro RBAC: %q", def.Slug)
	}
	resource := string(auth.DynamicResource(def.Slug))

	// Conjunto de acciones de permiso del módulo en orden determinista: siempre
	// `ver` (listar/visibilidad) más la acción de permiso de cada acción
	// declarada, deduplicadas.
	actions := PermActionsForDefinition(def)

	return r.db.Transaction(func(tx *gorm.DB) error {
		// Limpieza preventiva de colgantes para TODOS los roles: cuando el
		// conjunto de acciones del módulo se reduce (se borró una acción), las
		// filas role_permissions del recurso con una acción ya NO declarada
		// quedarían colgantes para cualquier rol que las tuviera concedidas. Se
		// eliminan en la MISMA transacción del update de la definición, espejo de
		// UnregisterModule (que borra todas las filas del recurso al borrar el
		// módulo). Así se evita el colgante de raíz sin depender del rol admin.
		if err := tx.Table("role_permissions").
			Where("resource = ? AND action NOT IN ?", resource, actions).
			Delete(nil).Error; err != nil {
			return fmt.Errorf("modules: error purgando acciones colgantes del módulo: %w", err)
		}

		// Localiza el rol de administración total; si no existe, adminRoleID queda
		// vacío (entorno sin seed): no es error y el reemplazo del admin se omite,
		// pero la concesión aditiva a los roles creadores sí puede proceder.
		var adminRoleID string
		row := tx.Table("roles").Select("id").Where("name = ?", adminRoleName).Limit(1).Scan(&adminRoleID)
		if row.Error != nil {
			return fmt.Errorf("modules: error localizando el rol admin: %w", row.Error)
		}

		// Localiza los roles que pueden CREAR módulos: aquellos con una fila
		// role_permissions(resource="module-builder", action="crear"). El criterio
		// es por PERMISO, no por identidad del creador. Un resultado vacío NO es
		// error: significa que solo el admin recibirá las acciones (o nadie si
		// tampoco hay admin).
		var creatorRoleIDs []string
		if err := tx.Table("role_permissions").
			Select("DISTINCT role_id").
			Where("resource = ? AND action = ?", string(auth.ResourceModuleBuilder), string(auth.ActionCrear)).
			Order("role_id").
			Scan(&creatorRoleIDs).Error; err != nil {
			return fmt.Errorf("modules: error localizando roles con permiso de crear módulos: %w", err)
		}

		// Reemplazo idempotente para el rol admin (comportamiento actual): elimina
		// las filas previas del recurso para este rol y vuelve a insertarlas según
		// la definición actual (sincroniza al editar un módulo que cambió sus
		// acciones). Se omite si no hay admin.
		if adminRoleID != "" {
			if err := tx.Table("role_permissions").
				Where("role_id = ? AND resource = ?", adminRoleID, resource).
				Delete(nil).Error; err != nil {
				return fmt.Errorf("modules: error limpiando permisos previos del módulo: %w", err)
			}
			for _, action := range actions {
				if err := tx.Exec(
					"INSERT INTO role_permissions (role_id, resource, action) VALUES (?, ?, ?)",
					adminRoleID, resource, action,
				).Error; err != nil {
					return fmt.Errorf("modules: error concediendo permiso %q del módulo: %w", action, err)
				}
			}
		}

		// Concesión ADITIVA para los roles creadores que NO son el admin: inserta
		// las acciones vigentes con upsert idempotente SIN borrar primero sus filas
		// del recurso, de modo que no se pisen permisos que un admin les haya
		// concedido manualmente. ON CONFLICT DO NOTHING se apoya en la UNIQUE
		// constraint (role_id, resource, action) y garantiza la no duplicación al
		// reejecutar. La purga de colgantes (arriba) ya retira las acciones que
		// dejaron de declararse para TODOS los roles, incluidos estos.
		for _, roleID := range targetCreatorRoleIDs(creatorRoleIDs, adminRoleID) {
			for _, action := range actions {
				if err := tx.Exec(
					"INSERT INTO role_permissions (role_id, resource, action) VALUES (?, ?, ?) ON CONFLICT (role_id, resource, action) DO NOTHING",
					roleID, resource, action,
				).Error; err != nil {
					return fmt.Errorf("modules: error concediendo la acción %q del módulo al rol %q: %w", action, roleID, err)
				}
			}
		}
		return nil
	})
}

// UnregisterModule elimina del modelo de permisos todas las referencias al
// recurso dinámico del módulo (filas de role_permissions con resource =
// mod:<slug>), de modo que al borrar un módulo ningún rol conserve permisos
// colgantes sobre él (Req. 2.5). Es idempotente: si no hay filas, no falla.
func (r *RBACRegistrar) UnregisterModule(slug string) error {
	resource := string(auth.DynamicResource(slug))
	if err := r.db.
		Table("role_permissions").
		Where("resource = ?", resource).
		Delete(nil).Error; err != nil {
		return fmt.Errorf("modules: no se pudieron limpiar los permisos del módulo %q: %w", slug, err)
	}
	return nil
}
