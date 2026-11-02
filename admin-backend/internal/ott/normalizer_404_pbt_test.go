// Feature: panel-admin-ott, Property 1: El 404 de listas se normaliza como "sin resultados"
//
// Prueba basada en propiedades (PBT) del normalizador de respuesta OTT.
//
// Validates: Requirements 6.5, 9.5, 13.5, 17.3
//
// Propiedad: *para toda* respuesta OTT de un endpoint de listado con código
// HTTP 404, el normalizador produce un resultado exitoso (OK=true) con una
// colección vacía y no-nula como Data, metadatos de paginación en cero
// (coherentes, no-nulos) y sin señalar error (ErrorCode vacío). El 404 nunca
// se trata como un fallo.
//
// Se usa la librería estándar `testing/quick` para generar cuerpos de respuesta
// arbitrarios: JSON válido (ambas variantes del envoltorio OTT), JSON inválido,
// cuerpos vacíos y bytes aleatorios. La propiedad debe mantenerse para todos.
package ott

import (
	"encoding/json"
	"math/rand"
	"net/http"
	"reflect"
	"testing"
	"testing/quick"
)

// pbtIterations es el número mínimo de iteraciones exigido por las convenciones
// de PBT del proyecto (mínimo 100).
const pbtIterations = 300

// bodyKind enumera las distintas familias de cuerpos que generamos para cubrir
// el espacio de entrada del normalizador (JSON válido variante A/B, JSON
// inválido, vacío y bytes crudos aleatorios).
type bodyKind int

const (
	kindEmpty bodyKind = iota
	kindRandomBytes
	kindInvalidJSON
	kindVariantA // resultCode/resultMessage/errorCode/result
	kindVariantB // status/http_code/message/data
	kindArbitraryJSON
	numBodyKinds
)

// arbitraryBody implementa quick.Generator para producir cuerpos de respuesta
// representativos y variados: válidos, inválidos, vacíos y aleatorios.
type arbitraryBody struct {
	bytes []byte
}

// Generate satisface la interfaz quick.Generator.
func (arbitraryBody) Generate(rnd *rand.Rand, size int) reflect.Value {
	kind := bodyKind(rnd.Intn(int(numBodyKinds)))

	var b []byte
	switch kind {
	case kindEmpty:
		// Alterna entre nil y slice vacío para cubrir ambos casos.
		if rnd.Intn(2) == 0 {
			b = nil
		} else {
			b = []byte{}
		}

	case kindRandomBytes:
		n := rnd.Intn(size + 1)
		b = make([]byte, n)
		for i := range b {
			b[i] = byte(rnd.Intn(256))
		}

	case kindInvalidJSON:
		// Fragmentos que parecen JSON pero están malformados.
		fragments := []string{
			"{", "}", "[", "]", `{"result":`, `{"data": [`,
			`{"resultCode": }`, `not json at all`, `{,}`, `[1,2,`,
			`{"status": "ok",}`, `"unterminated`, `{"page": }`,
		}
		b = []byte(fragments[rnd.Intn(len(fragments))])

	case kindVariantA:
		// Variante A, opcionalmente con metadatos de paginación embebidos.
		env := map[string]interface{}{
			"resultCode":    randString(rnd, 6),
			"resultMessage": randString(rnd, 10),
			"errorCode":     randString(rnd, 5),
		}
		env["result"] = randResultPayload(rnd)
		b, _ = json.Marshal(env)

	case kindVariantB:
		env := map[string]interface{}{
			"status":    randString(rnd, 4),
			"http_code": http.StatusNotFound,
			"message":   randString(rnd, 10),
		}
		env["data"] = randResultPayload(rnd)
		b, _ = json.Marshal(env)

	case kindArbitraryJSON:
		// JSON válido pero sin la forma del envoltorio OTT.
		shapes := []interface{}{
			map[string]interface{}{randString(rnd, 4): rnd.Intn(100)},
			[]interface{}{1, "a", true, nil},
			rnd.Intn(1000),
			randString(rnd, 8),
			true,
			nil,
		}
		b, _ = json.Marshal(shapes[rnd.Intn(len(shapes))])

	default:
		b = nil
	}

	return reflect.ValueOf(arbitraryBody{bytes: b})
}

// randResultPayload genera una carga útil `result`/`data` que a veces incluye
// metadatos de paginación (para asegurar que, aun así, el 404 los ignora y
// devuelve paginación en cero) y a veces una colección o un objeto arbitrario.
func randResultPayload(rnd *rand.Rand) interface{} {
	switch rnd.Intn(4) {
	case 0:
		return map[string]interface{}{
			"totalRecords":   rnd.Intn(1000),
			"page":           rnd.Intn(50) + 1,
			"recordsPerPage": rnd.Intn(100) + 1,
			"totalPages":     rnd.Intn(50) + 1,
			"items":          []interface{}{randString(rnd, 5), randString(rnd, 5)},
		}
	case 1:
		return []interface{}{randString(rnd, 5), rnd.Intn(100)}
	case 2:
		return map[string]interface{}{randString(rnd, 4): randString(rnd, 6)}
	default:
		return nil
	}
}

func randString(rnd *rand.Rand, n int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789 _-"
	buf := make([]byte, n)
	for i := range buf {
		buf[i] = alphabet[rnd.Intn(len(alphabet))]
	}
	return string(buf)
}

// isEmptyNonNilCollection verifica que Data sea una colección vacía y no-nula.
// El normalizador devuelve []interface{}{} para un 404.
func isEmptyNonNilCollection(data interface{}) bool {
	if data == nil {
		return false
	}
	v := reflect.ValueOf(data)
	if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
		return false
	}
	return v.Len() == 0
}

// TestNormalize404AlwaysNoResults verifica la Property 1: para cualquier cuerpo
// de respuesta, un código HTTP 404 siempre se normaliza como "sin resultados".
func TestNormalize404AlwaysNoResults(t *testing.T) {
	property := func(body arbitraryBody) bool {
		got := Normalize(http.StatusNotFound, body.bytes)

		// OK=true: el 404 nunca es un fallo.
		if !got.OK {
			return false
		}
		// Data es una colección vacía y no-nula.
		if !isEmptyNonNilCollection(got.Data) {
			return false
		}
		// Paginación presente (no-nula) y en cero.
		if got.Pagination == nil {
			return false
		}
		if *got.Pagination != (Pagination{}) {
			return false
		}
		// Sin código de error: el 404 no se señala como error.
		if got.ErrorCode != "" {
			return false
		}
		return true
	}

	cfg := &quick.Config{MaxCount: pbtIterations}
	if err := quick.Check(property, cfg); err != nil {
		t.Fatalf("Property 1 violada: el 404 debe normalizarse como \"sin resultados\": %v", err)
	}
}

// TestNormalize404KnownBodies cubre explícitamente casos frontera concretos,
// complementando la generación aleatoria con ejemplos deterministas.
func TestNormalize404KnownBodies(t *testing.T) {
	cases := []struct {
		name string
		body []byte
	}{
		{"nil body", nil},
		{"empty body", []byte{}},
		{"invalid json", []byte("{not json")},
		{"variant A with pagination", []byte(`{"resultCode":"0","resultMessage":"no encontrado","result":{"totalRecords":42,"page":3,"recordsPerPage":10,"totalPages":5,"items":[1,2,3]}}`)},
		{"variant B with data array", []byte(`{"status":"error","http_code":404,"message":"not found","data":[{"id":1}]}`)},
		{"arbitrary json object", []byte(`{"foo":"bar"}`)},
		{"json array", []byte(`[1,2,3]`)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Normalize(http.StatusNotFound, tc.body)

			if !got.OK {
				t.Errorf("OK = false; se esperaba true para 404")
			}
			if !isEmptyNonNilCollection(got.Data) {
				t.Errorf("Data = %#v; se esperaba una colección vacía y no-nula", got.Data)
			}
			if got.Pagination == nil {
				t.Fatalf("Pagination = nil; se esperaba paginación en cero no-nula")
			}
			if *got.Pagination != (Pagination{}) {
				t.Errorf("Pagination = %#v; se esperaba paginación en cero", *got.Pagination)
			}
			if got.ErrorCode != "" {
				t.Errorf("ErrorCode = %q; se esperaba cadena vacía (el 404 no es error)", got.ErrorCode)
			}
		})
	}
}
