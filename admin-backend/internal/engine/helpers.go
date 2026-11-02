package engine

import (
	"fmt"

	"admin-backend/internal/modules"
)

// findListAction devuelve la Acción Listar de la definición (la de Kind
// ActionKindList), si existe. La comparten los ejecutores para localizar la
// acción que alimenta la tabla del módulo.
func findListAction(def modules.Definition) (modules.Action, bool) {
	for _, a := range def.Actions {
		if a.Kind == modules.ActionKindList {
			return a, true
		}
	}
	return modules.Action{}, false
}

// stringFromConfig lee una clave string de un mapa de configuración no sensible
// (p. ej. fetch_mapping o list_params), devolviendo el valor por defecto cuando
// la clave falta, no es string o está vacía.
func stringFromConfig(cfg map[string]any, key, def string) string {
	if cfg == nil {
		return def
	}
	if v, ok := cfg[key]; ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return def
}

// inferObjectID intenta derivar un identificador del input para la auditoría
// (claves comunes), devolviendo cadena vacía si no encuentra ninguna.
func inferObjectID(input map[string]any) string {
	for _, k := range []string{"id", "external_id", "customer_id", "plan_id", "subscription_id", "card_id"} {
		if v, ok := input[k]; ok {
			return fmt.Sprintf("%v", v)
		}
	}
	return ""
}
