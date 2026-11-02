import type { Customer } from "@/features/customers/hooks";

/**
 * Normaliza un campo de texto opcional del cliente: recorta espacios y trata
 * las cadenas vacías o compuestas solo por espacios como ausentes.
 *
 * @param value - Valor crudo que puede venir del BFF (posiblemente `undefined`).
 * @returns El texto recortado si contiene algo útil; en caso contrario `undefined`.
 */
function cleanText(value?: string): string | undefined {
  if (typeof value !== "string") {
    return undefined;
  }
  const trimmed = value.trim();
  return trimmed === "" ? undefined : trimmed;
}

/**
 * Deriva una etiqueta legible para un cliente, priorizando el nombre sobre el
 * correo (Req 6.2, 6.5).
 *
 * Reglas de resolución:
 * - Si existe `first_name` y/o `last_name` (no vacíos), devuelve el nombre
 *   completo recortado (p. ej. `"First Last"`, o solo una parte si la otra
 *   falta).
 * - Si no hay nombre utilizable pero sí un `email` no vacío, devuelve el correo.
 * - Si no hay nada utilizable, o el cliente es `undefined`, devuelve `undefined`
 *   para que el consumidor pueda recurrir a un respaldo (p. ej. el id crudo).
 *
 * Las cadenas vacías o formadas solo por espacios se consideran ausentes.
 *
 * @param customer - Cliente a describir (puede ser `undefined`).
 * @returns El nombre completo, el correo, o `undefined` si no hay dato legible.
 */
export function readableCustomer(customer?: Customer): string | undefined {
  if (!customer) {
    return undefined;
  }

  const firstName = cleanText(customer.first_name);
  const lastName = cleanText(customer.last_name);
  const name = [firstName, lastName].filter(Boolean).join(" ");
  if (name !== "") {
    return name;
  }

  return cleanText(customer.email);
}
