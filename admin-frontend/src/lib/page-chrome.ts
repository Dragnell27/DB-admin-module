/**
 * Título contextual de la página mostrado en la BARRA SUPERIOR del panel
 * (Entrega B, bloque «título en header»). Antes cada vista pintaba su propio
 * `<h1>` dentro del cuerpo; ahora el título vive en la topbar para ganar
 * espacio vertical y dar una jerarquía coherente (un solo `<h1>` por vista,
 * renderizado por el header).
 *
 * Mismo patrón SSR-safe que `lib/sidebar-collapse.ts` y `lib/theme.ts`: un bus
 * de suscriptores in-tab para `useSyncExternalStore`, snapshot cacheado y
 * estable (string primitivo o null) y snapshot de servidor fijo (`null`) para
 * que SSR y el primer render del cliente coincidan. NO usa `localStorage`: el
 * título es efímero (vive mientras la vista está montada), así que solo se
 * mantiene en memoria.
 */
import * as React from "react";

type Listener = () => void;

const listeners = new Set<Listener>();

/** Título actual de la página (null = sin título declarado). */
let currentTitle: string | null = null;

/** Snapshot del CLIENTE: el título en memoria. */
export function getPageTitle(): string | null {
  return currentTitle;
}

/**
 * Snapshot del SERVIDOR: SIEMPRE null. Garantiza que SSR y el primer render del
 * cliente coincidan (el título lo fija un efecto tras montar la vista).
 */
export function getServerPageTitle(): null {
  return null;
}

/** Fija el título de la página y notifica a los suscriptores. */
export function setPageTitle(title: string | null): void {
  if (currentTitle === title) {
    return;
  }
  currentTitle = title;
  notify();
}

/** Suscribe un listener a los cambios del título. Devuelve el desuscriptor. */
export function subscribePageTitle(listener: Listener): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

function notify(): void {
  for (const listener of listeners) {
    listener();
  }
}

/**
 * Lee el título de la página de forma reactiva y SSR-safe. Lo usa la topbar
 * (`AppHeader`) para renderizar el `<h1>` de la vista activa.
 */
export function usePageTitleValue(): string | null {
  return React.useSyncExternalStore(
    subscribePageTitle,
    getPageTitle,
    getServerPageTitle,
  );
}

/**
 * Declara el título contextual de la vista actual. La vista llama
 * `usePageTitle("Módulos")` y la topbar lo pinta como `<h1>`. Al desmontar, el
 * título se limpia para que la siguiente vista no herede uno obsoleto. Si varias
 * vistas se solapan durante una transición, la última en montar gana; al
 * desmontar solo limpia si el título vigente sigue siendo el suyo.
 */
export function usePageTitle(title: string | null): void {
  React.useEffect(() => {
    setPageTitle(title);
    return () => {
      if (getPageTitle() === title) {
        setPageTitle(null);
      }
    };
  }, [title]);
}
