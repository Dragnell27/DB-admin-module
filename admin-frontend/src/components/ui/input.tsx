import * as React from "react";

import { cn } from "@/lib/utils";

/**
 * Input de formulario reutilizable (shadcn/ui, estilo new-york) adaptado al
 * tema oscuro del panel: fondo `--input` (~#0f1220), borde sutil y radio de
 * 8px (Req. 1.2). Lo reutilizan la pantalla de login (tarea 13.2) y los
 * formularios de recurso (tareas 14.x/16.x).
 */
export type InputProps = React.InputHTMLAttributes<HTMLInputElement>;

const Input = React.forwardRef<HTMLInputElement, InputProps>(
  ({ className, type, ...props }, ref) => {
    return (
      <input
        type={type}
        ref={ref}
        className={cn(
          "flex h-10 w-full rounded-md border border-input-border bg-[var(--input-surface,var(--input))] px-3 py-2 text-sm text-foreground ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50 file:border-0 file:bg-transparent file:text-sm file:font-medium",
          className,
        )}
        {...props}
      />
    );
  },
);
Input.displayName = "Input";

export { Input };
