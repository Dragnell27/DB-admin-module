package auth

import (
	"errors"
	"fmt"
	"strings"
)

// Resource identifica un recurso administrable del panel (p. ej. "customers",
// "plans"). El catálogo de recursos es cerrado: solo los valores declarados en
// las constantes Resource* son válidos (Req. 1.2).
type Resource string

// Action identifica una operación sobre un recurso (p. ej. "ver", "crear"). El
// conjunto de acciones válidas depende del recurso; ver ActionsForResource
// (Req. 1.3, 1.4).
type Action string

// Permission es la terna atómica de autorización Recurso × Acción × País
// (Req. 1.1). Se serializa de forma compacta con claves cortas para reducir el
// tamaño de los documentos y claims que la contienen.
type Permission struct {
	Resource Resource `json:"r"`
	Action   Action   `json:"a"`
	Country  string   `json:"c"`
}

// Constantes de recursos del catálogo cerrado (Req. 1.2).
const (
	ResourceCustomers     Resource = "customers"
	ResourcePlans         Resource = "plans"
	ResourceSubscriptions Resource = "subscriptions"
	ResourceCards         Resource = "cards"
	ResourceUsers         Resource = "users"
	ResourceRoles         Resource = "roles"
	ResourceAudit         Resource = "audit"
	// ResourceModuleBuilder gobierna el CRUD de definiciones del constructor de
	// módulos (module-builder): ver/crear/editar/eliminar módulos (Req. 2.6).
	// Es un recurso del panel conocido en compilación, de ahí que pertenezca al
	// catálogo cerrado. NO se codifica en el bitmask compacto de perm_codec
	// (uint8 casi agotado): sus permisos se transportan por el claim `modperms`
	// bajo el slug reservado "__defs__" (ver EffectiveModulePermissions y la
	// emisión del token), evitando consumir el único bit libre del bitmask.
	ResourceModuleBuilder Resource = "module-builder"
	// ResourceConnections gobierna el CRUD de plantillas de conexión
	// (connection-templates): ver/crear/editar/eliminar (Req. 5). Igual que
	// module-builder, es un recurso del catálogo cerrado que NO se codifica en el
	// bitmask: sus permisos viajan por el claim `modperms` bajo el slug reservado
	// ConnectionsSlug ("__connections__"), sellados por país (D-6).
	ResourceConnections Resource = "connections"
)

// ModuleDefsSlug es el slug reservado bajo el cual se transportan en `modperms`
// los permisos del recurso estático ResourceModuleBuilder (CRUD de definiciones
// de módulo). No es un identificador de módulo creable por el usuario: el doble
// guion bajo lo distingue de cualquier slug kebab-case válido.
const ModuleDefsSlug = "__defs__"

// ConnectionsSlug es el slug reservado bajo el cual se transportan en `modperms`
// los permisos del recurso estático ResourceConnections (CRUD de plantillas de
// conexión). Análogo a ModuleDefsSlug; el doble guion bajo lo distingue de
// cualquier slug kebab-case válido.
const ConnectionsSlug = "__connections__"

// Constantes de acciones. "cambiar" y "cancelar" solo aplican a subscriptions
// (Req. 1.3, 1.4).
const (
	ActionVer      Action = "ver"
	ActionCrear    Action = "crear"
	ActionEditar   Action = "editar"
	ActionEliminar Action = "eliminar"
	ActionCambiar  Action = "cambiar"  // solo subscriptions
	ActionCancelar Action = "cancelar" // solo subscriptions
	ActionRecargar Action = "recargar" // solo recursos cacheables (customers/plans/subscriptions)
)

// Errores exportados de validación de permisos. Se usan con errors.Is para
// distinguir el componente inválido y envueltos con fmt.Errorf para añadir el
// valor concreto que falló (Req. 1.7, 2.3, 5.4).
var (
	// ErrInvalidResource indica que el recurso no pertenece al catálogo cerrado.
	ErrInvalidResource = errors.New("auth: recurso inválido")
	// ErrInvalidAction indica que la acción no es válida para el recurso dado.
	ErrInvalidAction = errors.New("auth: acción inválida para el recurso")
	// ErrInvalidCountry indica que el país no está entre los países configurados.
	ErrInvalidCountry = errors.New("auth: país no configurado")
)

// AllResources devuelve el catálogo completo de recursos administrables, en un
// orden estable (Req. 1.2). Se expone como helper para que otros componentes
// (p. ej. el seed del rol de administración) puedan enumerar todos los recursos
// sin duplicar la lista. El slice devuelto es nuevo en cada llamada, de modo
// que el llamador puede mutarlo sin afectar al catálogo interno.
func AllResources() []Resource {
	return []Resource{
		ResourceCustomers,
		ResourcePlans,
		ResourceSubscriptions,
		ResourceCards,
		ResourceUsers,
		ResourceRoles,
		ResourceAudit,
		ResourceModuleBuilder,
		ResourceConnections,
	}
}

// ActionsForResource devuelve el catálogo de acciones válidas para el recurso
// indicado (Req. 1.3, 1.4). Para un recurso desconocido devuelve nil. El slice
// devuelto es nuevo en cada llamada, de modo que el llamador puede mutarlo sin
// afectar al catálogo interno.
func ActionsForResource(r Resource) []Action {
	switch r {
	case ResourceCustomers, ResourcePlans:
		// Recursos cacheables: además de CRUD, soportan `recargar` (botón que
		// fuerza el refresco del caché del BFF).
		return []Action{ActionVer, ActionCrear, ActionEditar, ActionEliminar, ActionRecargar}
	case ResourceUsers, ResourceRoles:
		return []Action{ActionVer, ActionCrear, ActionEditar, ActionEliminar}
	case ResourceCards:
		return []Action{ActionVer, ActionCrear, ActionEliminar} // sin editar; no cacheable
	case ResourceSubscriptions:
		// Cacheable: `recargar` además de las acciones propias de suscripciones.
		return []Action{ActionVer, ActionCrear, ActionCambiar, ActionCancelar, ActionRecargar}
	case ResourceAudit:
		// La bitácora solo se consulta: única acción `ver`.
		return []Action{ActionVer}
	case ResourceModuleBuilder:
		// CRUD de definiciones de módulo (Req. 2.6).
		return []Action{ActionVer, ActionCrear, ActionEditar, ActionEliminar}
	case ResourceConnections:
		// CRUD de plantillas de conexión (Req. 5.2).
		return []Action{ActionVer, ActionCrear, ActionEditar, ActionEliminar}
	default:
		return nil
	}
}

// IsValidResource reporta si el recurso pertenece al catálogo cerrado
// (Req. 1.2). Equivale a que ActionsForResource(r) no sea nil.
func IsValidResource(r Resource) bool {
	return ActionsForResource(r) != nil
}

// IsValidAction reporta si la acción es válida para el recurso dado, es decir,
// si pertenece a ActionsForResource(r) (Req. 1.3). Si el recurso no es válido,
// ninguna acción lo es.
func IsValidAction(r Resource, a Action) bool {
	for _, valid := range ActionsForResource(r) {
		if valid == a {
			return true
		}
	}
	return false
}

// ValidatePermission valida los tres componentes de una terna de permiso contra
// el catálogo cerrado y la lista de países configurados (Req. 1.7, 2.3, 5.4):
//
//   - El recurso debe pertenecer al catálogo (IsValidResource).
//   - La acción debe ser válida para ese recurso (IsValidAction).
//   - El país debe coincidir con alguno de configuredCountries, comparando sin
//     distinguir mayúsculas/minúsculas y normalizando a minúsculas.
//
// Devuelve nil si la terna es válida. En caso contrario devuelve un error claro
// en español que identifica el campo inválido (recurso, acción o país) y envuelve
// el error centinela correspondiente (ErrInvalidResource/ErrInvalidAction/
// ErrInvalidCountry). Nunca entra en pánico.
func ValidatePermission(p Permission, configuredCountries []string) error {
	if !IsValidResource(p.Resource) {
		return fmt.Errorf("%w: %q", ErrInvalidResource, string(p.Resource))
	}

	if !IsValidAction(p.Resource, p.Action) {
		return fmt.Errorf("%w: la acción %q no es válida para el recurso %q",
			ErrInvalidAction, string(p.Action), string(p.Resource))
	}

	country := strings.ToLower(strings.TrimSpace(p.Country))
	if country == "" {
		return fmt.Errorf("%w: %q", ErrInvalidCountry, p.Country)
	}

	for _, configured := range configuredCountries {
		if strings.ToLower(strings.TrimSpace(configured)) == country {
			return nil
		}
	}

	return fmt.Errorf("%w: %q", ErrInvalidCountry, p.Country)
}

// ValidateRolePermission valida un permiso de ROL, que en el nuevo modelo es
// únicamente la dupla Recurso × Acción (sin país): el país ya no vive en el rol
// sino en el scope del usuario. Devuelve ErrInvalidResource si el recurso no
// pertenece al catálogo o ErrInvalidAction si la acción no es válida para ese
// recurso; nil si la dupla es válida. Nunca entra en pánico.
func ValidateRolePermission(r Resource, a Action) error {
	if !IsValidResource(r) {
		return fmt.Errorf("%w: %q", ErrInvalidResource, string(r))
	}
	if !IsValidAction(r, a) {
		return fmt.Errorf("%w: la acción %q no es válida para el recurso %q",
			ErrInvalidAction, string(a), string(r))
	}
	return nil
}

// IsValidCountry indica si country (normalizado a minúsculas y sin espacios de
// relleno) pertenece a la lista de países configurados. Útil para validar el
// scope de países de un usuario contra los países con credenciales OTT.
func IsValidCountry(country string, configuredCountries []string) bool {
	c := strings.ToLower(strings.TrimSpace(country))
	if c == "" {
		return false
	}
	for _, configured := range configuredCountries {
		if strings.ToLower(strings.TrimSpace(configured)) == c {
			return true
		}
	}
	return false
}
