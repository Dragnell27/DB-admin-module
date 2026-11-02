"use client";

/**
 * Hook `useToast()` del sistema de notificaciones flotantes del panel.
 *
 * Lee la lista de toasts del store SSR-safe (`lib/toast-store.ts`) con
 * `useSyncExternalStore` —el mismo patrón que los hooks del menú lateral— y
 * expone la API imperativa `toast.success/error/warning` junto con `dismiss`.
 *
 * El `getToasts` se pasa también como snapshot del SERVIDOR (devuelve el array
 * vacío estable), igual que el `() => ""` de los hooks del menú lateral, para
 * que SSR y la hidratación coincidan.
 *
 * Los call sites que SOLO disparan un toast (callbacks de TanStack Query,
 * efectos de `?saved=1`) pueden importar `toast` directamente desde este módulo
 * o desde `lib/toast-store` sin suscribirse al estado.
 */
import * as React from "react";

import {
  dismissToast,
  getToasts,
  subscribeToasts,
  toast,
} from "@/lib/toast-store";

export { toast };

export function useToast() {
  const toasts = React.useSyncExternalStore(
    subscribeToasts,
    getToasts,
    getToasts,
  );
  return { toasts, toast, dismiss: dismissToast };
}
