/**
 * Preferencia de tema CLARO/OSCURO del panel (Entrega A).
 *
 * Espejo directo del patrón SSR-safe de `lib/sidebar-collapse.ts`: lectura y
 * escritura directa de `localStorage` con guardas `isBrowser()`, un bus de
 * suscriptores in-tab para `useSyncExternalStore`, el evento `storage` para
 * sincronizar entre pestañas, y degradación silenciosa cuando el almacenamiento
 * está bloqueado. NO usa `next-themes` ni ninguna dependencia nueva.
 *
 * Por defecto el tema es OSCURO (no se sigue `prefers-color-scheme`): "sin
 * estado" equivale a `"dark"`. El snapshot de servidor devuelve SIEMPRE `"dark"`
 * (primitivo estable), de modo que SSR y el primer render del cliente coinciden
 * con el primer paint que fija el script anti-FOUC (ver `lib/theme-script.ts`).
 *
 * Además de persistir, `setTheme`/`toggleTheme` APLICAN la clase `dark` en
 * `<html>` (`classList.toggle` + `style.colorScheme`), para que el cambio sea
 * inmediato sin depender de que React re-pinte el `<html>` (el `RootLayout` es
 * un Server Component y no se re-renderiza en cliente).
 */

export type Theme = "light" | "dark";

/** Clave de almacenamiento de la preferencia de tema. */
export const THEME_KEY = "panel-admin-ott.theme";

/** Tema por defecto (sin preferencia guardada): OSCURO. */
export const DEFAULT_THEME: Theme = "dark";

type Listener = () => void;

/** Suscriptores a cambios del tema (para `useSyncExternalStore`). */
const listeners = new Set<Listener>();

/**
 * Snapshot cacheado del tema. `useSyncExternalStore` compara snapshots con
 * `Object.is`; al ser un string primitivo la estabilidad es trivial, pero se
 * mantiene la caché para releer el almacenamiento solo cuando cambia. La caché
 * se invalida al notificar un cambio y en cada evento `storage`.
 */
let cachedSnapshot: Theme | null = null;

function isBrowser(): boolean {
  return typeof window !== "undefined";
}

/** Normaliza un valor arbitrario a un `Theme`, cayendo al defecto oscuro. */
function normalizeTheme(value: unknown): Theme {
  return value === "light" || value === "dark" ? value : DEFAULT_THEME;
}

/**
 * Snapshot del CLIENTE: lee la preferencia del almacenamiento. Devuelve `"dark"`
 * (defecto) en el servidor, el primer render, o si el almacenamiento está
 * bloqueado o el valor es inválido.
 */
export function getTheme(): Theme {
  if (cachedSnapshot !== null) {
    return cachedSnapshot;
  }
  cachedSnapshot = readTheme();
  return cachedSnapshot;
}

/**
 * Snapshot del SERVIDOR: devuelve SIEMPRE el primitivo estable `"dark"`. Igual
 * que el `() => false` del menú lateral, garantiza que SSR y el primer render
 * del cliente coincidan (y con el primer paint que fija el script anti-FOUC).
 */
export function getServerTheme(): Theme {
  return DEFAULT_THEME;
}

/** Lee y normaliza el tema desde el almacenamiento (sin caché). */
function readTheme(): Theme {
  if (!isBrowser()) {
    return DEFAULT_THEME;
  }
  try {
    return normalizeTheme(window.localStorage.getItem(THEME_KEY));
  } catch {
    return DEFAULT_THEME;
  }
}

/**
 * Aplica la clase `dark` y el `color-scheme` en `<html>`. Idéntico efecto al
 * del script anti-FOUC, para que el cambio en caliente sea inmediato.
 */
function applyTheme(theme: Theme): void {
  if (!isBrowser()) {
    return;
  }
  const root = document.documentElement;
  root.classList.toggle("dark", theme === "dark");
  root.style.colorScheme = theme === "dark" ? "dark" : "light";
}

/**
 * Persiste el tema, invalida la caché del snapshot, aplica la clase en `<html>`
 * y notifica a los suscriptores.
 */
export function setTheme(theme: Theme): void {
  // Invalida la caché para que el próximo `getTheme` relea el nuevo estado.
  cachedSnapshot = null;
  if (isBrowser()) {
    try {
      window.localStorage.setItem(THEME_KEY, theme);
    } catch {
      // Silencioso: si no se puede persistir, la app sigue funcionando.
    }
  }
  applyTheme(theme);
  notify();
}

/** Alterna el tema entre claro y oscuro. */
export function toggleTheme(): void {
  setTheme(getTheme() === "dark" ? "light" : "dark");
}

/**
 * Suscribe un listener a los cambios de tema (in-tab y entre pestañas).
 * Devuelve la función para desuscribir.
 */
export function subscribeTheme(listener: Listener): () => void {
  listeners.add(listener);
  if (isBrowser()) {
    window.addEventListener("storage", handleStorage);
  }
  return () => {
    listeners.delete(listener);
    if (isBrowser() && listeners.size === 0) {
      window.removeEventListener("storage", handleStorage);
    }
  };
}

function handleStorage(event: StorageEvent): void {
  if (event.key === null || event.key === THEME_KEY) {
    // Cambio desde otra pestaña: invalida la caché, re-aplica la clase en
    // <html> y notifica.
    cachedSnapshot = null;
    applyTheme(getTheme());
    notify();
  }
}

function notify(): void {
  for (const listener of listeners) {
    listener();
  }
}

/**
 * Invalida la caché del snapshot. SOLO para pruebas que manipulan
 * `localStorage` directamente (fuera de la API de este módulo) y necesitan que
 * `getTheme` relea el almacenamiento. En producción la caché se invalida sola
 * en cada escritura.
 */
export function __resetThemeCacheForTests(): void {
  cachedSnapshot = null;
}
