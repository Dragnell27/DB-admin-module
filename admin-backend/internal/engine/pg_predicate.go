package engine

import (
	"fmt"
	"strings"

	"admin-backend/internal/modules"
)

// colMode decide cómo se compara la columna y cómo se normalizan los args.
type colMode int

const (
	// modeText compara texto normalizado: lower(btrim(col::text)) con args en
	// lower+trim (condiciones de listado, filtros de catálogo y de acción).
	modeText colMode = iota
	// modeRaw compara la columna sin normalizar y con args crudos (constantes del
	// ON): Postgres coacciona el tipo del parámetro (p. ej. string→numérico).
	modeRaw
)

// likeEscaper escapa los comodines de LIKE y la propia barra en una sola pasada.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

func esc(s string) string { return likeEscaper.Replace(s) }

// predicateGenerator produce el fragmento SQL y los args de UN operador. colExpr
// ya viene entrecomillada y validada contra la lista blanca por el llamador.
type predicateGenerator func(colExpr string, values []string, n int, mode colMode) (string, []any)

// predicateGenerators es el mapa CERRADO operador→generador. El operador nunca se
// concatena en el SQL: solo se despacha por este mapa. Añadir un operador exige
// una entrada aquí y en modules.operatorArity (TestEveryOperatorHasGenerator).
var predicateGenerators = map[modules.Operator]predicateGenerator{
	modules.OpEq:         func(c string, v []string, n int, m colMode) (string, []any) { return compareOne(c, "=", v, n, m) },
	modules.OpNeq:        func(c string, v []string, n int, m colMode) (string, []any) { return compareOne(c, "<>", v, n, m) },
	modules.OpIn:         func(c string, v []string, n int, m colMode) (string, []any) { return compareList(c, "IN", v, n, m) },
	modules.OpNotIn:      func(c string, v []string, n int, m colMode) (string, []any) { return compareList(c, "NOT IN", v, n, m) },
	modules.OpContains:   func(c string, v []string, n int, m colMode) (string, []any) { return like(c, "%", "%", v, n, m) },
	modules.OpStartsWith: func(c string, v []string, n int, m colMode) (string, []any) { return like(c, "", "%", v, n, m) },
	modules.OpEndsWith:   func(c string, v []string, n int, m colMode) (string, []any) { return like(c, "%", "", v, n, m) },
	modules.OpIsNull:     func(c string, _ []string, _ int, _ colMode) (string, []any) { return c + " IS NULL", nil },
	modules.OpIsNotNull:  func(c string, _ []string, _ int, _ colMode) (string, []any) { return c + " IS NOT NULL", nil },
}

// textExpr es la expresión comparada en modo texto.
func textExpr(colExpr string, mode colMode) string {
	if mode == modeText {
		return fmt.Sprintf("lower(btrim(%s::text))", colExpr)
	}
	return colExpr
}

// bindArg normaliza un valor según el modo.
func bindArg(v string, mode colMode) string {
	if mode == modeText {
		return strings.ToLower(strings.TrimSpace(v))
	}
	return v
}

func compareOne(colExpr, sqlOp string, values []string, n int, mode colMode) (string, []any) {
	return fmt.Sprintf("%s %s $%d", textExpr(colExpr, mode), sqlOp, n), []any{bindArg(values[0], mode)}
}

func compareList(colExpr, sqlOp string, values []string, n int, mode colMode) (string, []any) {
	placeholders := make([]string, len(values))
	args := make([]any, len(values))
	for i, v := range values {
		placeholders[i] = fmt.Sprintf("$%d", n+i)
		args[i] = bindArg(v, mode)
	}
	return fmt.Sprintf("%s %s (%s)", textExpr(colExpr, mode), sqlOp, strings.Join(placeholders, ", ")), args
}

// like arma el patrón completo en Go (prefix + valor escapado + suffix) y lo liga
// como UN solo parámetro con ESCAPE '\'. ILIKE ya ignora mayúsculas.
func like(colExpr, prefix, suffix string, values []string, n int, mode colMode) (string, []any) {
	v := values[0]
	expr := colExpr + "::text"
	if mode == modeText {
		v = strings.TrimSpace(v)
		expr = fmt.Sprintf("btrim(%s::text)", colExpr)
	}
	return fmt.Sprintf(`%s ILIKE $%d ESCAPE '\'`, expr, n), []any{prefix + esc(v) + suffix}
}

// buildPredicate compone el predicado de UN operador sobre colExpr. strict=false
// solo cuando la definición no declara operador (ruta legada): tolera valores
// vacíos como antes. Devuelve el fragmento, los args y el siguiente $N libre. Un
// operador desconocido o una aridad errónea devuelven ErrValidation sin SQL.
func buildPredicate(op modules.Operator, colExpr string, values []string, startParam int, mode colMode, strict bool) (string, []any, int, error) {
	if err := modules.ValidateOperatorValues(op, values, strict); err != nil {
		return "", nil, startParam, fmt.Errorf("%w: %v", ErrValidation, err)
	}
	gen, ok := predicateGenerators[op]
	if !ok {
		return "", nil, startParam, fmt.Errorf("%w: operador de comparación sin generador %q", ErrValidation, string(op))
	}
	frag, args := gen(colExpr, values, startParam, mode)
	return frag, args, startParam + len(args), nil
}
