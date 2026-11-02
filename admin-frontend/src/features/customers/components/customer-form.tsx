"use client";

/**
 * Formulario de Customer — crear y editar (tarea 16.1, Req. 7 y 8).
 *
 * Es un único componente parametrizado por `mode`:
 * - `mode="create"`: presenta todos los campos (external_id, first_name,
 *   last_name, email, phone, birth_date, external_created y los opcionales
 *   second_last_name, gender, address, device_fingerprint) y valida con
 *   `customerCreateSchema` (Req. 7.1-7.9).
 * - `mode="edit"`: presenta solo los campos editables (first_name, last_name,
 *   phone, birth_date, gender, address, device_fingerprint) y muestra
 *   `external_id` y `email` DESHABILITADOS/solo lectura, ya que son inmutables
 *   (Req. 8.1, 8.2); valida con `customerEditSchema`. `second_last_name` NO
 *   aparece en edición: el API solo lo acepta al crear.
 *
 * El envío real (crear/editar) y el feedback de éxito/error los maneja el
 * contenedor (las páginas de ruta); aquí solo se validan y se recogen los datos.
 * Construido con React Hook Form + zodResolver y las primitivas existentes
 * (Button/Card/Input/Label) para no introducir primitivas compartidas nuevas.
 */
import { Controller, useForm, type Resolver } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { Loader2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { DateField } from "@/components/ui/date-field";
import { GENDERS, maxBirthDate } from "@/lib/validation";
import {
  customerCreateSchema,
  customerEditSchema,
  type CustomerCreateInput,
  type CustomerEditInput,
} from "@/features/customers/schema";

/** Valores por defecto de un formulario de creación. */
export type CustomerFormCreateValues = CustomerCreateInput;
/** Valores por defecto de un formulario de edición (campos editables). */
export type CustomerFormEditValues = CustomerEditInput;

interface CommonProps {
  /** Se invoca cuando el formulario es válido y se envía. */
  readonly isSubmitting?: boolean;
  /** Texto del botón de envío. */
  readonly submitLabel?: string;
  /** Se invoca al cancelar (p. ej. volver al listado). */
  readonly onCancel?: () => void;
}

interface CreateProps extends CommonProps {
  readonly mode: "create";
  readonly defaultValues?: Partial<CustomerFormCreateValues>;
  readonly onSubmit: (values: CustomerFormCreateValues) => void;
}

interface EditProps extends CommonProps {
  readonly mode: "edit";
  /**
   * Identificadores inmutables mostrados deshabilitados (Req. 8.2). No forman
   * parte del envío; se usan solo para localizar al cliente aguas arriba.
   */
  readonly immutable: { readonly external_id?: string; readonly email?: string };
  readonly defaultValues?: Partial<CustomerFormEditValues>;
  readonly onSubmit: (values: CustomerFormEditValues) => void;
}

export type CustomerFormProps = CreateProps | EditProps;

/** Campo de texto con etiqueta, mensaje de error y marcado de accesibilidad. */
function Field({
  id,
  label,
  error,
  children,
  hint,
}: {
  readonly id: string;
  readonly label: string;
  readonly error?: string;
  readonly children: React.ReactNode;
  readonly hint?: string;
}) {
  return (
    <div className="space-y-2">
      <Label htmlFor={id}>{label}</Label>
      {children}
      {hint ? <p className="text-xs text-muted-foreground">{hint}</p> : null}
      {error ? (
        <p className="text-sm text-destructive" role="alert">
          {error}
        </p>
      ) : null}
    </div>
  );
}

export function CustomerForm(props: CustomerFormProps) {
  if (props.mode === "create") {
    return <CustomerCreateForm {...props} />;
  }
  return <CustomerEditForm {...props} />;
}

function CustomerCreateForm({
  defaultValues,
  onSubmit,
  onCancel,
  isSubmitting = false,
  submitLabel = "Crear cliente",
}: CreateProps) {
  const {
    register,
    control,
    handleSubmit,
    formState: { errors },
  } = useForm<CustomerFormCreateValues>({
    resolver: zodResolver(
      customerCreateSchema,
    ) as Resolver<CustomerFormCreateValues>,
    defaultValues: {
      external_id: "",
      first_name: "",
      last_name: "",
      email: "",
      phone: "",
      birth_date: "",
      external_created: "",
      second_last_name: "",
      address: "",
      device_fingerprint: "",
      ...defaultValues,
    },
  });

  return (
    <form
      noValidate
      onSubmit={handleSubmit(onSubmit)}
      className="grid gap-4 sm:grid-cols-2"
    >
      <Field id="external_id" label="External ID" error={errors.external_id?.message}>
        <Input id="external_id" disabled={isSubmitting} {...register("external_id")} />
      </Field>
      <Field id="email" label="Email" error={errors.email?.message}>
        <Input id="email" type="email" disabled={isSubmitting} {...register("email")} />
      </Field>
      <Field id="first_name" label="Nombre" error={errors.first_name?.message}>
        <Input id="first_name" disabled={isSubmitting} {...register("first_name")} />
      </Field>
      <Field id="last_name" label="Apellido" error={errors.last_name?.message}>
        <Input id="last_name" disabled={isSubmitting} {...register("last_name")} />
      </Field>
      <Field
        id="second_last_name"
        label="Segundo apellido (opcional)"
        error={errors.second_last_name?.message}
      >
        <Input
          id="second_last_name"
          disabled={isSubmitting}
          {...register("second_last_name")}
        />
      </Field>
      <Field
        id="phone"
        label="Teléfono"
        error={errors.phone?.message}
        hint="Formato: '+' seguido de 8 a 15 dígitos (p. ej. +50255551234)."
      >
        <Input id="phone" disabled={isSubmitting} {...register("phone")} />
      </Field>
      <Field
        id="birth_date"
        label="Fecha de nacimiento"
        error={errors.birth_date?.message}
        hint="Selecciona la fecha; debe ser mayor de 18 años."
      >
        <Controller
          control={control}
          name="birth_date"
          render={({ field }) => (
            <DateField
              id="birth_date"
              mode="date"
              value={field.value ?? ""}
              onChange={field.onChange}
              onBlur={field.onBlur}
              disabled={isSubmitting}
              max={maxBirthDate()}
              aria-invalid={errors.birth_date ? "true" : undefined}
            />
          )}
        />
      </Field>
      <Field
        id="external_created"
        label="Fecha de creación externa"
        error={errors.external_created?.message}
        hint="Selecciona el día; la hora se fija al momento de seleccionarlo (UTC)."
      >
        <Controller
          control={control}
          name="external_created"
          render={({ field }) => (
            <DateField
              id="external_created"
              mode="datetime"
              value={field.value ?? ""}
              onChange={field.onChange}
              onBlur={field.onBlur}
              disabled={isSubmitting}
              aria-invalid={errors.external_created ? "true" : undefined}
            />
          )}
        />
      </Field>
      <Field id="gender" label="Género (opcional)" error={errors.gender?.message}>
        <select
          id="gender"
          disabled={isSubmitting}
          className="flex h-10 w-full rounded-md border border-input-border bg-input px-3 py-2 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
          defaultValue=""
          {...register("gender")}
        >
          <option value="">Sin especificar</option>
          {GENDERS.map((g) => (
            <option key={g} value={g}>
              {g}
            </option>
          ))}
        </select>
      </Field>
      <Field id="address" label="Dirección (opcional)" error={errors.address?.message}>
        <Input id="address" disabled={isSubmitting} {...register("address")} />
      </Field>
      <Field
        id="device_fingerprint"
        label="Device fingerprint (opcional)"
        error={errors.device_fingerprint?.message}
      >
        <Input
          id="device_fingerprint"
          disabled={isSubmitting}
          {...register("device_fingerprint")}
        />
      </Field>

      <FormActions
        isSubmitting={isSubmitting}
        submitLabel={submitLabel}
        onCancel={onCancel}
      />
    </form>
  );
}

function CustomerEditForm({
  immutable,
  defaultValues,
  onSubmit,
  onCancel,
  isSubmitting = false,
  submitLabel = "Guardar cambios",
}: EditProps) {
  const {
    register,
    control,
    handleSubmit,
    formState: { errors },
  } = useForm<CustomerFormEditValues>({
    resolver: zodResolver(
      customerEditSchema,
    ) as Resolver<CustomerFormEditValues>,
    defaultValues: {
      first_name: "",
      last_name: "",
      phone: "",
      birth_date: "",
      address: "",
      device_fingerprint: "",
      ...defaultValues,
    },
  });

  return (
    <form
      noValidate
      onSubmit={handleSubmit(onSubmit)}
      className="grid gap-4 sm:grid-cols-2"
    >
      {/* Campos inmutables (Req. 8.2): visibles pero deshabilitados. */}
      <Field id="external_id" label="External ID (no editable)">
        <Input
          id="external_id"
          value={immutable.external_id ?? ""}
          disabled
          readOnly
        />
      </Field>
      <Field id="email" label="Email (no editable)">
        <Input id="email" value={immutable.email ?? ""} disabled readOnly />
      </Field>

      <Field id="first_name" label="Nombre" error={errors.first_name?.message}>
        <Input id="first_name" disabled={isSubmitting} {...register("first_name")} />
      </Field>
      <Field id="last_name" label="Apellido" error={errors.last_name?.message}>
        <Input id="last_name" disabled={isSubmitting} {...register("last_name")} />
      </Field>
      <Field
        id="phone"
        label="Teléfono (opcional)"
        error={errors.phone?.message}
        hint="Formato: '+' seguido de 8 a 15 dígitos."
      >
        <Input id="phone" disabled={isSubmitting} {...register("phone")} />
      </Field>
      <Field
        id="birth_date"
        label="Fecha de nacimiento (opcional)"
        error={errors.birth_date?.message}
        hint="Selecciona la fecha; debe ser mayor de 18 años."
      >
        <Controller
          control={control}
          name="birth_date"
          render={({ field }) => (
            <DateField
              id="birth_date"
              mode="date"
              value={field.value ?? ""}
              onChange={field.onChange}
              onBlur={field.onBlur}
              disabled={isSubmitting}
              max={maxBirthDate()}
              aria-invalid={errors.birth_date ? "true" : undefined}
            />
          )}
        />
      </Field>
      <Field id="gender" label="Género (opcional)" error={errors.gender?.message}>
        <select
          id="gender"
          disabled={isSubmitting}
          className="flex h-10 w-full rounded-md border border-input-border bg-input px-3 py-2 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
          defaultValue=""
          {...register("gender")}
        >
          <option value="">Sin especificar</option>
          {GENDERS.map((g) => (
            <option key={g} value={g}>
              {g}
            </option>
          ))}
        </select>
      </Field>
      <Field id="address" label="Dirección (opcional)" error={errors.address?.message}>
        <Input id="address" disabled={isSubmitting} {...register("address")} />
      </Field>
      <Field
        id="device_fingerprint"
        label="Device fingerprint (opcional)"
        error={errors.device_fingerprint?.message}
      >
        <Input
          id="device_fingerprint"
          disabled={isSubmitting}
          {...register("device_fingerprint")}
        />
      </Field>

      <FormActions
        isSubmitting={isSubmitting}
        submitLabel={submitLabel}
        onCancel={onCancel}
      />
    </form>
  );
}

/** Botonera de envío/cancelación compartida por ambos modos. */
function FormActions({
  isSubmitting,
  submitLabel,
  onCancel,
}: {
  readonly isSubmitting: boolean;
  readonly submitLabel: string;
  readonly onCancel?: () => void;
}) {
  return (
    <div className="flex items-center justify-end gap-2 sm:col-span-2">
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
        {isSubmitting ? (
          <>
            <Loader2 className="size-4 animate-spin" aria-hidden="true" />
            Guardando...
          </>
        ) : (
          submitLabel
        )}
      </Button>
    </div>
  );
}

export default CustomerForm;
