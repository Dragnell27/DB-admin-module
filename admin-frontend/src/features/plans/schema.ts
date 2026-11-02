/**
 * Schemas Zod del recurso Plan (crear/editar) con mensajes en español.
 *
 * Replican la validación server-side espejo del admin-backend
 * (`admin-backend/internal/validation/resources.go`: PlanInput,
 * ValidatePlanCreate/Edit, validatePlanConditionalGroups; y currency.go para el
 * mapeo moneda↔país). Cubre monto variable y campos emparejados
 * (Req. 10.2-10.12, 11.1, 11.2).
 *
 * Nota: la coincidencia de `country` con el país del JWT OTT activo la valida el
 * BFF (Req. 10.5); aquí solo se valida el formato de 2 letras, país soportado y
 * la coherencia moneda↔país.
 *
 * Los tipos inferidos se exportan para su uso por las mutations de TanStack
 * Query (tarea 15.2) y los formularios (tarea 16.2).
 */
import { z } from "zod";

import {
  COUNTRY_TO_CURRENCY,
  isSupportedCountry,
  LIMITS,
  PERIOD_TYPES,
  positiveInt,
  positiveNumber,
  requiredString,
} from "@/lib/validation";

/** Países soportados como string para mensajes de error. */
const SUPPORTED = Object.keys(COUNTRY_TO_CURRENCY).join(", ");

/**
 * Refinamiento del grupo de monto variable (Req. 10.10): cuando
 * `variable_amount` es verdadero, se exige `minimum_amount` y `maximum_amount`
 * ambos > 0 con `minimum_amount < maximum_amount`. Espejo de
 * `validatePlanConditionalGroups` (bloque de monto variable).
 */
function refineVariableAmount<
  T extends {
    variable_amount?: boolean;
    minimum_amount?: number;
    maximum_amount?: number;
  },
>(schema: z.ZodType<T>): z.ZodEffects<z.ZodType<T>> {
  return schema.superRefine((v, ctx) => {
    if (!v.variable_amount) {
      return;
    }
    const min = v.minimum_amount;
    const max = v.maximum_amount;
    if (
      min === undefined ||
      max === undefined ||
      min <= 0 ||
      max <= 0 ||
      min >= max
    ) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ["minimum_amount"],
        message:
          "Con monto variable, minimum_amount y maximum_amount deben ser > 0 y minimum_amount < maximum_amount",
      });
    }
  });
}

/**
 * Refinamiento de la pareja `frequency_test` + `test_type_period` (Req. 10.11):
 * ambos presentes o ambos ausentes. Un `test_type_period` vacío cuenta como
 * ausente, igual que en el backend (TrimSpace).
 */
function refineFrequencyTestPair<
  T extends { frequency_test?: number; test_type_period?: string },
>(schema: z.ZodType<T>): z.ZodEffects<z.ZodType<T>> {
  return schema.superRefine((v, ctx) => {
    const freqPresent = v.frequency_test !== undefined;
    const typePresent =
      v.test_type_period !== undefined && v.test_type_period.trim() !== "";
    if (freqPresent !== typePresent) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ["frequency_test"],
        message:
          "frequency_test y test_type_period deben enviarse juntos o ninguno",
      });
    }
  });
}

/**
 * Refinamiento del grupo oneshot (Req. 10.12): `oneshot_amount`,
 * `oneshot_frequency` y `oneshot_period` como grupo completo o totalmente
 * ausente. Espejo de `validateGroupPresence`.
 */
function refineOneshotGroup<
  T extends {
    oneshot_amount?: number;
    oneshot_frequency?: number;
    oneshot_period?: string;
  },
>(schema: z.ZodType<T>): z.ZodEffects<z.ZodType<T>> {
  return schema.superRefine((v, ctx) => {
    const presence = [
      v.oneshot_amount !== undefined,
      v.oneshot_frequency !== undefined,
      v.oneshot_period !== undefined && v.oneshot_period.trim() !== "",
    ];
    const anyPresent = presence.some(Boolean);
    const allPresent = presence.every(Boolean);
    if (anyPresent && !allPresent) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ["oneshot_amount"],
        message:
          "oneshot_amount, oneshot_frequency y oneshot_period deben enviarse como grupo completo o ninguno",
      });
    }
  });
}

/**
 * Aplica los tres refinamientos de grupo condicional de Plan a cualquier schema
 * de objeto base (creación o edición).
 */
function withPlanConditionalGroups<
  T extends {
    variable_amount?: boolean;
    minimum_amount?: number;
    maximum_amount?: number;
    frequency_test?: number;
    test_type_period?: string;
    oneshot_amount?: number;
    oneshot_frequency?: number;
    oneshot_period?: string;
  },
>(schema: z.ZodType<T>) {
  return refineOneshotGroup(
    refineFrequencyTestPair(refineVariableAmount(schema)) as z.ZodType<T>,
  );
}

/**
 * Validación conjunta de moneda↔país (Property 10, Req. 10.4, 10.7). Se aplica a
 * nivel de objeto para poder emitir el error en el campo `currency`.
 */
function refineCountryCurrency<
  T extends { country?: string; currency?: string },
>(schema: z.ZodType<T>): z.ZodEffects<z.ZodType<T>> {
  return schema.superRefine((v, ctx) => {
    const { country, currency } = v;
    // En edición ambos pueden estar ausentes; solo validamos si viene alguno.
    if (!country && !currency) {
      return;
    }
    if (!country || !isSupportedCountry(country)) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ["country"],
        message: `country debe ser uno de los países soportados (${SUPPORTED})`,
      });
      return;
    }
    const expected = COUNTRY_TO_CURRENCY[country.trim().toLowerCase()];
    if (!currency || currency.trim().toUpperCase() !== expected) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ["currency"],
        message: `currency debe coincidir con la moneda del país (${country ? `${country}=${expected}` : SUPPORTED})`,
      });
    }
  });
}

/** Campos condicionales comunes de Plan (monto variable, parejas y grupos). */
const planConditionalFields = {
  variable_amount: z.boolean().default(false),
  minimum_amount: positiveNumber("minimum_amount").optional(),
  maximum_amount: positiveNumber("maximum_amount").optional(),
  frequency_test: z
    .number({ invalid_type_error: "frequency_test debe ser un número" })
    .int("frequency_test debe ser un número entero")
    .optional(),
  test_type_period: z.string().optional(),
  oneshot_amount: z
    .number({ invalid_type_error: "oneshot_amount debe ser un número" })
    .optional(),
  oneshot_frequency: z
    .number({ invalid_type_error: "oneshot_frequency debe ser un número" })
    .int("oneshot_frequency debe ser un número entero")
    .optional(),
  oneshot_period: z.string().optional(),
};

/** Campos "extra" de Plan presentes en el diseño con sus valores por defecto. */
const planExtraFields = {
  max_retrys: z.number().int().default(3),
  can_duplicate: z.boolean().optional(),
  max_user_subscription: z.number().int().default(1),
  can_subscribe: z.boolean().optional(),
  status: z.string().default("active"),
};

/**
 * Plan — crear (Req. 10.2-10.12).
 *
 * - name: req, máx 255
 * - external_id: req, máx 100
 * - country: 2 letras y país soportado (coincidencia con token la valida el BFF)
 * - currency: 3 letras y coherente con el país (sv=USD, cr=CRC, gt=GTQ, hn=HNL, ni=NIO)
 * - amount: > 0
 * - frequency: entero > 0
 * - period_type: dia|semana|mes|año
 * - variable_amount + minimum/maximum, frequency_test/test_type_period, oneshot_*: reglas condicionales
 */
const planCreateBase = z.object({
  name: requiredString("name", LIMITS.planName),
  external_id: requiredString("external_id", LIMITS.externalId),
  country: z
    .string({ required_error: "country es requerido" })
    .length(2, "country debe tener exactamente 2 letras"),
  amount: positiveNumber("amount"),
  currency: z
    .string({ required_error: "currency es requerido" })
    .length(3, "currency debe tener exactamente 3 letras"),
  frequency: positiveInt("frequency"),
  period_type: z.enum(PERIOD_TYPES, {
    errorMap: () => ({ message: "period_type debe ser dia, semana, mes o año" }),
  }),
  ...planConditionalFields,
  ...planExtraFields,
});

export const planCreateSchema = refineCountryCurrency(
  withPlanConditionalGroups(planCreateBase),
);

/**
 * Plan — editar (Req. 11.1, 11.2). Todos los campos son opcionales pero, si se
 * envían, cumplen las mismas reglas de formato/rango/emparejamiento. La
 * identificación del plan es por `plan_id` (manejado por el handler/formulario).
 */
const planEditBase = z.object({
  name: requiredString("name", LIMITS.planName).optional(),
  external_id: requiredString("external_id", LIMITS.externalId).optional(),
  country: z
    .string()
    .length(2, "country debe tener exactamente 2 letras")
    .optional(),
  amount: positiveNumber("amount").optional(),
  currency: z
    .string()
    .length(3, "currency debe tener exactamente 3 letras")
    .optional(),
  frequency: positiveInt("frequency").optional(),
  period_type: z
    .enum(PERIOD_TYPES, {
      errorMap: () => ({
        message: "period_type debe ser dia, semana, mes o año",
      }),
    })
    .optional(),
  ...planConditionalFields,
  variable_amount: z.boolean().optional(),
  max_retrys: z.number().int().optional(),
  can_duplicate: z.boolean().optional(),
  max_user_subscription: z.number().int().optional(),
  can_subscribe: z.boolean().optional(),
  status: z.string().optional(),
});

export const planEditSchema = refineCountryCurrency(
  withPlanConditionalGroups(planEditBase),
);

export type PlanCreateInput = z.infer<typeof planCreateSchema>;
export type PlanEditInput = z.infer<typeof planEditSchema>;
