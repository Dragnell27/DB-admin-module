// Feature: panel-admin-ott, Property 5: La validación de paginación rechaza parámetros fuera de rango
//
// Prueba basada en propiedades (PBT) de la validación de paginación del
// admin-backend (BFF).
//
// Validates: Requirements 6.3, 9.3, 13.4, 20.5
//
// Propiedad: *para todo* par de enteros `page` y `recordsPerPage`,
// ValidatePagination devuelve un resultado válido (sin error) SI Y SOLO SI
// `page > 0` Y `1 <= recordsPerPage <= 100`. En cualquier otro caso devuelve el
// error correspondiente: ErrInvalidPage cuando `page` está fuera de rango
// (evaluado primero), o ErrInvalidRecordsPerPage cuando `page` es válido pero
// `recordsPerPage` está fuera de rango.
//
// Se usa la librería estándar `testing/quick` para generar enteros arbitrarios
// que cubren un amplio espectro: negativos, cero, los límites 1/100/101 y
// valores grandes. La propiedad debe mantenerse para todos.
package validation

import (
	"errors"
	"math/rand"
	"reflect"
	"testing"
	"testing/quick"
)

// pagPBTIterations es el número mínimo de iteraciones exigido por las
// convenciones de PBT del proyecto (mínimo 100).
const pagPBTIterations = 300

// pagPBTInt envuelve un entero y actúa como generador para `testing/quick`,
// sesgando los valores hacia la frontera relevante (negativos, cero, 1, 100,
// 101) además de valores grandes, para ejercitar los casos límite con mayor
// frecuencia que una distribución uniforme.
type pagPBTInt struct {
	v int
}

// Generate satisface la interfaz quick.Generator.
func (pagPBTInt) Generate(rnd *rand.Rand, size int) reflect.Value {
	// Valores frontera concretos que nos interesa muestrear con frecuencia.
	boundary := []int{
		-size - 1, -100, -1, 0, 1, 2, 19, 20, 99, 100, 101, 1000, size + 1,
	}

	var v int
	switch rnd.Intn(3) {
	case 0:
		// Caso frontera determinista.
		v = boundary[rnd.Intn(len(boundary))]
	case 1:
		// Rango pequeño alrededor del cero (incluye negativos).
		v = rnd.Intn(205) - 102 // [-102, 102]
	default:
		// Valores arbitrarios, posiblemente grandes y de cualquier signo.
		v = rnd.Int() - rnd.Int()
	}

	return reflect.ValueOf(pagPBTInt{v: v})
}

// pagPBTValidInput indica si el par (page, recordsPerPage) debe ser aceptado
// según la especificación: page > 0 y 1 <= recordsPerPage <= 100.
func pagPBTValidInput(page, recordsPerPage int) bool {
	return page >= MinPage && recordsPerPage >= 1 && recordsPerPage <= MaxRecordsPerPage
}

// TestValidatePaginationProperty5 verifica la Property 5: ValidatePagination
// acepta un par de parámetros si y solo si están dentro de rango, y en caso
// contrario devuelve el error esperado.
func TestValidatePaginationProperty5(t *testing.T) {
	property := func(pageGen, rppGen pagPBTInt) bool {
		page, recordsPerPage := pageGen.v, rppGen.v

		got, err := ValidatePagination(page, recordsPerPage)
		valid := pagPBTValidInput(page, recordsPerPage)

		if valid {
			// Entrada válida: sin error y la paginación refleja la entrada.
			if err != nil {
				return false
			}
			return got.Page == page && got.RecordsPerPage == recordsPerPage
		}

		// Entrada inválida: debe haber error y la paginación debe ser cero.
		if err == nil {
			return false
		}
		if got != (Pagination{}) {
			return false
		}

		// El error debe ser el apropiado: `page` se evalúa primero, por lo que
		// un `page` fuera de rango produce ErrInvalidPage; de lo contrario, el
		// problema está en `recordsPerPage` y produce ErrInvalidRecordsPerPage.
		if page < MinPage {
			return errors.Is(err, ErrInvalidPage)
		}
		return errors.Is(err, ErrInvalidRecordsPerPage)
	}

	cfg := &quick.Config{MaxCount: pagPBTIterations}
	if err := quick.Check(property, cfg); err != nil {
		t.Fatalf("Property 5 violada: la validación de paginación no respeta el rango esperado: %v", err)
	}
}

// TestValidatePaginationProperty5KnownBoundaries complementa la generación
// aleatoria con casos frontera deterministas explícitos.
func TestValidatePaginationProperty5KnownBoundaries(t *testing.T) {
	cases := []struct {
		name           string
		page           int
		recordsPerPage int
		wantErr        error // nil => se espera éxito
	}{
		{"página y tamaño mínimos válidos", 1, 1, nil},
		{"tamaño máximo válido", 1, 100, nil},
		{"página grande y tamaño medio", 1000, 50, nil},
		{"página cero inválida", 0, 20, ErrInvalidPage},
		{"página negativa inválida", -1, 20, ErrInvalidPage},
		{"records_per_page cero inválido", 1, 0, ErrInvalidRecordsPerPage},
		{"records_per_page negativo inválido", 1, -5, ErrInvalidRecordsPerPage},
		{"records_per_page por encima del máximo", 1, 101, ErrInvalidRecordsPerPage},
		{"page inválido tiene prioridad sobre rpp inválido", 0, 101, ErrInvalidPage},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ValidatePagination(tc.page, tc.recordsPerPage)

			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("ValidatePagination(%d, %d) error = %v; se esperaba nil", tc.page, tc.recordsPerPage, err)
				}
				if got.Page != tc.page || got.RecordsPerPage != tc.recordsPerPage {
					t.Errorf("ValidatePagination(%d, %d) = %#v; se esperaba reflejar la entrada", tc.page, tc.recordsPerPage, got)
				}
				return
			}

			if !errors.Is(err, tc.wantErr) {
				t.Errorf("ValidatePagination(%d, %d) error = %v; se esperaba %v", tc.page, tc.recordsPerPage, err, tc.wantErr)
			}
			if got != (Pagination{}) {
				t.Errorf("ValidatePagination(%d, %d) = %#v; se esperaba Pagination cero en caso de error", tc.page, tc.recordsPerPage, got)
			}
		})
	}
}
