"use client";

/**
 * Campo de fecha reutilizable con calendario nativo (`<input type="date">`).
 *
 * Dos modos:
 *   - `mode="date"`: el valor es una fecha `YYYY-MM-DD` (p. ej. birth_date).
 *     El valor que entra y sale por `value`/`onChange` es exactamente la fecha.
 *   - `mode="datetime"`: el usuario elige el DÍA en el calendario y el campo
 *     compone un timestamp RFC3339 en UTC usando la HORA DEL MOMENTO en que se
 *     seleccionó (horas, minutos y segundos actuales). El valor que sale por
 *     `onChange` es el RFC3339 completo (`YYYY-MM-DDTHH:MM:SSZ`); el que entra
 *     por `value` puede ser RFC3339 y se muestra el día correspondiente.
 *     Debajo del campo se muestra la hora registrada para que sea evidente.
 *
 * Es un componente controlado (recibe `value` y `onChange`), pensado para
 * integrarse con react-hook-form mediante un `Controller`.
 */
import * as React from "react";

import { cn } from "@/lib/utils";

export type DateFieldMode = "date" | "datetime";

export interface DateFieldProps {
  readonly id?: string;
  readonly mode: DateFieldMode;
  /**
   * Valor controlado. En modo `date` es `YYYY-MM-DD`. En modo `datetime` es un
   * RFC3339 (`YYYY-MM-DDTHH:MM:SSZ`) o vacío.
   */
  readonly value?: string;
  /** Reporta el nuevo valor (fecha `YYYY-MM-DD` o RFC3339 según el modo). */
  readonly onChange: (value: string) => void;
  readonly onBlur?: () => void;
  readonly disabled?: boolean;
  /** Límite inferior del calendario (`YYYY-MM-DD`). */
  readonly min?: string;
  /** Límite superior del calendario (`YYYY-MM-DD`). */
  readonly max?: string;
  readonly className?: string;
  readonly "aria-invalid"?: boolean | "true" | "false";
}

/** Extrae la parte de fecha (`YYYY-MM-DD`) de un valor `date` o RFC3339. */
function toDateInputValue(value: string | undefined): string {
  if (!value) {
    return "";
  }
  // Ya es YYYY-MM-DD.
  if (/^\d{4}-\d{2}-\d{2}$/.test(value)) {
    return value;
  }
  // RFC3339: toma lo anterior a la "T".
  const tIndex = value.indexOf("T");
  if (tIndex > 0) {
    const datePart = value.slice(0, tIndex);
    if (/^\d{4}-\d{2}-\d{2}$/.test(datePart)) {
      return datePart;
    }
  }
  // Último recurso: intenta parsear y formatear en UTC.
  const d = new Date(value);
  if (!Number.isNaN(d.getTime())) {
    return d.toISOString().slice(0, 10);
  }
  return "";
}

/**
 * Combina un día (`YYYY-MM-DD`) elegido por el usuario con la hora ACTUAL
 * (horas, minutos y segundos del momento de la selección) y devuelve un RFC3339
 * en UTC. Se usa el instante actual para la parte de tiempo, tal como se pidió:
 * al seleccionar el día, la hora queda fijada al momento de la selección.
 */
function composeRFC3339FromDay(day: string): string {
  const [y, m, d] = day.split("-").map((p) => Number(p));
  const now = new Date();
  // Fecha elegida + hora actual, interpretada en UTC para emitir "...Z".
  const composed = new Date(
    Date.UTC(
      y,
      m - 1,
      d,
      now.getUTCHours(),
      now.getUTCMinutes(),
      now.getUTCSeconds(),
    ),
  );
  // ISO sin milisegundos: YYYY-MM-DDTHH:MM:SSZ
  return composed.toISOString().replace(/\.\d{3}Z$/, "Z");
}

/** Formatea un RFC3339 a una hora legible (UTC) para mostrar al usuario. */
function formatRegisteredTime(value: string | undefined): string | null {
  if (!value) {
    return null;
  }
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) {
    return null;
  }
  const hh = String(d.getUTCHours()).padStart(2, "0");
  const mm = String(d.getUTCMinutes()).padStart(2, "0");
  const ss = String(d.getUTCSeconds()).padStart(2, "0");
  return `${hh}:${mm}:${ss} UTC`;
}

const inputClassName =
  "flex h-10 w-full rounded-md border border-input bg-[var(--input-surface,var(--input))] px-3 py-2 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50";

export function DateField({
  id,
  mode,
  value,
  onChange,
  onBlur,
  disabled,
  min,
  max,
  className,
  "aria-invalid": ariaInvalid,
}: DateFieldProps) {
  const dayValue = toDateInputValue(value);

  const handleChange = React.useCallback(
    (event: React.ChangeEvent<HTMLInputElement>) => {
      const day = event.target.value; // YYYY-MM-DD o "" si se limpia
      if (day === "") {
        onChange("");
        return;
      }
      if (mode === "datetime") {
        onChange(composeRFC3339FromDay(day));
      } else {
        onChange(day);
      }
    },
    [mode, onChange],
  );

  const registeredTime =
    mode === "datetime" ? formatRegisteredTime(value) : null;

  return (
    <div className="space-y-1">
      <input
        id={id}
        type="date"
        value={dayValue}
        onChange={handleChange}
        onBlur={onBlur}
        disabled={disabled}
        min={min}
        max={max}
        aria-invalid={ariaInvalid}
        className={cn(inputClassName, className)}
      />
      {registeredTime ? (
        <p className="text-xs text-muted-foreground">
          Hora registrada: {registeredTime} (se fija al seleccionar el día)
        </p>
      ) : null}
    </div>
  );
}

export default DateField;
