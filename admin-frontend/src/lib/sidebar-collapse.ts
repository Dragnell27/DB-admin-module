/**
 * Estado contraído/expandido del menú lateral GLOBAL del panel protegido
 * (FEAT-002). A diferencia de `lib/sidebar-folders.ts` (que recuerda qué
 * carpetas del árbol de módulos están abiertas), este módulo recuerda si el
 * menú lateral completo se muestra COLAPSADO (solo iconos, riel estrecho) o
 * EXPANDIDO (con etiquetas), reclamando ancho para el contenido en todo el
 * panel.
 *
 * Sigue el mismo patrón SSR-safe que `lib/sidebar-folders.ts` y `lib/country.ts`:
 * lectura y escritura directa de `localStorage` con guardas `isBrowser()`, un
 * bus de suscriptores in-tab para `useSyncExternalStore`, el evento `storage`
 * para sincronizar entre pestañas, y degradación silenciosa a "expandido"
 * cuando el almacenamiento está bloqueado.
 *
 * Por defecto el menú arranca EXPANDIDO: "sin estado" equivale a `false`
 * (expandido) sin tener que pre-sembrar nada. El valor persistido es un boolean
 * primitivo, así que la estabilidad de referencia que exige `useSyncExternalStore`
 * (comparación con `Object.is`) se satisface automáticamente; aun así se invalida
 * la caché en cada escritura y en cada evento `storage`.
 */

/** Clave de almacenamiento del estado contraído del menú lateral. */
export const SIDEBAR_COLLAPSE_KEY = "panel-admin-ott.sidebar-collapsed";

type Listener = () => void;

/** Suscriptores a cambios del estado contraído (para `useSyncExternalStore`). */
const listeners = new Set<Listener>();

/**
 * Snapshot cacheado del estado contraído. `useSyncExternalStore` compara
 * snapshots con `Object.is`; al ser un boolean primitivo la estabilidad es
 * trivial, pero se mantiene la caché para releer el almacenamiento solo cuando
 * cambia. La caché se invalida al notificar un cambio.
 */
let cachedSnapshot: boolean | null = null;

function isBrowser(): boolean {
  return typeof window !== "undefined";
}

/**
 * Lee el estado contraído del almacenamiento. Devuelve `false` (expandido) en
 * el servidor, el primer render, o si el almacenamiento está bloqueado o el
 * valor es inválido.
 */
export function getCollapsed(): boolean {
  if (cachedSnapshot !== null) {
    return cachedSnapshot;
  }
  cachedSnapshot = readCollapsed();
  return cachedSnapshot;
}

/** Lee y normaliza el boolean desde el almacenamiento (sin caché). */
function readCollapsed(): boolean {
  if (!isBrowser()) {
    return false;
  }
  try {
    const raw = window.localStorage.getItem(SIDEBAR_COLLAPSE_KEY);
    if (!raw) {
      return false;
    }
    const parsed: unknown = JSON.parse(raw);
    return parsed === true;
  } catch {
    return false;
  }
}

/**
 * Persiste el estado contraído, invalida la caché del snapshot y notifica a los
 * suscriptores.
 */
export function setCollapsed(value: boolean): void {
  // Invalida la caché para que el próximo `getCollapsed` relea el nuevo estado.
  cachedSnapshot = null;
  if (isBrowser()) {
    try {
      window.localStorage.setItem(SIDEBAR_COLLAPSE_KEY, JSON.stringify(value));
    } catch {
      // Silencioso: si no se puede persistir, la app sigue funcionando.
    }
  }
  notify();
}

/** Alterna el estado contraído/expandido del menú lateral. */
export function toggleCollapsed(): void {
  setCollapsed(!getCollapsed());
}

/**
 * Suscribe un listener a los cambios del estado contraído (in-tab y entre
 * pestañas). Devuelve la función para desuscribir.
 */
export function subscribeCollapsed(listener: Listener): () => void {
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
  if (event.key === null || event.key === SIDEBAR_COLLAPSE_KEY) {
    // Cambio desde otra pestaña: invalida la caché antes de notificar.
    cachedSnapshot = null;
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
 * `getCollapsed` relea el almacenamiento. En producción la caché se invalida
 * sola en cada escritura.
 */
export function __resetCollapseCacheForTests(): void {
  cachedSnapshot = null;
}
