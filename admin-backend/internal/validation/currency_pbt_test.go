// Feature: panel-admin-ott, Property 10: Coherencia moneda↔país en Plan
//
// Prueba basada en propiedades (PBT) de la validación conjunta de `country` y
// `currency` de un Plan contra el país del JWT OTT activo
// (ValidatePlanCountryCurrency) del admin-backend (BFF).
//
// Validates: Requirements 10.4, 10.7
//
// Propiedad: *para todo* trío (`tokenCountry`, `country`, `currency`),
// ValidatePlanCountryCurrency devuelve nil (acepta la petición) SI Y SOLO SI:
//   - `country` tiene exactamente 2 letras y pertenece al conjunto soportado
//     {gt, sv, ni, hn, cr};
//   - `country` coincide, sin distinción de mayúsculas/minúsculas, con
//     `tokenCountry`, el país del JWT OTT activo (que a su vez debe ser un país
//     soportado);
//   - `currency` tiene exactamente 3 letras y coincide, sin distinción de
//     mayúsculas, con la moneda mapeada del país según `sv=USD, cr=CRC, gt=GTQ,
//     hn=HNL, ni=NIO`.
//
// Se usa la librería estándar `testing/quick` para generar tríos arbitrarios de
// un espacio realista: países soportados y no soportados, monedas que coinciden
// y que no, valores con longitud incorrecta y mezcla de mayúsculas/minúsculas.
// El oráculo del "si y solo si" se construye de forma independiente a partir de
// CurrencyForCountry, sin reutilizar la lógica interna bajo prueba.
package validation

import (
	"errors"
	"math/rand"
	"reflect"
	"strings"
	"testing"
	"testing/quick"
)

// curPBTIterations es el número mínimo de iteraciones exigido por las
// convenciones de PBT del proyecto (mínimo 100).
const curPBTIterations = 400

// curPBTSupportedCountries enumera los países soportados en minúsculas (forma
// canónica), usados tanto por el generador como por el oráculo.
var curPBTSupportedCountries = []string{"gt", "sv", "ni", "hn", "cr"}

// curPBTAllCurrencies enumera las monedas conocidas del mapeo. Se usa para
// generar monedas que a veces coinciden con el país y a veces no.
var curPBTAllCurrencies = []string{"USD", "CRC", "GTQ", "HNL", "NIO"}

// curPBTInput es la entrada generada para una iteración de la propiedad.
type curPBTInput struct {
	tokenCountry string
	country      string
	currency     string
}

// Generate satisface quick.Generator produciendo combinaciones variadas de
// tokenCountry/country/currency que cubren tanto los casos aceptados como el
// amplio espacio de rechazo.
func (curPBTInput) Generate(rnd *rand.Rand, size int) reflect.Value {
	in := curPBTInput{
		tokenCountry: curPBTGenCountry(rnd),
		country:      curPBTGenCountry(rnd),
		currency:     curPBTGenCurrency(rnd),
	}

	// Con cierta frecuencia construimos un caso "casi correcto" para ejercitar
	// la frontera de aceptación: tokenCountry y country iguales (con mayúsculas
	// mezcladas) y la moneda mapeada correspondiente.
	switch rnd.Intn(4) {
	case 0:
		base := curPBTSupportedCountries[rnd.Intn(len(curPBTSupportedCountries))]
		in.tokenCountry = curPBTRandomCase(rnd, base)
		in.country = curPBTRandomCase(rnd, base)
		if cur, ok := CurrencyForCountry(base); ok {
			in.currency = curPBTRandomCase(rnd, cur)
		}
	case 1:
		// País soportado coincidente pero moneda potencialmente equivocada.
		base := curPBTSupportedCountries[rnd.Intn(len(curPBTSupportedCountries))]
		in.tokenCountry = base
		in.country = base
		in.currency = curPBTAllCurrencies[rnd.Intn(len(curPBTAllCurrencies))]
	default:
		// Se mantienen los valores aleatorios ya asignados arriba.
	}

	return reflect.ValueOf(in)
}

// curPBTGenCountry produce un país soportado (a veces con mayúsculas mezcladas)
// la mayoría de las veces y, de vez en cuando, un valor no soportado o con
// longitud incorrecta para ejercitar las ramas de rechazo.
func curPBTGenCountry(rnd *rand.Rand) string {
	switch rnd.Intn(6) {
	case 0:
		// País no soportado de 2 letras.
		notSupported := []string{"us", "mx", "es", "br", "ar", "pa", "do"}
		return notSupported[rnd.Intn(len(notSupported))]
	case 1:
		// Longitud incorrecta (1, 3 o más letras) o con dígitos/símbolos.
		bad := []string{"g", "gtq", "", "g1", "sv ", " sv", "s v", "GTQX"}
		return bad[rnd.Intn(len(bad))]
	case 2:
		// País soportado con mayúsculas mezcladas.
		return curPBTRandomCase(rnd, curPBTSupportedCountries[rnd.Intn(len(curPBTSupportedCountries))])
	default:
		// País soportado en minúsculas.
		return curPBTSupportedCountries[rnd.Intn(len(curPBTSupportedCountries))]
	}
}

// curPBTGenCurrency produce una moneda conocida (a veces con mayúsculas
// mezcladas) la mayoría de las veces y, ocasionalmente, un valor con longitud
// incorrecta o caracteres no válidos.
func curPBTGenCurrency(rnd *rand.Rand) string {
	switch rnd.Intn(5) {
	case 0:
		// Longitud incorrecta o con caracteres no permitidos.
		bad := []string{"US", "USDD", "", "US1", "u$d", "usd ", " usd"}
		return bad[rnd.Intn(len(bad))]
	case 1:
		// Moneda conocida con mayúsculas mezcladas.
		return curPBTRandomCase(rnd, curPBTAllCurrencies[rnd.Intn(len(curPBTAllCurrencies))])
	default:
		// Moneda conocida en mayúsculas.
		return curPBTAllCurrencies[rnd.Intn(len(curPBTAllCurrencies))]
	}
}

// curPBTRandomCase cambia aleatoriamente el caso de cada letra de s.
func curPBTRandomCase(rnd *rand.Rand, s string) string {
	b := []byte(s)
	for i := range b {
		c := b[i]
		if c >= 'a' && c <= 'z' && rnd.Intn(2) == 0 {
			b[i] = c - ('a' - 'A')
		} else if c >= 'A' && c <= 'Z' && rnd.Intn(2) == 0 {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}

// curPBTIsTwoLetters indica, de forma independiente a la implementación bajo
// prueba, si s tiene exactamente 2 letras ASCII.
func curPBTIsTwoLetters(s string) bool {
	return curPBTIsNLetters(s, 2)
}

// curPBTIsThreeLetters indica, de forma independiente, si s tiene exactamente 3
// letras ASCII.
func curPBTIsThreeLetters(s string) bool {
	return curPBTIsNLetters(s, 3)
}

// curPBTIsNLetters indica si s tiene exactamente n caracteres y todos son
// letras ASCII (a-z, A-Z). Es una reimplementación deliberadamente simple para
// servir de oráculo independiente.
func curPBTIsNLetters(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
			return false
		}
	}
	return true
}

// curPBTOracleAccepts implementa, de forma independiente, el predicado del "si y
// solo si" descrito por la Property 10. Deriva la moneda esperada únicamente de
// CurrencyForCountry (helper público) sin invocar la lógica combinada bajo
// prueba.
func curPBTOracleAccepts(tokenCountry, country, currency string) bool {
	// El país del token debe ser soportado; de lo contrario no puede
	// verificarse la coherencia y la petición no se acepta.
	if _, ok := CurrencyForCountry(tokenCountry); !ok {
		return false
	}
	// `country` debe tener exactamente 2 letras y ser soportado.
	if !curPBTIsTwoLetters(country) {
		return false
	}
	expected, ok := CurrencyForCountry(country)
	if !ok {
		return false
	}
	// `country` debe coincidir con `tokenCountry` sin distinción de mayúsculas.
	if !strings.EqualFold(country, tokenCountry) {
		return false
	}
	// `currency` debe tener exactamente 3 letras y coincidir con la moneda
	// mapeada del país, sin distinción de mayúsculas.
	if !curPBTIsThreeLetters(currency) {
		return false
	}
	return strings.EqualFold(currency, expected)
}

// TestValidatePlanCountryCurrencyProperty10 verifica la Property 10:
// ValidatePlanCountryCurrency devuelve nil si y solo si el oráculo independiente
// acepta el trío (tokenCountry, country, currency).
func TestValidatePlanCountryCurrencyProperty10(t *testing.T) {
	property := func(in curPBTInput) bool {
		err := ValidatePlanCountryCurrency(in.tokenCountry, in.country, in.currency)
		accepted := err == nil
		shouldAccept := curPBTOracleAccepts(in.tokenCountry, in.country, in.currency)
		return accepted == shouldAccept
	}

	cfg := &quick.Config{MaxCount: curPBTIterations}
	if err := quick.Check(property, cfg); err != nil {
		t.Fatalf("Property 10 violada: ValidatePlanCountryCurrency debe aceptar si y solo si country (2 letras, soportado) coincide con el token y currency (3 letras) coincide con la moneda del país: %v", err)
	}
}

// TestValidatePlanCountryCurrencyAcceptsEveryMapping recorre explícitamente cada
// país soportado con su moneda mapeada (incluyendo mayúsculas mezcladas en
// ambos) y verifica que todos se aceptan.
func TestValidatePlanCountryCurrencyAcceptsEveryMapping(t *testing.T) {
	for _, country := range curPBTSupportedCountries {
		expected, ok := CurrencyForCountry(country)
		if !ok {
			t.Fatalf("país soportado %q sin moneda mapeada", country)
		}
		variants := []struct {
			tokenCountry string
			country      string
			currency     string
		}{
			{country, country, expected},
			{strings.ToUpper(country), country, expected},
			{country, strings.ToUpper(country), strings.ToLower(expected)},
			{strings.ToUpper(country), strings.ToUpper(country), strings.ToUpper(expected)},
		}
		for _, v := range variants {
			if err := ValidatePlanCountryCurrency(v.tokenCountry, v.country, v.currency); err != nil {
				t.Errorf("ValidatePlanCountryCurrency(%q, %q, %q) error inesperado: %v", v.tokenCountry, v.country, v.currency, err)
			}
		}
	}
}

// TestValidatePlanCountryCurrencyRejectsKnownBadInputs cubre casos frontera
// deterministas que deben rechazarse, complementando la generación aleatoria y
// documentando el error esperado para cada motivo de rechazo.
func TestValidatePlanCountryCurrencyRejectsKnownBadInputs(t *testing.T) {
	cases := []struct {
		name         string
		tokenCountry string
		country      string
		currency     string
		wantErr      error
	}{
		{"país del token no soportado", "us", "gt", "GTQ", ErrUnsupportedCountry},
		{"country con longitud incorrecta", "gt", "gtq", "GTQ", ErrInvalidCountryFormat},
		{"country no soportado", "gt", "mx", "GTQ", ErrUnsupportedCountry},
		{"country no coincide con el token", "gt", "sv", "USD", ErrCountryMismatch},
		{"currency con longitud incorrecta", "gt", "gt", "GT", ErrInvalidCurrencyFormat},
		{"currency no coincide con el país", "gt", "gt", "USD", ErrCurrencyMismatch},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidatePlanCountryCurrency(tc.tokenCountry, tc.country, tc.currency)
			if err == nil {
				t.Fatalf("ValidatePlanCountryCurrency(%q, %q, %q) = nil; se esperaba error %v", tc.tokenCountry, tc.country, tc.currency, tc.wantErr)
			}
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("ValidatePlanCountryCurrency(%q, %q, %q) error = %v; se esperaba %v", tc.tokenCountry, tc.country, tc.currency, err, tc.wantErr)
			}
		})
	}
}
