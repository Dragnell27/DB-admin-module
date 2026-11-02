"use client";

/**
 * Formulario de creación/edición de Plan (tarea 16.2, Req. 10 y 11).
 *
 * Un único componente cubre ambos modos:
 * - `mode="create"` usa `planCreateSchema` y exige los campos obligatorios.
 * - `mode="edit"` usa `planEditSchema`, donde todos los campos son opcionales
 *   pero, si se envían, cumplen las mismas reglas (Req. 11.1, 11.2). La
 *   identificación del plan por `plan_id` la maneja el padre (vista).
 *
 * Se construye con React Hook Form + zodResolver y las primitivas existentes
 * (`Input`, `Label`, `Button`) más elementos nativos (`<select>` para enums),
 * sin añadir primitivas compartidas. Los campos numéricos opcionales se
 * normalizan a `undefined` cuando quedan vacíos, de modo que no violen las
 * reglas de emparejamiento (frequency_test/test_type_period y el trío oneshot).
 *
 * El mapeo moneda↔país (Req. 10.7) se refleja en la UI autocompletando
 * `currency` al elegir `country`, aunque la validación real la impone el schema.
 */
import * as React from "react";
import {
  useForm,
  type Resolver,
  type ResolverOptions,
} from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { Loader2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  COUNTRY_TO_CURRENCY,
  PERIOD_TYPES,
  SUPPORTED_COUNTRIES,
} from "@/lib/validation";
import {
  planCreateSchema,
  planEditSchema,
  type PlanCreateInput,
  type PlanEditInput,
} from "@/features/plans/schema";
import type { Plan } from "@/features/plans/hooks";
import { FeedbackBanner } from "@/features/plans/components/feedback-banner";

/** Valores del formulario en modo creación. */
export type PlanFormCreateValues = PlanCreateInput;
/** Valores del formulario en modo edición. */
export type PlanFormEditValues = PlanEditInput;

export interface PlanFormProps {
  readonly mode: "create" | "edit";
  /** Plan existente a precargar en modo edición. */
  readonly initialPlan?: Plan;
  /** Indica un envío en curso. */
  readonly isSubmitting?: boolean;
  /** Mensaje de error del servidor (BFF/OTT), si lo hubo. */
  readonly serverError?: string | null;
  /** Envío validado: entrega los valores ya parseados por el schema. */
  readonly onSubmit: (values: PlanCreateInput | PlanEditInput) => void;
  /** Cancela el formulario sin enviar. */
  readonly onCancel: () => void;
}

/** Estructura plana de los campos del formulario (todos como texto/boolean). */
interface PlanFormShape {
  name: string;
  external_id: string;
  country: string;
  currency: string;
  amount: string;
  frequency: string;
  period_type: string;
  max_retrys: string;
  max_user_subscription: string;
  status: string;
  can_duplicate: boolean;
  can_subscribe: boolean;
  variable_amount: boolean;
  minimum_amount: string;
  maximum_amount: string;
  frequency_test: string;
  test_type_period: string;
  oneshot_amount: string;
  oneshot_frequency: string;
  oneshot_period: string;
}

/** Convierte un valor desconocido del Plan a texto para el input. */
function toText(value: unknown): string {
  if (value === null || value === undefined) {
    return "";
  }
  return String(value);
}

/**
 * Convierte un valor del formulario a número, o `undefined` si está vacío.
 *
 * Tolera que el valor llegue como number o string: aunque el `PlanFormShape`
 * usa strings, algún input (p. ej. `type="number"`) puede entregar un number,
 * en cuyo caso llamar `.trim()` reventaría. Se normaliza a string primero.
 */
function toOptionalNumber(value: unknown): number | undefined {
  if (value === null || value === undefined) {
    return undefined;
  }
  if (typeof value === "number") {
    return Number.isNaN(value) ? undefined : value;
  }
  const trimmed = String(value).trim();
  if (trimmed === "") {
    return undefined;
  }
  const parsed = Number(trimmed);
  return Number.isNaN(parsed) ? undefined : parsed;
}

/** Texto opcional: `undefined` cuando queda vacío. Tolera valores no-string. */
function toOptionalText(value: unknown): string | undefined {
  if (value === null || value === undefined) {
    return undefined;
  }
  const trimmed = String(value).trim();
  return trimmed === "" ? undefined : trimmed;
}

/** Deriva los valores iniciales del formulario a partir de un Plan existente. */
function initialValues(plan?: Plan): PlanFormShape {
  return {
    name: toText(plan?.name),
    external_id: toText(plan?.external_id),
    country: toText(plan?.country).toLowerCase(),
    currency: toText(plan?.currency).toUpperCase(),
    amount: toText(plan?.amount),
    frequency: toText(plan?.frequency),
    period_type: toText(plan?.period_type),
    max_retrys: toText(plan?.max_retrys),
    max_user_subscription: toText(plan?.max_user_subscription),
    status: toText(plan?.status),
    can_duplicate: Boolean(plan?.can_duplicate),
    can_subscribe: Boolean(plan?.can_subscribe),
    variable_amount: Boolean(plan?.variable_amount),
    minimum_amount: toText(plan?.minimum_amount),
    maximum_amount: toText(plan?.maximum_amount),
    frequency_test: toText(plan?.frequency_test),
    test_type_period: toText(plan?.test_type_period),
    oneshot_amount: toText(plan?.oneshot_amount),
    oneshot_frequency: toText(plan?.oneshot_frequency),
    oneshot_period: toText(plan?.oneshot_period),
  };
}

/**
 * Ensambla el payload tipado para el schema a partir de los campos de texto.
 * En modo edición, los campos de texto vacíos se omiten (quedan `undefined`)
 * para respetar que todo es opcional (Req. 11.1).
 */
function buildPayload(
  shape: PlanFormShape,
  mode: "create" | "edit",
): Record<string, unknown> {
  const payload: Record<string, unknown> = {};
  // Campos "extra" (variable_amount, min/max, max_retrys, max_user_subscription,
  // status, can_duplicate, can_subscribe) NO se envían: el OTT los rechaza en
  // creación ("Unallowed fields") y asigna sus propios valores por defecto. Al
  // editar tampoco se usan por ahora.

  const assignText = (key: string, value: unknown, required: boolean): void => {
    const trimmed = value === null || value === undefined ? "" : String(value).trim();
    if (required || trimmed !== "") {
      payload[key] = trimmed;
    }
  };

  const assignNumber = (
    key: string,
    value: unknown,
    required: boolean,
  ): void => {
    const parsed = toOptionalNumber(value);
    if (required) {
      // Deja pasar NaN como number inválido para que el schema reporte el error.
      payload[key] =
        parsed ?? Number(value === null || value === undefined ? "" : String(value).trim());
    } else if (parsed !== undefined) {
      payload[key] = parsed;
    }
  };

  const required = mode === "create";

  assignText("name", shape.name, required);
  assignText("external_id", shape.external_id, required);
  assignText("country", shape.country, required);
  assignText("currency", shape.currency, required);
  assignNumber("amount", shape.amount, required);
  assignNumber("frequency", shape.frequency, required);
  assignText("period_type", shape.period_type, required);

  // (Los campos "extra" y el grupo de monto variable no se envían; ver nota
  // arriba: el OTT los rechaza en creación y usa sus propios defaults.)

  // Pareja frequency_test + test_type_period.
  const freqTest = toOptionalNumber(shape.frequency_test);
  if (freqTest !== undefined) {
    payload.frequency_test = freqTest;
  }
  const testPeriod = toOptionalText(shape.test_type_period);
  if (testPeriod !== undefined) {
    payload.test_type_period = testPeriod;
  }

  // Trío oneshot.
  const oneshotAmount = toOptionalNumber(shape.oneshot_amount);
  if (oneshotAmount !== undefined) {
    payload.oneshot_amount = oneshotAmount;
  }
  const oneshotFreq = toOptionalNumber(shape.oneshot_frequency);
  if (oneshotFreq !== undefined) {
    payload.oneshot_frequency = oneshotFreq;
  }
  const oneshotPeriod = toOptionalText(shape.oneshot_period);
  if (oneshotPeriod !== undefined) {
    payload.oneshot_period = oneshotPeriod;
  }

  return payload;
}

/** Mensaje de error de un campo, extraído del árbol de errores de RHF. */
function fieldError(
  errors: Record<string, unknown>,
  key: string,
): string | undefined {
  const entry = errors[key] as { message?: unknown } | undefined;
  return typeof entry?.message === "string" ? entry.message : undefined;
}

export function PlanForm({
  mode,
  initialPlan,
  isSubmitting = false,
  serverError = null,
  onSubmit,
  onCancel,
}: PlanFormProps) {
  const schema = mode === "create" ? planCreateSchema : planEditSchema;

  const {
    register,
    handleSubmit,
    watch,
    setValue,
    formState: { errors },
  } = useForm<PlanFormShape>({
    // El resolver valida el payload ensamblado, no la forma plana; ver abajo.
    resolver: ((
      values: PlanFormShape,
      context: unknown,
      options: ResolverOptions<Record<string, unknown>>,
    ) => {
      const payload = buildPayload(values, mode);
      const resolver = zodResolver(schema) as Resolver<Record<string, unknown>>;
      return resolver(payload, context, options);
    }) as unknown as Resolver<PlanFormShape>,
    defaultValues: initialValues(initialPlan),
  });

  const variableAmount = watch("variable_amount");

  // Autocompleta la moneda al elegir país (coherencia moneda↔país, Req. 10.7).
  const handleCountryChange = React.useCallback(
    (event: React.ChangeEvent<HTMLSelectElement>) => {
      const country = event.target.value;
      setValue("country", country, { shouldValidate: true });
      const currency = COUNTRY_TO_CURRENCY[country];
      if (currency) {
        setValue("currency", currency, { shouldValidate: true });
      }
    },
    [setValue],
  );

  const submit = handleSubmit((values) => {
    const payload = buildPayload(values, mode);
    onSubmit(payload as PlanCreateInput | PlanEditInput);
  });

  const errorMap = errors as unknown as Record<string, unknown>;
  const optionalHint = mode === "edit" ? " (opcional)" : "";

  return (
    <form noValidate onSubmit={submit} className="space-y-6">
      {serverError ? (
        <FeedbackBanner tone="error" message={serverError} />
      ) : null}

      {/* Datos principales */}
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label={`Nombre${optionalHint}`} htmlFor="plan-name" error={fieldError(errorMap, "name")}>
          <Input id="plan-name" {...register("name")} />
        </Field>

        <Field
          label={`External ID${optionalHint}`}
          htmlFor="plan-external-id"
          error={fieldError(errorMap, "external_id")}
        >
          <Input id="plan-external-id" {...register("external_id")} />
        </Field>

        <Field label={`País${optionalHint}`} htmlFor="plan-country" error={fieldError(errorMap, "country")}>
          <select
            id="plan-country"
            {...register("country")}
            onChange={handleCountryChange}
            className="h-10 w-full rounded-md border border-input bg-input px-3 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <option value="">Selecciona un país</option>
            {SUPPORTED_COUNTRIES.map((c) => (
              <option key={c} value={c}>
                {c.toUpperCase()} ({COUNTRY_TO_CURRENCY[c]})
              </option>
            ))}
          </select>
        </Field>

        <Field label={`Moneda${optionalHint}`} htmlFor="plan-currency" error={fieldError(errorMap, "currency")}>
          <Input id="plan-currency" maxLength={3} {...register("currency")} />
        </Field>

        <Field label={`Monto${optionalHint}`} htmlFor="plan-amount" error={fieldError(errorMap, "amount")}>
          <Input id="plan-amount" type="number" step="any" {...register("amount")} />
        </Field>

        <Field
          label={`Frecuencia${optionalHint}`}
          htmlFor="plan-frequency"
          error={fieldError(errorMap, "frequency")}
        >
          <Input id="plan-frequency" type="number" step="1" {...register("frequency")} />
        </Field>

        <Field
          label={`Tipo de periodo${optionalHint}`}
          htmlFor="plan-period-type"
          error={fieldError(errorMap, "period_type")}
        >
          <select
            id="plan-period-type"
            {...register("period_type")}
            className="h-10 w-full rounded-md border border-input bg-input px-3 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <option value="">Selecciona un periodo</option>
            {PERIOD_TYPES.map((p) => (
              <option key={p} value={p}>
                {p}
              </option>
            ))}
          </select>
        </Field>

        {/* Campos aún no usados en la operación actual: ocultos en crear y
            editar. El schema aplica sus valores por defecto (max_retrys=3,
            max_user_subscription=1, status="active") al crear. */}
        {false ? (
          <>
            <Field
              label="Máximo de reintentos (opcional)"
              htmlFor="plan-max-retrys"
              error={fieldError(errorMap, "max_retrys")}
            >
              <Input id="plan-max-retrys" type="number" step="1" {...register("max_retrys")} />
            </Field>

            <Field
              label="Máx. suscripciones por usuario (opcional)"
              htmlFor="plan-max-user-sub"
              error={fieldError(errorMap, "max_user_subscription")}
            >
              <Input
                id="plan-max-user-sub"
                type="number"
                step="1"
                {...register("max_user_subscription")}
              />
            </Field>

            <Field label="Estado (opcional)" htmlFor="plan-status" error={fieldError(errorMap, "status")}>
              <Input id="plan-status" placeholder="active" {...register("status")} />
            </Field>
          </>
        ) : null}
      </div>

      {/* Flags booleanos (aún no usados): ocultos en crear y editar. */}
      {false ? (
        <div className="flex flex-wrap gap-6">
          <label className="flex items-center gap-2 text-sm text-foreground">
            <input type="checkbox" {...register("can_duplicate")} className="size-4" />
            Permite duplicar
          </label>
          <label className="flex items-center gap-2 text-sm text-foreground">
            <input type="checkbox" {...register("can_subscribe")} className="size-4" />
            Permite suscribirse
          </label>
        </div>
      ) : null}

      {/* Monto variable (aún no usado): oculto en crear y editar. */}
      {false ? (
      <fieldset className="space-y-4 rounded-md border border-border p-4">
        <label className="flex items-center gap-2 text-sm font-medium text-foreground">
          <input type="checkbox" {...register("variable_amount")} className="size-4" />
          Monto variable
        </label>
        {variableAmount ? (
          <div className="grid gap-4 sm:grid-cols-2">
            <Field
              label="Monto mínimo"
              htmlFor="plan-min-amount"
              error={fieldError(errorMap, "minimum_amount")}
            >
              <Input
                id="plan-min-amount"
                type="number"
                step="any"
                {...register("minimum_amount")}
              />
            </Field>
            <Field
              label="Monto máximo"
              htmlFor="plan-max-amount"
              error={fieldError(errorMap, "maximum_amount")}
            >
              <Input
                id="plan-max-amount"
                type="number"
                step="any"
                {...register("maximum_amount")}
              />
            </Field>
          </div>
        ) : null}
      </fieldset>
      ) : null}

      {/* Pareja frequency_test + test_type_period */}
      <fieldset className="space-y-4 rounded-md border border-border p-4">
        <legend className="px-1 text-sm font-medium text-foreground">
          Periodo de prueba (ambos o ninguno)
        </legend>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field
            label="Frecuencia de prueba"
            htmlFor="plan-freq-test"
            error={fieldError(errorMap, "frequency_test")}
          >
            <Input id="plan-freq-test" type="number" step="1" {...register("frequency_test")} />
          </Field>
          <Field
            label="Tipo de periodo de prueba"
            htmlFor="plan-test-type-period"
            error={fieldError(errorMap, "test_type_period")}
          >
            <select
              id="plan-test-type-period"
              {...register("test_type_period")}
              className="h-10 w-full rounded-md border border-input bg-input px-3 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              <option value="">Ninguno</option>
              {PERIOD_TYPES.map((p) => (
                <option key={p} value={p}>
                  {p}
                </option>
              ))}
            </select>
          </Field>
        </div>
      </fieldset>

      {/* Trío oneshot */}
      <fieldset className="space-y-4 rounded-md border border-border p-4">
        <legend className="px-1 text-sm font-medium text-foreground">
          Cargo único / oneshot (grupo completo o ninguno)
        </legend>
        <div className="grid gap-4 sm:grid-cols-3">
          <Field
            label="Monto oneshot"
            htmlFor="plan-oneshot-amount"
            error={fieldError(errorMap, "oneshot_amount")}
          >
            <Input
              id="plan-oneshot-amount"
              type="number"
              step="any"
              {...register("oneshot_amount")}
            />
          </Field>
          <Field
            label="Frecuencia oneshot"
            htmlFor="plan-oneshot-frequency"
            error={fieldError(errorMap, "oneshot_frequency")}
          >
            <Input
              id="plan-oneshot-frequency"
              type="number"
              step="1"
              {...register("oneshot_frequency")}
            />
          </Field>
          <Field
            label="Periodo oneshot"
            htmlFor="plan-oneshot-period"
            error={fieldError(errorMap, "oneshot_period")}
          >
            <select
              id="plan-oneshot-period"
              {...register("oneshot_period")}
              className="h-10 w-full rounded-md border border-input bg-input px-3 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              <option value="">Ninguno</option>
              {PERIOD_TYPES.map((p) => (
                <option key={p} value={p}>
                  {p}
                </option>
              ))}
            </select>
          </Field>
        </div>
      </fieldset>

      <div className="flex justify-end gap-2">
        <Button type="button" variant="outline" onClick={onCancel} disabled={isSubmitting}>
          Cancelar
        </Button>
        <Button type="submit" disabled={isSubmitting}>
          {isSubmitting ? (
            <>
              <Loader2 className="size-4 animate-spin" aria-hidden="true" />
              Guardando...
            </>
          ) : mode === "create" ? (
            "Crear plan"
          ) : (
            "Guardar cambios"
          )}
        </Button>
      </div>
    </form>
  );
}

/** Campo de formulario con etiqueta y mensaje de error. */
function Field({
  label,
  htmlFor,
  error,
  children,
}: {
  label: string;
  htmlFor: string;
  error?: string;
  children: React.ReactNode;
}) {
  return (
    <div className="space-y-2">
      <Label htmlFor={htmlFor}>{label}</Label>
      {children}
      {error ? (
        <p className="text-sm text-destructive" role="alert">
          {error}
        </p>
      ) : null}
    </div>
  );
}

export default PlanForm;
