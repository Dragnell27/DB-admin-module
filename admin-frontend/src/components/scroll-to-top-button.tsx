"use client";

/**
 * `ScrollToTopButton`: botón flotante para volver al inicio de la vista.
 *
 * Reemplaza a la antigua barra de filtros fija (que se superponía sobre las
 * filas de la tabla al desplazar). Es un botón redondo, solo con icono, anclado
 * a la esquina inferior derecha del área visible (`fixed bottom-6 right-6`), que
 * desplaza la vista hasta su tope.
 *
 * El scroll del panel vive en el `<main className="flex-1 overflow-y-auto p-6">`
 * de `src/app/(protected)/layout.tsx`; el desplazamiento real lo hace
 * `useScrollToTop` sobre un ancla al tope de cada vista, respetando
 * `prefers-reduced-motion`. Este componente solo dispara esa callback.
 *
 * `z-40` lo mantiene por encima del contenido de la vista pero por debajo de los
 * overlays/diálogos (que usan su propio stacking vía portal).
 */
import * as React from "react";
import { ArrowUp } from "lucide-react";

import { cn } from "@/lib/utils";

export interface ScrollToTopButtonProps {
  /** Desplaza la vista hasta su tope (normalmente `scrollToTop` del hook). */
  readonly onClick: () => void;
  /** Clases adicionales opcionales. */
  readonly className?: string;
}

export function ScrollToTopButton({
  onClick,
  className,
}: ScrollToTopButtonProps) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-label="Ir al inicio de la vista"
      title="Ir al inicio"
      className={cn(
        "fixed bottom-6 right-6 z-40 flex size-11 items-center justify-center rounded-full",
        "border border-border bg-primary text-primary-foreground shadow-lg",
        "transition-colors hover:bg-primary/90",
        "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background",
        className,
      )}
    >
      <ArrowUp className="size-5" aria-hidden="true" />
    </button>
  );
}

export default ScrollToTopButton;
