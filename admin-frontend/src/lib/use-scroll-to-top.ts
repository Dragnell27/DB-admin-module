"use client";

/**
 * Hook `useScrollToTop()` para desplazar la vista hasta su borde superior.
 *
 * El scroll vertical del panel NO vive en `window`: vive en el contenedor
 * `<main className="flex-1 overflow-y-auto p-6">` de
 * `src/app/(protected)/layout.tsx` (el `AppHeader` queda fuera del `<main>`).
 * Por eso no se usa `window.scrollTo`, sino un ancla invisible al tope de la
 * vista sobre la que se llama `scrollIntoView`: el navegador sube por los
 * ancestros hasta el contenedor scrolleable correcto (`<main>`), sin necesidad
 * de obtener una referencia manual a ese nodo.
 *
 * Devuelve `{ topRef, scrollToTop }`:
 * - `topRef`: se asigna a un `<div aria-hidden="true" />` colocado como primer
 *   hijo del contenedor raíz de la vista (el ancla de "Ir al inicio").
 * - `scrollToTop`: desplaza la vista hasta ese ancla.
 *
 * El `behavior` del scroll respeta `prefers-reduced-motion` vía
 * `getScrollBehavior()` (ver abajo). Todo acceso a `window`/`matchMedia` ocurre
 * dentro del handler, nunca en render, para no romper el render del servidor
 * (SSR).
 */
import * as React from "react";

/**
 * Decide el `behavior` de los `scrollIntoView` respetando la preferencia de
 * movimiento reducido del usuario: `"auto"` (sin animación) cuando prefiere
 * movimiento reducido, `"smooth"` en caso contrario.
 *
 * Se exporta porque el auto-scroll del payload de Auditoría reutiliza la misma
 * lógica (coherencia de accesibilidad en todos los desplazamientos de la UI).
 * El acceso a `window`/`matchMedia` está encapsulado aquí y protegido para SSR.
 */
export function getScrollBehavior(): ScrollBehavior {
  if (
    typeof window !== "undefined" &&
    window.matchMedia("(prefers-reduced-motion: reduce)").matches
  ) {
    return "auto";
  }
  return "smooth";
}

/** Resultado del hook: el ancla al tope y el disparador del desplazamiento. */
export interface UseScrollToTopResult {
  readonly topRef: React.RefObject<HTMLDivElement | null>;
  readonly scrollToTop: () => void;
}

export function useScrollToTop(): UseScrollToTopResult {
  const topRef = React.useRef<HTMLDivElement | null>(null);

  const scrollToTop = React.useCallback(() => {
    topRef.current?.scrollIntoView({
      behavior: getScrollBehavior(),
      block: "start",
    });
  }, []);

  return { topRef, scrollToTop };
}
