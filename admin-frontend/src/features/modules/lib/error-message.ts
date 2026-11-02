/**
 * Helper puro para sanear el mensaje de error mostrado en el banner de datos
 * del módulo dinámico (Punto 2 / Decisión D2 del diseño UI/UX).
 *
 * Objetivo: no exponer al usuario detalles crudos de infraestructura del
 * datasource (host, puerto, usuario de BD, `dial tcp ...`, errores de driver
 * PostgreSQL) cuando el origen de datos no está alcanzable, pero CONSERVAR los
 * mensajes útiles del BFF (validación 422, negocio, 401/403, 404).
 *
 * Es puro y total: no lanza y siempre devuelve un `string`.
 */
import type { ApiError } from "@/lib/api-client";

/** Mensaje genérico que se muestra ante fallos de conexión/datasource. */
export const GENERIC_DATASOURCE_MESSAGE =
  "No se pudo conectar con el origen de datos. Contacta al administrador.";

/**
 * Patrones INEQUÍVOCOS de infraestructura. Si el mensaje del error contiene
 * alguno (case-insensitive) se considera un fallo de conexión/datasource y se
 * sustituye por el mensaje genérico. NO se incluyen subcadenas genéricas como
 * `sql`/`database`/`timeout`: aparecen en mensajes de validación/negocio
 * legítimos y sanearlas ocultaría información útil.
 */
const INFRA_PATTERNS: readonly string[] = [
  "dial tcp",
  "connection refused",
  "could not connect",
  "connection reset",
  "pq:",
  "econnrefused",
  "etimedout",
  "no such host",
];

/**
 * Devuelve el mensaje seguro para mostrar al usuario.
 *
 * Reglas (conservadoras): se devuelve el genérico cuando
 * - `error` es `null`/`undefined`, o
 * - `status` es `500` o `502` (origen caído / fallo no controlado), o
 * - `status` es `undefined` (fallo de red/timeout: el cliente no adjunta
 *   `status`), o
 * - el `message` (en minúsculas) contiene algún `INFRA_PATTERNS`, o
 * - el `message` está vacío/ausente.
 *
 * En cualquier otro caso se conserva `error.message` (422/validación, 401/403,
 * 404, códigos de negocio).
 */
export function safeModuleErrorMessage(
  error: ApiError | null | undefined,
): string {
  if (process.env.NODE_ENV !== "production") {
    // El detalle técnico solo se vuelca a consola en desarrollo; nunca a la UI.
    console.error("[module] datasource error:", error);
  }

  if (!error) {
    return GENERIC_DATASOURCE_MESSAGE;
  }

  const { status, message } = error;

  if (status === 500 || status === 502 || status === undefined) {
    return GENERIC_DATASOURCE_MESSAGE;
  }

  if (typeof message !== "string" || message.trim() === "") {
    return GENERIC_DATASOURCE_MESSAGE;
  }

  const normalized = message.toLowerCase();
  if (INFRA_PATTERNS.some((pattern) => normalized.includes(pattern))) {
    return GENERIC_DATASOURCE_MESSAGE;
  }

  return message;
}
