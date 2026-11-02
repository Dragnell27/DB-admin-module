/**
 * Estado expandido/contraído de las carpetas del árbol de módulos del menú
 * lateral (connection-templates, Req. 6).
 *
 * El árbol de módulos agrupa los módulos por su `menu_path` en carpetas
 * (segmentos intermedios) y hojas (los módulos). Este módulo recuerda qué
 * carpetas están EXPANDIDAS, persistiéndolo entre recargas en `localStorage`,
 * con el mismo patrón SSR-safe que `lib/country.ts` y `lib/session.ts`: lectura
 * y escritura directa de `localStorage` con guardas `isBrowser()`, un bus de
 * suscriptores in-tab para `useSyncExternalStore`, y degradación silenciosa a
 * "todo contraído" cuando el almacenamiento está bloqueado.
 *
 * Por defecto TODAS las carpetas arrancan CONTRAÍDAS: el mapa almacenado solo
 * guarda las rutas EXPANDIDAS (`true`), así que "sin estado" equivale a
 * contraído sin tener que pre-sembrar nada. Una carpeta se identifica por su
 * RUTA COMPLETA desde la raíz (p. ej. `CENAM/Recaudo`), estable entre recargas
 * porque deriva del `menu_path`.
 *
 * La lógica de decisión vive en el helper PURO `isExpanded`, testeable sin
 * `localStorage` ni React.
 */

/** Clave de almacenamiento del estado de carpetas del menú (expandidas). */
export const SIDEBAR_FOLDERS_KEY = "panel-admin-ott.sidebar-folders";

/** Mapa ruta-de-carpeta → `true` si está expandida (ausente/false = contraída). */
export type ExpandedFolders = Record<string, boolean>;

type Listener = () => void;

/** Suscriptores a cambios de estado de carpetas (para `useSyncExternalStore`). */
const listeners = new Set<Listener>();

/**
 * Snapshot cacheado del mapa de carpetas expandidas. `useSyncExternalStore`
 * compara snapshots con `Object.is`, así que `getExpandedFolders` DEBE devolver
 * la misma referencia mientras el almacenamiento no cambie; de lo contrario se
 * dispara un bucle de re-render. La caché se invalida al notificar un cambio.
 */
let cachedSnapshot: ExpandedFolders | null = null;

function isBrowser(): boolean {
  return typeof window !== "undefined";
}

/**
 * Une la ruta de la carpeta padre con la etiqueta del nodo para formar la RUTA
 * COMPLETA de una carpeta (clave estable de persistencia). La raíz usa `""`
 * como padre, de modo que un nodo de primer nivel «CENAM» produce `CENAM` y su
 * hijo «Recaudo» produce `CENAM/Recaudo`. Helper puro.
 */
export function folderPath(parentPath: string, label: string): string {
  return parentPath === "" ? label : `${parentPath}/${label}`;
}

/**
 * Decide si la carpeta en `path` debe mostrarse EXPANDIDA. Helper puro: solo
 * depende de sus argumentos. Una carpeta está expandida cuando el mapa la marca
 * explícitamente con `true`; cualquier otro caso (ausente o `false`) es
 * contraída, que es el estado por defecto.
 *
 * `activeAncestorPaths` permite auto-expandir los ancestros de la ruta activa:
 * una carpeta ancestro de la hoja activa se muestra expandida AUNQUE no esté en
 * el mapa, salvo que el usuario la haya contraído explícitamente
 * (`expandedMap[path] === false`), que siempre gana.
 */
export function isExpanded(
  expandedMap: Readonly<ExpandedFolders>,
  path: string,
  activeAncestorPaths?: ReadonlySet<string>,
): boolean {
  if (expandedMap[path] === true) {
    return true;
  }
  if (expandedMap[path] === false) {
    return false;
  }
  return activeAncestorPaths?.has(path) ?? false;
}

/**
 * Lee el mapa de carpetas expandidas del almacenamiento. Devuelve `{}` (todo
 * contraído) en el servidor, el primer render, o si el almacenamiento está
 * bloqueado o el valor es inválido.
 */
export function getExpandedFolders(): ExpandedFolders {
  if (cachedSnapshot !== null) {
    return cachedSnapshot;
  }
  cachedSnapshot = readExpandedFolders();
  return cachedSnapshot;
}

/** Lee y normaliza el mapa desde el almacenamiento (sin caché). */
function readExpandedFolders(): ExpandedFolders {
  if (!isBrowser()) {
    return {};
  }
  try {
    const raw = window.localStorage.getItem(SIDEBAR_FOLDERS_KEY);
    if (!raw) {
      return {};
    }
    const parsed: unknown = JSON.parse(raw);
    if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
      return {};
    }
    const result: ExpandedFolders = {};
    for (const [key, value] of Object.entries(parsed)) {
      if (typeof value === "boolean") {
        result[key] = value;
      }
    }
    return result;
  } catch {
    return {};
  }
}

/**
 * Persiste el mapa de carpetas expandidas, invalida la caché del snapshot y
 * notifica a los suscriptores.
 */
function writeExpandedFolders(map: ExpandedFolders): void {
  // Invalida la caché para que el próximo `getExpandedFolders` relea el nuevo
  // estado (referencia distinta → `useSyncExternalStore` re-renderiza).
  cachedSnapshot = null;
  if (!isBrowser()) {
    return;
  }
  try {
    window.localStorage.setItem(SIDEBAR_FOLDERS_KEY, JSON.stringify(map));
  } catch {
    // Silencioso: si no se puede persistir, la app sigue con todo contraído.
  }
  notify();
}

/**
 * Fija el estado expandido/contraído de una carpeta por su ruta completa y
 * notifica. Al contraer, se guarda `false` explícito para que gane sobre el
 * auto-expandido de la ruta activa.
 */
export function setFolderExpanded(path: string, expanded: boolean): void {
  // Copia inmutable del snapshot actual (no se muta el objeto cacheado).
  const map: ExpandedFolders = { ...getExpandedFolders(), [path]: expanded };
  writeExpandedFolders(map);
}

/** Alterna el estado expandido/contraído de una carpeta por su ruta completa. */
export function toggleFolder(path: string): void {
  const map = getExpandedFolders();
  setFolderExpanded(path, map[path] !== true);
}

/**
 * Suscribe un listener a los cambios de estado de carpetas (in-tab y entre
 * pestañas). Devuelve la función para desuscribir.
 */
export function subscribeFolders(listener: Listener): () => void {
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
  if (event.key === null || event.key === SIDEBAR_FOLDERS_KEY) {
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
 * `getExpandedFolders` relea el almacenamiento. En producción la caché se
 * invalida sola en cada escritura.
 */
export function __resetFoldersCacheForTests(): void {
  cachedSnapshot = null;
}
