// Feature: panel-admin-ott, Property 7: Validación de email de Customer —
// para toda cadena de email, la validación la acepta si y solo si tiene formato
// de correo válido (una sola arroba, sin espacios, con dominio y TLD) y su
// longitud no excede 255 caracteres.
// Validates: Requirements 7.4
//
// La prueba contrasta `isValidEmail` (y, por coherencia, `emailSchema`) contra
// un oráculo independiente que reconstruye estructuralmente la misma regla
// espejo del backend (`validateEmail` en resources.go: rechaza vacío, longitud
// > 255 y cualquier dirección que no sea un único correo bien formado).

import { describe, expect, it } from "vitest";
import fc from "fast-check";

import { emailSchema, isValidEmail, LIMITS } from "@/lib/validation";

/**
 * Oráculo independiente de la regla de email. No reutiliza la expresión regular
 * de `isValidEmail`; en su lugar descompone la cadena en partes y comprueba las
 * condiciones una a una, de forma equivalente al requisito del backend.
 */
function emailOracle(value: string): boolean {
  // Regla de longitud (Req. 7.4): no vacío y máximo 255 caracteres.
  if (value.length === 0 || value.length > LIMITS.email) {
    return false;
  }
  // Sin espacios en blanco de ningún tipo en toda la cadena.
  if (/\s/.test(value)) {
    return false;
  }
  // Exactamente una arroba, separando local y dominio no vacíos.
  const parts = value.split("@");
  if (parts.length !== 2) {
    return false;
  }
  const [local, domain] = parts;
  if (local.length === 0 || domain.length === 0) {
    return false;
  }
  // El dominio debe tener al menos un punto y ninguna etiqueta vacía
  // (es decir, dominio + TLD, sin puntos iniciales/finales ni dobles).
  if (!domain.includes(".")) {
    return false;
  }
  const labels = domain.split(".");
  return labels.every((label) => label.length > 0);
}

describe("Property 7: validación de email de Customer (Req. 7.4)", () => {
  // Generador de correos bien formados: etiquetas sin espacios ni arrobas ni
  // puntos, con un TLD, cuya longitud total no exceda el límite.
  const label = fc
    .string({ minLength: 1, maxLength: 12 })
    .filter((s) => /^[^\s@.]+$/.test(s));

  const validEmailArb = fc
    .tuple(label, label, label)
    .map(([local, domain, tld]) => `${local}@${domain}.${tld}`)
    .filter((email) => email.length <= LIMITS.email);

  // Generadores de correos inválidos por distintas razones.
  const emptyArb = fc.constant("");

  const noAtArb = fc
    .string({ minLength: 1, maxLength: 40 })
    .filter((s) => !s.includes("@") && !/\s/.test(s));

  const withSpaceArb = fc
    .tuple(label, label, label)
    .map(([a, b, c]) => `${a}@${b}.${c} `);

  const multipleAtArb = fc
    .tuple(label, label, label)
    .map(([a, b, c]) => `${a}@${b}@${c}.com`);

  const missingDomainArb = fc.tuple(label).map(([a]) => `${a}@`);

  const missingTldArb = fc
    .tuple(label, label)
    .map(([a, b]) => `${a}@${b}`);

  const tooLongArb = fc
    .tuple(label, label)
    .map(([domain, tld]) => {
      // Local part lo bastante largo para superar 255 caracteres totales.
      const padLen = LIMITS.email + 10 - (domain.length + tld.length + 2);
      const local = "a".repeat(Math.max(padLen, 1));
      return `${local}@${domain}.${tld}`;
    })
    .filter((email) => email.length > LIMITS.email);

  it("acepta un correo si y solo si el oráculo lo acepta (IFF)", () => {
    fc.assert(
      fc.property(
        fc.oneof(
          validEmailArb,
          emptyArb,
          noAtArb,
          withSpaceArb,
          multipleAtArb,
          missingDomainArb,
          missingTldArb,
          tooLongArb,
          // Correos arbitrarios de fast-check para cubrir el espacio general.
          fc.emailAddress(),
          fc.string({ maxLength: 40 }),
        ),
        (email) => {
          expect(isValidEmail(email)).toBe(emailOracle(email));
        },
      ),
      { numRuns: 300 },
    );
  });

  it("acepta todos los correos bien formados (<=255)", () => {
    fc.assert(
      fc.property(validEmailArb, (email) => {
        expect(isValidEmail(email)).toBe(true);
      }),
      { numRuns: 200 },
    );
  });

  it("rechaza correos inválidos (vacío, sin @, con espacios, múltiples @, sin dominio/TLD, >255)", () => {
    const invalidArb = fc.oneof(
      emptyArb,
      noAtArb,
      withSpaceArb,
      multipleAtArb,
      missingDomainArb,
      missingTldArb,
      tooLongArb,
    );
    fc.assert(
      fc.property(invalidArb, (email) => {
        expect(isValidEmail(email)).toBe(false);
      }),
      { numRuns: 200 },
    );
  });

  it("emailSchema.safeParse coincide con isValidEmail para toda cadena", () => {
    fc.assert(
      fc.property(
        fc.oneof(
          validEmailArb,
          emptyArb,
          noAtArb,
          withSpaceArb,
          multipleAtArb,
          missingDomainArb,
          missingTldArb,
          tooLongArb,
          fc.emailAddress(),
          fc.string({ maxLength: 40 }),
        ),
        (email) => {
          expect(emailSchema.safeParse(email).success).toBe(
            isValidEmail(email),
          );
        },
      ),
      { numRuns: 300 },
    );
  });
});
