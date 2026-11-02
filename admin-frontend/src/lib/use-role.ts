"use client";

/**
 * Hook `useRole()` (Req. 5.1): lee el rol del JWT de sesión del admin para
 * condicionar la UI (p. ej. mostrar los botones Crear/Editar/Eliminar/Cancelar
 * sólo a `admin`). La seguridad real la aplica el BFF; esto es sólo visibilidad.
 *
 * El valor se calcula desde `getRole()` (ver `session.ts`) y se mantiene
 * sincronizado con el almacenamiento:
 * - Se recalcula al montar en el cliente (evita desajustes de hidratación: el
 *   render del servidor y el primer render del cliente devuelven `null`).
 * - Se actualiza ante el evento `storage` del navegador, de modo que un login o
 *   logout en otra pestaña se refleje sin recargar.
 */
import { useEffect, useState } from "react";

import {
  SESSION_TOKEN_KEY,
  getRole,
  subscribeSession,
  type Role,
} from "@/lib/session";

/**
 * Devuelve el rol de la sesión actual (`admin` | `viewer`), o `null` cuando no
 * hay sesión utilizable, el claim de rol no es reconocido, o aún no se ha
 * hidratado en el cliente.
 */
export function useRole(): Role | null {
  // Primer render (incluido SSR): `null` para que servidor y cliente coincidan.
  const [role, setRole] = useState<Role | null>(null);

  useEffect(() => {
    // Tras montar, leemos el rol real desde el almacenamiento del cliente.
    const read = (): void => setRole(getRole());
    read();

    // Reaccionar a cambios de sesión en otras pestañas (login/logout).
    function handleStorage(event: StorageEvent): void {
      if (event.key === null || event.key === SESSION_TOKEN_KEY) {
        read();
      }
    }

    // Reaccionar a cambios de sesión in-tab (p. ej. "Recargar permisos").
    const unsub = subscribeSession(read);
    window.addEventListener("storage", handleStorage);
    return () => {
      window.removeEventListener("storage", handleStorage);
      unsub();
    };
  }, []);

  return role;
}
