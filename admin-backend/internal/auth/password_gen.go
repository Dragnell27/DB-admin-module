package auth

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

// Conjuntos de caracteres usados por el generador de contraseñas. Se dividen por
// clase para poder garantizar que cada contraseña incluye al menos un carácter
// de cada una (Req. 3.2, 13.1).
const (
	// passwordLowercase son las letras minúsculas.
	passwordLowercase = "abcdefghijklmnopqrstuvwxyz"
	// passwordUppercase son las letras mayúsculas.
	passwordUppercase = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	// passwordDigits son los dígitos decimales.
	passwordDigits = "0123456789"
	// passwordSymbols es un conjunto razonable de símbolos. Se evitan caracteres
	// ambiguos o problemáticos en algunos contextos (comillas, barras invertidas,
	// espacios) para reducir errores al copiarlas.
	passwordSymbols = "!@#$%^&*()-_=+[]{};:,.?"
)

// generatedPasswordLength es la longitud por defecto de las contraseñas
// generadas. Debe ser siempre >= 16 según la política (Req. 3.2, 13.1).
const generatedPasswordLength = 20

// passwordAlphabet es la unión de todas las clases de caracteres, usada para
// rellenar la contraseña una vez garantizada la presencia de cada clase.
const passwordAlphabet = passwordLowercase + passwordUppercase + passwordDigits + passwordSymbols

// GeneratePassword crea una contraseña aleatoria segura usando crypto/rand:
// longitud generatedPasswordLength (>= 16) y al menos una minúscula, una
// mayúscula, un dígito y un símbolo (Req. 3.2, 13.1).
//
// Para garantizar que SIEMPRE cumple la política, primero toma un carácter de
// cada clase obligatoria y luego rellena el resto desde el alfabeto combinado.
// Finalmente baraja el resultado con un Fisher-Yates que usa crypto/rand, de
// modo que la posición de los caracteres obligatorios no sea predecible.
//
// Solo devuelve error si crypto/rand falla al generar aleatoriedad.
func GeneratePassword() (string, error) {
	// Clases obligatorias: garantizan que la política se cumple siempre.
	classes := []string{
		passwordLowercase,
		passwordUppercase,
		passwordDigits,
		passwordSymbols,
	}

	chars := make([]byte, 0, generatedPasswordLength)

	// Un carácter de cada clase obligatoria.
	for _, class := range classes {
		ch, err := randomChar(class)
		if err != nil {
			return "", err
		}
		chars = append(chars, ch)
	}

	// Relleno del resto desde el alfabeto combinado.
	for len(chars) < generatedPasswordLength {
		ch, err := randomChar(passwordAlphabet)
		if err != nil {
			return "", err
		}
		chars = append(chars, ch)
	}

	// Barajado Fisher-Yates seguro para que los caracteres obligatorios no
	// queden siempre al principio.
	if err := shuffle(chars); err != nil {
		return "", err
	}

	return string(chars), nil
}

// randomChar devuelve un carácter aleatorio del conjunto indicado usando
// crypto/rand como fuente de aleatoriedad. Devuelve error solo si crypto/rand
// falla.
func randomChar(set string) (byte, error) {
	idx, err := randomIndex(len(set))
	if err != nil {
		return 0, err
	}
	return set[idx], nil
}

// randomIndex devuelve un entero aleatorio uniforme en [0, n) usando crypto/rand
// mediante rand.Int con un big.Int. Devuelve error si crypto/rand falla.
func randomIndex(n int) (int, error) {
	if n <= 0 {
		return 0, fmt.Errorf("auth: tamaño de conjunto inválido: %d", n)
	}
	max := big.NewInt(int64(n))
	idx, err := rand.Int(rand.Reader, max)
	if err != nil {
		return 0, fmt.Errorf("auth: no se pudo generar aleatoriedad: %w", err)
	}
	return int(idx.Int64()), nil
}

// shuffle baraja el slice en el sitio con el algoritmo Fisher-Yates, usando
// crypto/rand para elegir cada índice. Devuelve error si crypto/rand falla.
func shuffle(b []byte) error {
	for i := len(b) - 1; i > 0; i-- {
		j, err := randomIndex(i + 1)
		if err != nil {
			return err
		}
		b[i], b[j] = b[j], b[i]
	}
	return nil
}
