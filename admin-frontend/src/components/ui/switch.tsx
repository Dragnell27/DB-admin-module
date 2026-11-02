import * as React from "react";

import { cn } from "@/lib/utils";

/**
 * Switch (interruptor) accesible construido con Tailwind, sin dependencias
 * nuevas. Se implementa sobre un `<button role="switch">` con `aria-checked`,
 * en lugar de un checkbox nativo, para dar a los toggles del constructor de
 * módulos una jerarquía visual clara en el tema oscuro (Visible, Llave
 * primaria, Editable) con color semántico.
 *
 * Accesibilidad:
 *  - `role="switch"` + `aria-checked` reflejan el estado a los lectores de
 *    pantalla; el `<button>` es enfocable y operable con teclado por defecto.
 *  - Quien lo use debe asociar una etiqueta accesible: con `aria-label`/
 *    `aria-labelledby`, o emparejándolo con un texto visible vía un `id`.
 *  - Estado deshabilitado: `disabled` nativo (no recibe foco ni eventos) y se
 *    atenúa para comunicarlo visualmente.
 *
 * El color del estado ENCENDIDO es configurable (`tone`) para transmitir
 * semántica usando SIEMPRE tokens del tema (sin colores crudos): "primary"
 * (acento del tema, p. ej. Visible/Editable) y "accent" (un acento distinto y
 * consistente con el tema para la Llave primaria, por su significado especial).
 * Apagado usa una superficie neutra del tema (`bg-muted`).
 */
export type SwitchTone = "primary" | "accent";

export interface SwitchProps
  extends Omit<
    React.ButtonHTMLAttributes<HTMLButtonElement>,
    "onChange" | "type" | "role" | "aria-checked"
  > {
  /** Estado actual del interruptor. */
  readonly checked: boolean;
  /** Notifica el nuevo estado al alternar (clic/teclado). */
  readonly onCheckedChange: (checked: boolean) => void;
  /** Color del estado encendido. Por defecto "primary" (acento del tema). */
  readonly tone?: SwitchTone;
}

const TONE_ON: Record<SwitchTone, string> = {
  // Solo tokens del tema (sin hex ni paleta cruda): el acento principal y el
  // de gráficos (púrpura del tema) para distinguir la Llave primaria.
  primary: "bg-primary",
  accent: "bg-chart-3",
};

const Switch = React.forwardRef<HTMLButtonElement, SwitchProps>(
  (
    { checked, onCheckedChange, tone = "primary", disabled, className, ...props },
    ref,
  ) => (
    <button
      ref={ref}
      type="button"
      role="switch"
      aria-checked={checked}
      disabled={disabled}
      onClick={() => onCheckedChange(!checked)}
      className={cn(
        "relative inline-flex h-5 w-9 shrink-0 cursor-pointer items-center rounded-full border border-transparent transition-colors",
        "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background",
        "disabled:cursor-not-allowed disabled:opacity-50",
        checked ? TONE_ON[tone] : "bg-muted",
        className,
      )}
      {...props}
    >
      <span
        aria-hidden="true"
        className={cn(
          "pointer-events-none inline-block size-4 transform rounded-full shadow transition-transform",
          // Pastilla contrastada en AMBOS estados usando tokens del tema:
          // sobre pista clara (encendido) va oscura; sobre pista oscura
          // (apagado, bg-muted) va clara.
          checked ? "translate-x-4 bg-background" : "translate-x-0.5 bg-foreground",
        )}
      />
    </button>
  ),
);
Switch.displayName = "Switch";

export { Switch };
