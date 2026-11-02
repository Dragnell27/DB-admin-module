"use client";

/**
 * Botón de cierre de sesión (Req. 2.4).
 *
 * Control de UI listo para usar que invoca `useLogout()` al hacer clic: borra
 * el JWT de sesión del admin y redirige a `/login`. Es una primitiva componible
 * que la tarea 14.1 puede colocar en el sidebar/cabecera del layout protegido.
 *
 * Hereda la API de `Button` (variant, size, className, etc.), de modo que el
 * layout pueda ajustar su apariencia sin reescribir la lógica de cierre.
 */
import * as React from "react";
import { LogOut } from "lucide-react";

import { Button, type ButtonProps } from "@/components/ui/button";
import { useLogout } from "@/lib/use-logout";

export interface LogoutButtonProps extends Omit<ButtonProps, "onClick"> {
  /** Oculta el icono de salida (por defecto se muestra). */
  readonly hideIcon?: boolean;
}

/**
 * Botón que cierra la sesión del administrador. Por defecto muestra un icono y
 * el texto "Cerrar sesión"; se puede sobreescribir pasando `children`.
 */
export function LogoutButton({
  children,
  hideIcon = false,
  variant = "ghost",
  type = "button",
  ...props
}: LogoutButtonProps) {
  const logout = useLogout();

  return (
    <Button type={type} variant={variant} onClick={logout} {...props}>
      {hideIcon ? null : <LogOut aria-hidden="true" />}
      {children ?? "Cerrar sesión"}
    </Button>
  );
}

export default LogoutButton;
