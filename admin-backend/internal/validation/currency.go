package validation

import (
	"errors"
	"fmt"
	"strings"
)

// Países OTT soportados (claim country_code del JWT OTT): gt, sv, ni, hn, cr
// (design.md "Manejo del país", Req. 3.7, 10.5). Cada país tiene exactamente una
// moneda válida según el mapeo del diseño (Req. 10.7).
const (
	CountryGT = "gt"
	CountrySV = "sv"
	CountryNI = "ni"
	CountryHN = "hn"
	CountryCR = "cr"
)

// countryToCurrency define el mapeo moneda↔país exigido por el diseño:
// `sv=USD, cr=CRC, gt=GTQ, hn=HNL, ni=NIO` (design.md, Req. 10.7).
//
// Las claves (país) se almacenan en minúsculas y los valores (moneda) en
// mayúsculas, que es la forma canónica usada en las comparaciones.
var countryToCurrency = map[string]string{
	CountrySV: "USD",
	CountryCR: "CRC",
	CountryGT: "GTQ",
	CountryHN: "HNL",
	CountryNI: "NIO",
}

// Errores de validación de país y moneda. Se exponen como variables para que los
// handlers y las pruebas puedan compararlos con errors.Is.
var (
	// ErrInvalidCountryFormat indica que `country` no tiene exactamente 2 letras.
	ErrInvalidCountryFormat = errors.New("country debe tener exactamente 2 letras")
	// ErrUnsupportedCountry indica que el país no pertenece al conjunto
	// soportado {gt, sv, ni, hn, cr}.
	ErrUnsupportedCountry = errors.New("country no está en el conjunto soportado {gt, sv, ni, hn, cr}")
	// ErrCountryMismatch indica que el `country` enviado no coincide con el país
	// del JWT OTT activo (Req. 10.5).
	ErrCountryMismatch = errors.New("country no coincide con el país del JWT OTT activo")
	// ErrInvalidCurrencyFormat indica que `currency` no tiene exactamente 3 letras.
	ErrInvalidCurrencyFormat = errors.New("currency debe tener exactamente 3 letras")
	// ErrCurrencyMismatch indica que la moneda no coincide con la del país según
	// el mapeo (Req. 10.7).
	ErrCurrencyMismatch = errors.New("currency no coincide con la moneda del país")
)

// SupportedCountries devuelve el conjunto de países soportados en orden
// determinista. Útil para mensajes de error y pruebas.
func SupportedCountries() []string {
	return []string{CountryCR, CountryGT, CountryHN, CountryNI, CountrySV}
}

// IsSupportedCountry indica si `country` (sin distinción de mayúsculas) pertenece
// al conjunto de países soportados.
func IsSupportedCountry(country string) bool {
	_, ok := countryToCurrency[strings.ToLower(strings.TrimSpace(country))]
	return ok
}

// CurrencyForCountry devuelve la moneda (3 letras, mayúsculas) correspondiente a
// un país soportado. El segundo valor es false si el país no está soportado.
func CurrencyForCountry(country string) (string, bool) {
	currency, ok := countryToCurrency[strings.ToLower(strings.TrimSpace(country))]
	return currency, ok
}

// isTwoLetters indica si s está compuesta por exactamente 2 letras ASCII.
func isTwoLetters(s string) bool {
	return isLetters(s, 2)
}

// isThreeLetters indica si s está compuesta por exactamente 3 letras ASCII.
func isThreeLetters(s string) bool {
	return isLetters(s, 3)
}

// isLetters indica si s tiene exactamente n caracteres y todos son letras ASCII
// (a-z, A-Z).
func isLetters(s string, n int) bool {
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

// ValidateCountry valida el formato y soporte de un `country` de Plan de forma
// aislada (2 letras y país soportado), sin compararlo contra el país del token.
//
// Devuelve ErrInvalidCountryFormat si no son exactamente 2 letras y
// ErrUnsupportedCountry si no pertenece al conjunto {gt, sv, ni, hn, cr}.
func ValidateCountry(country string) error {
	if !isTwoLetters(country) {
		return ErrInvalidCountryFormat
	}
	if !IsSupportedCountry(country) {
		return ErrUnsupportedCountry
	}
	return nil
}

// ValidateCurrencyForCountry valida que `currency` tenga exactamente 3 letras y
// coincida con la moneda del `country` según el mapeo (Req. 10.7).
//
// Precondición: `country` ya es un país soportado válido. La comparación de la
// moneda es sin distinción de mayúsculas. Devuelve ErrInvalidCurrencyFormat si
// `currency` no son 3 letras, ErrUnsupportedCountry si el país no está soportado
// (defensa en profundidad), o ErrCurrencyMismatch si la moneda no coincide.
func ValidateCurrencyForCountry(country, currency string) error {
	if !isThreeLetters(currency) {
		return ErrInvalidCurrencyFormat
	}
	expected, ok := CurrencyForCountry(country)
	if !ok {
		return ErrUnsupportedCountry
	}
	if !strings.EqualFold(currency, expected) {
		return ErrCurrencyMismatch
	}
	return nil
}

// ValidatePlanCountryCurrency valida de forma conjunta el `country` y la
// `currency` de un Plan contra el país del JWT OTT activo (Property 10,
// Req. 10.4, 10.5, 10.7).
//
// La petición se acepta si y solo si:
//   - `country` tiene exactamente 2 letras y pertenece al conjunto soportado;
//   - `country` coincide (sin distinción de mayúsculas) con `tokenCountry`, el
//     país del JWT OTT activo;
//   - `currency` tiene exactamente 3 letras y coincide con la moneda del país
//     según el mapeo `sv=USD, cr=CRC, gt=GTQ, hn=HNL, ni=NIO`.
//
// `tokenCountry` debe ser el país activo del BFF (del conjunto soportado). Si no
// lo es, se devuelve un error, ya que la coherencia no puede verificarse.
func ValidatePlanCountryCurrency(tokenCountry, country, currency string) error {
	if !IsSupportedCountry(tokenCountry) {
		return fmt.Errorf("%w: país del token %q no soportado", ErrUnsupportedCountry, tokenCountry)
	}
	if err := ValidateCountry(country); err != nil {
		return err
	}
	if !strings.EqualFold(strings.TrimSpace(country), strings.TrimSpace(tokenCountry)) {
		return ErrCountryMismatch
	}
	return ValidateCurrencyForCountry(country, currency)
}
