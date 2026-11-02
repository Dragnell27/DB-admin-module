package validation

import (
	"errors"
	"regexp"
)

// uuidPattern valida un UUID canónico de 36 caracteres en formato
// 8-4-4-4-12 con dígitos hexadecimales, sin distinción de mayúsculas
// (p. ej. "123e4567-e89b-12d3-a456-426614174000").
//
// Se usa el patrón "genérico" (acepta cualquier versión/variante) porque los
// identificadores provienen del contrato OTT y no se restringe a una versión
// específica. La validación es la que exige el diseño para `plan_id`,
// `customer_id`, `payment_method_token` y `card_token` (Property 12).
var uuidPattern = regexp.MustCompile(
	`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`,
)

// ErrInvalidUUID indica que un identificador no tiene formato de UUID válido.
var ErrInvalidUUID = errors.New("identificador no es un UUID válido")

// IsUUID indica si s es un UUID canónico (formato 8-4-4-4-12, hex,
// insensible a mayúsculas). No considera la longitud máxima adicional que
// algunos campos imponen (ver ValidateUUIDField).
func IsUUID(s string) bool {
	return uuidPattern.MatchString(s)
}

// ValidateUUID valida que s sea un UUID válido, devolviendo ErrInvalidUUID en
// caso contrario.
func ValidateUUID(s string) error {
	if !IsUUID(s) {
		return ErrInvalidUUID
	}
	return nil
}
