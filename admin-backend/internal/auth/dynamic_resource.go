package auth

import (
	"regexp"
	"strings"
)

// Recursos dinámicos del constructor de módulos (module-builder).
//
// A diferencia del catálogo cerrado (Resource* + ActionsForResource), los
// módulos creados por el usuario introducen recursos que NO existen en tiempo
// de compilación. Para no romper el RBAC estático (ni el bitmask compacto de
// perm_codec, que no escala a recursos arbitrarios), un módulo con identificador
// kebab-case `slug` se representa como el recurso dinámico `mod:<slug>`.
//
// El prefijo "mod:" distingue inequívocamente estos recursos de los estáticos
// (ninguno del catálogo cerrado contiene ":"), de modo que ambos espacios de
// nombres conviven sin colisión. La validación de un recurso dinámico y de sus
// acciones vive aquí y NO depende del switch cerrado de ActionsForResource: las
// acciones válidas de un módulo las declara su propia Definición (Req. 11.1,
// 11.2).

// dynamicResourcePrefix es el prefijo que marca un recurso como dinámico
// (perteneciente a un módulo del constructor).
const dynamicResourcePrefix = "mod:"

// slugPattern valida un slug kebab-case: minúsculas y dígitos separados por
// guiones simples, sin guion inicial/final ni dobles. Debe coincidir con la
// validación de slug del frontend y del CRUD de definiciones (Req. 1.2).
var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// IsValidSlug reporta si s es un identificador de módulo válido en kebab-case.
// El slug reservado del CRUD de definiciones (ModuleDefsSlug, "__defs__") NO es
// un slug válido de módulo creable: contiene guiones bajos, que este patrón
// rechaza, de modo que nunca colisiona con un módulo real.
func IsValidSlug(s string) bool {
	return slugPattern.MatchString(s)
}

// DynamicResource construye el recurso dinámico `mod:<slug>` para un slug dado.
// No valida el slug; el llamador debe asegurarse con IsValidSlug cuando el
// origen del slug no sea de confianza.
func DynamicResource(slug string) Resource {
	return Resource(dynamicResourcePrefix + slug)
}

// IsDynamicResource reporta si r es un recurso dinámico de módulo bien formado:
// tiene el prefijo "mod:" seguido de un slug kebab-case válido.
func IsDynamicResource(r Resource) bool {
	s := string(r)
	if !strings.HasPrefix(s, dynamicResourcePrefix) {
		return false
	}
	return IsValidSlug(strings.TrimPrefix(s, dynamicResourcePrefix))
}

// SlugFromDynamicResource extrae el slug de un recurso dinámico `mod:<slug>`.
// Devuelve (slug, true) si r es un recurso dinámico bien formado; ("", false)
// en caso contrario.
func SlugFromDynamicResource(r Resource) (string, bool) {
	if !IsDynamicResource(r) {
		return "", false
	}
	return strings.TrimPrefix(string(r), dynamicResourcePrefix), true
}

// SplitEffectivePermissions separa un conjunto de permisos efectivos en dos:
// los permisos del catálogo ESTÁTICO (recursos sin prefijo mod: y distintos del
// recurso de definiciones) y los permisos de MÓDULOS dinámicos
// (EffectiveModulePermissions), listos para transportarse en claims separados
// (`perms` y `modperms`).
//
// Reglas de partición:
//   - resource == ResourceModuleBuilder ("module-builder"): va a módulos bajo el
//     slug reservado ModuleDefsSlug ("__defs__"), para el CRUD de definiciones.
//   - resource con prefijo "mod:": va a módulos bajo su <slug>.
//   - cualquier otro recurso: permanece en el conjunto estático.
//
// El país de cada permiso se conserva. Un módulo cuyo permiso tenga país vacío
// se transporta como "cualquier país" (lo normaliza EffectiveModulePermissions).
func SplitEffectivePermissions(e EffectivePermissions) (EffectivePermissions, EffectiveModulePermissions) {
	staticList := make([]Permission, 0)
	moduleList := make([]ModulePermission, 0)

	for _, p := range e.List() {
		switch {
		case p.Resource == ResourceModuleBuilder:
			moduleList = append(moduleList, ModulePermission{
				Slug:    ModuleDefsSlug,
				Action:  p.Action,
				Country: p.Country,
			})
		case p.Resource == ResourceConnections:
			moduleList = append(moduleList, ModulePermission{
				Slug:    ConnectionsSlug,
				Action:  p.Action,
				Country: p.Country,
			})
		case IsDynamicResource(p.Resource):
			slug, _ := SlugFromDynamicResource(p.Resource)
			moduleList = append(moduleList, ModulePermission{
				Slug:    slug,
				Action:  p.Action,
				Country: p.Country,
			})
		default:
			staticList = append(staticList, p)
		}
	}
	return NewEffectivePermissions(staticList), NewEffectiveModulePermissions(moduleList)
}

// ValidateDynamicModuleAction valida la dupla (recurso dinámico × acción) de un
// módulo contra las acciones DECLARADAS por su Definición, en lugar de contra el
// catálogo cerrado ActionsForResource (Req. 11.1, 11.2). declaredActions es el
// conjunto de acciones de permiso que el módulo expone (p. ej. "ver", "crear",
// "custom:activar").
//
// Devuelve nil si el recurso es un recurso dinámico bien formado y la acción
// pertenece a declaredActions. En caso contrario devuelve ErrInvalidResource
// (recurso dinámico mal formado) o ErrInvalidAction (acción no declarada).
func ValidateDynamicModuleAction(r Resource, a Action, declaredActions []Action) error {
	if !IsDynamicResource(r) {
		return ErrInvalidResource
	}
	for _, declared := range declaredActions {
		if declared == a {
			return nil
		}
	}
	return ErrInvalidAction
}
