// Package secrets cifra y descifra en reposo los datos sensibles del
// constructor de módulos (module-builder): credenciales de orígenes de datos
// (API key, token estático, client_secret OAuth, cadena de conexión Postgres).
//
// Usa AES-256-GCM (cifrado autenticado) con un nonce aleatorio por operación.
// El formato de salida es `nonce || ciphertext` (el nonce se antepone al texto
// cifrado), de modo que Open pueda recuperar el nonce sin almacenarlo aparte.
// GCM además autentica el contenido: un ciphertext manipulado o truncado falla
// al abrirse en lugar de devolver datos corruptos.
//
// La clave (32 bytes = AES-256) se inyecta ya decodificada vía New; este
// paquete NO lee variables de entorno (de eso se encarga internal/config, que
// decodifica MODULE_SECRETS_KEY de base64 y valida su longitud). La clave nunca
// se registra en logs ni se expone en errores.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
)

// KeySize es la longitud en bytes de la clave AES-256 esperada por New.
const KeySize = 32

// Errores exportados para que el llamador pueda distinguir con errors.Is la
// causa (clave inválida vs. ciphertext corrupto) sin inspeccionar el texto.
var (
	// ErrInvalidKey indica que la clave no mide exactamente KeySize bytes.
	ErrInvalidKey = errors.New("secrets: la clave debe medir 32 bytes (AES-256)")
	// ErrInvalidCiphertext indica que el ciphertext está truncado, corrupto o
	// no supera la verificación de autenticidad de GCM.
	ErrInvalidCiphertext = errors.New("secrets: el texto cifrado es inválido o fue manipulado")
)

// Sealer cifra y descifra valores con una clave AES-256-GCM fija. Es seguro para
// uso concurrente: el cipher.AEAD subyacente no mantiene estado mutable entre
// llamadas y cada operación genera su propio nonce.
type Sealer struct {
	aead cipher.AEAD
}

// New construye un Sealer a partir de una clave de 32 bytes (AES-256). Devuelve
// ErrInvalidKey si la longitud no es la esperada. La clave debe venir ya
// decodificada (p. ej. desde la base64 de MODULE_SECRETS_KEY que carga
// internal/config).
func New(key []byte) (*Sealer, error) {
	if len(key) != KeySize {
		return nil, fmt.Errorf("%w: se recibieron %d bytes", ErrInvalidKey, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		// aes.NewCipher solo falla por longitud de clave inválida, ya cubierta
		// arriba; se envuelve por robustez sin filtrar la clave.
		return nil, fmt.Errorf("%w", ErrInvalidKey)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("secrets: no se pudo inicializar AES-GCM: %w", err)
	}
	return &Sealer{aead: aead}, nil
}

// Seal cifra plaintext y devuelve `nonce || ciphertext`. Genera un nonce
// aleatorio nuevo por operación con crypto/rand, por lo que cifrar el mismo
// valor dos veces produce salidas distintas. El resultado nunca es igual al
// plaintext (incluye el nonce y la etiqueta de autenticación de GCM).
func (s *Sealer) Seal(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("secrets: no se pudo generar el nonce: %w", err)
	}
	// Seal anexa el ciphertext (con su etiqueta) al dst, que ya contiene el
	// nonce; así el resultado es nonce || ciphertext en un solo slice.
	return s.aead.Seal(nonce, nonce, plaintext, nil), nil
}

// Open descifra un valor producido por Seal (formato `nonce || ciphertext`).
// Devuelve ErrInvalidCiphertext si el dato está truncado (más corto que el
// nonce) o si la verificación de autenticidad de GCM falla (dato manipulado o
// clave distinta).
func (s *Sealer) Open(ciphertext []byte) ([]byte, error) {
	nonceSize := s.aead.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, ErrInvalidCiphertext
	}
	nonce, body := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := s.aead.Open(nil, nonce, body, nil)
	if err != nil {
		return nil, ErrInvalidCiphertext
	}
	return plaintext, nil
}
