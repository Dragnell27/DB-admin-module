import type { ComparisonOperator } from "./types";

/** Operadores soportados con su etiqueta en español, en orden de presentación. */
export const OPERATOR_OPTIONS: ReadonlyArray<{
  value: ComparisonOperator;
  label: string;
}> = [
  { value: "eq", label: "igual a" },
  { value: "neq", label: "distinto de" },
  { value: "in", label: "está en" },
  { value: "not_in", label: "no está en" },
  { value: "contains", label: "contiene" },
  { value: "starts_with", label: "empieza con" },
  { value: "ends_with", label: "termina con" },
  { value: "is_null", label: "es vacío" },
  { value: "is_not_null", label: "no es vacío" },
];

/** Cantidad de valores por operador (max < 0 = sin tope). Espejo del backend. */
const ARITY: Record<ComparisonOperator, { min: number; max: number }> = {
  eq: { min: 1, max: 1 },
  neq: { min: 1, max: 1 },
  in: { min: 1, max: -1 },
  not_in: { min: 1, max: -1 },
  contains: { min: 1, max: 1 },
  starts_with: { min: 1, max: 1 },
  ends_with: { min: 1, max: 1 },
  is_null: { min: 0, max: 0 },
  is_not_null: { min: 0, max: 0 },
};

/** Devuelve el operador si es uno de los soportados; si no, undefined. */
export function toOperator(raw: unknown): ComparisonOperator | undefined {
  return OPERATOR_OPTIONS.find((o) => o.value === raw)?.value;
}

/** Operadores que no usan valor (el input de valores se oculta). */
export function isNullOperator(op: ComparisonOperator | undefined): boolean {
  return op === "is_null" || op === "is_not_null";
}

/** Operadores cuyo valor es una lista separada por comas. */
function isListOperator(op: ComparisonOperator | undefined): boolean {
  return op === "in" || op === "not_in";
}

/**
 * Convierte el texto del input en la lista de valores según el operador.
 * in/not_in parten por comas (trim, sin vacíos); eq/neq/LIKE usan el texto
 * completo (sin partir por comas); nulidad no usa valores.
 */
export function splitValues(op: ComparisonOperator, text: string): string[] {
  if (isNullOperator(op)) return [];
  if (isListOperator(op)) {
    return text
      .split(",")
      .map((v) => v.trim())
      .filter((v) => v !== "");
  }
  const t = text.trim();
  return t === "" ? [] : [t];
}

/** Mensaje en español si la cantidad de valores no es válida; null si es válida. */
export function validateOperatorValues(
  op: ComparisonOperator,
  values: string[],
): string | null {
  const label = OPERATOR_OPTIONS.find((o) => o.value === op)?.label ?? op;
  const { min, max } = ARITY[op];
  // splitValues nunca produce más de un valor para los operadores de 1 valor.
  if (max === 0) {
    return values.length > 0
      ? `el operador «${label}» no admite valores`
      : null;
  }
  if (values.length < min) {
    return max < 0
      ? `el operador «${label}» necesita al menos un valor`
      : `el operador «${label}» necesita un valor`;
  }
  return null;
}
