package ottcache

import (
	"context"

	"admin-backend/internal/ott"
)

// maxPages es el límite de seguridad de páginas que LoadAllPages recorrerá para
// un recurso+país. Evita bucles infinitos si los metadatos de paginación de OTT
// son inconsistentes. Con records_per_page=100, cubre hasta 100k registros.
const maxPages = 1000

// defaultRecordsPerPage es el tamaño de página usado al recorrer OTT para
// construir el conjunto completo. Un valor alto reduce el número de llamadas.
const defaultRecordsPerPage = 100

// PageFetcher obtiene UNA página del recurso desde OTT y devuelve la respuesta
// ya normalizada. Lo implementa cada handler (sabe su ruta/consulta OTT y el
// país). `page` y `recordsPerPage` son los parámetros de paginación a enviar.
type PageFetcher func(ctx context.Context, page, recordsPerPage int) (ott.NormalizedResponse, error)

// LoadAllPages recorre todas las páginas de un recurso en OTT usando fetch y
// acumula el conjunto COMPLETO de registros. Es la base de los Loader de cada
// handler: aísla el patrón de recorrido/acumulación/corte.
//
// Reglas:
//   - Usa los metadatos de paginación (TotalPages o TotalRecords) para saber
//     cuándo terminar. Si no hay metadatos fiables, corta cuando una página
//     vuelve vacía.
//   - Un 404 de listado (OTT) llega normalizado como OK con colección vacía:
//     produce un conjunto vacío sin error.
//   - Si fetch devuelve error (red/sesión/5xx), se propaga y NO se entrega un
//     conjunto parcial (el llamador/caché no lo almacenará).
//   - Respeta maxPages como tope de seguridad.
func LoadAllPages(ctx context.Context, fetch PageFetcher) ([]Record, error) {
	all := make([]Record, 0)

	for page := 1; page <= maxPages; page++ {
		norm, err := fetch(ctx, page, defaultRecordsPerPage)
		if err != nil {
			return nil, err
		}

		records := RecordsFromData(norm.Data)
		all = append(all, records...)

		// Determinar si hay más páginas.
		if norm.Pagination != nil && norm.Pagination.TotalPages > 0 {
			if page >= norm.Pagination.TotalPages {
				break
			}
			continue
		}
		if norm.Pagination != nil && norm.Pagination.TotalRecords > 0 {
			if int64(len(all)) >= norm.Pagination.TotalRecords {
				break
			}
			// Si la página vino vacía pero aún no se alcanza el total declarado,
			// se corta igualmente para no ciclar ante metadatos inconsistentes.
			if len(records) == 0 {
				break
			}
			continue
		}

		// Sin metadatos de paginación fiables: cortar cuando la página es vacía
		// o trae menos de lo pedido (última página).
		if len(records) == 0 || len(records) < defaultRecordsPerPage {
			break
		}
	}

	return all, nil
}

// RecordsFromData extrae la colección de registros del campo Data de una
// respuesta normalizada. Tolera las dos formas en que OTT entrega listados:
//   - un arreglo directo (`[]interface{}`), o
//   - un objeto que contiene la colección bajo alguna clave de arreglo (p. ej.
//     `{ "customers": [...], "totalRecords": N }`), junto con metadatos.
//
// Devuelve siempre un slice no nil (vacío si no se reconoce una colección).
func RecordsFromData(data interface{}) []Record {
	switch v := data.(type) {
	case nil:
		return []Record{}
	case []interface{}:
		return toRecords(v)
	case map[string]interface{}:
		// Buscar el primer valor que sea un arreglo: esa es la colección del
		// listado. Los metadatos de paginación son escalares, así que no
		// colisionan con esta heurística.
		for _, val := range v {
			if arr, ok := val.([]interface{}); ok {
				return toRecords(arr)
			}
		}
		return []Record{}
	default:
		return []Record{}
	}
}

// toRecords convierte un arreglo genérico en []Record, descartando los elementos
// que no sean objetos JSON.
func toRecords(arr []interface{}) []Record {
	out := make([]Record, 0, len(arr))
	for _, item := range arr {
		if obj, ok := item.(map[string]interface{}); ok {
			out = append(out, obj)
		}
	}
	return out
}
