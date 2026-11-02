package validation

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Resource identifica el recurso OTT al que aplican las reglas de ordenamiento.
// Cada recurso define su propio conjunto de campos permitidos para `order_by`
// (design.md: "order_by/order restringidos a campos permitidos por recurso",
// Req. 6.4, 9.4).
type Resource string

const (
	ResourceCustomers     Resource = "customers"
	ResourcePlans         Resource = "plans"
	ResourceSubscriptions Resource = "subscriptions"
	ResourceCards         Resource = "cards"
)

// Direcciones de ordenamiento válidas (`order ∈ {asc, desc}`, Req. 6.4, 9.4).
const (
	OrderAsc  = "asc"
	OrderDesc = "desc"
)

// DefaultOrderBy es el campo de ordenamiento por defecto cuando no se especifica
// uno. El contrato OTT usa `created_at` como campo de orden por defecto.
const DefaultOrderBy = "created_at"

// DefaultOrder es la dirección de ordenamiento por defecto cuando no se especifica.
const DefaultOrder = OrderDesc

// Errores de validación de ordenamiento.
var (
	// ErrUnknownResource indica que el recurso no tiene campos de orden definidos.
	ErrUnknownResource = errors.New("recurso desconocido para ordenamiento")
	// ErrInvalidOrderBy indica que `order_by` no está en el conjunto permitido.
	ErrInvalidOrderBy = errors.New("order_by no está permitido para este recurso")
	// ErrInvalidOrderDirection indica que `order` no es `asc` ni `desc`.
	ErrInvalidOrderDirection = fmt.Errorf("order debe ser %q o %q", OrderAsc, OrderDesc)
)

// allowedOrderFields define, por recurso, el conjunto de campos permitidos para
// `order_by`. Los campos derivan del contrato OTT y de los DTOs de cada recurso;
// `created_at` está permitido en todos los recursos como campo de orden base.
var allowedOrderFields = map[Resource]map[string]struct{}{
	ResourceCustomers: setOf(
		"created_at", "updated_at", "first_name", "last_name", "email", "external_id",
	),
	ResourcePlans: setOf(
		"created_at", "updated_at", "name", "external_id", "amount", "country",
	),
	ResourceSubscriptions: setOf(
		"created_at", "updated_at", "customer_id", "plan_id", "status",
		"subscription_start_date",
	),
	ResourceCards: setOf(
		"created_at", "brand", "last4",
	),
}

// Ordering representa parámetros de ordenamiento ya validados y normalizados.
type Ordering struct {
	OrderBy string
	Order   string
}

// AllowedOrderFields devuelve, en orden alfabético, los campos de `order_by`
// permitidos para un recurso. Es útil para construir mensajes de error y para
// las pruebas. Devuelve nil si el recurso no está definido.
func AllowedOrderFields(resource Resource) []string {
	fields, ok := allowedOrderFields[resource]
	if !ok {
		return nil
	}
	out := make([]string, 0, len(fields))
	for f := range fields {
		out = append(out, f)
	}
	sort.Strings(out)
	return out
}

// IsAllowedOrderField indica si `orderBy` pertenece al conjunto permitido del
// recurso dado.
func IsAllowedOrderField(resource Resource, orderBy string) bool {
	fields, ok := allowedOrderFields[resource]
	if !ok {
		return false
	}
	_, allowed := fields[orderBy]
	return allowed
}

// ValidateOrdering valida los parámetros de ordenamiento de un recurso.
//
// La petición se acepta si y solo si `order_by` pertenece al conjunto permitido
// del recurso y `order ∈ {asc, desc}` (Property 6, Req. 6.4, 9.4). La dirección
// `order` se compara sin distinción de mayúsculas y se normaliza a minúsculas en
// el resultado; `order_by` debe coincidir exactamente con un campo permitido.
//
// Si el recurso no tiene campos de orden definidos, devuelve ErrUnknownResource.
func ValidateOrdering(resource Resource, orderBy, order string) (Ordering, error) {
	fields, ok := allowedOrderFields[resource]
	if !ok {
		return Ordering{}, ErrUnknownResource
	}
	if _, allowed := fields[orderBy]; !allowed {
		return Ordering{}, ErrInvalidOrderBy
	}
	normalizedOrder := strings.ToLower(order)
	if normalizedOrder != OrderAsc && normalizedOrder != OrderDesc {
		return Ordering{}, ErrInvalidOrderDirection
	}
	return Ordering{OrderBy: orderBy, Order: normalizedOrder}, nil
}

// NormalizeOrdering aplica valores por defecto a parámetros de ordenamiento
// ausentes y luego valida el resultado.
//
// Un `orderBy` vacío se reemplaza por `DefaultOrderBy` y un `order` vacío por
// `DefaultOrder`. Cualquier valor presente pero inválido produce un error de
// validación. Esto permite que los handlers pasen directamente los valores de la
// query (cadena vacía cuando el parámetro no viene).
func NormalizeOrdering(resource Resource, orderBy, order string) (Ordering, error) {
	if strings.TrimSpace(orderBy) == "" {
		orderBy = DefaultOrderBy
	}
	if strings.TrimSpace(order) == "" {
		order = DefaultOrder
	}
	return ValidateOrdering(resource, orderBy, order)
}

// setOf construye un conjunto (map[string]struct{}) a partir de una lista de
// campos. Función auxiliar interna para declarar los campos permitidos.
func setOf(fields ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(fields))
	for _, f := range fields {
		set[f] = struct{}{}
	}
	return set
}
