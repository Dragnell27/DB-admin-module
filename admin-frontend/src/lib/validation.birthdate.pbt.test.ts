// Feature: panel-admin-ott, Property 9: Validación de edad mínima y fecha de
// nacimiento — para toda fecha de nacimiento, la validación la acepta si y solo
// si tiene formato YYYY-MM-DD, corresponde a una fecha real del calendario, no
// es futura respecto a la fecha de referencia y la edad completa a esa fecha es
// de al menos 18 años (MIN_ADULT_AGE).
// Validates: Requirements 7.6
//
// La prueba contrasta `isValidBirthDate(d, ref)` contra un oráculo independiente
// que reconstruye estructuralmente la misma regla espejo del backend
// (`validateBirthDate` + `ageInYears` en resources.go, en UTC): patrón estricto
// YYYY-MM-DD, fecha de calendario existente, no posterior a `ref` y edad >= 18.
// Para el determinismo se inyecta una `ref` fija en cada llamada.
//
// Enfoque de concordancia con el schema Zod (`birthDateSchema`): el refinamiento
// del schema usa la fecha actual INTERNAMENTE (llama a `isValidBirthDate(v)` sin
// `ref`), por lo que no es determinista frente a una `ref` arbitraria. Para
// contrastarlo de forma determinista se verifica el subconjunto que el schema
// puede confirmar sin depender del "hoy": (a) toda cadena que NO casa el patrón
// YYYY-MM-DD o que NO es una fecha real es rechazada por el schema; y (b) para
// fechas de referencia próximas a "ahora" se comprueba que el schema coincide
// con `isValidBirthDate(v, now)` usando la fecha actual real como referencia.

import { describe, expect, it } from "vitest";
import fc from "fast-check";

import {
  BIRTH_DATE_PATTERN,
  birthDateSchema,
  isValidBirthDate,
  MIN_ADULT_AGE,
} from "@/lib/validation";

// Fecha de referencia fija para todas las propiedades deterministas.
// Año no bisiesto a mitad de año para evitar casos borde triviales de 29/02.
const REF = new Date(Date.UTC(2025, 5, 15)); // 2025-06-15 (UTC)

/** Formatea un día del calendario (UTC) a la cadena YYYY-MM-DD. */
function fmt(year: number, month: number, day: number): string {
  const y = String(year).padStart(4, "0");
  const m = String(month).padStart(2, "0");
  const d = String(day).padStart(2, "0");
  return `${y}-${m}-${d}`;
}

/**
 * Oráculo independiente de la regla de fecha de nacimiento. No reutiliza
 * `isValidBirthDate`; descompone la cadena y comprueba las condiciones una a una
 * de forma equivalente al requisito del backend, operando en UTC.
 */
function birthDateOracle(value: string, ref: Date): boolean {
  // 1) Formato estricto YYYY-MM-DD.
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) {
    return false;
  }
  const [ys, ms, ds] = value.split("-");
  const year = Number(ys);
  const month = Number(ms);
  const day = Number(ds);

  // 2) Fecha real del calendario: construimos la fecha UTC y verificamos que
  //    sus componentes coincidan (así se rechazan 2021-02-30, 2021-13-01, etc.).
  const parsed = new Date(Date.UTC(year, month - 1, day));
  if (
    parsed.getUTCFullYear() !== year ||
    parsed.getUTCMonth() !== month - 1 ||
    parsed.getUTCDate() !== day
  ) {
    return false;
  }

  // 3) No futura respecto a `ref` (comparando solo la parte de fecha en UTC).
  const refDay = new Date(
    Date.UTC(ref.getUTCFullYear(), ref.getUTCMonth(), ref.getUTCDate()),
  );
  if (parsed.getTime() > refDay.getTime()) {
    return false;
  }

  // 4) Edad completa >= 18. Calculada de forma independiente: comparamos la
  //    fecha del 18º cumpleaños con la referencia.
  const eighteenth = new Date(Date.UTC(year + MIN_ADULT_AGE, month - 1, day));
  return eighteenth.getTime() <= refDay.getTime();
}

describe("Property 9: validación de edad mínima y fecha de nacimiento (Req. 7.6)", () => {
  const refYear = REF.getUTCFullYear();

  // --- Generadores de fechas VÁLIDAS (adultos, edad 18..100) -----------------
  // Para evitar ambigüedades de días por mes, nos restringimos a días 1..28,
  // que existen en todos los meses. El año se escoge para garantizar edad >= 18
  // en la fecha de referencia; con mes/día <= los de REF, la edad es year-diff.
  const validAdultArb = fc
    .tuple(
      fc.integer({ min: MIN_ADULT_AGE, max: 100 }), // edad objetivo
      fc.integer({ min: 1, max: 12 }),
      fc.integer({ min: 1, max: 28 }),
    )
    .map(([age, month, day]) => {
      const year = refYear - age;
      return fmt(year, month, day);
    })
    // Filtramos a casos donde el oráculo confirma validez (no futura y >= 18),
    // cubriendo el posible desfase de un año según mes/día vs la referencia.
    .filter((s) => birthDateOracle(s, REF));

  // --- Generadores de fechas de MENORES de edad (0..17 años) -----------------
  const underageArb = fc
    .tuple(
      fc.integer({ min: 0, max: 17 }),
      fc.integer({ min: 1, max: 12 }),
      fc.integer({ min: 1, max: 28 }),
    )
    .map(([age, month, day]) => fmt(refYear - age, month, day))
    .filter((s) => !birthDateOracle(s, REF));

  // Casos borde del 18º cumpleaños: exactamente hoy (válido) y un día antes
  // (aún con 17, inválido). Usamos componentes de REF para construirlos.
  const refMonth = REF.getUTCMonth() + 1;
  const refDate = REF.getUTCDate();
  const exactlyEighteen = fmt(refYear - MIN_ADULT_AGE, refMonth, refDate);
  // Día siguiente al nacimiento → el 18º cumpleaños cae un día DESPUÉS de REF.
  const dayBeforeEighteen = fmt(refYear - MIN_ADULT_AGE, refMonth, refDate + 1);

  // --- Generadores de fechas FUTURAS -----------------------------------------
  const futureArb = fc
    .tuple(
      fc.integer({ min: 1, max: 50 }),
      fc.integer({ min: 1, max: 12 }),
      fc.integer({ min: 1, max: 28 }),
    )
    .map(([yearsAhead, month, day]) => fmt(refYear + yearsAhead, month, day));

  // --- Generadores de cadenas MALFORMADAS ------------------------------------
  // Fechas imposibles con formato correcto pero sin existir en el calendario.
  const impossibleArb = fc.constantFrom(
    "2021-02-30",
    "2021-13-01",
    "2021-00-10",
    "2021-01-00",
    "2000-04-31",
    "2019-02-29", // 2019 no es bisiesto
    "2021-12-32",
  );

  // Formatos erróneos (separadores, longitud, no numéricos).
  const malformedFormatArb = fc.oneof(
    fc.constant(""),
    fc.constant("2000/01/01"),
    fc.constant("01-01-2000"),
    fc.constant("2000-1-1"),
    fc.constant("20000-01-01"),
    fc.constant("2000-01-01 "),
    fc.constant(" 2000-01-01"),
    fc.constant("abcd-ef-gh"),
    fc.string({ maxLength: 12 }),
  );

  // Unión de todos los generadores para la propiedad IFF global.
  const anyBirthDateArb = fc.oneof(
    validAdultArb,
    underageArb,
    futureArb,
    impossibleArb,
    malformedFormatArb,
    fc.constant(exactlyEighteen),
    fc.constant(dayBeforeEighteen),
  );

  it("acepta una fecha si y solo si el oráculo la acepta (IFF) con ref fija", () => {
    fc.assert(
      fc.property(anyBirthDateArb, (d) => {
        expect(isValidBirthDate(d, REF)).toBe(birthDateOracle(d, REF));
      }),
      { numRuns: 300 },
    );
  });

  it("acepta todas las fechas de adultos (edad 18..100, no futuras)", () => {
    fc.assert(
      fc.property(validAdultArb, (d) => {
        expect(isValidBirthDate(d, REF)).toBe(true);
      }),
      { numRuns: 200 },
    );
  });

  it("rechaza menores de edad (0..17 años)", () => {
    fc.assert(
      fc.property(underageArb, (d) => {
        expect(isValidBirthDate(d, REF)).toBe(false);
      }),
      { numRuns: 200 },
    );
  });

  it("rechaza fechas futuras", () => {
    fc.assert(
      fc.property(futureArb, (d) => {
        expect(isValidBirthDate(d, REF)).toBe(false);
      }),
      { numRuns: 100 },
    );
  });

  it("rechaza cadenas malformadas y fechas imposibles", () => {
    fc.assert(
      fc.property(fc.oneof(impossibleArb, malformedFormatArb), (d) => {
        expect(isValidBirthDate(d, REF)).toBe(false);
      }),
      { numRuns: 100 },
    );
  });

  it("acepta exactamente el 18º cumpleaños y rechaza el día anterior (borde)", () => {
    expect(isValidBirthDate(exactlyEighteen, REF)).toBe(true);
    expect(isValidBirthDate(dayBeforeEighteen, REF)).toBe(false);
  });

  // --- Concordancia con el schema Zod (enfoque documentado arriba) -----------

  it("birthDateSchema rechaza todo lo que no case patrón o no sea fecha real", () => {
    // Subconjunto determinista: estas entradas son rechazadas por el schema
    // independientemente del 'hoy' (fallan patrón o realidad de la fecha).
    fc.assert(
      fc.property(fc.oneof(malformedFormatArb, impossibleArb), (d) => {
        const matchesPattern = BIRTH_DATE_PATTERN.test(d);
        if (!matchesPattern) {
          // El regex del schema ya lo rechaza.
          expect(birthDateSchema.safeParse(d).success).toBe(false);
        } else {
          // Casa patrón pero es una fecha imposible → el refinamiento la rechaza.
          expect(birthDateSchema.safeParse(d).success).toBe(false);
        }
      }),
      { numRuns: 100 },
    );
  });

  it("birthDateSchema.safeParse coincide con isValidBirthDate(v, now) usando la fecha actual", () => {
    // El schema usa la fecha actual internamente; contrastamos contra
    // `isValidBirthDate(v, now)` con la MISMA referencia (ahora) para que sea
    // determinista dentro de la ejecución de la prueba.
    const now = new Date();
    const nowYear = now.getUTCFullYear();

    const schemaArb = fc.oneof(
      // Adultos relativos a "ahora".
      fc
        .tuple(
          fc.integer({ min: MIN_ADULT_AGE + 1, max: 90 }),
          fc.integer({ min: 1, max: 12 }),
          fc.integer({ min: 1, max: 28 }),
        )
        .map(([age, m, d]) => fmt(nowYear - age, m, d)),
      // Menores relativos a "ahora".
      fc
        .tuple(
          fc.integer({ min: 0, max: 16 }),
          fc.integer({ min: 1, max: 12 }),
          fc.integer({ min: 1, max: 28 }),
        )
        .map(([age, m, d]) => fmt(nowYear - age, m, d)),
      // Futuras, imposibles y malformadas.
      futureArb,
      impossibleArb,
      malformedFormatArb,
    );

    fc.assert(
      fc.property(schemaArb, (d) => {
        expect(birthDateSchema.safeParse(d).success).toBe(
          isValidBirthDate(d, now),
        );
      }),
      { numRuns: 200 },
    );
  });
});
