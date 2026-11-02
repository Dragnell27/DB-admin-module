package auth

import "sort"

// EffectiveModulePermissions es el análogo de EffectivePermissions para los
// recursos dinámicos del constructor de módulos (module-builder). Representa, sin
// duplicados, las ternas (slug de módulo × acción × país) que un usuario posee
// sobre módulos creados en runtime (Req. 11.5, 11.6).
//
// Se mantiene SEPARADO de EffectivePermissions porque los permisos de módulo no
// viajan en el bitmask compacto de perm_codec (que no escala a recursos
// arbitrarios), sino en el claim `modperms` del JWT. La implementación reutiliza
// la misma idea de clave compuesta normalizada que effective_permissions.go.
//
// Para módulos con ámbito de país "none" (country_scope sin país), el país
// efectivo se normaliza a ModuleCountryAny, de modo que el matching no dependa
// del X-Country de la petición (Req. 11.7).
//
// El valor cero (EffectiveModulePermissions{}) es un conjunto vacío usable.
type EffectiveModulePermissions struct {
	set map[string]struct{}
}

// ModuleCountryAny es el país canónico usado para módulos sin ámbito de país
// (country_scope = "none"). Un permiso almacenado con este país concede acceso
// con independencia del país efectivo de la petición (Req. 11.7).
const ModuleCountryAny = "*"

// modulePermKey construye la clave compuesta estable para una terna de módulo ya
// normalizada (slug|acción|país).
func modulePermKey(slug string, a Action, country string) string {
	return slug + "|" + string(a) + "|" + country
}

// NewEffectiveModulePermissions construye el conjunto a partir de las ternas
// dadas, eliminando duplicados. El país de cada terna se normaliza a minúsculas
// (consistente con el resto del RBAC); una terna con país vacío se trata como
// ModuleCountryAny (módulo sin ámbito de país). No muta la entrada.
func NewEffectiveModulePermissions(perms []ModulePermission) EffectiveModulePermissions {
	set := make(map[string]struct{}, len(perms))
	for _, p := range perms {
		country := normalizeModuleCountry(p.Country)
		set[modulePermKey(p.Slug, p.Action, country)] = struct{}{}
	}
	return EffectiveModulePermissions{set: set}
}

// ModulePermission es la terna atómica de autorización de un módulo: slug ×
// acción × país. El país vacío denota "cualquier país" (módulo sin ámbito).
type ModulePermission struct {
	Slug    string
	Action  Action
	Country string
}

// normalizeModuleCountry normaliza el país de una terna de módulo: un país vacío
// se convierte en ModuleCountryAny; cualquier otro valor se normaliza a
// minúsculas sin espacios (igual que normalizeCountry del RBAC estático).
func normalizeModuleCountry(country string) string {
	norm := normalizeCountry(country)
	if norm == "" {
		return ModuleCountryAny
	}
	return norm
}

// HasModulePermission es la función pura central de autorización de módulos
// (Req. 11.5, 11.6, 13.4). Devuelve true si el conjunto concede la acción sobre
// el módulo `slug` en el país dado. Un permiso almacenado como ModuleCountryAny
// (módulo sin ámbito de país) concede acceso para cualquier país; en caso
// contrario, el país debe coincidir exactamente (normalizado). Nunca entra en
// pánico, incluso con el conjunto de valor cero.
func HasModulePermission(e EffectiveModulePermissions, slug string, a Action, country string) bool {
	if e.set == nil {
		return false
	}
	// Permiso global del módulo (sin ámbito de país).
	if _, ok := e.set[modulePermKey(slug, a, ModuleCountryAny)]; ok {
		return true
	}
	// Permiso específico del país efectivo.
	_, ok := e.set[modulePermKey(slug, a, normalizeCountry(country))]
	return ok
}

// List devuelve las ternas del conjunto en orden estable (slug, acción, país),
// de modo que el resultado sea determinista para pruebas y serialización.
// Devuelve un slice vacío (no nil) cuando el conjunto está vacío.
func (e EffectiveModulePermissions) List() []ModulePermission {
	out := make([]ModulePermission, 0, len(e.set))
	for key := range e.set {
		slug, action, country := splitModulePermKey(key)
		out = append(out, ModulePermission{Slug: slug, Action: action, Country: country})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Slug != out[j].Slug {
			return out[i].Slug < out[j].Slug
		}
		if out[i].Action != out[j].Action {
			return out[i].Action < out[j].Action
		}
		return out[i].Country < out[j].Country
	})
	return out
}

// CompactModulePerms es la forma embebible en el claim `modperms` del JWT de
// sesión: país -> slug -> lista de acciones. Es independiente del bitmask
// compacto de perm_codec (CompactPerms); al listar solo las ternas que el
// usuario posee, mantiene el token razonablemente pequeño. El país
// ModuleCountryAny ("*") representa los módulos sin ámbito de país.
type CompactModulePerms map[string]map[string][]string

// EncodeModulePermissions produce la forma compacta `modperms` a partir del
// conjunto efectivo de permisos de módulo. Agrupa por país y, dentro de cada
// país, por slug, acumulando la lista de acciones. El orden de las acciones es
// estable (el de List) para que la serialización sea determinista.
func EncodeModulePermissions(e EffectiveModulePermissions) CompactModulePerms {
	out := make(CompactModulePerms)
	for _, p := range e.List() {
		bySlug, ok := out[p.Country]
		if !ok {
			bySlug = make(map[string][]string)
			out[p.Country] = bySlug
		}
		bySlug[p.Slug] = append(bySlug[p.Slug], string(p.Action))
	}
	return out
}

// DecodeModulePermissions reconstruye el conjunto efectivo de permisos de módulo
// desde la forma compacta `modperms`. Tolera entradas mal formadas de forma
// segura (nunca entra en pánico): valores no esperados se ignoran. El país se
// mantiene tal cual (ya normalizado al codificar; "*" denota sin ámbito).
func DecodeModulePermissions(c CompactModulePerms) EffectiveModulePermissions {
	perms := make([]ModulePermission, 0, len(c))
	for country, bySlug := range c {
		for slug, actions := range bySlug {
			for _, a := range actions {
				perms = append(perms, ModulePermission{
					Slug:    slug,
					Action:  Action(a),
					Country: country,
				})
			}
		}
	}
	return NewEffectiveModulePermissions(perms)
}

// splitModulePermKey descompone una clave "slug|acción|país" en sus partes. Se
// asume el formato producido por modulePermKey (slug y acción sin "|").
func splitModulePermKey(key string) (slug string, action Action, country string) {
	// Partir por "|" en exactamente tres segmentos.
	first := -1
	last := -1
	for i := 0; i < len(key); i++ {
		if key[i] == '|' {
			if first == -1 {
				first = i
			} else {
				last = i
			}
		}
	}
	if first == -1 || last == -1 || first == last {
		return key, "", ""
	}
	return key[:first], Action(key[first+1 : last]), key[last+1:]
}
