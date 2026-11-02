"use client";

/**
 * Formulario de creación de Card (tarea 16.4; Req. 18.1-18.10).
 *
 * Presenta los campos customer_id, card_number, expiration_date (MM/YY),
 * card_token, payment_method_name y card_franchise, validados con
 * `cardCreateSchema` vía `zodResolver` para que las reglas coincidan con la
 * validación espejo del BFF (Req. 18.2-18.7). El envío se delega al hook
 * `useCreateCard` (Req. 18.8). El éxito/fallo se comunica al contenedor por
 * callbacks, que muestra el banner correspondiente (Req. 18.9, 18.10).
 *
 * Solo se renderiza para el rol `admin` (la vista aplica `useRole`); la
 * seguridad real la impone el BFF.
 */
import * as React from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { Loader2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { isApiError } from "@/lib/api-client";
import { useCreateCard } from "@/features/cards/hooks";
import {
  cardCreateSchema,
  type CardCreateInput,
} from "@/features/cards/schema";

/** Campos del formulario, en orden de presentación (Req. 18.1). */
const FIELDS: ReadonlyArray<{
  readonly name: keyof CardCreateInput;
  readonly label: string;
  readonly placeholder: string;
  readonly autoComplete?: string;
}> = [
  {
    name: "customer_id",
    label: "ID de cliente",
    placeholder: "UUID del cliente",
  },
  {
    name: "card_number",
    label: "Número de tarjeta",
    placeholder: "4111 1111 1111 1111",
  },
  {
    name: "expiration_date",
    label: "Expiración (MM/YY)",
    placeholder: "12/30",
  },
  {
    name: "card_token",
    label: "Token de tarjeta",
    placeholder: "UUID del token",
  },
  {
    name: "payment_method_name",
    label: "Método de pago",
    placeholder: "Visa débito",
  },
  {
    name: "card_franchise",
    label: "Franquicia",
    placeholder: "VISA",
  },
];

export interface CardFormProps {
  /**
   * `customer_id` del cliente seleccionado en la vista. Se usa como valor
   * inicial para no reescribirlo en cada alta.
   */
  readonly defaultCustomerId?: string;
  /** Se invoca tras una creación exitosa (Req. 18.9). */
  readonly onSuccess?: (createdCustomerId: string) => void;
  /** Se invoca al cancelar/cerrar el formulario. */
  readonly onCancel?: () => void;
}

export function CardForm({
  defaultCustomerId = "",
  onSuccess,
  onCancel,
}: CardFormProps) {
  const [submitError, setSubmitError] = React.useState<string | null>(null);

  const {
    register,
    handleSubmit,
    reset,
    formState: { errors },
  } = useForm<CardCreateInput>({
    resolver: zodResolver(cardCreateSchema),
    defaultValues: {
      customer_id: defaultCustomerId,
      card_number: "",
      expiration_date: "",
      card_token: "",
      payment_method_name: "",
      card_franchise: "",
    },
  });

  const createCard = useCreateCard({
    onSuccess: (_data, variables) => {
      setSubmitError(null);
      reset({
        customer_id: variables.customer_id,
        card_number: "",
        expiration_date: "",
        card_token: "",
        payment_method_name: "",
        card_franchise: "",
      });
      onSuccess?.(variables.customer_id);
    },
    onError: (error) => {
      // Mensaje del backend OTT cuando esté disponible (Req. 18.10).
      setSubmitError(
        isApiError(error)
          ? error.message
          : "No se pudo crear la tarjeta. Intenta de nuevo.",
      );
    },
  });

  const onSubmit = handleSubmit((values) => {
    setSubmitError(null);
    createCard.mutate(values);
  });

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-lg">Nueva tarjeta</CardTitle>
        <CardDescription>
          Registra un medio de pago para el cliente seleccionado.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={onSubmit} noValidate className="space-y-4">
          {submitError ? (
            <div
              role="alert"
              className="rounded-md border border-destructive/50 bg-destructive/10 px-3 py-2 text-sm text-destructive"
            >
              {submitError}
            </div>
          ) : null}

          <div className="grid gap-4 sm:grid-cols-2">
            {FIELDS.map((field) => {
              const fieldError = errors[field.name];
              const describedBy = fieldError
                ? `${field.name}-error`
                : undefined;
              return (
                <div key={field.name} className="space-y-1.5">
                  <Label htmlFor={field.name}>{field.label}</Label>
                  <Input
                    id={field.name}
                    placeholder={field.placeholder}
                    aria-invalid={fieldError ? true : undefined}
                    aria-describedby={describedBy}
                    {...register(field.name)}
                  />
                  {fieldError ? (
                    <p
                      id={describedBy}
                      className="text-sm text-destructive"
                      role="alert"
                    >
                      {fieldError.message}
                    </p>
                  ) : null}
                </div>
              );
            })}
          </div>

          <div className="flex items-center justify-end gap-2">
            {onCancel ? (
              <Button
                type="button"
                variant="outline"
                onClick={onCancel}
                disabled={createCard.isPending}
              >
                Cancelar
              </Button>
            ) : null}
            <Button type="submit" disabled={createCard.isPending}>
              {createCard.isPending ? (
                <>
                  <Loader2 className="animate-spin" aria-hidden="true" />
                  Creando...
                </>
              ) : (
                "Crear tarjeta"
              )}
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  );
}

export default CardForm;
