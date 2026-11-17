import { OPERATOR_OPTIONS } from "@/features/modules/operators";
import type { ComparisonOperator } from "@/features/modules/types";

/**
 * Selector del operador de comparación (9 operadores, etiquetas en español).
 * `value` ausente se muestra como `defaultOperator` (el default del sitio donde
 * se usa: "in" en condiciones y filtros, "eq" en constantes del JOIN y filtros
 * de acción). `className` lo aporta el llamador para igualar el estilo de los
 * demás `<select>` del asistente.
 */
export function OperatorSelect({
  value,
  defaultOperator,
  onChange,
  className,
  ariaLabel = "Operador",
}: {
  readonly value: ComparisonOperator | undefined;
  readonly defaultOperator: ComparisonOperator;
  readonly onChange: (next: ComparisonOperator) => void;
  readonly className?: string;
  readonly ariaLabel?: string;
}) {
  return (
    <select
      className={className}
      aria-label={ariaLabel}
      value={value ?? defaultOperator}
      onChange={(e) => onChange(e.target.value as ComparisonOperator)}
    >
      {OPERATOR_OPTIONS.map((o) => (
        <option key={o.value} value={o.value}>
          {o.label}
        </option>
      ))}
    </select>
  );
}
