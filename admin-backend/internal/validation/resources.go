package validation

import (
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"time"
)

// Este archivo contiene la validación server-side ESPEJO de las reglas por
// recurso que el admin-backend (BFF) replica desde las reglas del contrato OTT y
// de los schemas del front (design.md "Modelos del dominio" y "Validaciones por
// recurso"). El objetivo es no depender únicamente de la validación del front
// (Req. 20.2). Cubre Customer, Plan (incl. monto variable y campos emparejados),
// Subscription y Card.
//
// Reutiliza el tipo `Resource` y sus constantes definidos en ordering.go
// (ResourceCustomers/ResourcePlans/ResourceSubscriptions/ResourceCards), así
// como los ayudantes de currency.go (país↔moneda) y uuid.go (UUID).

// Límites de longitud y formatos replicados del diseño (design.md, Req. 7-19).
const (
	// maxExternalID: Customer.external_id y Plan.external_id, máx 100 (Req. 7.2, 10.3).
	maxExternalID = 100
	// maxName: first_name/last_name de Customer máx 100 (Req. 7.3); Plan.name
	// usa maxPlanName.
	maxNameLen = 100
	// maxEmail: Customer.email máx 255 (Req. 7.4).
	maxEmailLen = 255
	// maxAddress: Customer.address máx 255 (Req. 7.9).
	maxAddressLen = 255
	// maxPlanName: Plan.name máx 255 (Req. 10.2).
	maxPlanName = 255
	// maxCardNumber: Card.card_number máx 50 (Req. 18.3).
	maxCardNumber = 50
	// maxCardToken: Card.card_token máx 255 (Req. 18.5).
	maxCardToken = 255
	// maxExpirationDate: Card.expiration_date máx 10 (Req. 18.4).
	maxExpirationDate = 10
	// maxPaymentMethodName: Card.payment_method_name máx 100 (Req. 18.6).
	maxPaymentMethodName = 100
	// maxCardFranchise: Card.card_franchise máx 50 (Req. 18.7).
	maxCardFranchise = 50
	// maxCancelReason: Subscription cancel_reason máx 255 (Req. 16.3).
	maxCancelReason = 255
	// minAdultAge: edad mínima de un Customer (Req. 7.6).
	minAdultAge = 18
)

// Patrones de formato replicados del diseño.
var (
	// phonePattern: un signo '+' seguido de 8 a 15 dígitos (Req. 7.5).
	phonePattern = regexp.MustCompile(`^\+\d{8,15}$`)
	// birthDatePattern: formato YYYY-MM-DD (Req. 7.6).
	birthDatePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	// expirationPattern: formato MM/YY (Req. 18.4).
	expirationPattern = regexp.MustCompile(`^\d{2}/\d{2}$`)
)

// period_type permitido en Plan (Req. 10.9).
var allowedPeriodTypes = map[string]struct{}{
	"dia":    {},
	"semana": {},
	"mes":    {},
	"año":    {},
}

// gender permitido en Customer (Req. 7.8).
var allowedGenders = map[string]struct{}{
	"M": {},
	"F": {},
	"O": {},
}

// ValidationError describe un fallo de validación asociado a un campo concreto.
// Permite a los handlers construir respuestas de error claras indicando qué
// campo y por qué falló, y envuelve un error centinela comparable con errors.Is.
type ValidationError struct {
	Field string
	Err   error
}

func (e *ValidationError) Error() string {
	if e.Field == "" {
		return e.Err.Error()
	}
	return fmt.Sprintf("%s: %s", e.Field, e.Err.Error())
}

func (e *ValidationError) Unwrap() error { return e.Err }

// fieldErr es un atajo para construir un *ValidationError.
func fieldErr(field string, err error) *ValidationError {
	return &ValidationError{Field: field, Err: err}
}

// Errores centinela genéricos de validación de recurso.
var (
	ErrRequired       = errors.New("es requerido")
	ErrTooLong        = errors.New("excede la longitud máxima")
	ErrInvalidFormat  = errors.New("formato inválido")
	ErrNotPositive    = errors.New("debe ser mayor que 0")
	ErrInvalidValue   = errors.New("valor no permitido")
	ErrNotFutureDate  = errors.New("la fecha no puede ser futura")
	ErrFutureRequired = errors.New("la fecha debe ser futura")
	ErrUnderage       = fmt.Errorf("debe ser mayor de %d años", minAdultAge)
	ErrPairIncomplete = errors.New("los campos emparejados deben enviarse juntos")
	ErrAmountRange    = errors.New("minimum_amount y maximum_amount deben ser > 0 con minimum_amount < maximum_amount")
)

// ---------------------------------------------------------------------------
// Customer
// ---------------------------------------------------------------------------

// CustomerInput son los campos de un Customer sujetos a validación espejo.
// Los punteros representan campos opcionales (ausentes cuando son nil).
type CustomerInput struct {
	ExternalID        string
	FirstName         string
	LastName          string
	Email             string
	Phone             string
	BirthDate         string // YYYY-MM-DD
	ExternalCreated   string // RFC3339
	SecondLastName    *string
	Gender            *string
	Address           *string
	DeviceFingerprint *string
}

// ValidateCustomerCreate valida la creación de un Customer replicando las reglas
// del diseño (Req. 7.2-7.9). Devuelve el primer *ValidationError encontrado.
func ValidateCustomerCreate(in CustomerInput) error {
	if err := validateRequiredLen("external_id", in.ExternalID, maxExternalID); err != nil {
		return err
	}
	if err := validateRequiredLen("first_name", in.FirstName, maxNameLen); err != nil {
		return err
	}
	if err := validateRequiredLen("last_name", in.LastName, maxNameLen); err != nil {
		return err
	}
	if err := validateEmail(in.Email); err != nil {
		return err
	}
	if err := validatePhone(in.Phone); err != nil {
		return err
	}
	if err := validateBirthDate(in.BirthDate); err != nil {
		return err
	}
	if err := validateRFC3339("external_created", in.ExternalCreated, true); err != nil {
		return err
	}
	return validateCustomerOptional(in)
}

// ValidateCustomerEdit valida la edición de un Customer. Solo son editables
// first_name, last_name, phone, second_last_name, gender, address,
// device_fingerprint y birth_date (Req. 8.1); external_id y email NO son
// editables (Req. 8.2) y, por tanto, no se validan aquí aunque vengan presentes.
//
// Los campos editables se validan con sus mismas reglas de formato/longitud.
// Un campo editable "ausente" en edición se representa pasando su valor vacío
// para strings requeridos de creación que aquí son opcionales; sin embargo, el
// diseño trata estos campos como editables con las mismas reglas, por lo que si
// se envían deben cumplirlas. Para distinguir ausencia, los campos opcionales se
// mantienen como punteros y first_name/last_name/phone/birth_date se validan
// solo si no están vacíos.
func ValidateCustomerEdit(in CustomerInput) error {
	if in.FirstName != "" {
		if err := validateLen("first_name", in.FirstName, maxNameLen); err != nil {
			return err
		}
	}
	if in.LastName != "" {
		if err := validateLen("last_name", in.LastName, maxNameLen); err != nil {
			return err
		}
	}
	if in.Phone != "" {
		if err := validatePhone(in.Phone); err != nil {
			return err
		}
	}
	if in.BirthDate != "" {
		if err := validateBirthDate(in.BirthDate); err != nil {
			return err
		}
	}
	return validateCustomerOptional(in)
}

// validateCustomerOptional valida los campos opcionales comunes de Customer
// (gender, address) cuando están presentes.
func validateCustomerOptional(in CustomerInput) error {
	if in.Gender != nil {
		if _, ok := allowedGenders[*in.Gender]; !ok {
			return fieldErr("gender", ErrInvalidValue)
		}
	}
	if in.Address != nil {
		if err := validateLen("address", *in.Address, maxAddressLen); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Plan
// ---------------------------------------------------------------------------

// PlanInput son los campos de un Plan sujetos a validación espejo. Los punteros
// representan campos opcionales/emparejados; nil significa ausente.
type PlanInput struct {
	Name           string
	ExternalID     string
	Country        string
	Amount         float64
	Currency       string
	Frequency      int
	PeriodType     string
	VariableAmount bool

	// Monto variable (Req. 10.10): requeridos cuando VariableAmount es true.
	MinimumAmount *float64
	MaximumAmount *float64

	// Pareja frequency_test + test_type_period (Req. 10.11).
	FrequencyTest  *int
	TestTypePeriod *string

	// Grupo oneshot_amount + oneshot_frequency + oneshot_period (Req. 10.12).
	OneshotAmount    *float64
	OneshotFrequency *int
	OneshotPeriod    *string
}

// ValidatePlanCreate valida la creación de un Plan replicando las reglas del
// diseño (Req. 10.2-10.12), incluyendo el mapeo moneda↔país contra el país del
// JWT OTT activo (`tokenCountry`, Req. 10.4, 10.5, 10.7).
func ValidatePlanCreate(tokenCountry string, in PlanInput) error {
	if err := validateRequiredLen("name", in.Name, maxPlanName); err != nil {
		return err
	}
	if err := validateRequiredLen("external_id", in.ExternalID, maxExternalID); err != nil {
		return err
	}
	if err := ValidatePlanCountryCurrency(tokenCountry, in.Country, in.Currency); err != nil {
		return err
	}
	if in.Amount <= 0 {
		return fieldErr("amount", ErrNotPositive)
	}
	if in.Frequency <= 0 {
		return fieldErr("frequency", ErrNotPositive)
	}
	if _, ok := allowedPeriodTypes[in.PeriodType]; !ok {
		return fieldErr("period_type", ErrInvalidValue)
	}
	return validatePlanConditionalGroups(in)
}

// ValidatePlanEdit valida la edición de un Plan. Todos los campos son opcionales
// (Req. 11.1) pero, si se envían, cumplen las mismas reglas de formato, rango y
// emparejamiento (Req. 11.2). La identificación del plan es por `plan_id` y se
// valida en el handler, no aquí.
//
// `country`/`currency` se validan juntos solo si al menos uno viene presente:
// en ese caso se exige el par completo y coherente contra el token.
func ValidatePlanEdit(tokenCountry string, in PlanInput) error {
	if in.Name != "" {
		if err := validateLen("name", in.Name, maxPlanName); err != nil {
			return err
		}
	}
	if in.ExternalID != "" {
		if err := validateLen("external_id", in.ExternalID, maxExternalID); err != nil {
			return err
		}
	}
	if in.Country != "" || in.Currency != "" {
		if err := ValidatePlanCountryCurrency(tokenCountry, in.Country, in.Currency); err != nil {
			return err
		}
	}
	if in.Amount != 0 && in.Amount <= 0 {
		return fieldErr("amount", ErrNotPositive)
	}
	if in.Frequency != 0 && in.Frequency <= 0 {
		return fieldErr("frequency", ErrNotPositive)
	}
	if in.PeriodType != "" {
		if _, ok := allowedPeriodTypes[in.PeriodType]; !ok {
			return fieldErr("period_type", ErrInvalidValue)
		}
	}
	return validatePlanConditionalGroups(in)
}

// validatePlanConditionalGroups valida las reglas de monto variable y de campos
// emparejados de Plan (Property 11, Req. 10.10, 10.11, 10.12), independientes de
// creación/edición.
func validatePlanConditionalGroups(in PlanInput) error {
	// Monto variable (Req. 10.10): minimum_amount y maximum_amount > 0 con
	// minimum_amount < maximum_amount cuando variable_amount es verdadero.
	if in.VariableAmount {
		if in.MinimumAmount == nil || in.MaximumAmount == nil {
			return fieldErr("variable_amount", ErrAmountRange)
		}
		if *in.MinimumAmount <= 0 || *in.MaximumAmount <= 0 || *in.MinimumAmount >= *in.MaximumAmount {
			return fieldErr("variable_amount", ErrAmountRange)
		}
	}

	// Pareja frequency_test + test_type_period (Req. 10.11): ambos o ninguno.
	// test_type_period presente se interpreta como un puntero no nil con valor no vacío.
	if err := validatePairPresence(
		"frequency_test/test_type_period",
		in.FrequencyTest != nil,
		in.TestTypePeriod != nil && strings.TrimSpace(*in.TestTypePeriod) != "",
	); err != nil {
		return err
	}

	// Grupo oneshot (Req. 10.12): los tres juntos o ninguno.
	if err := validateGroupPresence(
		"oneshot_amount/oneshot_frequency/oneshot_period",
		in.OneshotAmount != nil,
		in.OneshotFrequency != nil,
		in.OneshotPeriod != nil && strings.TrimSpace(*in.OneshotPeriod) != "",
	); err != nil {
		return err
	}
	return nil
}

// ---------------------------------------------------------------------------
// Subscription
// ---------------------------------------------------------------------------

// SubscriptionInput son los campos de una Subscription sujetos a validación.
type SubscriptionInput struct {
	PlanID             string
	CustomerID         string
	PaymentMethod      string
	PaymentMethodToken string
	Amount             *float64 // opcional
	StartDate          *string  // opcional, RFC3339
}

// ValidateSubscriptionCreate valida la creación de una Subscription
// (Req. 14.2-14.7). Los identificadores UUID se validan con IsUUID (Property 12).
func ValidateSubscriptionCreate(in SubscriptionInput) error {
	if err := validateUUIDRequired("plan_id", in.PlanID); err != nil {
		return err
	}
	if err := validateUUIDRequired("customer_id", in.CustomerID); err != nil {
		return err
	}
	if strings.TrimSpace(in.PaymentMethod) == "" {
		return fieldErr("payment_method", ErrRequired)
	}
	if err := validateUUIDRequired("payment_method_token", in.PaymentMethodToken); err != nil {
		return err
	}
	if in.Amount != nil && *in.Amount <= 0 {
		return fieldErr("amount", ErrNotPositive)
	}
	if in.StartDate != nil {
		if err := validateRFC3339("start_date", *in.StartDate, true); err != nil {
			return err
		}
	}
	return nil
}

// SubscriptionChangeInput son los campos de un cambio de Subscription (Req. 15).
// Todos opcionales; se validan UUID/formato cuando están presentes.
type SubscriptionChangeInput struct {
	PlanID             *string
	CustomerID         *string
	PaymentMethodToken *string
	Amount             *float64
	StartDate          *string
}

// ValidateSubscriptionChange valida un cambio de Subscription (Req. 15.2, 15.3).
// Cada identificador presente debe ser un UUID válido; `start_date` presente
// debe ser RFC3339; `amount` presente debe ser > 0.
func ValidateSubscriptionChange(in SubscriptionChangeInput) error {
	if err := validateUUIDOptional("plan_id", in.PlanID); err != nil {
		return err
	}
	if err := validateUUIDOptional("customer_id", in.CustomerID); err != nil {
		return err
	}
	if err := validateUUIDOptional("payment_method_token", in.PaymentMethodToken); err != nil {
		return err
	}
	if in.Amount != nil && *in.Amount <= 0 {
		return fieldErr("amount", ErrNotPositive)
	}
	if in.StartDate != nil {
		if err := validateRFC3339("start_date", *in.StartDate, true); err != nil {
			return err
		}
	}
	return nil
}

// SubscriptionCancelInput son los campos de la cancelación de Subscription
// (Req. 16): customer_id (UUID requerido) y cancel_reason (opcional, máx 255).
type SubscriptionCancelInput struct {
	CustomerID   string
	CancelReason *string
}

// ValidateSubscriptionCancel valida la cancelación de una Subscription
// (Req. 16.2, 16.3).
func ValidateSubscriptionCancel(in SubscriptionCancelInput) error {
	if err := validateUUIDRequired("customer_id", in.CustomerID); err != nil {
		return err
	}
	if in.CancelReason != nil {
		if err := validateLen("cancel_reason", *in.CancelReason, maxCancelReason); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Card
// ---------------------------------------------------------------------------

// CardInput son los campos de una Card sujetos a validación (Req. 18).
type CardInput struct {
	CustomerID        string
	CardNumber        string
	ExpirationDate    string // MM/YY, futura, máx 10
	CardToken         string // UUID, máx 255
	PaymentMethodName string
	CardFranchise     string
}

// ValidateCardCreate valida la creación de una Card (Req. 18.2-18.7). El
// customer_id y card_token se validan como UUID (Property 12); expiration_date
// debe ser MM/YY y representar una fecha futura.
func ValidateCardCreate(in CardInput) error {
	if err := validateUUIDRequired("customer_id", in.CustomerID); err != nil {
		return err
	}
	if err := validateRequiredLen("card_number", in.CardNumber, maxCardNumber); err != nil {
		return err
	}
	if err := validateExpirationDate(in.ExpirationDate); err != nil {
		return err
	}
	if err := validateCardToken(in.CardToken); err != nil {
		return err
	}
	if err := validateRequiredLen("payment_method_name", in.PaymentMethodName, maxPaymentMethodName); err != nil {
		return err
	}
	if err := validateRequiredLen("card_franchise", in.CardFranchise, maxCardFranchise); err != nil {
		return err
	}
	return nil
}

// ---------------------------------------------------------------------------
// Ayudantes de validación de campo
// ---------------------------------------------------------------------------

// validateRequiredLen exige que value no esté vacío y no exceda max caracteres.
func validateRequiredLen(field, value string, max int) error {
	if strings.TrimSpace(value) == "" {
		return fieldErr(field, ErrRequired)
	}
	return validateLen(field, value, max)
}

// validateLen exige que value no exceda max caracteres (medidos en runes para
// manejar correctamente caracteres multibyte como la "ñ").
func validateLen(field, value string, max int) error {
	if len([]rune(value)) > max {
		return fieldErr(field, ErrTooLong)
	}
	return nil
}

// validateEmail valida formato de correo y longitud máxima (Req. 7.4).
func validateEmail(email string) error {
	if strings.TrimSpace(email) == "" {
		return fieldErr("email", ErrRequired)
	}
	if len([]rune(email)) > maxEmailLen {
		return fieldErr("email", ErrTooLong)
	}
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email {
		return fieldErr("email", ErrInvalidFormat)
	}
	return nil
}

// validatePhone valida el formato '+' seguido de 8 a 15 dígitos (Req. 7.5).
func validatePhone(phone string) error {
	if !phonePattern.MatchString(phone) {
		return fieldErr("phone", ErrInvalidFormat)
	}
	return nil
}

// validateBirthDate valida formato YYYY-MM-DD, que no sea futura y que
// corresponda a una edad de al menos 18 años (Req. 7.6).
func validateBirthDate(birthDate string) error {
	if !birthDatePattern.MatchString(birthDate) {
		return fieldErr("birth_date", ErrInvalidFormat)
	}
	t, err := time.Parse("2006-01-02", birthDate)
	if err != nil {
		return fieldErr("birth_date", ErrInvalidFormat)
	}
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	if t.After(today) {
		return fieldErr("birth_date", ErrNotFutureDate)
	}
	if ageInYears(t, today) < minAdultAge {
		return fieldErr("birth_date", ErrUnderage)
	}
	return nil
}

// ageInYears calcula la edad completa en años de alguien nacido en birth a la
// fecha ref.
func ageInYears(birth, ref time.Time) int {
	years := ref.Year() - birth.Year()
	// Ajuste si aún no ha cumplido años en el año de referencia.
	if ref.Month() < birth.Month() || (ref.Month() == birth.Month() && ref.Day() < birth.Day()) {
		years--
	}
	return years
}

// validateRFC3339 valida que value sea una fecha/hora RFC3339. Si required es
// true, un value vacío produce ErrRequired.
func validateRFC3339(field, value string, required bool) error {
	if strings.TrimSpace(value) == "" {
		if required {
			return fieldErr(field, ErrRequired)
		}
		return nil
	}
	if _, err := time.Parse(time.RFC3339, value); err != nil {
		return fieldErr(field, ErrInvalidFormat)
	}
	return nil
}

// validateExpirationDate valida el formato MM/YY, longitud máxima y que la fecha
// sea futura (Req. 18.4). Se interpreta YY como 20YY y la expiración como válida
// hasta el final del mes indicado.
func validateExpirationDate(exp string) error {
	if strings.TrimSpace(exp) == "" {
		return fieldErr("expiration_date", ErrRequired)
	}
	if len([]rune(exp)) > maxExpirationDate {
		return fieldErr("expiration_date", ErrTooLong)
	}
	if !expirationPattern.MatchString(exp) {
		return fieldErr("expiration_date", ErrInvalidFormat)
	}
	t, err := time.Parse("01/06", exp) // 01 = mes, 06 = año de 2 dígitos
	if err != nil {
		return fieldErr("expiration_date", ErrInvalidFormat)
	}
	// La tarjeta es válida hasta el último instante del mes de expiración:
	// primer día del mes siguiente marca el límite.
	endOfMonth := t.AddDate(0, 1, 0)
	if !endOfMonth.After(time.Now().UTC()) {
		return fieldErr("expiration_date", ErrFutureRequired)
	}
	return nil
}

// validateCardToken valida que card_token sea un UUID y no exceda 255
// caracteres (Req. 18.5).
func validateCardToken(token string) error {
	if strings.TrimSpace(token) == "" {
		return fieldErr("card_token", ErrRequired)
	}
	if len([]rune(token)) > maxCardToken {
		return fieldErr("card_token", ErrTooLong)
	}
	if !IsUUID(token) {
		return fieldErr("card_token", ErrInvalidUUID)
	}
	return nil
}

// validateUUIDRequired exige un UUID no vacío y válido para un campo.
func validateUUIDRequired(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return fieldErr(field, ErrRequired)
	}
	if !IsUUID(value) {
		return fieldErr(field, ErrInvalidUUID)
	}
	return nil
}

// validateUUIDOptional valida un UUID solo si el puntero no es nil. Un valor
// presente pero vacío se considera inválido.
func validateUUIDOptional(field string, value *string) error {
	if value == nil {
		return nil
	}
	if !IsUUID(*value) {
		return fieldErr(field, ErrInvalidUUID)
	}
	return nil
}

// validatePairPresence exige que una pareja de campos esté completa o totalmente
// ausente (Req. 10.11).
func validatePairPresence(field string, aPresent, bPresent bool) error {
	if aPresent != bPresent {
		return fieldErr(field, ErrPairIncomplete)
	}
	return nil
}

// validateGroupPresence exige que un grupo de campos esté completo o totalmente
// ausente (Req. 10.12).
func validateGroupPresence(field string, present ...bool) error {
	anyPresent := false
	allPresent := true
	for _, p := range present {
		if p {
			anyPresent = true
		} else {
			allPresent = false
		}
	}
	if anyPresent && !allPresent {
		return fieldErr(field, ErrPairIncomplete)
	}
	return nil
}
