"use client";

/**
 * Hook `usePermissions()` (Req. 7.2, 15.2): lee los permisos efectivos del JWT
 * de sesión del admin para condicionar la UI (ocultar botones por acción,
 * módulos del sidebar o filtrar el selector de país). La seguridad real la
 * aplica siempre el BFF; esto es sólo visibilidad.
 *
 * El valor se calcula desde `getPermissions()` (ver `session.ts`) y se mantiene
 * sincronizado con el almacenamiento, igual que `useRole()`:
 * - Se inicializa a `[]` para que el render del servidor y el primer render del
 *   cliente coincidan (evita desajustes de hidratación SSR/cliente).
 * - Se recalcula al montar en el cliente, cuando ya hay acceso a `localStorage`.
 * - Se actualiza ante el evento `storage` del navegador, de modo que un login o
 *   logout en otra pestaña se refleje sin recargar.
 */
import { useEffect, useState } from "react";

import {
  can,
  type Action,
  type EffectivePermissions,
  type ModulePermissions,
  type Resource,
} from "@/lib/permissions";
import {
  SESSION_TOKEN_KEY,
  getModulePermissions,
  getPermissions,
  subscribeSession,
} from "@/lib/session";

/**
 * Devuelve los permisos efectivos de la sesión actual, o una lista vacía cuando
 * no hay sesión utilizable o aún no se ha hidratado en el cliente.
 */
export function usePermissions(): EffectivePermissions {
  // Primer render (incluido SSR): `[]` para que servidor y cliente coincidan.
  const [perms, setPerms] = useState<EffectivePermissions>([]);

  useEffect(() => {
    // Tras montar, leemos los permisos reales desde el almacenamiento del cliente.
    const read = (): void => setPerms(getPermissions());
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

  return perms;
}

/**
 * Hook de conveniencia `useCan()`: captura los permisos actuales (vía
 * `usePermissions()`) y devuelve una función `can` ya ligada a ellos, de modo
 * que las vistas sólo pasen la terna (recurso, acción, país).
 *
 * Equivale a llamar `can(usePermissions(), resource, action, country)`, pero
 * resulta más cómodo cuando un componente evalúa varias ternas.
 */
export function useCan(): (
  resource: Resource,
  action: Action,
  country: string,
) => boolean {
  const perms = usePermissions();
  return (resource, action, country) => can(perms, resource, action, country);
}

/**
 * Hook `useModulePermissions()`: lee los permisos sobre módulos dinámicos
 * (module-builder) del JWT de sesión, para condicionar la visibilidad de los
 * módulos en el sidebar y de sus botones de acción. Sigue el mismo patrón
 * SSR-safe que `usePermissions` (inicia en `[]`, recalcula al montar y reacciona
 * al evento `storage`). La seguridad real la aplica el BFF.
 */
export function useModulePermissions(): ModulePermissions {
  const [perms, setPerms] = useState<ModulePermissions>([]);

  useEffect(() => {
    const read = (): void => setPerms(getModulePermissions());
    read();

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

  return perms;
}
