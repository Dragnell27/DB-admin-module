/**
 * Store SSR-safe de NOTIFICACIONES FLOTANTES (toasts) del panel protegido.
 *
 * Migra los mensajes de RESULTADO efímero (éxito/error de guardar y de
 * acciones) del banner inline `FeedbackBanner` a toasts flotantes anclados
 * arriba a la derecha, SIEMPRE visibles sin importar el scroll (crítico para
 * los errores, que antes quedaban fuera de pantalla). Los mensajes CONTEXTUALES
 * (validación por campo/sección, degradaciones no bloqueantes) siguen siendo
 * banners inline.
 *
 * Sigue el MISMO patrón SSR-safe que `lib/sidebar-collapse.ts` y
 * `lib/sidebar-folders.ts`: estado a nivel de módulo, snapshot inmutable
 * cacheado (para que `useSyncExternalStore` no entre en bucle al comparar con
 * `Object.is`), bus de suscriptores in-tab y guardas `isBrowser()`. A diferencia
 * de los stores del menú lateral, NO hay `localStorage`: los toasts viven en
 * memoria, con alcance de sesión. No añade ninguna dependencia nueva.
 *
 * El store NO posee temporizadores (se mantiene puro y testeable): el
 * auto-descarte lo gestiona cada componente `Toast` con su propio `setTimeout`
 * (ver `components/toast-viewport.tsx`). Aquí vive la lógica de add/cap/dismiss,
 * unit-testable sin renderizar.
 */

export type ToastTone = "success" | "error" | "warning";

export interface Toast {
  readonly id: string;
  readonly tone: ToastTone;
  readonly message: string;
  readonly createdAt: number;
}

/** Máximo de toasts visibles a la vez: al llegar uno nuevo se descarta el más viejo. */
export const TOAST_CAP = 5;

/**
 * Milisegundos de auto-descarte por tono. `null` = persiste hasta que el
 * usuario lo cierre manualmente. Los errores PERSISTEN (null) para no perderse
 * si el usuario no está mirando; éxito/aviso se auto-descartan.
 */
export const AUTO_DISMISS_MS: Readonly<Record<ToastTone, number | null>> = {
  success: 4000,
  warning: 6000,
  error: null,
};

type Listener = () => void;

/** Snapshot vacío estable para el servidor y el primer render (hidratación). */
const EMPTY: readonly Toast[] = [];

/** Estado a nivel de módulo: lista inmutable de toasts actuales. */
let toasts: readonly Toast[] = EMPTY;

/** Suscriptores a cambios (para `useSyncExternalStore`). */
const listeners = new Set<Listener>();

/** Contador de respaldo para ids cuando `crypto.randomUUID` no está disponible. */
let idCounter = 0;

/** Genera un id único; cae a un contador incremental si no hay `crypto.randomUUID`. */
function nextId(): string {
  try {
    if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
      return crypto.randomUUID();
    }
  } catch {
    // Degrada silenciosamente al contador.
  }
  return `t${idCounter++}`;
}

/**
 * Devuelve el snapshot inmutable actual. `useSyncExternalStore` lo compara con
 * `Object.is`, así que la referencia solo cambia cuando cambia la lista. En el
 * servidor y el primer render devuelve la misma referencia `EMPTY` estable.
 */
export function getToasts(): readonly Toast[] {
  return toasts;
}

/**
 * Suscribe un listener a los cambios de la lista de toasts. Devuelve la función
 * para desuscribir.
 */
export function subscribeToasts(listener: Listener): () => void {
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
 * Helper PURO: agrega `next` al final de `list` y, si se supera `cap`, descarta
 * los MÁS VIEJOS (del inicio) hasta respetar el tope. Sin temporizadores ni
 * React: unit-testable de forma aislada.
 */
export function pushCapped(
  list: readonly Toast[],
  next: Toast,
  cap: number,
): readonly Toast[] {
  const appended = [...list, next];
  if (appended.length <= cap) {
    return appended;
  }
  return appended.slice(appended.length - cap);
}

/**
 * Agrega un toast con el tono y mensaje dados y devuelve su id. Un mensaje
 * vacío/undefined NO crea toast (degrada en silencio) y devuelve `""`.
 */
export function addToast(tone: ToastTone, message: string): string {
  if (!message) {
    return "";
  }
  const toast: Toast = {
    id: nextId(),
    tone,
    message,
    createdAt: Date.now(),
  };
  toasts = pushCapped(toasts, toast, TOAST_CAP);
  notify();
  return toast.id;
}

/** Descarta el toast con el id dado (si existe). */
export function dismissToast(id: string): void {
  const nextList = toasts.filter((t) => t.id !== id);
  if (nextList.length === toasts.length) {
    return;
  }
  toasts = nextList.length === 0 ? EMPTY : nextList;
  notify();
}

/** Limpia todos los toasts. Útil en pruebas y para resets globales. */
export function clearToasts(): void {
  if (toasts === EMPTY) {
    return;
  }
  toasts = EMPTY;
  notify();
}

/**
 * API imperativa reexportada desde `use-toast`. Permite que código fuera de
 * componentes (callbacks de TanStack Query, efectos de `?saved=1`) dispare un
 * toast directamente: `toast.error(mensaje)`.
 */
export const toast = {
  success: (m: string) => addToast("success", m),
  error: (m: string) => addToast("error", m),
  warning: (m: string) => addToast("warning", m),
};

/**
 * Reinicia el estado a vacío. SOLO para pruebas (espejo de
 * `__resetCollapseCacheForTests`). No notifica, para no interferir con
 * suscriptores de otros tests.
 */
export function __resetToastsForTests(): void {
  toasts = EMPTY;
  idCounter = 0;
}
