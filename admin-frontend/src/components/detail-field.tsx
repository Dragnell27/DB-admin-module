import * as React from "react";

import { cn } from "@/lib/utils";

/**
 * Par etiqueta/valor para las vistas de detalle de recursos. Presenta un campo
 * de forma legible y accesible (la etiqueta describe el valor). Si `mono` es
 * true, el valor se muestra en tipografía monoespaciada (útil para IDs y
 * valores técnicos).
 */
export interface DetailFieldProps {
  /** Etiqueta del campo (p. ej. "Nombre", "Correo"). */
  readonly label: string;
  /** Valor del campo. Si es vacío/undefined se muestra un guion. */
  readonly value?: React.ReactNode;
  /** Muestra el valor en monoespaciado (ids, tokens, etc.). */
  readonly mono?: boolean;
  readonly className?: string;
}

export function DetailField({ label, value, mono, className }: DetailFieldProps) {
  const isEmpty =
    value === undefined ||
    value === null ||
    (typeof value === "string" && value.trim() === "");

  return (
    <div className={cn("space-y-1", className)}>
      <dt className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
        {label}
      </dt>
      <dd
        className={cn(
          "text-sm text-foreground",
          mono && "font-mono break-all",
          isEmpty && "text-muted-foreground",
        )}
      >
        {isEmpty ? "—" : value}
      </dd>
    </div>
  );
}

export default DetailField;
