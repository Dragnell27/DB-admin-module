import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";

import { cn } from "@/lib/utils";

/**
 * Label de formulario reutilizable (shadcn/ui, estilo new-york).
 *
 * La variante oficial de shadcn se apoya en `@radix-ui/react-label`; aquí se
 * implementa sobre un `<label>` nativo para no introducir una dependencia
 * adicional, conservando la misma API (`htmlFor`, clases) y los mismos estilos
 * de tipografía/estado deshabilitado. Es genérico y lo reutilizan la pantalla
 * de login (tarea 13.2) y los formularios de recurso (tareas 14.x/16.x).
 */
const labelVariants = cva(
  "text-sm font-medium leading-none text-foreground peer-disabled:cursor-not-allowed peer-disabled:opacity-70",
);

export type LabelProps = React.LabelHTMLAttributes<HTMLLabelElement> &
  VariantProps<typeof labelVariants>;

const Label = React.forwardRef<HTMLLabelElement, LabelProps>(
  ({ className, ...props }, ref) => (
    <label ref={ref} className={cn(labelVariants(), className)} {...props} />
  ),
);
Label.displayName = "Label";

export { Label };
