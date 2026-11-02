// Feature: panel-admin-ott, Property 13: La edición de Customer no modifica campos inmutables
//
// Prueba basada en propiedades (PBT) de la inmutabilidad de `external_id` y
// `email` en la edición de Customer del admin-backend (BFF).
//
// Validates: Requirements 8.1, 8.2
//
// La Property 13 del diseño expresa que, para toda solicitud de edición de
// Customer, los valores de `email` y `external_id` enviados al OTT son los
// originales (no editables) y solo se envían los campos editables permitidos.
// En el admin-backend esto se materializa en dos capas:
//   - El handler PUT /customers (internal/api/customers_handler.go) nunca
//     reenvía `external_id`/`email` como campos del cuerpo editable y los deja
//     vacíos al proyectar hacia validation.CustomerInput (toValidationInput).
//   - La función pura validation.ValidateCustomerEdit valida ÚNICAMENTE los
//     campos editables (first_name, last_name, phone, birth_date,
//     second_last_name, gender, address, device_fingerprint) y NO valida ni
//     restringe `external_id` ni `email` (Req. 8.2).
//
// El invariante más fuerte comprobable en internal/validation (lógica pura, sin
// dependencias de red ni de Gin) es la INVARIANCIA de ValidateCustomerEdit
// frente a ExternalID y Email: dos entradas de edición que difieren únicamente
// en ExternalID y/o Email SIEMPRE producen el mismo resultado de validación
// (misma aceptación/rechazo y, cuando se rechaza, el mismo *ValidationError:
// mismo campo y mismo error centinela). Es decir, variar `external_id`/`email`
// nunca cambia el veredicto de la validación de edición. Esto encapsula de
// forma verificable que esos campos son inmutables en la ruta de edición.
//
// Se usa la librería estándar `testing/quick` (mínimo 100 iteraciones). El
// generador produce payloads de edición arbitrarios (válidos e inválidos) junto
// con valores alternativos arbitrarios de external_id/email, y la propiedad
// compara el resultado de validar con los valores originales frente a validar
// con los valores alternativos.
package validation

import (
	"errors"
	"math/rand"
	"reflect"
	"testing"
	"testing/quick"
)

// custImmutPBTIterations es el número mínimo de iteraciones exigido por las
// convenciones de PBT del proyecto (mínimo 100).
const custImmutPBTIterations = 300

// custImmutPBTCase representa una iteración: un payload de edición de Customer
// (campos editables) más dos conjuntos arbitrarios de valores para los campos
// inmutables external_id/email que deben ser irrelevantes para la validación.
type custImmutPBTCase struct {
	// Campos editables del payload de edición.
	firstName         string
	lastName          string
	phone             string
	birthDate         string
	secondLastName    *string
	gender            *string
	address           *string
	deviceFingerprint *string

	// Par de valores inmutables "originales".
	externalIDA string
	emailA      string

	// Par de valores inmutables "alternativos" (deben dar el mismo veredicto).
	externalIDB string
	emailB      string
}

// custImmutPBTEditableFirstNames mezcla valores válidos e inválidos (incluye uno
// que excede maxNameLen) para ejercitar tanto aceptación como rechazo.
var custImmutPBTEditableFirstNames = []string{
	"",                 // ausente: no se valida
	"Ana",              // válido
	"José María",       // válido con acentos/espacios
	custImmutPBTLong(), // inválido: excede longitud máxima
}

var custImmutPBTEditableLastNames = []string{
	"",
	"Pérez",
	"de la Cruz",
	custImmutPBTLong(),
}

// custImmutPBTPhones mezcla ausencia, válidos e inválidos (Req. 7.5).
var custImmutPBTPhones = []string{
	"",            // ausente
	"+50212345678", // válido
	"12345678",    // inválido: sin '+'
	"+12",         // inválido: muy corto
}

// custImmutPBTBirthDates mezcla ausencia, válidos e inválidos (Req. 7.6).
var custImmutPBTBirthDates = []string{
	"",           // ausente
	"1990-05-20", // válido (mayor de edad)
	"2020-01-01", // inválido: menor de edad
	"05-20-1990", // inválido: formato
}

// custImmutPBTGenders mezcla ausencia, permitidos y no permitidos (Req. 7.8).
var custImmutPBTGenders = []*string{
	nil,
	custImmutPBTS("M"),
	custImmutPBTS("F"),
	custImmutPBTS("X"), // no permitido
}

// custImmutPBTExtAndEmail son valores arbitrarios de external_id/email que
// incluyen casos "válidos" e "inválidos" si se validaran: la propiedad exige que
// NINGUNO afecte el veredicto de la edición.
var custImmutPBTExternalIDs = []string{
	"",
	"ext-123",
	custImmutPBTLong(),       // excedería maxExternalID si se validara
	"cualquier cosa aquí !!", // formato arbitrario
}

var custImmutPBTEmails = []string{
	"",
	"user@example.com",      // válido
	"no-es-un-email",        // inválido si se validara
	custImmutPBTLongEmail(), // excedería maxEmailLen si se validara
}

// custImmutPBTLong genera una cadena que excede maxNameLen (100 runas).
func custImmutPBTLong() string {
	return custImmutPBTRepeat('a', maxNameLen+5)
}

// custImmutPBTLongEmail genera un "email" que excede maxEmailLen (255 runas).
func custImmutPBTLongEmail() string {
	return custImmutPBTRepeat('x', maxEmailLen+5) + "@e.com"
}

// custImmutPBTRepeat construye una cadena de n veces el runa r.
func custImmutPBTRepeat(r rune, n int) string {
	buf := make([]rune, n)
	for i := range buf {
		buf[i] = r
	}
	return string(buf)
}

// custImmutPBTS devuelve un puntero a string.
func custImmutPBTS(v string) *string { return &v }

// custImmutPBTPick elige un elemento al azar de un slice de string.
func custImmutPBTPick(rnd *rand.Rand, xs []string) string {
	return xs[rnd.Intn(len(xs))]
}

// Generate satisface quick.Generator produciendo casos variados: campos
// editables válidos/inválidos/ausentes y pares arbitrarios de external_id/email.
func (custImmutPBTCase) Generate(rnd *rand.Rand, size int) reflect.Value {
	c := custImmutPBTCase{
		firstName:   custImmutPBTPick(rnd, custImmutPBTEditableFirstNames),
		lastName:    custImmutPBTPick(rnd, custImmutPBTEditableLastNames),
		phone:       custImmutPBTPick(rnd, custImmutPBTPhones),
		birthDate:   custImmutPBTPick(rnd, custImmutPBTBirthDates),
		gender:      custImmutPBTGenders[rnd.Intn(len(custImmutPBTGenders))],
		externalIDA: custImmutPBTPick(rnd, custImmutPBTExternalIDs),
		emailA:      custImmutPBTPick(rnd, custImmutPBTEmails),
		externalIDB: custImmutPBTPick(rnd, custImmutPBTExternalIDs),
		emailB:      custImmutPBTPick(rnd, custImmutPBTEmails),
	}

	// second_last_name opcional (si está presente, puede ser válido).
	if rnd.Intn(2) == 0 {
		c.secondLastName = custImmutPBTS("González")
	}
	// address opcional: a veces presente y a veces excesivamente largo.
	switch rnd.Intn(3) {
	case 0:
		c.address = custImmutPBTS("Calle Falsa 123")
	case 1:
		c.address = custImmutPBTS(custImmutPBTRepeat('b', maxAddressLen+5)) // inválido
	}
	// device_fingerprint opcional (no tiene reglas de longitud propias).
	if rnd.Intn(2) == 0 {
		c.deviceFingerprint = custImmutPBTS("fp-abc-123")
	}

	return reflect.ValueOf(c)
}

// custImmutPBTInput construye un CustomerInput de edición con los campos
// editables del caso y los valores inmutables indicados.
func custImmutPBTInput(c custImmutPBTCase, externalID, email string) CustomerInput {
	return CustomerInput{
		ExternalID:        externalID,
		Email:             email,
		FirstName:         c.firstName,
		LastName:          c.lastName,
		Phone:             c.phone,
		BirthDate:         c.birthDate,
		SecondLastName:    c.secondLastName,
		Gender:            c.gender,
		Address:           c.address,
		DeviceFingerprint: c.deviceFingerprint,
	}
}

// custImmutPBTSameOutcome compara dos resultados de ValidateCustomerEdit y
// determina si representan el mismo veredicto: ambos nil, o ambos no-nil con el
// mismo campo y el mismo error centinela subyacente.
func custImmutPBTSameOutcome(a, b error) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}

	var va, vb *ValidationError
	aIsVE := errors.As(a, &va)
	bIsVE := errors.As(b, &vb)
	if aIsVE != bIsVE {
		return false
	}
	if aIsVE && vb != nil {
		if va.Field != vb.Field {
			return false
		}
		// Mismo error centinela subyacente.
		return errors.Is(va.Err, vb.Err) && errors.Is(vb.Err, va.Err)
	}
	// Ninguno es *ValidationError: comparar por igualdad de mensaje.
	return a.Error() == b.Error()
}

// TestValidateCustomerEditImmutableProperty13 verifica la Property 13:
// ValidateCustomerEdit es INVARIANTE frente a external_id y email. Dos entradas
// de edición que difieren únicamente en esos campos inmutables producen siempre
// el mismo veredicto de validación.
func TestValidateCustomerEditImmutableProperty13(t *testing.T) {
	property := func(c custImmutPBTCase) bool {
		// Veredicto con los valores "originales" (A).
		base := ValidateCustomerEdit(custImmutPBTInput(c, c.externalIDA, c.emailA))

		// 1) Variar SOLO external_id (de A a B, mismo email A) no cambia el veredicto.
		onlyExt := ValidateCustomerEdit(custImmutPBTInput(c, c.externalIDB, c.emailA))
		if !custImmutPBTSameOutcome(base, onlyExt) {
			return false
		}

		// 2) Variar SOLO email (de A a B, mismo external_id A) no cambia el veredicto.
		onlyEmail := ValidateCustomerEdit(custImmutPBTInput(c, c.externalIDA, c.emailB))
		if !custImmutPBTSameOutcome(base, onlyEmail) {
			return false
		}

		// 3) Variar AMBOS a la vez (A -> B) tampoco cambia el veredicto.
		both := ValidateCustomerEdit(custImmutPBTInput(c, c.externalIDB, c.emailB))
		if !custImmutPBTSameOutcome(base, both) {
			return false
		}

		// 4) Vaciar ambos campos inmutables (como hace el handler) no cambia el
		//    veredicto respecto de cualquier valor arbitrario.
		empty := ValidateCustomerEdit(custImmutPBTInput(c, "", ""))
		return custImmutPBTSameOutcome(base, empty)
	}

	cfg := &quick.Config{MaxCount: custImmutPBTIterations}
	if err := quick.Check(property, cfg); err != nil {
		t.Fatalf("Property 13 violada: ValidateCustomerEdit no es invariante frente a external_id/email: %v", err)
	}
}

// TestValidateCustomerEditImmutableKnownCases complementa la generación aleatoria
// con casos deterministas que confirman que valores inválidos de external_id y
// email (que serían rechazados en creación) NO provocan error en edición.
func TestValidateCustomerEditImmutableKnownCases(t *testing.T) {
	cases := []struct {
		name       string
		externalID string
		email      string
	}{
		{name: "ambos vacíos (handler)", externalID: "", email: ""},
		{name: "external_id excesivamente largo", externalID: custImmutPBTLong(), email: ""},
		{name: "email con formato inválido", externalID: "ext-1", email: "no-es-email"},
		{name: "email excesivamente largo", externalID: "", email: custImmutPBTLongEmail()},
		{name: "ambos con valores arbitrarios", externalID: "x!y@#", email: "basura"},
	}

	// Payload de edición con campos editables válidos: el veredicto esperado es
	// aceptación (nil) independientemente de external_id/email.
	editable := custImmutPBTCase{
		firstName: "Ana",
		lastName:  "Pérez",
		phone:     "+50212345678",
		birthDate: "1990-05-20",
		gender:    custImmutPBTS("F"),
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := custImmutPBTInput(editable, tc.externalID, tc.email)
			if err := ValidateCustomerEdit(in); err != nil {
				t.Fatalf("ValidateCustomerEdit con external_id=%q email=%q error = %v; se esperaba nil (campos inmutables no se validan)", tc.externalID, tc.email, err)
			}
		})
	}
}
