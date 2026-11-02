"use client";

/**
 * Hook `useLogout()` (Req. 2.4): cierre de sesión del administrador.
 *
 * Devuelve una función estable que, al invocarse:
 * 1. Borra el JWT de sesión del admin almacenado (`clearSessionToken()`), y
 * 2. Redirige a la pantalla de login (`/login`) mediante `next/navigation`.
 *
 * Es una primitiva reutilizable y componible: la tarea 14.1 (layout protegido
 * con sidebar) y el `LogoutButton` la consumen para disparar el cierre de
 * sesión desde la UI. No implementa aquí ningún control visual.
 */
import { useCallback } from "react";
import { useRouter } from "next/navigation";

import { clearSessionToken } from "@/lib/session";

/** Ruta pública de login a la que se redirige tras cerrar sesión. */
const LOGIN_PATH = "/login";

/**
 * Devuelve una función `logout` memoizada que borra el token de sesión y
 * redirige a `/login`. La identidad de la función se mantiene estable entre
 * renders (seguro para pasarla como `onClick` o dependencia de efectos).
 */
export function useLogout(): () => void {
  const router = useRouter();

  return useCallback(() => {
    clearSessionToken();
    router.replace(LOGIN_PATH);
  }, [router]);
}

export default useLogout;
