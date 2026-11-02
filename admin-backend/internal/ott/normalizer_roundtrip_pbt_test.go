// Feature: panel-admin-ott, Property 2: Round-trip de normalización de respuestas estándar
//
// Prueba basada en propiedades (PBT) del normalizador de respuesta OTT.
//
// Validates: Requirements 20.3, 20.4
//
// Propiedad: *para toda* respuesta OTT bien formada en cualquiera de las dos
// variantes del envoltorio estándar —variante A (`resultCode/resultMessage/
// errorCode/result`) o variante B (`status/http_code/message/data`)— con un
// código HTTP exitoso (2xx), al normalizarla el BFF produce un envoltorio
// uniforme `NormalizedResponse` que **preserva sin pérdida** la carga útil
// subyacente (bajo `Data`) y los metadatos de paginación presentes en la
// entrada (`totalRecords`, `page`, `recordsPerPage`, `totalPages`) bajo
// `Pagination`, además de preservar el mensaje legible.
//
// Se usa la librería estándar `testing/quick` para generar envoltorios OTT
// bien formados y variados (variante A y B; cargas útiles que son colecciones,
// objetos o escalares; con y sin metadatos de paginación embebidos). La
// propiedad debe mantenerse para todas las entradas generadas.
//
// Nota de oráculo: el valor esperado para `Data` se obtiene decodificando la
// MISMA carga útil con `encoding/json` a un valor genérico de Go. Así se
// compara "lo que entró" contra "lo que salió" tras el round-trip JSON del
// normalizador: la igualdad confirma que no se pierde ni se altera información.
package ott

import (
	"encoding/json"
	"math/rand"
	"net/http"
	"reflect"
	"testing"
	"testing/quick"
)

// roundTripIterations es el número de iteraciones de la PBT (mínimo 100 por las
// convenciones del proyecto).
const roundTripIterations = 300

// envelopeVariant distingue las dos formas de la respuesta estándar OTT.
type envelopeVariant int

const (
	variantA envelopeVariant = iota // resultCode/resultMessage/errorCode/result
	variantB                        // status/http_code/message/data
	numVariants
)

// successStatusCodes son códigos HTTP 2xx representativos usados para generar
// respuestas exitosas (el 404 se cubre en la Property 1, no aquí).
var successStatusCodes = []int{
	http.StatusOK,
	http.StatusCreated,
	http.StatusAccepted,
	http.StatusNoContent, // 204: aun así el cuerpo puede traer datos en OTT
}

// wellFormedResponse representa una respuesta OTT bien formada ya materializada:
// el cuerpo crudo (JSON del envoltorio), el código HTTP exitoso, y los valores
// "esperados" que el normalizador debe reproducir (payload decodificado,
// paginación embebida —si la hay— y el mensaje).
type wellFormedResponse struct {
	statusCode int
	body       []byte

	// expectedData es la carga útil decodificada a valor genérico de Go.
	expectedData interface{}
	// expectedPagination son los metadatos de paginación esperados, o nil si la
	// carga útil no embebe metadatos de paginación.
	expectedPagination *Pagination
	// expectedMessage es el mensaje legible esperado (resultMessage o message).
	expectedMessage string
}

// Generate satisface quick.Generator produciendo respuestas OTT bien formadas.
func (wellFormedResponse) Generate(rnd *rand.Rand, size int) reflect.Value {
	variant := envelopeVariant(rnd.Intn(int(numVariants)))
	status := successStatusCodes[rnd.Intn(len(successStatusCodes))]

	// Construir la carga útil (`result`/`data`) y, si procede, los metadatos de
	// paginación que quedarán embebidos en ella.
	payload, pagination := genPayload(rnd)

	message := roundTripRandString(rnd, 1+rnd.Intn(16))
	errorCode := roundTripRandString(rnd, rnd.Intn(6)) // puede ser vacío

	env := map[string]interface{}{}
	switch variant {
	case variantA:
		env["resultCode"] = roundTripRandString(rnd, 1+rnd.Intn(4))
		env["resultMessage"] = message
		env["errorCode"] = errorCode
		env["result"] = payload
	case variantB:
		env["status"] = roundTripRandString(rnd, 1+rnd.Intn(4))
		env["http_code"] = status
		env["message"] = message
		env["data"] = payload
	}

	body, err := json.Marshal(env)
	if err != nil {
		// Un envoltorio bien formado siempre debe serializar; si no, degradar a
		// un cuerpo vacío (no debería ocurrir con los generadores de arriba).
		body = []byte{}
	}

	// El valor esperado de Data es la carga útil decodificada igual que lo hace
	// el normalizador (JSON -> valor genérico de Go), para una comparación justa.
	wfr := wellFormedResponse{
		statusCode:         status,
		body:               body,
		expectedData:       decodeExpectedPayload(payload),
		expectedPagination: pagination,
		expectedMessage:    message,
	}
	return reflect.ValueOf(wfr)
}

// genPayload genera una carga útil `result`/`data` bien formada y devuelve
// además los metadatos de paginación esperados (nil si no se embeben).
//
// El normalizador solo expone `Pagination` cuando la carga útil es un objeto
// JSON que contiene al menos uno de los campos de paginación. Para reflejar
// fielmente ese comportamiento, cuando generamos un objeto con paginación
// devolvemos los metadatos esperados; en el resto de los casos devolvemos nil.
func genPayload(rnd *rand.Rand) (payload interface{}, pagination *Pagination) {
	switch rnd.Intn(5) {
	case 0:
		// Objeto de listado con metadatos de paginación completos + colección.
		tr := int64(rnd.Intn(100000))
		page := rnd.Intn(500) + 1
		rpp := rnd.Intn(100) + 1
		tp := rnd.Intn(500) + 1
		obj := map[string]interface{}{
			"totalRecords":   tr,
			"page":           page,
			"recordsPerPage": rpp,
			"totalPages":     tp,
			"items":          genCollection(rnd),
			// Un campo arbitrario adicional para asegurar que también se preserva.
			roundTripRandString(rnd, 4): roundTripRandString(rnd, 6),
		}
		return obj, &Pagination{
			TotalRecords:   tr,
			Page:           page,
			RecordsPerPage: rpp,
			TotalPages:     tp,
		}
	case 1:
		// Objeto de listado con paginación parcial (solo algunos campos). El
		// normalizador deja en cero los ausentes.
		obj := map[string]interface{}{}
		expected := &Pagination{}
		if rnd.Intn(2) == 0 {
			tr := int64(rnd.Intn(100000))
			obj["totalRecords"] = tr
			expected.TotalRecords = tr
		}
		if rnd.Intn(2) == 0 {
			page := rnd.Intn(500) + 1
			obj["page"] = page
			expected.Page = page
		}
		// Garantizar que haya AL MENOS un campo de paginación para que el
		// normalizador la reconozca; si por azar ninguno se añadió, forzar uno.
		if len(obj) == 0 {
			rpp := rnd.Intn(100) + 1
			obj["recordsPerPage"] = rpp
			expected.RecordsPerPage = rpp
		}
		obj["items"] = genCollection(rnd)
		return obj, expected
	case 2:
		// Colección pura (arreglo), sin metadatos de paginación.
		return genCollection(rnd), nil
	case 3:
		// Objeto de detalle sin ningún campo de paginación.
		obj := map[string]interface{}{
			"id":    roundTripRandString(rnd, 8),
			"name":  roundTripRandString(rnd, 10),
			"count": rnd.Intn(1000),
			"flag":  rnd.Intn(2) == 0,
		}
		return obj, nil
	default:
		// Escalar simple (cadena o número), sin paginación.
		if rnd.Intn(2) == 0 {
			return roundTripRandString(rnd, 1+rnd.Intn(12)), nil
		}
		return rnd.Intn(100000), nil
	}
}

// genCollection crea una colección arbitraria de elementos heterogéneos.
func genCollection(rnd *rand.Rand) []interface{} {
	n := rnd.Intn(5)
	out := make([]interface{}, 0, n)
	for i := 0; i < n; i++ {
		switch rnd.Intn(4) {
		case 0:
			out = append(out, roundTripRandString(rnd, 6))
		case 1:
			out = append(out, rnd.Intn(10000))
		case 2:
			out = append(out, map[string]interface{}{
				"k": roundTripRandString(rnd, 5),
				"v": rnd.Intn(100),
			})
		default:
			out = append(out, rnd.Intn(2) == 0)
		}
	}
	return out
}

// decodeExpectedPayload reproduce la decodificación que hace el normalizador
// (`decodeGeneric`): serializa la carga útil a JSON y la decodifica a un valor
// genérico de Go, de modo que números -> float64, objetos -> map[string]..., etc.
// Así el oráculo compara exactamente la forma que produce `Normalize`.
func decodeExpectedPayload(payload interface{}) interface{} {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	return v
}

func roundTripRandString(rnd *rand.Rand, n int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-"
	if n <= 0 {
		return ""
	}
	buf := make([]byte, n)
	for i := range buf {
		buf[i] = alphabet[rnd.Intn(len(alphabet))]
	}
	return string(buf)
}

// TestNormalizeRoundTripPreservesDataAndPagination verifica la Property 2:
// para toda respuesta OTT bien formada (variante A o B) con código 2xx, la
// normalización preserva la carga útil y la paginación sin pérdida.
func TestNormalizeRoundTripPreservesDataAndPagination(t *testing.T) {
	property := func(wfr wellFormedResponse) bool {
		got := Normalize(wfr.statusCode, wfr.body)

		// OK=true para respuestas 2xx.
		if !got.OK {
			return false
		}

		// Data preserva la carga útil subyacente sin pérdida.
		if !reflect.DeepEqual(got.Data, wfr.expectedData) {
			return false
		}

		// Paginación: se expone exactamente si (y solo si) la entrada la embebe.
		if wfr.expectedPagination == nil {
			if got.Pagination != nil {
				return false
			}
		} else {
			if got.Pagination == nil {
				return false
			}
			if *got.Pagination != *wfr.expectedPagination {
				return false
			}
		}

		// El mensaje legible se preserva.
		if got.Message != wfr.expectedMessage {
			return false
		}

		// En respuestas exitosas no se propaga un código de error.
		if got.ErrorCode != "" {
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: roundTripIterations}
	if err := quick.Check(property, cfg); err != nil {
		t.Fatalf("Property 2 violada: la normalización debe preservar datos y paginación sin pérdida: %v", err)
	}
}

// TestNormalizeRoundTripKnownEnvelopes complementa la generación aleatoria con
// ejemplos deterministas concretos de ambas variantes.
func TestNormalizeRoundTripKnownEnvelopes(t *testing.T) {
	cases := []struct {
		name            string
		statusCode      int
		body            []byte
		wantData        interface{}
		wantPagination  *Pagination
		wantMessage     string
	}{
		{
			name:       "variante A listado con paginación",
			statusCode: http.StatusOK,
			body:       []byte(`{"resultCode":"0","resultMessage":"ok","errorCode":"","result":{"totalRecords":42,"page":3,"recordsPerPage":10,"totalPages":5,"items":[{"id":"a"},{"id":"b"}]}}`),
			wantData: map[string]interface{}{
				"totalRecords":   float64(42),
				"page":           float64(3),
				"recordsPerPage": float64(10),
				"totalPages":     float64(5),
				"items": []interface{}{
					map[string]interface{}{"id": "a"},
					map[string]interface{}{"id": "b"},
				},
			},
			wantPagination: &Pagination{TotalRecords: 42, Page: 3, RecordsPerPage: 10, TotalPages: 5},
			wantMessage:    "ok",
		},
		{
			name:       "variante B listado con paginación",
			statusCode: http.StatusOK,
			body:       []byte(`{"status":"success","http_code":200,"message":"hecho","data":{"totalRecords":7,"page":1,"recordsPerPage":25,"totalPages":1,"items":["x"]}}`),
			wantData: map[string]interface{}{
				"totalRecords":   float64(7),
				"page":           float64(1),
				"recordsPerPage": float64(25),
				"totalPages":     float64(1),
				"items":          []interface{}{"x"},
			},
			wantPagination: &Pagination{TotalRecords: 7, Page: 1, RecordsPerPage: 25, TotalPages: 1},
			wantMessage:    "hecho",
		},
		{
			name:           "variante A detalle sin paginación",
			statusCode:     http.StatusCreated,
			body:           []byte(`{"resultCode":"0","resultMessage":"creado","result":{"id":"c1","email":"a@b.com"}}`),
			wantData:       map[string]interface{}{"id": "c1", "email": "a@b.com"},
			wantPagination: nil,
			wantMessage:    "creado",
		},
		{
			name:           "variante B colección pura sin paginación",
			statusCode:     http.StatusOK,
			body:           []byte(`{"status":"success","http_code":200,"message":"lista","data":[1,2,3]}`),
			wantData:       []interface{}{float64(1), float64(2), float64(3)},
			wantPagination: nil,
			wantMessage:    "lista",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Normalize(tc.statusCode, tc.body)

			if !got.OK {
				t.Errorf("OK = false; se esperaba true para %d", tc.statusCode)
			}
			if !reflect.DeepEqual(got.Data, tc.wantData) {
				t.Errorf("Data = %#v; se esperaba %#v", got.Data, tc.wantData)
			}
			if tc.wantPagination == nil {
				if got.Pagination != nil {
					t.Errorf("Pagination = %#v; se esperaba nil", *got.Pagination)
				}
			} else {
				if got.Pagination == nil {
					t.Fatalf("Pagination = nil; se esperaba %#v", *tc.wantPagination)
				}
				if *got.Pagination != *tc.wantPagination {
					t.Errorf("Pagination = %#v; se esperaba %#v", *got.Pagination, *tc.wantPagination)
				}
			}
			if got.Message != tc.wantMessage {
				t.Errorf("Message = %q; se esperaba %q", got.Message, tc.wantMessage)
			}
			if got.ErrorCode != "" {
				t.Errorf("ErrorCode = %q; se esperaba cadena vacía en éxito", got.ErrorCode)
			}
		})
	}
}
