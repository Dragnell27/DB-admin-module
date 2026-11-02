// Package auth contiene la autenticación de sesión del admin-backend:
// hashing de contraseñas con bcrypt y (en tareas posteriores) la emisión y
// validación del JWT de sesión.
package auth

import (
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// ErrEmptyPassword se devuelve cuando se intenta hashear una contraseña vacía.
// Las contraseñas vacías no son válidas y nunca deben almacenarse.
var ErrEmptyPassword = errors.New("auth: la contraseña no puede estar vacía")

// ErrPasswordMismatch se devuelve por CheckPassword cuando la contraseña en
// texto plano no corresponde con el hash bcrypt almacenado.
var ErrPasswordMismatch = errors.New("auth: la contraseña no coincide")

// HashPassword genera un hash bcrypt de la contraseña en texto plano usando el
// coste indicado (p. ej. config.BcryptCost, por defecto 12). Nunca se almacena
// la contraseña en texto plano: solo el hash resultante (Req. 1.6, 1.7).
//
// Si cost queda fuera del rango soportado por bcrypt se ajusta al rango válido
// más cercano (bcrypt usa su DefaultCost cuando cost es menor que MinCost y
// rechaza valores por encima de MaxCost). bcrypt limita la entrada a 72 bytes.
func HashPassword(plain string, cost int) (string, error) {
	if plain == "" {
		return "", ErrEmptyPassword
	}

	if cost < bcrypt.MinCost {
		cost = bcrypt.DefaultCost
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(plain), cost)
	if err != nil {
		return "", fmt.Errorf("auth: no se pudo generar el hash de la contraseña: %w", err)
	}

	return string(hashed), nil
}

// CheckPassword verifica una contraseña en texto plano contra un hash bcrypt
// almacenado usando bcrypt.CompareHashAndPassword (Req. 1.5). Devuelve nil si
// coinciden, ErrPasswordMismatch si no coinciden, o un error envuelto si el
// hash almacenado no es un hash bcrypt válido.
func CheckPassword(hash, plain string) error {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain))
	if err == nil {
		return nil
	}

	if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
		return ErrPasswordMismatch
	}

	return fmt.Errorf("auth: no se pudo verificar la contraseña: %w", err)
}
