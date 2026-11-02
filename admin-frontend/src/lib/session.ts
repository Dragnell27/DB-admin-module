/**
 * Sesión del administrador en el cliente (Req. 2.1, 2.2, 5.1).
 *
 * Este módulo centraliza TODO el manejo del JWT de sesión del admin en el
 * navegador:
 *
 * - Persistencia entre recargas en `localStorage` bajo una clave estable
 *   (Req. 2.1), con guardas seguras para SSR y para entornos donde el acceso a
 *   `localStorage` está bloqueado (modo privado, políticas del navegador).
 * - Decodificación de los claims del JWT (`sub`/`id`, `username`, `role`, `exp`)
 *   SIN verificar la firma: la validación criptográfica real la hace el BFF
 *   (admin-backend). Aquí sólo leemos los claims para condicionar la UI
 *   (p. ej. visibilidad por rol, Req. 5.1) y para decidir si existe una sesión
 *   utilizable (Req. 2.2).
 * - Manejo robusto de tokens malformados o expirados: se tratan como "sin
 *   sesión"; además, un token expirado se auto-elimina del almacenamiento.
 *
 * El cliente HTTP único (`src/lib/api-client.ts`) depende de
 * `getSessionToken()` y `clearSessionToken()`; esas firmas públicas se
 * preservan intactas.
 */

import {
  decodeCompactPerms,
  decodeModulePerms,
  type EffectivePermissions,
  type ModulePermissions,
} from "@/lib/permissions";

/** Clave de almacenamiento del JWT de sesión del admin. */
export const SESSION_TOKEN_KEY = "panel-admin-ott.session-token";

/**
 * Bus de sesión in-tab. El evento `storage` del navegador NO se dispara en la
 * misma pestaña que escribe `localStorage`, por lo que un refresco de token
 * dentro de esta pestaña (p. ej. "Recargar permisos") no se reflejaría en los
 * hooks de sesión. Este bus notifica a los suscriptores locales cuando el token
 * cambia vía `setSessionToken`/`clearSessionToken`.
 */
const sessionListeners = new Set<() => void>();

/**
 * Suscribe un listener a los cambios de sesión in-tab. Devuelve la función de
 * baja (`unsubscribe`) para limpiar en el `return` del efecto.
 */
export function subscribeSession(listener: () => void): () => void {
  sessionListeners.add(listener);
  return () => {
    sessionListeners.delete(listener);
  };
}

/** Notifica a los suscriptores in-tab que la sesión almacenada ha cambiado. */
function notifySessionChange(): void {
  for (const listener of sessionListeners) {
    listener();
  }
}

/** Roles reconocidos por el panel (ver design.md, Security). */
export type Role = "admin" | "viewer";

/**
 * Usuario de la sesión derivado de los claims del JWT de sesión del admin.
 * Es la vista normalizada que consume la UI; no incluye datos sensibles.
 */
export interface SessionUser {
  /** Identificador del usuario (claim `id`, con respaldo en `sub`). */
  readonly id: string | null;
  /** Nombre de usuario (claim `username`). */
  readonly username: string | null;
  /** Rol legacy del usuario (claim `role`), o `null`. Modelo RBAC: ver `roles`. */
  readonly role: Role | null;
  /**
   * Nombres de los roles RBAC asignados (claim `roles`). La UI los muestra en el
   * header. Vacío si el token no los lleva (sesión previa al rediseño).
   */
  readonly roles: ReadonlyArray<string>;
  /**
   * Permisos efectivos del usuario (Req. 7.2, 15.2), reconstruidos a partir del
   * claim compacto `perms` del JWT. Es la base para la visibilidad por permisos
   * en la UI; la autorización real la aplica siempre el BFF.
   */
  readonly permissions: EffectivePermissions;
  /**
   * Permisos sobre módulos dinámicos (module-builder), reconstruidos del claim
   * `modperms` del JWT. Base para la visibilidad de módulos y botones del
   * constructor; la autorización real la aplica el BFF.
   */
  readonly modulePermissions: ModulePermissions;
}

/**
 * Forma de los claims que nos interesan del JWT de sesión del admin.
 * El BFF emite `sub`, `username`, `role` y `exp` (ver design.md). Toleramos
 * también `id` como alias de identificador.
 */
interface SessionClaims {
  readonly sub?: unknown;
  readonly id?: unknown;
  readonly username?: unknown;
  readonly role?: unknown;
  readonly roles?: unknown;
  /**
   * Permisos en forma compacta (país → código de recurso → bitmask). Lo
   * decodifica `decodeCompactPerms` para construir `SessionUser.permissions`.
   */
  readonly perms?: unknown;
  /**
   * Permisos de módulos dinámicos en forma compacta (país → slug → acciones).
   * Lo decodifica `decodeModulePerms` para construir
   * `SessionUser.modulePermissions`.
   */
  readonly modperms?: unknown;
  /** Expiración en segundos desde epoch (estándar JWT). */
  readonly exp?: unknown;
}

/**
 * Indica si el código corre en el navegador. En el servidor (SSR) no hay
 * `localStorage`, por lo que los helpers degradan de forma segura a "sin sesión".
 */
function isBrowser(): boolean {
  return typeof window !== "undefined";
}

/**
 * Devuelve el JWT de sesión del admin almacenado, o `null` si no hay sesión
 * (o si se ejecuta en el servidor).
 *
 * Nota: este helper devuelve el token "tal cual" está almacenado y NO comprueba
 * su expiración; es la forma que necesita el interceptor de request del cliente
 * HTTP. Para saber si la sesión es utilizable, usa `isAuthenticated()`.
 */
export function getSessionToken(): string | null {
  if (!isBrowser()) {
    return null;
  }
  try {
    return window.localStorage.getItem(SESSION_TOKEN_KEY);
  } catch {
    // Acceso a storage bloqueado (modo privado, políticas): tratamos como sin sesión.
    return null;
  }
}

/**
 * Guarda el JWT de sesión del admin de forma persistente entre recargas
 * (Req. 2.1). La tarea 13.2 (login) usa este helper tras un login exitoso.
 * No-op en el servidor.
 */
export function setSessionToken(token: string): void {
  if (!isBrowser()) {
    return;
  }
  try {
    window.localStorage.setItem(SESSION_TOKEN_KEY, token);
  } catch {
    // Silencioso: si no se puede persistir, la app seguirá pidiendo login.
  }
  // Notifica a los suscriptores in-tab (el evento `storage` no se dispara aquí).
  notifySessionChange();
}

/**
 * Borra el JWT de sesión del admin. Lo invoca el interceptor de response ante
 * un 401 (sesión expirada/inválida, Req. 2.5) y el cierre de sesión (tarea 13.3).
 * No-op en el servidor.
 */
export function clearSessionToken(): void {
  if (!isBrowser()) {
    return;
  }
  try {
    window.localStorage.removeItem(SESSION_TOKEN_KEY);
  } catch {
    // Silencioso.
  }
  // Notifica a los suscriptores in-tab (el evento `storage` no se dispara aquí).
  notifySessionChange();
}

/**
 * Decodifica el segmento base64url de un JWT a texto UTF-8.
 * Devuelve `null` ante cualquier error de formato.
 */
function decodeBase64Url(segment: string): string | null {
  try {
    // base64url -> base64, restaurando el padding que JWT omite.
    const base64 = segment.replace(/-/g, "+").replace(/_/g, "/");
    const padded = base64.padEnd(
      base64.length + ((4 - (base64.length % 4)) % 4),
      "=",
    );

    if (typeof atob === "function") {
      // Navegador: `atob` produce una cadena binaria; la reinterpretamos como UTF-8.
      const binary = atob(padded);
      const bytes = Uint8Array.from(binary, (char) => char.charCodeAt(0));
      return new TextDecoder().decode(bytes);
    }

    // Entornos sin `atob` (SSR/Node): usamos Buffer si está disponible.
    const maybeBuffer = (
      globalThis as { Buffer?: { from(s: string, enc: string): { toString(enc: string): string } } }
    ).Buffer;
    if (maybeBuffer) {
      return maybeBuffer.from(padded, "base64").toString("utf-8");
    }

    return null;
  } catch {
    return null;
  }
}

/**
 * Decodifica los claims del JWT de sesión SIN verificar la firma.
 *
 * La verificación criptográfica es responsabilidad del BFF (admin-backend); el
 * front sólo necesita leer los claims para condicionar la UI. Ante un token
 * ausente, malformado o con payload no-JSON, devuelve `null`.
 */
function decodeClaims(token: string | null): SessionClaims | null {
  if (!token) {
    return null;
  }
  const parts = token.split(".");
  if (parts.length !== 3) {
    return null;
  }
  const json = decodeBase64Url(parts[1]);
  if (json === null) {
    return null;
  }
  try {
    const parsed: unknown = JSON.parse(json);
    if (typeof parsed !== "object" || parsed === null) {
      return null;
    }
    return parsed as SessionClaims;
  } catch {
    return null;
  }
}

/** Normaliza un valor desconocido a `string` no vacío, o `null`. */
function asString(value: unknown): string | null {
  return typeof value === "string" && value.trim() !== "" ? value : null;
}

/** Normaliza un valor desconocido a un arreglo de strings no vacíos. */
function asStringArray(value: unknown): string[] {
  if (!Array.isArray(value)) {
    return [];
  }
  return value.filter(
    (item): item is string => typeof item === "string" && item.trim() !== "",
  );
}

/** Normaliza un valor desconocido al tipo `Role`, o `null` si no es reconocido. */
function asRole(value: unknown): Role | null {
  return value === "admin" || value === "viewer" ? value : null;
}

/**
 * Indica si los claims están expirados según el claim `exp` (segundos epoch).
 * Si no hay `exp` numérico, consideramos el token como "no expirado" aquí y
 * dejamos que el BFF decida (el 401 limpiará la sesión vía interceptor).
 */
function isExpired(claims: SessionClaims): boolean {
  const exp = typeof claims.exp === "number" ? claims.exp : null;
  if (exp === null) {
    return false;
  }
  const nowSeconds = Math.floor(Date.now() / 1000);
  return exp <= nowSeconds;
}

/**
 * Devuelve los claims utilizables de la sesión actual, o `null` si no hay token,
 * está malformado o ha expirado. Como efecto secundario, si el token existe
 * pero está expirado, lo elimina del almacenamiento para que las vistas
 * protegidas no lo consideren válido (Req. 2.2).
 */
function getUsableClaims(): SessionClaims | null {
  const token = getSessionToken();
  const claims = decodeClaims(token);
  if (claims === null) {
    // Token ausente o malformado: si había algo almacenado inservible, limpiarlo.
    if (token !== null) {
      clearSessionToken();
    }
    return null;
  }
  if (isExpired(claims)) {
    clearSessionToken();
    return null;
  }
  return claims;
}

/**
 * Devuelve el usuario de la sesión (id, username, role) a partir de los claims
 * del JWT de sesión, o `null` si no hay una sesión utilizable (sin token,
 * malformado o expirado).
 */
export function getSessionUser(): SessionUser | null {
  const claims = getUsableClaims();
  if (claims === null) {
    return null;
  }
  return {
    id: asString(claims.id) ?? asString(claims.sub),
    username: asString(claims.username),
    role: asRole(claims.role),
    roles: asStringArray(claims.roles),
    permissions: decodeCompactPerms(claims.perms),
    modulePermissions: decodeModulePerms(claims.modperms),
  };
}

/**
 * Devuelve el rol del usuario autenticado (`admin` | `viewer`), o `null` si no
 * hay sesión utilizable o el claim de rol no es reconocido. Usado para la
 * visibilidad por rol en la UI (Req. 5.1); el BFF aplica la seguridad real.
 */
export function getRole(): Role | null {
  return getSessionUser()?.role ?? null;
}

/**
 * Devuelve los permisos efectivos del usuario autenticado (Req. 7.2, 15.2), o
 * una lista vacía si no hay sesión utilizable. Alimenta la visibilidad por
 * permisos en la UI (ocultar botones, módulos, filtrar países); la autorización
 * real la aplica siempre el BFF.
 */
export function getPermissions(): EffectivePermissions {
  return getSessionUser()?.permissions ?? [];
}

/**
 * Devuelve los permisos sobre módulos dinámicos (module-builder) del usuario
 * autenticado, o una lista vacía si no hay sesión utilizable. Alimenta la
 * visibilidad de módulos y botones del constructor; el BFF aplica la seguridad.
 */
export function getModulePermissions(): ModulePermissions {
  return getSessionUser()?.modulePermissions ?? [];
}

/**
 * Indica si existe una sesión válida y utilizable almacenada: hay un token, con
 * payload decodificable y no expirado (Req. 2.2). Un token expirado se elimina
 * como efecto de esta comprobación.
 */
export function isAuthenticated(): boolean {
  return getUsableClaims() !== null;
}
