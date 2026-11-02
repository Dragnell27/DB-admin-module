"use client";

/**
 * Formularios de Subscription: crear y cambiar (Req. 14, 15) con selectores de
 * búsqueda y precarga.
 *
 * Mejoras de UX (petición del usuario):
 * - Plan: selector por NOMBRE (SearchSelect) poblado con `usePlans`; se guarda
 *   el `plan_id`.
 * - Cliente: en CREAR es seleccionable por nombre (`useCustomers`). En CAMBIAR
 *   queda FIJO (solo lectura) mediante `lockedCustomerId`: se muestra el nombre
 *   del cliente y no puede modificarse (aunque el backend lo permita).
 * - Método de pago: selector de TARJETA del cliente (`useCards`); se guarda el
 *   `card_token` en `payment_method_token` y se DERIVA `payment_method` del
 *   `payment_method_name`/`card_franchise` de la tarjeta.
 * - Precarga: en CAMBIAR, el contenedor pasa `defaultValues` con el detalle de
 *   la suscripción; el formulario se rellena (incluye reset al llegar async).
 *
 * Validación con RHF + zodResolver (schemas create/change). El envío real y el
 * feedback los maneja el contenedor (`page.tsx`).
 */
import * as React from "react";
import { Controller, useForm, type Resolver } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";

import { Button } from "@/components/ui/button";
import { DateField } from "@/components/ui/date-field";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { SearchSelect, type SearchSelectOption } from "@/components/ui/search-select";
import { cn } from "@/lib/utils";
import { usePlans, planId } from "@/features/plans/hooks";
import { useCustomers, useCustomer } from "@/features/customers/hooks";
import { readableCustomer } from "@/features/customers/readable-customer";
import { useCards } from "@/features/cards/hooks";
import {
  subscriptionChangeSchema,
  subscriptionCreateSchema,
  type SubscriptionChangeInput,
  type SubscriptionCreateInput,
} from "@/features/subscriptions/schema";

/** Modo del formulario: crear una suscripción o cambiarla. */
export type SubscriptionFormMode = "create" | "change";

/** Unión de los payloads producidos por el formulario según el modo. */
export type SubscriptionFormValues =
  | SubscriptionCreateInput
  | SubscriptionChangeInput;

/** Valores de precarga (todos string para el DOM). */
export interface SubscriptionFormDefaults {
  plan_id?: string;
  customer_id?: string;
  payment_method?: string;
  payment_method_token?: string;
  amount?: string;
  start_date?: string;
}

export interface SubscriptionFormProps {
  readonly mode: SubscriptionFormMode;
  /** Precarga de campos (p. ej. al cambiar una suscripción existente). */
  readonly defaultValues?: SubscriptionFormDefaults;
  /**
   * Si viene, el cliente queda FIJO (solo lectura) con este `customer_id`: no
   * se muestra el selector de cliente y el payload usa este valor. Se emplea en
   * el modo "change" para impedir cambiar el cliente desde el front.
   */
  readonly lockedCustomerId?: string;
  /**
   * `subscription_id` de la suscripción a cambiar. El OTT lo exige en el cuerpo
   * del cambio; se inyecta en el payload en modo "change".
   */
  readonly subscriptionId?: string;
  readonly onSubmit: (values: SubscriptionFormValues) => void;
  readonly onCancel?: () => void;
  readonly isSubmitting?: boolean;
  readonly className?: string;
}

interface SubscriptionFormFields {
  subscription_id: string;
  plan_id: string;
  customer_id: string;
  payment_method: string;
  payment_method_token: string;
  amount: string;
  start_date: string;
}

function mergeDefaults(d?: SubscriptionFormDefaults): SubscriptionFormFields {
  return {
    subscription_id: "",
    plan_id: d?.plan_id ?? "",
    customer_id: d?.customer_id ?? "",
    payment_method: d?.payment_method ?? "",
    payment_method_token: d?.payment_method_token ?? "",
    amount: d?.amount ?? "",
    start_date: d?.start_date ?? "",
  };
}

/** Recorta un valor a string de forma segura (acepta no-string sin romper). */
function asStr(value: unknown): string {
  if (typeof value === "string") {
    return value.trim();
  }
  if (value == null) {
    return "";
  }
  return String(value).trim();
}

/** "" → undefined; recorta espacios. Tolerante a valores ya transformados. */
function trimOptional(value: unknown): string | undefined {
  const trimmed = asStr(value);
  return trimmed === "" ? undefined : trimmed;
}

/**
 * Convierte el valor de amount a número. Tolera que ya venga como number
 * (p. ej. cuando el resolver entregó valores ya coaccionados): en ese caso lo
 * devuelve tal cual. "" / undefined → undefined.
 */
function parseAmount(value: unknown): number | undefined {
  if (typeof value === "number") {
    return Number.isNaN(value) ? undefined : value;
  }
  const trimmed = asStr(value);
  if (trimmed === "") {
    return undefined;
  }
  return Number(trimmed);
}

function toCreateInput(
  fields: Partial<SubscriptionFormFields>,
): SubscriptionCreateInput {
  return {
    plan_id: asStr(fields.plan_id),
    customer_id: asStr(fields.customer_id),
    payment_method: asStr(fields.payment_method),
    payment_method_token: asStr(fields.payment_method_token),
    amount: parseAmount(fields.amount),
    start_date: trimOptional(fields.start_date),
  } as SubscriptionCreateInput;
}

function toChangeInput(
  fields: Partial<SubscriptionFormFields>,
): SubscriptionChangeInput {
  // En el cambio NO se envía start_date (la fecha de inicio no es editable).
  return {
    subscription_id: trimOptional(fields.subscription_id),
    plan_id: trimOptional(fields.plan_id),
    customer_id: trimOptional(fields.customer_id),
    payment_method_token: trimOptional(fields.payment_method_token),
    amount: parseAmount(fields.amount),
  } as SubscriptionChangeInput;
}

function buildResolver(
  mode: SubscriptionFormMode,
): Resolver<SubscriptionFormFields> {
  const schema =
    mode === "create" ? subscriptionCreateSchema : subscriptionChangeSchema;
  // Los dos schemas tienen formas distintas (unión); `zodResolver` no infiere
  // sobre la unión. Se castea a un resolver genérico (el payload se valida en
  // runtime igual).
  const zodValidate = zodResolver(
    schema as unknown as Parameters<typeof zodResolver>[0],
  );
  return async (values, context, options) => {
    const payload =
      mode === "create"
        ? toCreateInput(values as SubscriptionFormFields)
        : toChangeInput(values as SubscriptionFormFields);
    const result = await (
      zodValidate as unknown as Resolver<Record<string, unknown>>
    )(payload as Record<string, unknown>, context, {
      ...options,
      names: options?.names,
    });
    return result as Awaited<ReturnType<Resolver<SubscriptionFormFields>>>;
  };
}

export function SubscriptionForm({
  mode,
  defaultValues,
  lockedCustomerId,
  subscriptionId,
  onSubmit,
  onCancel,
  isSubmitting = false,
  className,
}: SubscriptionFormProps) {
  const resolver = React.useMemo(() => buildResolver(mode), [mode]);
  const {
    register,
    control,
    handleSubmit,
    reset,
    setValue,
    watch,
    formState: { errors },
  } = useForm<SubscriptionFormFields>({
    resolver,
    defaultValues: mergeDefaults(defaultValues),
    mode: "onSubmit",
  });

  const isCreate = mode === "create";

  // Precarga asíncrona: cuando llegan/cambian los defaults (p. ej. el detalle de
  // la suscripción en editar), reflejarlos en el formulario.
  const defaultsKey = JSON.stringify(defaultValues ?? {});
  React.useEffect(() => {
    reset(mergeDefaults(defaultValues));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [defaultsKey, reset]);

  // Cliente efectivo: fijo (locked) en editar, o el elegido en el formulario.
  const selectedCustomerId = watch("customer_id");
  const effectiveCustomerId = lockedCustomerId ?? selectedCustomerId;

  // --- Opciones de PLAN (selector por nombre) ---
  const plansQuery = usePlans({ page: 1, recordsPerPage: 100 });
  const planOptions: SearchSelectOption[] = React.useMemo(() => {
    const items = plansQuery.data?.items ?? [];
    return items.map((p): SearchSelectOption => {
      const id = planId(p);
      const country = typeof p.country === "string" ? p.country : undefined;
      const currency = typeof p.currency === "string" ? p.currency : undefined;
      const amount = typeof p.amount === "number" ? String(p.amount) : undefined;
      const hintParts = [country, currency, amount].filter(Boolean).join(" ");
      const name = typeof p.name === "string" && p.name !== "" ? p.name : id;
      return { value: id, label: name, hint: hintParts || undefined };
    });
  }, [plansQuery.data]);

  // Mapa `plan_id -> amount` para autollenar el monto al elegir un plan.
  const planAmountById = React.useMemo(() => {
    const m = new Map<string, number>();
    for (const p of plansQuery.data?.items ?? []) {
      const id = planId(p);
      if (id && typeof p.amount === "number") {
        m.set(id, p.amount);
      }
    }
    return m;
  }, [plansQuery.data]);

  // Al elegir un plan: fija el plan_id y autollena `amount` con el monto del
  // plan. El campo de monto queda editable para ajustarlo si se requiere.
  const applyPlan = React.useCallback(
    (newPlanId: string) => {
      setValue("plan_id", newPlanId, { shouldValidate: false });
      const planAmount = planAmountById.get(newPlanId);
      if (planAmount != null) {
        setValue("amount", String(planAmount), { shouldValidate: false });
      }
    },
    [planAmountById, setValue],
  );

  // --- Opciones de CLIENTE (solo en crear) ---
  const customersQuery = useCustomers(
    { page: 1, recordsPerPage: 100 },
    {},
    { enabled: isCreate && !lockedCustomerId },
  );
  const customerOptions: SearchSelectOption[] = React.useMemo(() => {
    const items = customersQuery.data?.items ?? [];
    return items.map((c): SearchSelectOption => ({
      value: typeof c.customer_id === "string" ? c.customer_id : "",
      label: readableCustomer(c) ?? (typeof c.customer_id === "string" ? c.customer_id : ""),
      hint: typeof c.email === "string" ? c.email : undefined,
    }));
  }, [customersQuery.data]);

  // Nombre del cliente fijo (editar): se resuelve por customer_id.
  const lockedCustomerQuery = useCustomer(
    { customer_id: lockedCustomerId },
    { enabled: Boolean(lockedCustomerId) },
  );
  const lockedCustomerLabel =
    readableCustomer(lockedCustomerQuery.data) ?? lockedCustomerId ?? "";

  // --- Opciones de TARJETA del cliente efectivo ---
  const cardsQuery = useCards(
    { customer_id: effectiveCustomerId },
    {},
    { enabled: Boolean(effectiveCustomerId) },
  );
  const cardItems = React.useMemo(
    () => cardsQuery.data?.items ?? [],
    [cardsQuery.data],
  );
  const cardByToken = React.useMemo(() => {
    const m = new Map<string, { payment_method_name?: string; card_franchise?: string }>();
    for (const card of cardItems) {
      const token = typeof card.card_token === "string" ? card.card_token : "";
      if (token) {
        m.set(token, {
          payment_method_name:
            typeof card.payment_method_name === "string" ? card.payment_method_name : undefined,
          card_franchise:
            typeof card.card_franchise === "string" ? card.card_franchise : undefined,
        });
      }
    }
    return m;
  }, [cardItems]);
  const cardOptions: SearchSelectOption[] = React.useMemo(() => {
    const out: SearchSelectOption[] = [];
    for (const card of cardItems) {
      const token = typeof card.card_token === "string" ? card.card_token : "";
      if (!token) {
        continue;
      }
      const name =
        (typeof card.payment_method_name === "string" && card.payment_method_name) ||
        (typeof card.card_franchise === "string" && card.card_franchise) ||
        "Tarjeta";
      const last4 =
        typeof card.card_number === "string" ? card.card_number.slice(-4) : "";
      out.push({
        value: token,
        label: last4 ? `${name} ···· ${last4}` : name,
        hint: typeof card.expiration_date === "string" ? card.expiration_date : undefined,
      });
    }
    return out;
  }, [cardItems]);

  // Token actualmente seleccionado/precargado (p. ej. el de la suscripción al
  // editar). Puede NO existir entre las tarjetas vigentes del cliente (tarjeta
  // cambiada/eliminada); en ese caso añadimos una opción sintética para que el
  // selector no se vea vacío y el usuario pueda conservarla o cambiarla.
  const currentToken = watch("payment_method_token");
  const cardOptionsWithCurrent: SearchSelectOption[] = React.useMemo(() => {
    if (!currentToken) {
      return cardOptions;
    }
    const exists = cardOptions.some((o) => o.value === currentToken);
    if (exists) {
      return cardOptions;
    }
    const short = currentToken.slice(0, 8);
    return [
      {
        value: currentToken,
        label: `Tarjeta actual (token ${short}…)`,
        hint: "No está en la lista vigente del cliente",
      },
      ...cardOptions,
    ];
  }, [cardOptions, currentToken]);

  // Al elegir una tarjeta, derivar payment_method del nombre/franquicia.
  const applyCardToken = React.useCallback(
    (token: string) => {
      setValue("payment_method_token", token, { shouldValidate: false });
      const card = cardByToken.get(token);
      const derived = card?.payment_method_name || card?.card_franchise || "card";
      setValue("payment_method", derived, { shouldValidate: false });
    },
    [cardByToken, setValue],
  );

  const paymentMethod = watch("payment_method");

  const submit = handleSubmit((fields) => {
    // `fields` llega ya coaccionado por el resolver (p. ej. amount: number);
    // los transformadores son tolerantes a ello. Se fuerza el cliente fijo en
    // editar para que no pueda cambiarse desde el front.
    const merged: Partial<SubscriptionFormFields> = {
      ...(fields as Partial<SubscriptionFormFields>),
      customer_id: lockedCustomerId ?? fields.customer_id,
      // El OTT exige subscription_id en el cuerpo del cambio.
      subscription_id: subscriptionId ?? fields.subscription_id,
    };
    const payload = isCreate ? toCreateInput(merged) : toChangeInput(merged);
    onSubmit(payload);
  });

  return (
    <form onSubmit={submit} className={cn("space-y-4", className)} noValidate>
      {/* Plan: selector por nombre */}
      <Field
        id="plan_id"
        label="Plan"
        hint="Busca y selecciona el plan por nombre."
        required={isCreate}
        error={errors.plan_id?.message}
      >
        <Controller
          control={control}
          name="plan_id"
          render={({ field }) => (
            <SearchSelect
              id="plan_id"
              value={field.value ?? ""}
              onChange={(v) => {
                field.onChange(v);
                // Autollena el monto con el del plan (editable después).
                applyPlan(v);
              }}
              onBlur={field.onBlur}
              options={planOptions}
              loading={plansQuery.isLoading}
              placeholder="Buscar plan por nombre..."
              disabled={isSubmitting}
              aria-invalid={Boolean(errors.plan_id)}
            />
          )}
        />
      </Field>

      {/* Cliente: seleccionable en crear; fijo (solo lectura) en editar */}
      {lockedCustomerId ? (
        <Field id="customer_id" label="Cliente">
          <Input
            id="customer_id"
            value={lockedCustomerLabel}
            readOnly
            disabled
          />
          <input type="hidden" {...register("customer_id")} />
        </Field>
      ) : (
        <Field
          id="customer_id"
          label="Cliente"
          hint="Busca y selecciona el cliente por nombre."
          required={isCreate}
          error={errors.customer_id?.message}
        >
          <Controller
            control={control}
            name="customer_id"
            render={({ field }) => (
              <SearchSelect
                id="customer_id"
                value={field.value ?? ""}
                onChange={(v) => {
                  field.onChange(v);
                  // Cambiar de cliente invalida la tarjeta elegida.
                  setValue("payment_method_token", "", { shouldValidate: false });
                  setValue("payment_method", "", { shouldValidate: false });
                }}
                onBlur={field.onBlur}
                options={customerOptions}
                loading={customersQuery.isLoading}
                placeholder="Buscar cliente por nombre..."
                disabled={isSubmitting}
                aria-invalid={Boolean(errors.customer_id)}
              />
            )}
          />
        </Field>
      )}

      {/* Método de pago: tarjeta del cliente (deriva token + payment_method) */}
      <Field
        id="payment_method_token"
        label="Método de pago (tarjeta)"
        hint={
          effectiveCustomerId
            ? "Selecciona una tarjeta del cliente."
            : "Selecciona primero un cliente."
        }
        required={isCreate}
        error={errors.payment_method_token?.message}
      >
        <Controller
          control={control}
          name="payment_method_token"
          render={({ field }) => (
            <SearchSelect
              id="payment_method_token"
              value={field.value ?? ""}
              onChange={applyCardToken}
              onBlur={field.onBlur}
              options={cardOptionsWithCurrent}
              loading={Boolean(effectiveCustomerId) && cardsQuery.isLoading}
              placeholder="Seleccionar tarjeta..."
              emptyText="El cliente no tiene tarjetas"
              disabled={isSubmitting || !effectiveCustomerId}
              aria-invalid={Boolean(errors.payment_method_token)}
            />
          )}
        />
        {paymentMethod ? (
          <p className="text-xs text-muted-foreground">
            Método derivado: <span className="font-medium">{paymentMethod}</span>
          </p>
        ) : null}
      </Field>

      <Field
        id="amount"
        label="Monto"
        hint="Se completa con el monto del plan al seleccionarlo; puedes editarlo si lo requieres."
        error={errors.amount?.message}
      >
        <Input
          id="amount"
          type="number"
          step="any"
          min="0"
          placeholder="Opcional"
          autoComplete="off"
          disabled={isSubmitting}
          aria-invalid={Boolean(errors.amount)}
          {...register("amount")}
        />
      </Field>

      {/* La fecha de inicio solo se captura al CREAR; en cambiar no es editable. */}
      {isCreate ? (
        <Field
          id="start_date"
          label="Fecha de inicio"
          hint="Opcional. Selecciona el día; la hora se completa con el momento de la selección (UTC)."
          error={errors.start_date?.message}
        >
          <Controller
            control={control}
            name="start_date"
            render={({ field }) => (
              <DateField
                id="start_date"
                mode="datetime"
                value={field.value ?? ""}
                onChange={field.onChange}
                onBlur={field.onBlur}
                disabled={isSubmitting}
                aria-invalid={Boolean(errors.start_date)}
              />
            )}
          />
        </Field>
      ) : null}

      <div className="flex items-center justify-end gap-2 pt-2">
        {onCancel ? (
          <Button
            type="button"
            variant="outline"
            onClick={onCancel}
            disabled={isSubmitting}
          >
            Cancelar
          </Button>
        ) : null}
        <Button type="submit" disabled={isSubmitting}>
          {isSubmitting
            ? "Guardando..."
            : isCreate
              ? "Crear suscripción"
              : "Guardar cambios"}
        </Button>
      </div>
    </form>
  );
}

function Field({
  id,
  label,
  hint,
  required,
  error,
  children,
}: {
  id: string;
  label: string;
  hint?: string;
  required?: boolean;
  error?: string;
  children: React.ReactNode;
}) {
  return (
    <div className="space-y-1.5">
      <Label htmlFor={id}>
        {label}
        {required ? <span className="ml-0.5 text-destructive">*</span> : null}
      </Label>
      {children}
      {hint && !error ? (
        <p className="text-xs text-muted-foreground">{hint}</p>
      ) : null}
      {error ? (
        <p className="text-xs text-destructive" role="alert">
          {error}
        </p>
      ) : null}
    </div>
  );
}

export default SubscriptionForm;
