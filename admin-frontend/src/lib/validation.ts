/**
 * Primitivas de validación Zod reutilizadas por los schemas por recurso.
 *
 * Estas reglas replican EXACTAMENTE la validación server-side espejo del
 * admin-backend (`admin-backend/internal/validation`: resources.go, currency.go,
 * pagination.go) para que cliente y servidor coincidan. Los mensajes están en
 * español, replicando las reglas del diseño (design.md "DTOs por recurso").
 *
 * Al ser lógica pura de validación, estas primitivas y los schemas que las usan
 * son el objetivo de las PBT con `fast-check` de las tareas 15.3 (email), 15.4
 * (teléfono) y 15.5 (edad/fecha de nacimiento).
 */
import { z } from "zod";

// ---------------------------------------------------------------------------
// Patrones y constantes espejo del backend (resources.go, currency.go)
// ---------------------------------------------------------------------------

/** Longitudes máximas por campo (resources.go). */
export const LIMITS = {
  externalId: 100,
  name: 100,
  email: 255,
  address: 255,
  planName: 255,
  cardNumber: 50,
  cardToken: 255,
  expirationDate: 10,
  paymentMethodName: 100,
  cardFranchise: 50,
  cancelReason: 255,
} as const;

/** Edad mínima de un Customer (Req. 7.6). */
export const MIN_ADULT_AGE = 18;

/**
 * Patrón de teléfono: un signo '+' seguido de 8 a 15 dígitos (Req. 7.5).
 * Espejo de `phonePattern` en resources.go.
 */
export const PHONE_PATTERN = /^\+\d{8,15}$/;

/**
 * Patrón de fecha de nacimiento: formato YYYY-MM-DD (Req. 7.6).
 * Espejo de `birthDatePattern` en resources.go.
 */
export const BIRTH_DATE_PATTERN = /^\d{4}-\d{2}-\d{2}$/;

/**
 * Patrón de expiración de tarjeta: formato MM/YY (Req. 18.4).
 * Espejo de `expirationPattern` en resources.go.
 */
export const EXPIRATION_PATTERN = /^\d{2}\/\d{2}$/;

/**
 * Patrón de UUID canónico 8-4-4-4-12 hexadecimal, insensible a mayúsculas.
 * Espejo de `uuidPattern` en uuid.go.
 */
export const UUID_PATTERN =
  /^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$/;

/** period_type permitido en Plan (Req. 10.9). Espejo de `allowedPeriodTypes`. */
export const PERIOD_TYPES = ["dia", "semana", "mes", "año"] as const;

/** gender permitido en Customer (Req. 7.8). Espejo de `allowedGenders`. */
export const GENDERS = ["M", "F", "O"] as const;

/** Países OTT soportados (currency.go: gt, sv, ni, hn, cr). */
export const SUPPORTED_COUNTRIES = ["gt", "sv", "ni", "hn", "cr"] as const;

/**
 * Mapeo moneda↔país exigido por el diseño (Req. 10.7). Espejo de
 * `countryToCurrency` en currency.go: `sv=USD, cr=CRC, gt=GTQ, hn=HNL, ni=NIO`.
 * Claves (país) en minúsculas; valores (moneda) en mayúsculas.
 */
export const COUNTRY_TO_CURRENCY: Readonly<Record<string, string>> = {
  sv: "USD",
  cr: "CRC",
  gt: "GTQ",
  hn: "HNL",
  ni: "NIO",
} as const;

// ---------------------------------------------------------------------------
// Validadores puros reutilizables (replican los helpers de resources.go)
// ---------------------------------------------------------------------------

/**
 * Valida que una cadena de email tenga formato válido y no exceda 255
 * caracteres (Property 7, Req. 7.4). Replica `validateEmail` en resources.go,
 * que exige que la dirección parseada sea idéntica a la entrada (sin nombre ni
 * espacios), por lo que usamos un patrón estricto equivalente.
 */
export function isValidEmail(email: string): boolean {
  if (email.length === 0 || email.length > LIMITS.email) {
    return false;
  }
  // Patrón estricto: una sola arroba, sin espacios, con dominio y TLD.
  // Equivalente al requisito de `mail.ParseAddress` + addr == email del backend.
  const emailPattern =
    /^[^\s@]+@[^\s@.]+(?:\.[^\s@.]+)+$/;
  return emailPattern.test(email);
}

/**
 * Valida el formato de teléfono '+' + 8..15 dígitos (Property 8, Req. 7.5).
 */
export function isValidPhone(phone: string): boolean {
  return PHONE_PATTERN.test(phone);
}

/**
 * Valida fecha de nacimiento: formato YYYY-MM-DD, fecha real, no futura y edad
 * de al menos 18 años (Property 9, Req. 7.6). Replica `validateBirthDate` +
 * `ageInYears` de resources.go usando UTC para evitar sesgos de zona horaria.
 */
export function isValidBirthDate(
  birthDate: string,
  ref: Date = new Date(),
): boolean {
  if (!BIRTH_DATE_PATTERN.test(birthDate)) {
    return false;
  }
  const [yearStr, monthStr, dayStr] = birthDate.split("-");
  const year = Number(yearStr);
  const month = Number(monthStr);
  const day = Number(dayStr);
  // Rechazar valores de mes/día imposibles o fechas inexistentes (p. ej. 02-30).
  if (month < 1 || month > 12 || day < 1 || day > 31) {
    return false;
  }
  const parsed = new Date(Date.UTC(year, month - 1, day));
  if (
    parsed.getUTCFullYear() !== year ||
    parsed.getUTCMonth() !== month - 1 ||
    parsed.getUTCDate() !== day
  ) {
    return false;
  }
  const today = new Date(
    Date.UTC(ref.getUTCFullYear(), ref.getUTCMonth(), ref.getUTCDate()),
  );
  if (parsed.getTime() > today.getTime()) {
    return false;
  }
  return ageInYears(parsed, today) >= MIN_ADULT_AGE;
}

/**
 * Devuelve la fecha máxima de nacimiento (`YYYY-MM-DD`) para cumplir la edad
 * mínima (≥18 años) a la fecha `ref`: es decir, hoy menos MIN_ADULT_AGE años.
 * Útil como atributo `max` del calendario de `birth_date` para que el usuario
 * no pueda elegir una fecha de menor de edad.
 */
export function maxBirthDate(ref: Date = new Date()): string {
  const y = ref.getUTCFullYear() - MIN_ADULT_AGE;
  const m = String(ref.getUTCMonth() + 1).padStart(2, "0");
  const d = String(ref.getUTCDate()).padStart(2, "0");
  return `${y}-${m}-${d}`;
}

/** Calcula la edad completa en años de alguien nacido en `birth` a la fecha `ref`. */
export function ageInYears(birth: Date, ref: Date): number {
  let years = ref.getUTCFullYear() - birth.getUTCFullYear();
  const refMonth = ref.getUTCMonth();
  const birthMonth = birth.getUTCMonth();
  if (
    refMonth < birthMonth ||
    (refMonth === birthMonth && ref.getUTCDate() < birth.getUTCDate())
  ) {
    years -= 1;
  }
  return years;
}

/**
 * Valida que una expiración MM/YY represente una fecha futura (Req. 18.4).
 * Replica `validateExpirationDate`: la tarjeta es válida hasta el último
 * instante del mes; el primer día del mes siguiente marca el límite.
 */
export function isFutureExpiration(exp: string, ref: Date = new Date()): boolean {
  if (!EXPIRATION_PATTERN.test(exp)) {
    return false;
  }
  const [monthStr, yearStr] = exp.split("/");
  const month = Number(monthStr);
  const year = 2000 + Number(yearStr);
  if (month < 1 || month > 12) {
    return false;
  }
  // Primer día del mes siguiente a la expiración (en UTC).
  const endOfMonth = new Date(Date.UTC(year, month, 1));
  return endOfMonth.getTime() > ref.getTime();
}

/** Indica si un `country` (2 letras) pertenece al conjunto soportado. */
export function isSupportedCountry(country: string): boolean {
  return Object.prototype.hasOwnProperty.call(
    COUNTRY_TO_CURRENCY,
    country.trim().toLowerCase(),
  );
}

/**
 * Valida la coherencia moneda↔país de un Plan (Property 10, Req. 10.4, 10.7):
 * `country` de 2 letras y soportado, `currency` de 3 letras y que coincida con
 * la moneda del país según el mapeo. La comparación de moneda es insensible a
 * mayúsculas. La coincidencia `country` vs país del token la valida el BFF.
 */
export function isValidCountryCurrency(
  country: string,
  currency: string,
): boolean {
  if (!/^[a-zA-Z]{2}$/.test(country)) {
    return false;
  }
  if (!/^[a-zA-Z]{3}$/.test(currency)) {
    return false;
  }
  const expected = COUNTRY_TO_CURRENCY[country.trim().toLowerCase()];
  if (!expected) {
    return false;
  }
  return currency.trim().toUpperCase() === expected;
}

// ---------------------------------------------------------------------------
// Primitivas Zod reutilizables
// ---------------------------------------------------------------------------

/** Cadena requerida con longitud máxima y mensajes en español. */
export function requiredString(label: string, max: number) {
  return z
    .string({ required_error: `${label} es requerido` })
    .min(1, `${label} es requerido`)
    .max(max, `${label} no puede exceder ${max} caracteres`);
}

/** Cadena opcional con longitud máxima. */
export function optionalString(label: string, max: number) {
  return z
    .string()
    .max(max, `${label} no puede exceder ${max} caracteres`)
    .optional();
}

/** Email de Customer: formato válido y máx 255 (Property 7, Req. 7.4). */
export const emailSchema = z
  .string({ required_error: "El email es requerido" })
  .min(1, "El email es requerido")
  .max(LIMITS.email, `El email no puede exceder ${LIMITS.email} caracteres`)
  .refine(isValidEmail, "El email no tiene un formato válido");

/** Teléfono de Customer: '+' + 8..15 dígitos (Property 8, Req. 7.5). */
export const phoneSchema = z
  .string({ required_error: "El teléfono es requerido" })
  .regex(PHONE_PATTERN, "El teléfono debe ser un '+' seguido de 8 a 15 dígitos");

/** Fecha de nacimiento: YYYY-MM-DD, no futura, ≥18 años (Property 9, Req. 7.6). */
export const birthDateSchema = z
  .string({ required_error: "La fecha de nacimiento es requerida" })
  .regex(BIRTH_DATE_PATTERN, "La fecha debe tener formato YYYY-MM-DD")
  .refine(
    (v) => isValidBirthDate(v),
    "La fecha no puede ser futura y debe corresponder a una edad de al menos 18 años",
  );

/** Fecha/hora RFC3339 (con offset). Espejo de `validateRFC3339`. */
export const rfc3339Schema = z
  .string()
  .datetime({ offset: true, message: "La fecha debe tener formato RFC3339" });

/** UUID canónico (Property 12). Espejo de `IsUUID`. */
export function uuidSchema(label: string) {
  return z
    .string({ required_error: `${label} es requerido` })
    .min(1, `${label} es requerido`)
    .regex(UUID_PATTERN, `${label} debe ser un UUID válido`);
}

/** Número positivo (> 0). Espejo de `ErrNotPositive`. */
export function positiveNumber(label: string) {
  return z
    .number({ required_error: `${label} es requerido`, invalid_type_error: `${label} debe ser un número` })
    .positive(`${label} debe ser mayor que 0`);
}

/** Entero positivo (> 0). */
export function positiveInt(label: string) {
  return z
    .number({ required_error: `${label} es requerido`, invalid_type_error: `${label} debe ser un número` })
    .int(`${label} debe ser un número entero`)
    .positive(`${label} debe ser mayor que 0`);
}
