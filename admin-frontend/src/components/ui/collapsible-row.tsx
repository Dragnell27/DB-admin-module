"use client";

/**
 * Patrón de ACORDEÓN reutilizable del panel: una fila/sección con encabezado
 * clicable (chevron + contenido de resumen) y un cuerpo colapsable. Unifica el
 * look de los dos usos del rediseño de navegación por colapso:
 *   - SECCIONES del asistente de módulos (module-wizard): varias secciones
 *     apiladas, cada una con su resumen a la derecha y su validez a la izquierda.
 *   - FILAS repetidas (campos de conexión, campos de acción, relaciones): fila
 *     colapsada con resumen → expandir para editar.
 *
 * Mismos tokens y gesto en todos los sitios: contenedor `rounded-lg border
 * bg-card shadow-elevation`, chevron `ChevronDown` que rota `-rotate-90` al
 * cerrar, hover `accent-hover`, panel separado por `border-soft`. Sin
 * dependencias nuevas; solo primitivas del proyecto.
 *
 * Accesibilidad: el encabezado ES un <button> con `aria-expanded` y
 * `aria-controls` apuntando al panel (que lleva el `id` correspondiente). Las
 * ACCIONES de la fila (p. ej. borrar) viven FUERA del botón, a su derecha, para
 * no anidar controles interactivos dentro del botón de toggle.
 */
import * as React from "react";
import { ChevronDown } from "lucide-react";

import { cn } from "@/lib/utils";

export interface CollapsibleRowProps {
  /** `id` del panel; el botón lo referencia con `aria-controls`. */
  readonly panelId: string;
  /** Estado abierto/cerrado (controlado por el consumidor). */
  readonly open: boolean;
  /** Alterna el estado abierto/cerrado. */
  readonly onToggle: () => void;
  /**
   * Contenido del encabezado (resumen): se muestra SIEMPRE, abierto o cerrado,
   * a la derecha del chevron. Suele ser el resumen legible de la fila/sección.
   */
  readonly header: React.ReactNode;
  /**
   * Adorno OPCIONAL a la izquierda del título, dentro del botón (p. ej. el
   * icono de validez de una sección). No interactivo.
   */
  readonly leading?: React.ReactNode;
  /**
   * Acciones OPCIONALES a la derecha del encabezado, FUERA del botón de toggle
   * (p. ej. el botón de borrar la fila). Interactivas.
   */
  readonly actions?: React.ReactNode;
  /** Cuerpo colapsable; se renderiza solo cuando `open` es verdadero. */
  readonly children: React.ReactNode;
  /** Clase extra para el contenedor externo. */
  readonly className?: string;
  /** Clase extra para el panel (cuerpo). Por defecto `p-3`. */
  readonly panelClassName?: string;
  /** Etiqueta accesible del botón de toggle si el `header` no basta. */
  readonly toggleLabel?: string;
}

export function CollapsibleRow({
  panelId,
  open,
  onToggle,
  header,
  leading,
  actions,
  children,
  className,
  panelClassName,
  toggleLabel,
}: CollapsibleRowProps) {
  return (
    <div
      className={cn(
        "rounded-lg border border-border bg-card shadow-elevation",
        className,
      )}
    >
      <div className="flex items-center gap-1">
        <button
          type="button"
          onClick={onToggle}
          aria-expanded={open}
          aria-controls={panelId}
          aria-label={toggleLabel}
          className="flex min-w-0 flex-1 items-center gap-3 rounded-lg px-3 py-2.5 text-left transition-colors hover:bg-accent-hover"
        >
          <ChevronDown
            className={cn(
              "size-4 shrink-0 text-muted-foreground transition-transform",
              open ? "" : "-rotate-90",
            )}
            aria-hidden="true"
          />
          {leading ? (
            <span className="shrink-0" aria-hidden="true">
              {leading}
            </span>
          ) : null}
          <span className="min-w-0 flex-1">{header}</span>
        </button>
        {actions ? <div className="flex shrink-0 items-center gap-1 pr-2">{actions}</div> : null}
      </div>
      {open ? (
        <div
          id={panelId}
          className={cn("border-t border-border-soft p-3", panelClassName)}
        >
          {children}
        </div>
      ) : null}
    </div>
  );
}

export default CollapsibleRow;
