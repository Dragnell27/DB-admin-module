package modules

import (
	"errors"
	"fmt"
	"strings"
)

// Operator es el operador de comparación de una condición de filtro. Es un enum
// CERRADO: cualquier valor fuera de AllOperators se rechaza (nunca se interpola
// en SQL; el motor despacha por un mapa cerrado de generadores).
type Operator string

const (
	OpEq         Operator = "eq"
	OpNeq        Operator = "neq"
	OpIn         Operator = "in"
	OpNotIn      Operator = "not_in"
	OpContains   Operator = "contains"
	OpStartsWith Operator = "starts_with"
	OpEndsWith   Operator = "ends_with"
	OpIsNull     Operator = "is_null"
	OpIsNotNull  Operator = "is_not_null"
)

// ErrInvalidCondition envuelve los rechazos de operador/valores de una
// condición. writeDefError lo mapea a 422 E422_VALIDATION.
var ErrInvalidCondition = errors.New("condición inválida")

// operatorArity es la cantidad de valores que admite cada operador (max < 0 =
// sin tope). Es la única tabla de aridad: la usan el validador de guardado y los
// builders del motor.
var operatorArity = map[Operator]struct{ min, max int }{
	OpEq:         {1, 1},
	OpNeq:        {1, 1},
	OpIn:         {1, -1},
	OpNotIn:      {1, -1},
	OpContains:   {1, 1},
	OpStartsWith: {1, 1},
	OpEndsWith:   {1, 1},
	OpIsNull:     {0, 0},
	OpIsNotNull:  {0, 0},
}

// AllOperators devuelve los operadores soportados en orden estable.
func AllOperators() []Operator {
	return []Operator{
		OpEq, OpNeq, OpIn, OpNotIn, OpContains, OpStartsWith, OpEndsWith, OpIsNull, OpIsNotNull,
	}
}

// Resolve devuelve def cuando el operador está ausente ("") y valida que, si
// viene, sea conocido.
func (o Operator) Resolve(def Operator) (Operator, error) {
	if o == "" {
		return def, nil
	}
	if err := ValidateOperator(o); err != nil {
		return "", err
	}
	return o, nil
}

// ValidateOperator comprueba que el operador sea conocido. "" es válido
// (equivale a ausente).
func ValidateOperator(op Operator) error {
	if op == "" {
		return nil
	}
	if _, ok := operatorArity[op]; !ok {
		names := make([]string, 0, len(operatorArity))
		for _, o := range AllOperators() {
			names = append(names, string(o))
		}
		return fmt.Errorf("%w: operador de comparación desconocido %q (válidos: %s)",
			ErrInvalidCondition, string(op), strings.Join(names, ", "))
	}
	return nil
}

// ValidateOperatorValues valida operador conocido y cardinalidad de values. Con
// strict, además, ningún valor puede quedar vacío tras TrimSpace. strict=false
// solo se usa en la ruta legada (definiciones sin operador declarado).
func ValidateOperatorValues(op Operator, values []string, strict bool) error {
	ar, ok := operatorArity[op]
	if !ok {
		if op == "" {
			return fmt.Errorf("%w: falta el operador de comparación", ErrInvalidCondition)
		}
		return ValidateOperator(op)
	}
	n := len(values)
	if n < ar.min || (ar.max >= 0 && n > ar.max) {
		switch {
		case ar.max == 0:
			return fmt.Errorf("%w: el operador %q no admite valores", ErrInvalidCondition, string(op))
		case ar.max < 0:
			return fmt.Errorf("%w: el operador %q admite al menos %d valor(es)", ErrInvalidCondition, string(op), ar.min)
		default:
			return fmt.Errorf("%w: el operador %q admite exactamente %d valor(es)", ErrInvalidCondition, string(op), ar.max)
		}
	}
	if strict {
		for _, v := range values {
			if strings.TrimSpace(v) == "" {
				return fmt.Errorf("%w: el operador %q no admite valores vacíos", ErrInvalidCondition, string(op))
			}
		}
	}
	return nil
}
