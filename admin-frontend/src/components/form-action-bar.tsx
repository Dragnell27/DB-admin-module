"use client";

/**
 * Barra de acciones de formulario SIEMPRE visible (presentacional, sin estado).
 *
 * Patrón compartido entre el asistente de módulos (module-builder) y el editor
 * de plantillas de conexión (connection-templates) para que las acciones
 * Guardar/Cancelar sean descubribles sin tener que desplazarse hasta el final.
 * Con `sticky` (por defecto) se fija al fondo del CONTENEDOR del formulario
 * (`sticky bottom-0`), alineada al ancho del contenido (NO `fixed` al viewport,
 * que ignoraría el nav lateral de la app). Con `sticky={false}` se comporta como
 * un bloque estructural normal: es el PIE anclado de un layout de tres zonas
 * (cabecera / cuerpo con scroll / pie) sin recurrir a hacks de posicionamiento.
 *
 * No contiene lógica de guardado: cada pantalla pasa sus propios `Button` como
 * `children` (conservando sus handlers, labels, spinner y estado `disabled`) y,
 * opcionalmente, un `hint` a la izquierda (p. ej. un aviso de sección inválida).
 * Sin colores hardcodeados: usa tokens del tema. Sin nuevas dependencias.
 */
import * as React from "react";

import { cn } from "@/lib/utils";

export interface FormActionBarProps {
  /** Acciones del formulario (p. ej. los Button Cancelar/Guardar). */
  readonly children: React.ReactNode;
  /** Zona izquierda opcional para mensajes de ayuda/error. */
  readonly hint?: React.ReactNode;
  readonly className?: string;
  /**
   * Si `true` (por defecto, para no romper llamadas existentes) la barra se
   * fija al fondo del contenedor con `sticky bottom-0`. Si `false`, se renderiza
   * como un bloque estructural normal (p. ej. el PIE de un layout de tres zonas
   * donde el scroll vive en el CUERPO), conservando el borde, fondo y layout.
   */
  readonly sticky?: boolean;
}

export function FormActionBar({ children, hint, className, sticky = true }: FormActionBarProps) {
  return (
    <div
      className={cn(
        "flex flex-wrap items-center justify-between gap-3 border-t border-border bg-background/95 px-4 py-3 backdrop-blur",
        sticky && "sticky bottom-0 z-10",
        className,
      )}
    >
      <div className="min-h-5 text-xs text-muted-foreground">{hint}</div>
      <div className="flex items-center gap-2">{children}</div>
    </div>
  );
}

export default FormActionBar;
