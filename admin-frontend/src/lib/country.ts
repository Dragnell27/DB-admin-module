/**
 * País OTT seleccionado en el cliente (selector de país).
 *
 * El BFF opera por petición sobre un país OTT: el front envía la cabecera
 * `X-Country` en cada llamada (ver `api-client.ts`). Este módulo centraliza el
 * país seleccionado para que:
 *   - el interceptor de request de axios lo lea de forma SÍNCRONA (sin hooks),
 *   - la UI (selector en el header) lo lea y actualice de forma reactiva,
 *   - persista entre recargas en `localStorage`.
 *
 * Si no hay país seleccionado (primer uso), `getCountry()` devuelve `null` y el
 * BFF usará su país por defecto (OTT_ACTIVE_COUNTRY). En cuanto el selector se
 * hidrata con la lista de `GET /meta/countries`, fija un país concreto.
 */

/** Clave de almacenamiento del país seleccionado. */
export const COUNTRY_KEY = "panel-admin-ott.country";

/** Nombres legibles de los países OTT soportados (para el selector). */
export const COUNTRY_LABELS: Readonly<Record<string, string>> = {
  gt: "Guatemala",
  sv: "El Salvador",
  ni: "Nicaragua",
  hn: "Honduras",
  cr: "Costa Rica",
};

type Listener = () => void;

/** Suscriptores a cambios de país (para `useSyncExternalStore`). */
const listeners = new Set<Listener>();

function isBrowser(): boolean {
  return typeof window !== "undefined";
}

/**
 * Devuelve el país seleccionado (normalizado a minúsculas), o `null` si no hay
 * selección o se ejecuta en el servidor. El interceptor HTTP usa esto: si es
 * `null`, no envía la cabecera y el BFF aplica su país por defecto.
 */
export function getCountry(): string | null {
  if (!isBrowser()) {
    return null;
  }
  try {
    const value = window.localStorage.getItem(COUNTRY_KEY);
    const trimmed = value?.trim().toLowerCase();
    return trimmed ? trimmed : null;
  } catch {
    return null;
  }
}

/**
 * Fija el país seleccionado y notifica a los suscriptores. Pasar `null` o vacío
 * borra la selección (vuelve al país por defecto del BFF).
 */
export function setCountry(country: string | null): void {
  if (!isBrowser()) {
    return;
  }
  try {
    const normalized = country?.trim().toLowerCase();
    if (normalized) {
      window.localStorage.setItem(COUNTRY_KEY, normalized);
    } else {
      window.localStorage.removeItem(COUNTRY_KEY);
    }
  } catch {
    // Silencioso: si no se puede persistir, la app sigue con el default.
  }
  notify();
}

/** Suscribe un listener a cambios de país. Devuelve la función para desuscribir. */
export function subscribeCountry(listener: Listener): () => void {
  listeners.add(listener);
  // Reaccionar también a cambios en otras pestañas.
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
  if (event.key === null || event.key === COUNTRY_KEY) {
    notify();
  }
}

function notify(): void {
  for (const listener of listeners) {
    listener();
  }
}

/** Etiqueta legible de un código de país (p. ej. "gt" -> "Guatemala (GT)"). */
export function countryLabel(code: string): string {
  const name = COUNTRY_LABELS[code.toLowerCase()];
  return name ? `${name} (${code.toUpperCase()})` : code.toUpperCase();
}
