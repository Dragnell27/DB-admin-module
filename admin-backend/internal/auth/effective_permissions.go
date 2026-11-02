package auth

import (
	"sort"
	"strings"
)

// EffectivePermissions es un conjunto de permisos sin duplicados por la terna
// (Resource, Action, Country) (Req. 7.1). Representa los permisos efectivos de
// un usuario, típicamente resultado de unir los permisos de todos sus roles.
//
// La implementación interna es un map cuya clave es la terna normalizada
// ("resource|action|country") y cuyo valor es la propia Permission ya
// normalizada. El país se almacena siempre en minúsculas y sin espacios de
// relleno, de forma consistente con ValidatePermission, para que el matching
// sea estable.
//
// El valor cero (EffectivePermissions{}) es un conjunto vacío usable: los
// métodos toleran un set interno nil.
type EffectivePermissions struct {
	set map[string]Permission
}

// normalizeCountry devuelve el país en la forma canónica usada como clave:
// sin espacios al inicio/fin y en minúsculas (consistente con
// ValidatePermission).
func normalizeCountry(country string) string {
	return strings.ToLower(strings.TrimSpace(country))
}

// permKey construye la clave compuesta estable para una terna ya normalizada.
func permKey(r Resource, a Action, country string) string {
	return string(r) + "|" + string(a) + "|" + country
}

// NewEffectivePermissions construye un conjunto de permisos efectivos a partir
// del slice dado, eliminando duplicados por la terna (Resource, Action,
// Country). El país de cada permiso se normaliza a minúsculas para que el
// matching sea estable (Req. 7.1). No muta el slice de entrada.
func NewEffectivePermissions(perms []Permission) EffectivePermissions {
	set := make(map[string]Permission, len(perms))
	for _, p := range perms {
		country := normalizeCountry(p.Country)
		np := Permission{Resource: p.Resource, Action: p.Action, Country: country}
		set[permKey(np.Resource, np.Action, country)] = np
	}
	return EffectivePermissions{set: set}
}

// Union devuelve un NUEVO conjunto con la unión de los permisos de e y other.
// No muta ninguno de los receptores (Req. 7.1).
func (e EffectivePermissions) Union(other EffectivePermissions) EffectivePermissions {
	set := make(map[string]Permission, len(e.set)+len(other.set))
	for k, v := range e.set {
		set[k] = v
	}
	for k, v := range other.set {
		set[k] = v
	}
	return EffectivePermissions{set: set}
}

// List devuelve las ternas del conjunto en orden estable (ordenadas por
// resource, luego action, luego country), de modo que el resultado sea
// determinista para pruebas. Devuelve un slice vacío (no nil) cuando el
// conjunto está vacío.
func (e EffectivePermissions) List() []Permission {
	out := make([]Permission, 0, len(e.set))
	for _, p := range e.set {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Resource != out[j].Resource {
			return out[i].Resource < out[j].Resource
		}
		if out[i].Action != out[j].Action {
			return out[i].Action < out[j].Action
		}
		return out[i].Country < out[j].Country
	})
	return out
}

// Countries devuelve los países distintos que aparecen en al menos un permiso
// del conjunto, ordenados y sin duplicados (Req. 9.4). Devuelve un slice vacío
// (no nil) cuando el conjunto está vacío.
func (e EffectivePermissions) Countries() []string {
	seen := make(map[string]struct{}, len(e.set))
	out := make([]string, 0, len(e.set))
	for _, p := range e.set {
		if _, ok := seen[p.Country]; ok {
			continue
		}
		seen[p.Country] = struct{}{}
		out = append(out, p.Country)
	}
	sort.Strings(out)
	return out
}

// HasPermission es la función pura central de autorización (Req. 1.6, 8.3, 8.4).
// Devuelve true si y solo si la terna (r, a, country) pertenece al conjunto e.
// El país se normaliza a minúsculas antes de comparar, de forma consistente con
// la construcción del conjunto. Nunca entra en pánico, incluso con un conjunto
// de valor cero.
func HasPermission(e EffectivePermissions, r Resource, a Action, country string) bool {
	if e.set == nil {
		return false
	}
	_, ok := e.set[permKey(r, a, normalizeCountry(country))]
	return ok
}
