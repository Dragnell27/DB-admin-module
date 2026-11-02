// Package validation contiene la validación server-side espejo del admin-backend
// (BFF). Replica las reglas que el backend OTT aplica, para no depender únicamente
// de la validación del front (Req. 20.2, 20.5).
package validation

import (
	"errors"
	"fmt"
)

// Límites y valores por defecto de paginación según el contrato OTT y el diseño:
// `records_per_page` máximo 100, `page > 0` (design.md, Req. 6.3, 9.3, 13.4, 20.5).
const (
	// MaxRecordsPerPage es el máximo permitido de registros por página.
	MaxRecordsPerPage = 100
	// MinPage es el menor número de página aceptado (las páginas inician en 1).
	MinPage = 1
	// DefaultPage se aplica cuando no se especifica un número de página.
	DefaultPage = 1
	// DefaultRecordsPerPage se aplica cuando no se especifica un tamaño de página.
	DefaultRecordsPerPage = 20
)

// Errores de validación de paginación. Se exponen como variables para que los
// handlers y las pruebas puedan compararlos con errors.Is.
var (
	// ErrInvalidPage indica que `page` no es mayor que 0.
	ErrInvalidPage = errors.New("page debe ser mayor que 0")
	// ErrInvalidRecordsPerPage indica que `records_per_page` está fuera de rango.
	ErrInvalidRecordsPerPage = fmt.Errorf("records_per_page debe estar entre 1 y %d", MaxRecordsPerPage)
)

// Pagination representa parámetros de paginación ya validados y normalizados.
type Pagination struct {
	Page           int
	RecordsPerPage int
}

// ValidatePagination valida los parámetros de paginación `page` y `records_per_page`.
//
// Acepta la petición si y solo si `page > 0` y `records_per_page <= 100`
// (Property 5, Req. 6.3, 9.3, 13.4, 20.5). Un `records_per_page` de 0 o negativo
// también se rechaza, ya que no representa un tamaño de página válido.
//
// Devuelve la paginación validada (los mismos valores de entrada) o un error de
// validación describiendo el primer parámetro inválido encontrado.
func ValidatePagination(page, recordsPerPage int) (Pagination, error) {
	if page < MinPage {
		return Pagination{}, ErrInvalidPage
	}
	if recordsPerPage < 1 || recordsPerPage > MaxRecordsPerPage {
		return Pagination{}, ErrInvalidRecordsPerPage
	}
	return Pagination{Page: page, RecordsPerPage: recordsPerPage}, nil
}

// NormalizePagination aplica valores por defecto a parámetros ausentes y luego
// valida el resultado.
//
// Un valor de 0 se interpreta como "no especificado" y se reemplaza por el valor
// por defecto correspondiente (`DefaultPage`, `DefaultRecordsPerPage`). Cualquier
// valor negativo, o un `records_per_page` que exceda el máximo, se considera
// inválido y produce un error de validación.
//
// Esto permite que los handlers pasen directamente los valores extraídos de la
// query (0 cuando el parámetro no viene) y obtengan parámetros listos para usar.
func NormalizePagination(page, recordsPerPage int) (Pagination, error) {
	if page == 0 {
		page = DefaultPage
	}
	if recordsPerPage == 0 {
		recordsPerPage = DefaultRecordsPerPage
	}
	return ValidatePagination(page, recordsPerPage)
}
