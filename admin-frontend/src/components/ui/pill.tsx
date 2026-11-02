/**
 * Pill de resumen reutilizable (FEAT-001).
 *
 * Centraliza los distintivos (<span> redondeados) que etiquetan tablas y
 * columnas en el constructor de módulos. Cuatro variantes semánticas:
 *   - principal → tabla principal (azul de marca, con punto indicador).
 *   - foranea   → tabla foránea (superficie suave neutra).
 *   - count     → recuento de columnas (superficie suave neutra).
 *   - pk        → recuento de llaves primarias (púrpura --chart-3).
 *
 * Presentacional: sin estado ni handlers. Sigue el patrón de mapa
 * Record<variant, string> del repo (como FeedbackBanner), no cva.
 */
import * as React from "react";

import { cn } from "@/lib/utils";

export type PillVariant = "principal" | "foranea" | "count" | "pk";

export interface PillProps {
  readonly variant: PillVariant;
  readonly children: React.ReactNode;
  readonly className?: string;
}

const variantStyles: Readonly<Record<PillVariant, string>> = {
  principal: "bg-pill-principal-bg text-ring",
  foranea: "bg-border-soft text-muted-foreground",
  count: "bg-border-soft text-muted-foreground",
  pk: "bg-pill-pk-bg text-[var(--chart-3)]",
};

/** Distintivo redondeado de resumen. Solo `principal` antepone un punto. */
export function Pill({ variant, children, className }: PillProps) {
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-xs font-semibold",
        variantStyles[variant],
        className,
      )}
    >
      {variant === "principal" ? (
        <span className="size-1.5 rounded-full bg-current" aria-hidden="true" />
      ) : null}
      {children}
    </span>
  );
}

export default Pill;
