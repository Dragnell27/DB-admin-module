// Feature: panel-admin-ott, Property 8: Validación de teléfono de Customer —
// para toda cadena de teléfono, la validación la acepta si y solo si cumple el
// patrón de un signo '+' seguido inmediatamente de 8 a 15 dígitos ASCII y nada
// más.
// Validates: Requirements 7.5
//
// La prueba contrasta `isValidPhone` (y, por coherencia, `phoneSchema`) contra
// un oráculo independiente que reconstruye estructuralmente la misma regla
// espejo del backend (`phonePattern = /^\+\d{8,15}$/` en resources.go) sin
// reutilizar la expresión regular de producción.

import { describe, expect, it } from "vitest";
import fc from "fast-check";

import { isValidPhone, phoneSchema } from "@/lib/validation";

/**
 * Oráculo independiente de la regla de teléfono. No reutiliza `PHONE_PATTERN`;
 * en su lugar descompone la cadena y comprueba las condiciones una a una, de
 * forma equivalente al requisito del backend: debe empezar por '+' y el resto
 * deben ser entre 8 y 15 dígitos ASCII (0-9), sin ningún otro carácter.
 */
function phoneOracle(value: string): boolean {
  // El primer carácter debe ser exactamente '+'.
  if (value.charCodeAt(0) !== 0x2b /* '+' */) {
    return false;
  }
  const rest = value.slice(1);
  // La cantidad de dígitos debe estar en el rango [8, 15].
  if (rest.length < 8 || rest.length > 15) {
    return false;
  }
  // Cada carácter restante debe ser un dígito ASCII 0-9.
  for (let i = 0; i < rest.length; i += 1) {
    const code = rest.charCodeAt(i);
    if (code < 0x30 || code > 0x39) {
      return false;
    }
  }
  return true;
}

describe("Property 8: validación de teléfono de Customer (Req. 7.5)", () => {
  // Dígito ASCII individual.
  const digit = fc.integer({ min: 0, max: 9 }).map((n) => String(n));

  // Teléfono bien formado: '+' seguido de 8..15 dígitos.
  const validPhoneArb = fc
    .array(digit, { minLength: 8, maxLength: 15 })
    .map((digits) => `+${digits.join("")}`);

  // --- Generadores de teléfonos inválidos por distintas razones ---

  // Vacío.
  const emptyArb = fc.constant("");

  // Sin el '+' inicial (solo dígitos, cantidad válida).
  const noPlusArb = fc
    .array(digit, { minLength: 8, maxLength: 15 })
    .map((digits) => digits.join(""));

  // '+' con muy pocos dígitos (0..7).
  const tooFewArb = fc
    .array(digit, { minLength: 0, maxLength: 7 })
    .map((digits) => `+${digits.join("")}`);

  // '+' con demasiados dígitos (16..25).
  const tooManyArb = fc
    .array(digit, { minLength: 16, maxLength: 25 })
    .map((digits) => `+${digits.join("")}`);

  // '+' + dígitos válidos pero con una letra incrustada en medio.
  const withLetterArb = fc
    .tuple(
      fc.array(digit, { minLength: 1, maxLength: 7 }),
      fc.constantFrom("a", "A", "x", "Z"),
      fc.array(digit, { minLength: 1, maxLength: 7 }),
    )
    .map(([left, letter, right]) => `+${left.join("")}${letter}${right.join("")}`);

  // '+' + dígitos válidos pero con un espacio incrustado.
  const withSpaceArb = fc
    .tuple(
      fc.array(digit, { minLength: 1, maxLength: 7 }),
      fc.array(digit, { minLength: 1, maxLength: 7 }),
    )
    .map(([left, right]) => `+${left.join("")} ${right.join("")}`);

  // Teléfono válido con basura al inicio (p. ej. espacios o signos).
  const leadingJunkArb = fc
    .tuple(fc.constantFrom(" ", "\t", "(", "00", "+"), validPhoneArb)
    .map(([junk, phone]) => `${junk}${phone}`);

  // Teléfono válido con basura al final.
  const trailingJunkArb = fc
    .tuple(validPhoneArb, fc.constantFrom(" ", "\n", ")", "#", "0"))
    .map(([phone, junk]) => `${phone}${junk}`);

  // '+' seguido de caracteres no dígitos (signos, símbolos).
  const extraCharsArb = fc
    .tuple(
      fc.array(digit, { minLength: 2, maxLength: 10 }),
      fc.constantFrom("-", ".", "+", "/", "e", "٥" /* dígito árabe-índico */),
    )
    .map(([digits, ch]) => `+${digits.join("")}${ch}`);

  // Unión de todos los generadores (válidos + inválidos + ruido general).
  const anyPhoneArb = fc.oneof(
    validPhoneArb,
    emptyArb,
    noPlusArb,
    tooFewArb,
    tooManyArb,
    withLetterArb,
    withSpaceArb,
    leadingJunkArb,
    trailingJunkArb,
    extraCharsArb,
    fc.string({ maxLength: 25 }),
  );

  it("acepta un teléfono si y solo si el oráculo lo acepta (IFF)", () => {
    fc.assert(
      fc.property(anyPhoneArb, (phone) => {
        expect(isValidPhone(phone)).toBe(phoneOracle(phone));
      }),
      { numRuns: 300 },
    );
  });

  it("acepta todo '+' seguido de 8..15 dígitos", () => {
    fc.assert(
      fc.property(validPhoneArb, (phone) => {
        expect(isValidPhone(phone)).toBe(true);
      }),
      { numRuns: 200 },
    );
  });

  it("rechaza inválidos (sin '+', letras, espacios, <8 o >15 dígitos, basura, vacío)", () => {
    // Nos aseguramos (vía el oráculo) de que estos generadores solo produzcan
    // cadenas realmente inválidas: ciertas combinaciones de "basura" (p. ej.
    // añadir un dígito al final de un teléfono válido) podrían dar por azar un
    // teléfono válido, lo que no corresponde a este caso.
    const invalidArb = fc
      .oneof(
        emptyArb,
        noPlusArb,
        tooFewArb,
        tooManyArb,
        withLetterArb,
        withSpaceArb,
        leadingJunkArb,
        trailingJunkArb,
        extraCharsArb,
      )
      .filter((phone) => !phoneOracle(phone));
    fc.assert(
      fc.property(invalidArb, (phone) => {
        expect(isValidPhone(phone)).toBe(false);
      }),
      { numRuns: 200 },
    );
  });

  it("phoneSchema.safeParse coincide con isValidPhone para toda cadena", () => {
    fc.assert(
      fc.property(anyPhoneArb, (phone) => {
        expect(phoneSchema.safeParse(phone).success).toBe(isValidPhone(phone));
      }),
      { numRuns: 300 },
    );
  });
});
