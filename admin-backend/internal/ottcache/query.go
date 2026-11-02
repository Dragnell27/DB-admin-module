package ottcache

import (
	"fmt"
	"sort"
	"strings"

	"admin-backend/internal/ott"
)

// PageParams describe la paginación y el ordenamiento a aplicar en memoria sobre
// el conjunto completo. OrderBy vacío significa "orden natural" (el que trae el
// conjunto cacheado); de lo contrario se ordena por ese campo.
type PageParams struct {
	Page           int
	RecordsPerPage int
	OrderBy        string
	Order          string // "asc" | "desc"; cualquier otro valor se trata como "asc"
}

// Page ordena (si procede) y recorta la página pedida del conjunto, devolviendo
// los registros de esa página y los metadatos de paginación coherentes con el
// contrato normalizado del BFF (ott.Pagination):
//
//   - TotalRecords = len(records)
//   - RecordsPerPage = p.RecordsPerPage (saneado a >= 1)
//   - Page = p.Page (saneado a >= 1; si excede TotalPages y hay registros, se
//     deja en el valor pedido y la página resultante queda vacía)
//   - TotalPages = ceil(TotalRecords / RecordsPerPage)
//
// No muta el slice de entrada (ordena sobre una copia).
func Page(records []Record, p PageParams) ([]Record, ott.Pagination) {
	perPage := p.RecordsPerPage
	if perPage < 1 {
		perPage = 10
	}
	page := p.Page
	if page < 1 {
		page = 1
	}

	total := int64(len(records))
	totalPages := 0
	if perPage > 0 {
		totalPages = int((total + int64(perPage) - 1) / int64(perPage))
	}

	// Ordenar sobre una copia para no alterar el conjunto recibido.
	ordered := make([]Record, len(records))
	copy(ordered, records)
	if strings.TrimSpace(p.OrderBy) != "" {
		sortRecords(ordered, p.OrderBy, p.Order)
	}

	// Recorte de la página.
	start := (page - 1) * perPage
	if start < 0 {
		start = 0
	}
	var pageRecords []Record
	if start < len(ordered) {
		end := start + perPage
		if end > len(ordered) {
			end = len(ordered)
		}
		pageRecords = ordered[start:end]
	} else {
		pageRecords = []Record{}
	}

	meta := ott.Pagination{
		TotalRecords:   total,
		Page:           page,
		RecordsPerPage: perPage,
		TotalPages:     totalPages,
	}
	return pageRecords, meta
}

// FilterByField devuelve los registros cuyo campo `field` equivale a `value`
// (comparación como string, insensible a espacios). Es la base de los filtros
// propios del BFF (p. ej. customers por customer_id, que OTT no soporta). Un
// value vacío devuelve el conjunto sin filtrar.
func FilterByField(records []Record, field, value string) []Record {
	target := strings.TrimSpace(value)
	if target == "" {
		return records
	}
	out := make([]Record, 0)
	for _, r := range records {
		if r == nil {
			continue
		}
		if strings.TrimSpace(fieldAsString(r[field])) == target {
			out = append(out, r)
		}
	}
	return out
}

// FilterByFieldContains devuelve los registros cuyo campo `field` CONTIENE
// `value` (subcadena, insensible a mayúsculas y a espacios de relleno). Útil
// para búsquedas por nombre u otros campos de texto libre. Un value vacío
// devuelve el conjunto sin filtrar.
func FilterByFieldContains(records []Record, field, value string) []Record {
	target := strings.ToLower(strings.TrimSpace(value))
	if target == "" {
		return records
	}
	out := make([]Record, 0)
	for _, r := range records {
		if r == nil {
			continue
		}
		haystack := strings.ToLower(strings.TrimSpace(fieldAsString(r[field])))
		if strings.Contains(haystack, target) {
			out = append(out, r)
		}
	}
	return out
}

// sortRecords ordena in situ por el campo dado. Compara numéricamente si ambos
// valores son numéricos; en caso contrario compara como string. "desc" invierte
// el orden; cualquier otro valor de order se trata como ascendente. El orden es
// estable para que registros con igual clave conserven su posición relativa.
func sortRecords(records []Record, field, order string) {
	desc := strings.EqualFold(strings.TrimSpace(order), "desc")
	sort.SliceStable(records, func(i, j int) bool {
		less := recordLess(records[i], records[j], field)
		if desc {
			// Para desc invertimos, pero mantenemos estabilidad: si no es "menor"
			// ni "mayor" (iguales), SliceStable preserva el orden original.
			return recordLess(records[j], records[i], field)
		}
		return less
	})
}

// recordLess indica si a < b comparando el campo `field`. Numérico si ambos son
// números; si no, comparación lexicográfica de su representación string.
func recordLess(a, b Record, field string) bool {
	av, aok := fieldAsFloat(a[field])
	bv, bok := fieldAsFloat(b[field])
	if aok && bok {
		return av < bv
	}
	return fieldAsString(a[field]) < fieldAsString(b[field])
}

// fieldAsString convierte un valor JSON genérico a string para comparar/ordenar.
func fieldAsString(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		return strings.TrimSpace(sprint(v))
	}
}

// fieldAsFloat intenta interpretar un valor JSON como número (float64). Devuelve
// (0, false) si no es numérico. Los números JSON decodificados por encoding/json
// llegan como float64.
func fieldAsFloat(v interface{}) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	default:
		return 0, false
	}
}

// sprint formatea un valor genérico a string de forma estable para ordenar
// valores no escalares (p. ej. objetos/arreglos anidados), caso poco común en
// los campos usados para ordenar.
func sprint(v interface{}) string {
	return fmt.Sprintf("%v", v)
}
