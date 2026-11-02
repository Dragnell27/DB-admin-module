// Feature: panel-admin-ott, Property 6: El ordenamiento solo admite campos permitidos por recurso
//
// Prueba basada en propiedades (PBT) de la validación de ordenamiento del BFF.
//
// Validates: Requirements 6.4, 9.4
//
// Propiedad: *para todo* recurso y todo valor de `order_by`/`order`,
// ValidateOrdering acepta la petición SI Y SOLO SI `order_by` pertenece al
// conjunto permitido del recurso (AllowedOrderFields) y `order ∈ {asc, desc}`
// comparado sin distinción de mayúsculas/minúsculas. En caso de éxito, el
// resultado conserva `order_by` tal cual y normaliza `order` a minúsculas.
//
// Se usa la librería estándar `testing/quick` para generar entradas arbitrarias:
// recursos válidos y desconocidos, `order_by` tomados tanto del conjunto
// permitido como cadenas aleatorias (muy probablemente no permitidas), y
// `order` tanto en direcciones válidas (asc/desc con mayúsculas mezcladas) como
// inválidas. La propiedad del "si y solo si" debe mantenerse para todas.
package validation

import (
	"math/rand"
	"reflect"
	"strings"
	"testing"
	"testing/quick"
)

// ordPBTIterations es el número mínimo de iteraciones exigido por las
// convenciones de PBT del proyecto (mínimo 100).
const ordPBTIterations = 300

// ordPBTKnownResources enumera los recursos con conjuntos de orden definidos.
// Se incluyen junto con recursos desconocidos en el generador para cubrir la
// rama ErrUnknownResource.
var ordPBTKnownResources = []Resource{
	ResourceCustomers,
	ResourcePlans,
	ResourceSubscriptions,
	ResourceCards,
}

// ordPBTInput es la entrada generada para una iteración de la propiedad.
type ordPBTInput struct {
	resource Resource
	orderBy  string
	order    string
}

// Generate satisface quick.Generator produciendo combinaciones variadas de
// recurso/orderBy/order que cubren tanto los casos aceptados como los rechazados.
func (ordPBTInput) Generate(rnd *rand.Rand, size int) reflect.Value {
	in := ordPBTInput{
		resource: ordPBTGenResource(rnd),
		orderBy:  "",
		order:    "",
	}
	in.orderBy = ordPBTGenOrderBy(rnd, in.resource)
	in.order = ordPBTGenOrder(rnd)
	return reflect.ValueOf(in)
}

// ordPBTGenResource devuelve un recurso conocido la mayoría de las veces y, de
// vez en cuando, un recurso desconocido para ejercitar ErrUnknownResource.
func ordPBTGenResource(rnd *rand.Rand) Resource {
	// ~1 de cada 6 veces: recurso desconocido.
	if rnd.Intn(6) == 0 {
		unknown := []Resource{
			Resource(""),
			Resource("unknown"),
			Resource("Customers"), // sensible a mayúsculas: no coincide
			Resource(ordPBTRandString(rnd, 1+rnd.Intn(8))),
		}
		return unknown[rnd.Intn(len(unknown))]
	}
	return ordPBTKnownResources[rnd.Intn(len(ordPBTKnownResources))]
}

// ordPBTGenOrderBy produce, con probabilidad similar, un campo permitido del
// recurso (si el recurso es conocido) o una cadena aleatoria casi siempre no
// permitida.
func ordPBTGenOrderBy(rnd *rand.Rand, resource Resource) string {
	allowed := AllowedOrderFields(resource)
	// Si el recurso es conocido, la mitad de las veces usamos un campo permitido.
	if len(allowed) > 0 && rnd.Intn(2) == 0 {
		return allowed[rnd.Intn(len(allowed))]
	}

	switch rnd.Intn(5) {
	case 0:
		return "" // vacío: no permitido (ValidateOrdering no aplica defaults)
	case 1:
		// Variante en mayúsculas de un campo permitido conocido: debe fallar,
		// porque order_by distingue mayúsculas.
		if len(allowed) > 0 {
			return strings.ToUpper(allowed[rnd.Intn(len(allowed))])
		}
		return ordPBTRandString(rnd, 1+rnd.Intn(10))
	case 2:
		// Campo conocido con espacios alrededor: no coincide exactamente.
		if len(allowed) > 0 {
			return " " + allowed[rnd.Intn(len(allowed))] + " "
		}
		return ordPBTRandString(rnd, 1+rnd.Intn(10))
	default:
		return ordPBTRandString(rnd, 1+rnd.Intn(12))
	}
}

// ordPBTGenOrder produce direcciones válidas (asc/desc con mayúsculas mezcladas)
// e inválidas, en proporciones mezcladas.
func ordPBTGenOrder(rnd *rand.Rand) string {
	switch rnd.Intn(6) {
	case 0:
		return "asc"
	case 1:
		return "desc"
	case 2:
		// Mayúsculas mezcladas de una dirección válida: debe aceptarse.
		return ordPBTRandomCase(rnd, []string{"asc", "desc"}[rnd.Intn(2)])
	case 3:
		return "" // vacío: inválido
	case 4:
		// Variantes cercanas pero inválidas.
		return []string{"ascending", "descending", "up", "down", "a", "d", "ASCC", "ds"}[rnd.Intn(8)]
	default:
		return ordPBTRandString(rnd, 1+rnd.Intn(6))
	}
}

// ordPBTRandomCase cambia aleatoriamente el caso de cada letra de s.
func ordPBTRandomCase(rnd *rand.Rand, s string) string {
	b := []byte(s)
	for i := range b {
		if rnd.Intn(2) == 0 {
			b[i] = byte(strings.ToUpper(string(b[i]))[0])
		}
	}
	return string(b)
}

// ordPBTRandString genera una cadena aleatoria. El alfabeto incluye letras,
// dígitos y algunos símbolos para explorar el espacio de entradas no permitidas.
func ordPBTRandString(rnd *rand.Rand, n int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-. "
	buf := make([]byte, n)
	for i := range buf {
		buf[i] = alphabet[rnd.Intn(len(alphabet))]
	}
	return string(buf)
}

// ordPBTIsValidDirection replica el predicado de dirección válida de forma
// independiente a la implementación bajo prueba: order ∈ {asc, desc} sin
// distinción de mayúsculas.
func ordPBTIsValidDirection(order string) bool {
	lower := strings.ToLower(order)
	return lower == OrderAsc || lower == OrderDesc
}

// TestValidateOrderingAllowedFieldsIFF verifica la Property 6: ValidateOrdering
// acepta la entrada si y solo si order_by está en AllowedOrderFields(resource) y
// order es una dirección válida (asc/desc, sin distinción de mayúsculas).
func TestValidateOrderingAllowedFieldsIFF(t *testing.T) {
	property := func(in ordPBTInput) bool {
		got, err := ValidateOrdering(in.resource, in.orderBy, in.order)

		// Oráculo independiente del "si y solo si".
		allowedField := IsAllowedOrderField(in.resource, in.orderBy)
		validDir := ordPBTIsValidDirection(in.order)
		shouldAccept := allowedField && validDir

		if shouldAccept {
			if err != nil {
				return false
			}
			// En éxito, order_by se conserva y order se normaliza a minúsculas.
			if got.OrderBy != in.orderBy {
				return false
			}
			if got.Order != strings.ToLower(in.order) {
				return false
			}
			if got.Order != OrderAsc && got.Order != OrderDesc {
				return false
			}
			return true
		}

		// Debe rechazar: error no-nil y Ordering cero.
		if err == nil {
			return false
		}
		return got == (Ordering{})
	}

	cfg := &quick.Config{MaxCount: ordPBTIterations}
	if err := quick.Check(property, cfg); err != nil {
		t.Fatalf("Property 6 violada: ValidateOrdering debe aceptar si y solo si order_by es permitido y order ∈ {asc,desc}: %v", err)
	}
}

// TestValidateOrderingAllAllowedFieldsAccepted recorre explícitamente cada campo
// permitido de cada recurso con ambas direcciones válidas (incluyendo mayúsculas
// mezcladas) y verifica que todos se aceptan y normalizan correctamente.
func TestValidateOrderingAllAllowedFieldsAccepted(t *testing.T) {
	directions := []struct {
		in   string
		want string
	}{
		{"asc", OrderAsc},
		{"desc", OrderDesc},
		{"ASC", OrderAsc},
		{"Desc", OrderDesc},
		{"aSc", OrderAsc},
	}

	for _, resource := range ordPBTKnownResources {
		for _, field := range AllowedOrderFields(resource) {
			for _, dir := range directions {
				got, err := ValidateOrdering(resource, field, dir.in)
				if err != nil {
					t.Errorf("ValidateOrdering(%q, %q, %q) error inesperado: %v", resource, field, dir.in, err)
					continue
				}
				if got.OrderBy != field {
					t.Errorf("ValidateOrdering(%q, %q, %q).OrderBy = %q; se esperaba %q", resource, field, dir.in, got.OrderBy, field)
				}
				if got.Order != dir.want {
					t.Errorf("ValidateOrdering(%q, %q, %q).Order = %q; se esperaba %q", resource, field, dir.in, got.Order, dir.want)
				}
			}
		}
	}
}

// TestValidateOrderingRejectsKnownBadInputs cubre casos frontera deterministas
// que deben rechazarse, complementando la generación aleatoria.
func TestValidateOrderingRejectsKnownBadInputs(t *testing.T) {
	cases := []struct {
		name     string
		resource Resource
		orderBy  string
		order    string
		wantErr  error
	}{
		{"recurso desconocido", Resource("unknown"), "created_at", "asc", ErrUnknownResource},
		{"recurso vacío", Resource(""), "created_at", "asc", ErrUnknownResource},
		{"order_by no permitido", ResourceCustomers, "ssn", "asc", ErrInvalidOrderBy},
		{"order_by vacío", ResourcePlans, "", "asc", ErrInvalidOrderBy},
		{"order_by sensible a mayúsculas", ResourceCustomers, "Email", "asc", ErrInvalidOrderBy},
		{"order_by de otro recurso", ResourceCards, "amount", "asc", ErrInvalidOrderBy},
		{"dirección inválida", ResourceCustomers, "email", "ascending", ErrInvalidOrderDirection},
		{"dirección vacía", ResourceCustomers, "email", "", ErrInvalidOrderDirection},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ValidateOrdering(tc.resource, tc.orderBy, tc.order)
			if err != tc.wantErr {
				t.Errorf("error = %v; se esperaba %v", err, tc.wantErr)
			}
			if got != (Ordering{}) {
				t.Errorf("Ordering = %#v; se esperaba el valor cero en caso de error", got)
			}
		})
	}
}
