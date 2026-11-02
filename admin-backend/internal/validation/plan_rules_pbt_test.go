// Feature: panel-admin-ott, Property 11: Reglas de monto variable y campos emparejados en Plan
//
// Prueba basada en propiedades (PBT) de las reglas condicionales de Plan del
// admin-backend (BFF): monto variable y grupos de campos emparejados.
//
// Validates: Requirements 10.10, 10.11, 10.12
//
// Propiedad: *para todo* Plan por lo demás válido (nombre, external_id, país
// coherente con el token, moneda correcta, amount > 0, frequency > 0 y
// period_type permitido), ValidatePlanCreate devuelve nil SI Y SOLO SI se
// cumplen simultáneamente:
//   - Regla de monto variable (Req. 10.10): si variable_amount es verdadero,
//     minimum_amount y maximum_amount están presentes, ambos > 0 y
//     minimum_amount < maximum_amount. Si variable_amount es falso, la regla no
//     impone restricción sobre min/max.
//   - Pareja frequency_test + test_type_period (Req. 10.11): ambos presentes o
//     ambos ausentes. test_type_period "presente" significa puntero no nil con
//     valor no vacío tras recortar espacios.
//   - Grupo oneshot (Req. 10.12): oneshot_amount + oneshot_frequency +
//     oneshot_period están los tres presentes o los tres ausentes.
//     oneshot_period "presente" significa puntero no nil con valor no vacío.
//
// En cualquier otro caso, ValidatePlanCreate devuelve un error. Para aislar
// estas reglas condicionales de las de país/moneda/formato, el generador
// construye siempre un Plan por lo demás válido con un `tokenCountry`
// consistente y varía únicamente variable_amount y los campos emparejados.
//
// Se usa la librería estándar `testing/quick` (mínimo 100 iteraciones). El
// oráculo de aceptación es independiente de la implementación bajo prueba.
package validation

import (
	"errors"
	"math/rand"
	"reflect"
	"strings"
	"testing"
	"testing/quick"
)

// planRulesPBTIterations es el número mínimo de iteraciones exigido por las
// convenciones de PBT del proyecto (mínimo 100).
const planRulesPBTIterations = 300

// planRulesPBTTokenCountry es el país del JWT OTT activo usado de forma
// consistente en toda la propiedad. Debe coincidir con el país del Plan y su
// moneda asociada para que la validación de país/moneda siempre pase y así
// aislar las reglas condicionales.
const planRulesPBTTokenCountry = CountryGT // "gt" -> GTQ

// planRulesPBTCurrency es la moneda coherente con planRulesPBTTokenCountry.
const planRulesPBTCurrency = "GTQ"

// planRulesPBTPeriods son valores válidos de period_type (Req. 10.9), usados
// tanto para el period_type base como para oneshot_period.
var planRulesPBTPeriods = []string{"dia", "semana", "mes", "año"}

// planRulesPBTInput describe los campos condicionales generados para una
// iteración, antes de ensamblarlos en un PlanInput por lo demás válido.
type planRulesPBTInput struct {
	variableAmount bool
	minimumAmount  *float64
	maximumAmount  *float64

	frequencyTest  *int
	testTypePeriod *string

	oneshotAmount    *float64
	oneshotFrequency *int
	oneshotPeriod    *string

	// basePeriodType es el period_type base del Plan (siempre uno válido).
	basePeriodType string
}

// Generate satisface quick.Generator produciendo combinaciones variadas de los
// campos condicionales, cubriendo presencia/ausencia y valores válidos/inválidos
// para ejercitar tanto los casos aceptados como los rechazados.
func (planRulesPBTInput) Generate(rnd *rand.Rand, size int) reflect.Value {
	in := planRulesPBTInput{
		basePeriodType: planRulesPBTPeriods[rnd.Intn(len(planRulesPBTPeriods))],
	}

	// --- Monto variable (Req. 10.10) ---
	in.variableAmount = rnd.Intn(2) == 0
	in.minimumAmount, in.maximumAmount = planRulesPBTGenMinMax(rnd)

	// --- Pareja frequency_test + test_type_period (Req. 10.11) ---
	in.frequencyTest, in.testTypePeriod = planRulesPBTGenPair(rnd)

	// --- Grupo oneshot (Req. 10.12) ---
	in.oneshotAmount, in.oneshotFrequency, in.oneshotPeriod = planRulesPBTGenOneshot(rnd)

	return reflect.ValueOf(in)
}

// planRulesPBTGenMinMax genera combinaciones de minimum/maximum: ausentes,
// parcialmente presentes, y presentes con valores válidos/inválidos (negativos,
// cero, iguales, invertidos, correctos).
func planRulesPBTGenMinMax(rnd *rand.Rand) (*float64, *float64) {
	switch rnd.Intn(8) {
	case 0:
		return nil, nil // ambos ausentes
	case 1:
		return planRulesPBTF(5), nil // solo min
	case 2:
		return nil, planRulesPBTF(10) // solo max
	case 3:
		return planRulesPBTF(5), planRulesPBTF(10) // válido: 0 < min < max
	case 4:
		return planRulesPBTF(10), planRulesPBTF(5) // invertido: min > max
	case 5:
		return planRulesPBTF(7), planRulesPBTF(7) // iguales: min == max
	case 6:
		return planRulesPBTF(0), planRulesPBTF(10) // min no positivo
	default:
		return planRulesPBTF(-3), planRulesPBTF(10) // min negativo
	}
}

// planRulesPBTGenPair genera combinaciones de la pareja frequency_test +
// test_type_period: ambos ausentes, solo uno, ambos presentes, incluyendo el
// caso borde de test_type_period presente pero vacío (se considera ausente).
func planRulesPBTGenPair(rnd *rand.Rand) (*int, *string) {
	switch rnd.Intn(6) {
	case 0:
		return nil, nil // ambos ausentes
	case 1:
		return planRulesPBTI(2), nil // solo frequency_test
	case 2:
		return nil, planRulesPBTS("mes") // solo test_type_period
	case 3:
		return planRulesPBTI(2), planRulesPBTS("mes") // ambos presentes
	case 4:
		// test_type_period puntero no nil pero vacío/espacios: cuenta como ausente.
		return planRulesPBTI(2), planRulesPBTS("   ")
	default:
		// Solo un test_type_period no vacío, sin frequency_test.
		return nil, planRulesPBTS("semana")
	}
}

// planRulesPBTGenOneshot genera combinaciones del trío oneshot: ninguno, todos,
// y subconjuntos parciales, incluyendo oneshot_period vacío (cuenta como
// ausente).
func planRulesPBTGenOneshot(rnd *rand.Rand) (*float64, *int, *string) {
	switch rnd.Intn(8) {
	case 0:
		return nil, nil, nil // ninguno
	case 1:
		return planRulesPBTF(100), planRulesPBTI(3), planRulesPBTS("mes") // todos
	case 2:
		return planRulesPBTF(100), nil, nil // solo amount
	case 3:
		return nil, planRulesPBTI(3), nil // solo frequency
	case 4:
		return nil, nil, planRulesPBTS("mes") // solo period
	case 5:
		return planRulesPBTF(100), planRulesPBTI(3), nil // falta period
	case 6:
		return planRulesPBTF(100), planRulesPBTI(3), planRulesPBTS("  ") // period vacío => ausente
	default:
		return nil, planRulesPBTI(3), planRulesPBTS("dia") // falta amount
	}
}

// planRulesPBTF devuelve un puntero a float64.
func planRulesPBTF(v float64) *float64 { return &v }

// planRulesPBTI devuelve un puntero a int.
func planRulesPBTI(v int) *int { return &v }

// planRulesPBTS devuelve un puntero a string.
func planRulesPBTS(v string) *string { return &v }

// planRulesPBTPlan ensambla un PlanInput por lo demás VÁLIDO (campos de
// país/moneda/nombre correctos) a partir de los campos condicionales generados.
func planRulesPBTPlan(in planRulesPBTInput) PlanInput {
	return PlanInput{
		Name:           "Plan de prueba",
		ExternalID:     "plan-ext-001",
		Country:        planRulesPBTTokenCountry,
		Amount:         100,
		Currency:       planRulesPBTCurrency,
		Frequency:      1,
		PeriodType:     in.basePeriodType,
		VariableAmount: in.variableAmount,
		MinimumAmount:  in.minimumAmount,
		MaximumAmount:  in.maximumAmount,
		FrequencyTest:  in.frequencyTest,
		TestTypePeriod: in.testTypePeriod,
		OneshotAmount:    in.oneshotAmount,
		OneshotFrequency: in.oneshotFrequency,
		OneshotPeriod:    in.oneshotPeriod,
	}
}

// planRulesPBTPresent replica el predicado de "presencia" para campos string por
// puntero: no nil y no vacío tras recortar espacios.
func planRulesPBTPresent(p *string) bool {
	return p != nil && strings.TrimSpace(*p) != ""
}

// planRulesPBTVariableAmountOK indica si la regla de monto variable se cumple
// (Req. 10.10), de forma independiente a la implementación.
func planRulesPBTVariableAmountOK(in planRulesPBTInput) bool {
	if !in.variableAmount {
		return true // sin restricción cuando variable_amount es falso
	}
	if in.minimumAmount == nil || in.maximumAmount == nil {
		return false
	}
	return *in.minimumAmount > 0 && *in.maximumAmount > 0 && *in.minimumAmount < *in.maximumAmount
}

// planRulesPBTPairOK indica si la pareja frequency_test + test_type_period está
// completa o totalmente ausente (Req. 10.11).
func planRulesPBTPairOK(in planRulesPBTInput) bool {
	a := in.frequencyTest != nil
	b := planRulesPBTPresent(in.testTypePeriod)
	return a == b
}

// planRulesPBTOneshotOK indica si el trío oneshot está todo presente o todo
// ausente (Req. 10.12).
func planRulesPBTOneshotOK(in planRulesPBTInput) bool {
	a := in.oneshotAmount != nil
	b := in.oneshotFrequency != nil
	c := planRulesPBTPresent(in.oneshotPeriod)
	allPresent := a && b && c
	nonePresent := !a && !b && !c
	return allPresent || nonePresent
}

// planRulesPBTShouldAccept es el oráculo del "si y solo si": la combinación de
// las tres reglas condicionales (sobre un Plan por lo demás válido).
func planRulesPBTShouldAccept(in planRulesPBTInput) bool {
	return planRulesPBTVariableAmountOK(in) && planRulesPBTPairOK(in) && planRulesPBTOneshotOK(in)
}

// TestValidatePlanConditionalRulesProperty11 verifica la Property 11:
// ValidatePlanCreate (sobre un Plan por lo demás válido) acepta la entrada si y
// solo si se cumplen la regla de monto variable y las de grupos emparejados.
func TestValidatePlanConditionalRulesProperty11(t *testing.T) {
	property := func(in planRulesPBTInput) bool {
		plan := planRulesPBTPlan(in)
		err := ValidatePlanCreate(planRulesPBTTokenCountry, plan)

		shouldAccept := planRulesPBTShouldAccept(in)

		if shouldAccept {
			// Debe aceptarse: sin error.
			return err == nil
		}

		// Debe rechazarse: error no-nil y, además, debe ser uno de los errores
		// centinela de las reglas condicionales (ErrAmountRange para monto
		// variable, ErrPairIncomplete para pareja/grupo). Como el Plan es por lo
		// demás válido, el error solo puede provenir de estas reglas.
		if err == nil {
			return false
		}
		if planRulesPBTVariableAmountOK(in) {
			// El fallo debe ser de pareja/grupo.
			return errors.Is(err, ErrPairIncomplete)
		}
		// La regla de monto variable se incumple.
		return errors.Is(err, ErrAmountRange)
	}

	cfg := &quick.Config{MaxCount: planRulesPBTIterations}
	if err := quick.Check(property, cfg); err != nil {
		t.Fatalf("Property 11 violada: ValidatePlanCreate no respeta las reglas de monto variable/campos emparejados: %v", err)
	}
}

// TestValidatePlanConditionalRulesKnownCases complementa la generación aleatoria
// con casos frontera deterministas explícitos sobre un Plan por lo demás válido.
func TestValidatePlanConditionalRulesKnownCases(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*PlanInput)
		wantErr error // nil => se espera éxito
	}{
		{
			name:    "sin campos condicionales (todos ausentes)",
			mutate:  func(p *PlanInput) {},
			wantErr: nil,
		},
		{
			name: "variable_amount con rango válido",
			mutate: func(p *PlanInput) {
				p.VariableAmount = true
				p.MinimumAmount = planRulesPBTF(5)
				p.MaximumAmount = planRulesPBTF(10)
			},
			wantErr: nil,
		},
		{
			name: "variable_amount sin min/max",
			mutate: func(p *PlanInput) {
				p.VariableAmount = true
			},
			wantErr: ErrAmountRange,
		},
		{
			name: "variable_amount con min >= max",
			mutate: func(p *PlanInput) {
				p.VariableAmount = true
				p.MinimumAmount = planRulesPBTF(10)
				p.MaximumAmount = planRulesPBTF(5)
			},
			wantErr: ErrAmountRange,
		},
		{
			name: "variable_amount con min no positivo",
			mutate: func(p *PlanInput) {
				p.VariableAmount = true
				p.MinimumAmount = planRulesPBTF(0)
				p.MaximumAmount = planRulesPBTF(10)
			},
			wantErr: ErrAmountRange,
		},
		{
			name: "variable_amount falso ignora min/max incoherentes",
			mutate: func(p *PlanInput) {
				p.VariableAmount = false
				p.MinimumAmount = planRulesPBTF(10)
				p.MaximumAmount = planRulesPBTF(5)
			},
			wantErr: nil,
		},
		{
			name: "pareja test completa",
			mutate: func(p *PlanInput) {
				p.FrequencyTest = planRulesPBTI(2)
				p.TestTypePeriod = planRulesPBTS("mes")
			},
			wantErr: nil,
		},
		{
			name: "pareja test solo frequency_test",
			mutate: func(p *PlanInput) {
				p.FrequencyTest = planRulesPBTI(2)
			},
			wantErr: ErrPairIncomplete,
		},
		{
			name: "pareja test solo test_type_period",
			mutate: func(p *PlanInput) {
				p.TestTypePeriod = planRulesPBTS("mes")
			},
			wantErr: ErrPairIncomplete,
		},
		{
			name: "pareja test con test_type_period vacío cuenta como ausente",
			mutate: func(p *PlanInput) {
				p.TestTypePeriod = planRulesPBTS("   ")
			},
			wantErr: nil,
		},
		{
			name: "oneshot completo",
			mutate: func(p *PlanInput) {
				p.OneshotAmount = planRulesPBTF(100)
				p.OneshotFrequency = planRulesPBTI(3)
				p.OneshotPeriod = planRulesPBTS("mes")
			},
			wantErr: nil,
		},
		{
			name: "oneshot parcial (falta period)",
			mutate: func(p *PlanInput) {
				p.OneshotAmount = planRulesPBTF(100)
				p.OneshotFrequency = planRulesPBTI(3)
			},
			wantErr: ErrPairIncomplete,
		},
		{
			name: "oneshot parcial (solo amount)",
			mutate: func(p *PlanInput) {
				p.OneshotAmount = planRulesPBTF(100)
			},
			wantErr: ErrPairIncomplete,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan := planRulesPBTPlan(planRulesPBTInput{basePeriodType: "mes"})
			tc.mutate(&plan)

			err := ValidatePlanCreate(planRulesPBTTokenCountry, plan)

			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("ValidatePlanCreate(...) error = %v; se esperaba nil", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("ValidatePlanCreate(...) error = %v; se esperaba %v", err, tc.wantErr)
			}
		})
	}
}
