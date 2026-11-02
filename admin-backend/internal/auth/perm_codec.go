package auth

// Codificación compacta de permisos (Req. 7.3, 13.4).
//
// Para mantener pequeño el token de sesión, los permisos efectivos se agrupan
// por país y, dentro de cada país, por recurso, usando un bitmask de acciones.
// La forma compacta se serializa como un mapa:
//
//	país (minúsculas) -> { código de recurso (2 letras) -> bitmask uint8 }
//
// Los códigos de recurso y los bits de acción son fijos, de modo que la
// codificación es estable y reversible. El objetivo es cumplir el round-trip:
// DecodePermissions(EncodePermissions(e)) produce el mismo conjunto de ternas.

// CompactPerms es la forma compacta embebible en el claim de sesión: país →
// código de recurso → bitmask de acciones.
type CompactPerms map[string]map[string]uint8

// Códigos de recurso de 2 letras. El mapa resourceToCode traduce de Resource al
// código compacto; codeToResource es su inverso para la decodificación.
var resourceToCode = map[Resource]string{
	ResourceCustomers:     "cu",
	ResourcePlans:         "pl",
	ResourceSubscriptions: "su",
	ResourceCards:         "ca",
	ResourceUsers:         "us",
	ResourceRoles:         "ro",
	ResourceAudit:         "au",
}

var codeToResource = map[string]Resource{
	"cu": ResourceCustomers,
	"pl": ResourcePlans,
	"su": ResourceSubscriptions,
	"ca": ResourceCards,
	"us": ResourceUsers,
	"ro": ResourceRoles,
	"au": ResourceAudit,
}

// Bits de acción fijos. El mapa actionToBit traduce de Action al bit del
// bitmask; bitToAction es su inverso para la decodificación.
var actionToBit = map[Action]uint8{
	ActionVer:      1,
	ActionCrear:    2,
	ActionEditar:   4,
	ActionEliminar: 8,
	ActionCambiar:  16,
	ActionCancelar: 32,
	ActionRecargar: 64,
}

var bitToAction = map[uint8]Action{
	1:  ActionVer,
	2:  ActionCrear,
	4:  ActionEditar,
	8:  ActionEliminar,
	16: ActionCambiar,
	32: ActionCancelar,
	64: ActionRecargar,
}

// EncodePermissions produce la forma compacta embebible en el claim (Req. 7.3).
//
// Recorre e.List() y agrupa por país (normalizado a minúsculas, consistente con
// effective_permissions.go) y por recurso, acumulando en un bitmask uint8 el
// bit de cada acción. Los recursos o acciones sin código/bit conocido se
// omiten de forma segura (nunca entra en pánico).
func EncodePermissions(e EffectivePermissions) CompactPerms {
	out := make(CompactPerms)
	for _, p := range e.List() {
		code, okRes := resourceToCode[p.Resource]
		if !okRes {
			continue // recurso desconocido: se ignora de forma segura
		}
		bit, okAct := actionToBit[p.Action]
		if !okAct {
			continue // acción desconocida: se ignora de forma segura
		}
		country := normalizeCountry(p.Country)
		byResource, ok := out[country]
		if !ok {
			byResource = make(map[string]uint8)
			out[country] = byResource
		}
		byResource[code] |= bit
	}
	return out
}

// DecodePermissions reconstruye el conjunto de permisos efectivos desde la forma
// compacta, expandiendo cada bit activo del bitmask a su acción (Req. 7.3,
// 13.4).
//
// Ignora de forma segura los códigos de recurso desconocidos y los bits sin
// acción asociada, de modo que nunca entra en pánico ante entradas corruptas o
// de una versión futura. El país se normaliza a minúsculas para mantener la
// consistencia con EncodePermissions y effective_permissions.go.
func DecodePermissions(c CompactPerms) EffectivePermissions {
	perms := make([]Permission, 0, len(c))
	for country, byResource := range c {
		normCountry := normalizeCountry(country)
		for code, mask := range byResource {
			resource, ok := codeToResource[code]
			if !ok {
				continue // código de recurso desconocido: se ignora
			}
			// Expande cada bit activo del bitmask a su acción.
			for bit, action := range bitToAction {
				if mask&bit != 0 {
					perms = append(perms, Permission{
						Resource: resource,
						Action:   action,
						Country:  normCountry,
					})
				}
			}
		}
	}
	return NewEffectivePermissions(perms)
}
