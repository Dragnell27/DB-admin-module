"use client";

/**
 * Hook `useTheme()` del tema CLARO/OSCURO del panel (Entrega A).
 *
 * Lee la preferencia del store SSR-safe (`lib/theme.ts`) con
 * `useSyncExternalStore` —el mismo patrón que `use-toast.ts` y los hooks del
 * menú lateral—. El snapshot de servidor (`getServerTheme`) devuelve el
 * primitivo estable `"dark"`, de modo que SSR y la hidratación coinciden.
 *
 * REGLA DE HIDRATACIÓN: como SSR y el primer render del cliente devuelven
 * `"dark"` aunque el usuario tenga `"light"`, todo consumidor que DERIVE MARCADO
 * del tema (texto, íconos, clases/atributos distintos) debe envolver ese nodo en
 * `suppressHydrationWarning` o, preferiblemente, dejar que el tema afecte SOLO
 * variables CSS (clases con tokens como `bg-card`, cuyo valor cambia por la
 * clase `.dark` sin alterar el árbol de React).
 */
import * as React from "react";

import { getServerTheme, getTheme, subscribeTheme, type Theme } from "@/lib/theme";

export function useTheme(): Theme {
  return React.useSyncExternalStore(subscribeTheme, getTheme, getServerTheme);
}
