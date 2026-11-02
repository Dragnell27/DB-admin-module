// Feature: panel-admin-ott, Property 12: Validación de identificadores UUID en Subscription y Card
//
// Prueba basada en propiedades (PBT) de la validación de identificadores con
// formato UUID en la creación de Subscription y de Card del admin-backend (BFF).
//
// Validates: Requirements 14.2, 14.3, 14.5, 16.2, 18.2, 18.5
//
// Propiedad: *para todo* campo identificador tipado como UUID de Subscription
// (`plan_id`, `customer_id`, `payment_method_token`) y de Card (`customer_id`,
// `card_token`), manteniendo el resto de campos SIEMPRE válidos, la validación
// acepta la solicitud SI Y SOLO SI *todos* los campos UUID del recurso son UUID
// canónicos válidos (formato 8-4-4-4-12, hex). Si alguno no es un UUID canónico,
// la validación rechaza la solicitud; además, un valor NO VACÍO pero malformado
// (longitud incorrecta, no hexadecimal, sin guiones, caracteres de más) se
// rechaza con un error que envuelve ErrInvalidUUID, mientras que un valor VACÍO
// en un campo UUID requerido se rechaza con ErrRequired (la implementación
// distingue "ausente" de "malformado").
//
// Para aislar la regla de UUID del resto de validaciones, el generador construye
// siempre un recurso por lo demás válido (payment_method, amount, start_date en
// Subscription; card_number, expiration_date futura, payment_method_name,
// card_franchise en Card) y varía únicamente el contenido de los campos UUID,
// alternando entre UUID canónicos construidos aleatoriamente y cadenas
// malformadas. El oráculo del "si y solo si" es independiente de la
// implementación bajo prueba: usa IsUUID sobre cada campo UUID del recurso.
//
// Se usa la librería estándar `testing/quick` (mínimo 100 iteraciones).
package validation

import (
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"testing"
	"testing/quick"
	"time"
)

// uuidPBTIterations es el número mínimo de iteraciones exigido por las
// convenciones de PBT del proyecto (mínimo 100).
const uuidPBTIterations = 400

// uuidPBTHexDigits son los dígitos hexadecimales usados para construir UUID
// canónicos válidos (se mezclan mayúsculas y minúsculas porque el patrón es
// insensible a mayúsculas).
const uuidPBTHexDigits = "0123456789abcdefABCDEF"

// uuidPBTFutureExpiration es una fecha de expiración de tarjeta en formato MM/YY
// garantizada futura, para mantener válido el resto de la Card. Se calcula una
// sola vez a partir de un año muy posterior al actual.
var uuidPBTFutureExpiration = fmt.Sprintf("12/%02d", (time.Now().UTC().Year()+5)%100)

// uuidPBTField representa el valor generado para un único campo UUID, junto con
// si debía ser un UUID válido, de modo que el oráculo pueda razonar sin depender
// de la implementación.
type uuidPBTField struct {
	value string
	valid bool // true si value es un UUID canónico válido
}

// uuidPBTInput agrupa los cinco campos UUID bajo prueba: tres de Subscription y
// dos de Card. Cada uno se genera de forma independiente como válido o
// malformado.
type uuidPBTInput struct {
	// Subscription
	planID             uuidPBTField
	subCustomerID      uuidPBTField
	paymentMethodToken uuidPBTField

	// Card
	cardCustomerID uuidPBTField
	cardToken      uuidPBTField
}

// Generate satisface quick.Generator produciendo, para cada campo UUID, bien un
// UUID canónico válido, bien una cadena malformada de alguna de las categorías
// de error, de forma que se ejerciten tanto las aceptaciones como los rechazos.
func (uuidPBTInput) Generate(rnd *rand.Rand, size int) reflect.Value {
	return reflect.ValueOf(uuidPBTInput{
		planID:             uuidPBTGenField(rnd),
		subCustomerID:      uuidPBTGenField(rnd),
		paymentMethodToken: uuidPBTGenField(rnd),
		cardCustomerID:     uuidPBTGenField(rnd),
		cardToken:          uuidPBTGenField(rnd),
	})
}

// uuidPBTGenField genera aleatoriamente un campo UUID. Aproximadamente la mitad
// de las veces produce un UUID válido y la otra mitad una variante malformada,
// para equilibrar los casos aceptados y rechazados.
func uuidPBTGenField(rnd *rand.Rand) uuidPBTField {
	if rnd.Intn(2) == 0 {
		return uuidPBTField{value: uuidPBTValidUUID(rnd), valid: true}
	}
	return uuidPBTField{value: uuidPBTInvalidUUID(rnd), valid: false}
}

// uuidPBTValidUUID construye un UUID canónico válido (8-4-4-4-12) con dígitos
// hexadecimales aleatorios, mezclando mayúsculas y minúsculas.
func uuidPBTValidUUID(rnd *rand.Rand) string {
	groups := []int{8, 4, 4, 4, 12}
	parts := make([]string, len(groups))
	for i, n := range groups {
		b := make([]byte, n)
		for j := 0; j < n; j++ {
			b[j] = uuidPBTHexDigits[rnd.Intn(len(uuidPBTHexDigits))]
		}
		parts[i] = string(b)
	}
	return fmt.Sprintf("%s-%s-%s-%s-%s", parts[0], parts[1], parts[2], parts[3], parts[4])
}

// uuidPBTInvalidUUID construye una cadena que NO es un UUID canónico, cubriendo
// varias categorías de error: vacía, longitud incorrecta, no hexadecimal, sin
// guiones, con caracteres de más y con un formato parecido pero inválido.
func uuidPBTInvalidUUID(rnd *rand.Rand) string {
	switch rnd.Intn(8) {
	case 0:
		return "" // vacío
	case 1:
		return "123" // demasiado corto
	case 2:
		// Grupos correctos salvo caracteres no hexadecimales (g, z).
		return "g23e4567-e89b-12d3-a456-42661417400z"
	case 3:
		// UUID válido pero sin guiones (longitud 32, patrón no coincide).
		return "123e4567e89b12d3a456426614174000"
	case 4:
		// UUID válido con un carácter de más al final.
		return "123e4567-e89b-12d3-a456-426614174000a"
	case 5:
		// Grupos con tamaños incorrectos (7-4-4-4-12).
		return "123e456-e89b-12d3-a456-426614174000"
	case 6:
		// Guiones mal ubicados.
		return "123e45-67e89b-12d3-a456-426614174000"
	default:
		// Cadena arbitraria no UUID.
		return "not-a-uuid"
	}
}

// uuidPBTSubscription ensambla un SubscriptionInput por lo demás VÁLIDO con los
// campos UUID generados. payment_method, amount y start_date se mantienen
// válidos para aislar la regla de UUID.
func uuidPBTSubscription(in uuidPBTInput) SubscriptionInput {
	amount := 100.0
	startDate := "2025-01-01T00:00:00Z"
	return SubscriptionInput{
		PlanID:             in.planID.value,
		CustomerID:         in.subCustomerID.value,
		PaymentMethod:      "card",
		PaymentMethodToken: in.paymentMethodToken.value,
		Amount:             &amount,
		StartDate:          &startDate,
	}
}

// uuidPBTCard ensambla un CardInput por lo demás VÁLIDO con los campos UUID
// generados. card_number, expiration_date (futura), payment_method_name y
// card_franchise se mantienen válidos para aislar la regla de UUID.
func uuidPBTCard(in uuidPBTInput) CardInput {
	return CardInput{
		CustomerID:        in.cardCustomerID.value,
		CardNumber:        "4111111111111111",
		ExpirationDate:    uuidPBTFutureExpiration,
		CardToken:         in.cardToken.value,
		PaymentMethodName: "Visa",
		CardFranchise:     "VISA",
	}
}

// uuidPBTOracle es el valor de un campo UUID evaluado por el oráculo, en el
// orden en que la implementación lo valida.
type uuidPBTOracle struct {
	value string
}

// uuidPBTCheck implementa el oráculo del "si y solo si" para una lista ordenada
// de campos UUID requeridos y el error devuelto por el validador:
//
//   - Si todos los campos son UUID canónicos válidos, se exige err == nil.
//   - En caso contrario, se identifica el PRIMER campo que no es UUID válido
//     (la implementación devuelve el primer error): si ese campo está vacío,
//     el error debe envolver ErrRequired; si no está vacío (malformado), debe
//     envolver ErrInvalidUUID.
//
// Esto es independiente de la implementación: solo usa IsUUID y la semántica de
// "requerido vs. malformado" declarada por el diseño.
func uuidPBTCheck(fields []uuidPBTOracle, err error) bool {
	for _, f := range fields {
		if IsUUID(f.value) {
			continue
		}
		// Primer campo UUID inválido: determina el error esperado.
		if f.value == "" {
			return err != nil && errors.Is(err, ErrRequired)
		}
		return err != nil && errors.Is(err, ErrInvalidUUID)
	}
	// Todos válidos: debe aceptarse.
	return err == nil
}

// TestValidateSubscriptionUUIDProperty12 verifica la Property 12 para la
// creación de Subscription: ValidateSubscriptionCreate (sobre una Subscription
// por lo demás válida) acepta la entrada si y solo si plan_id, customer_id y
// payment_method_token son UUID válidos; de lo contrario rechaza con un error
// que envuelve ErrInvalidUUID.
func TestValidateSubscriptionUUIDProperty12(t *testing.T) {
	property := func(in uuidPBTInput) bool {
		sub := uuidPBTSubscription(in)
		err := ValidateSubscriptionCreate(sub)

		// Oráculo independiente: aceptamos si y solo si todos los campos UUID
		// de la Subscription son UUID canónicos válidos. Los campos se evalúan
		// en el mismo orden que la implementación para razonar sobre cuál es el
		// primer campo UUID que falla.
		fields := []uuidPBTOracle{
			{sub.PlanID},
			{sub.CustomerID},
			{sub.PaymentMethodToken},
		}
		return uuidPBTCheck(fields, err)
	}

	cfg := &quick.Config{MaxCount: uuidPBTIterations}
	if err := quick.Check(property, cfg); err != nil {
		t.Fatalf("Property 12 violada (Subscription): ValidateSubscriptionCreate no respeta la validación de UUID: %v", err)
	}
}

// TestValidateCardUUIDProperty12 verifica la Property 12 para la creación de
// Card: ValidateCardCreate (sobre una Card por lo demás válida) acepta la
// entrada si y solo si customer_id y card_token son UUID válidos; de lo
// contrario rechaza con un error que envuelve ErrInvalidUUID.
func TestValidateCardUUIDProperty12(t *testing.T) {
	property := func(in uuidPBTInput) bool {
		card := uuidPBTCard(in)
		err := ValidateCardCreate(card)

		// Oráculo independiente: aceptamos si y solo si ambos campos UUID de la
		// Card son UUID canónicos válidos. (Un UUID canónico tiene 36 chars, muy
		// por debajo del máximo de 255 de card_token, por lo que la longitud no
		// altera el "si y solo si".) El orden coincide con la implementación.
		fields := []uuidPBTOracle{
			{card.CustomerID},
			{card.CardToken},
		}
		return uuidPBTCheck(fields, err)
	}

	cfg := &quick.Config{MaxCount: uuidPBTIterations}
	if err := quick.Check(property, cfg); err != nil {
		t.Fatalf("Property 12 violada (Card): ValidateCardCreate no respeta la validación de UUID: %v", err)
	}
}

// TestValidateUUIDProperty12KnownCases complementa la generación aleatoria con
// casos frontera deterministas explícitos sobre recursos por lo demás válidos,
// verificando tanto la aceptación (todos los UUID válidos) como el rechazo con
// ErrInvalidUUID por cada campo UUID malformado.
func TestValidateUUIDProperty12KnownCases(t *testing.T) {
	const validUUID = "123e4567-e89b-12d3-a456-426614174000"
	const badUUID = "no-es-uuid"

	// --- Subscription ---
	t.Run("subscription todos los UUID válidos se acepta", func(t *testing.T) {
		sub := uuidPBTSubscription(uuidPBTInput{
			planID:             uuidPBTField{value: validUUID, valid: true},
			subCustomerID:      uuidPBTField{value: validUUID, valid: true},
			paymentMethodToken: uuidPBTField{value: validUUID, valid: true},
		})
		if err := ValidateSubscriptionCreate(sub); err != nil {
			t.Fatalf("ValidateSubscriptionCreate(...) error = %v; se esperaba nil", err)
		}
	})

	subBadFields := []struct {
		name   string
		mutate func(*SubscriptionInput)
	}{
		{"plan_id malformado", func(s *SubscriptionInput) { s.PlanID = badUUID }},
		{"customer_id malformado", func(s *SubscriptionInput) { s.CustomerID = badUUID }},
		{"payment_method_token malformado", func(s *SubscriptionInput) { s.PaymentMethodToken = badUUID }},
	}
	for _, tc := range subBadFields {
		t.Run("subscription "+tc.name+" se rechaza con ErrInvalidUUID", func(t *testing.T) {
			sub := uuidPBTSubscription(uuidPBTInput{
				planID:             uuidPBTField{value: validUUID, valid: true},
				subCustomerID:      uuidPBTField{value: validUUID, valid: true},
				paymentMethodToken: uuidPBTField{value: validUUID, valid: true},
			})
			tc.mutate(&sub)
			err := ValidateSubscriptionCreate(sub)
			if !errors.Is(err, ErrInvalidUUID) {
				t.Fatalf("ValidateSubscriptionCreate(...) error = %v; se esperaba que envolviera ErrInvalidUUID", err)
			}
		})
	}

	// --- Card ---
	t.Run("card todos los UUID válidos se acepta", func(t *testing.T) {
		card := uuidPBTCard(uuidPBTInput{
			cardCustomerID: uuidPBTField{value: validUUID, valid: true},
			cardToken:      uuidPBTField{value: validUUID, valid: true},
		})
		if err := ValidateCardCreate(card); err != nil {
			t.Fatalf("ValidateCardCreate(...) error = %v; se esperaba nil", err)
		}
	})

	cardBadFields := []struct {
		name   string
		mutate func(*CardInput)
	}{
		{"customer_id malformado", func(c *CardInput) { c.CustomerID = badUUID }},
		{"card_token malformado", func(c *CardInput) { c.CardToken = badUUID }},
	}
	for _, tc := range cardBadFields {
		t.Run("card "+tc.name+" se rechaza con ErrInvalidUUID", func(t *testing.T) {
			card := uuidPBTCard(uuidPBTInput{
				cardCustomerID: uuidPBTField{value: validUUID, valid: true},
				cardToken:      uuidPBTField{value: validUUID, valid: true},
			})
			tc.mutate(&card)
			err := ValidateCardCreate(card)
			if !errors.Is(err, ErrInvalidUUID) {
				t.Fatalf("ValidateCardCreate(...) error = %v; se esperaba que envolviera ErrInvalidUUID", err)
			}
		})
	}
}
