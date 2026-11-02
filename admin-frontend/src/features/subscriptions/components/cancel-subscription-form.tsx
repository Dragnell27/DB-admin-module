"use client";

/**
 * Confirmación de cancelación de Subscription (tarea 16.3, Req. 16).
 *
 * No introduce un primitivo de diálogo compartido (evita colisiones con las
 * tareas 16.1/16.2/16.4): se renderiza como un panel de confirmación en línea
 * con estado local en el padre. Solicita `customer_id` (UUID requerido) y un
 * `cancel_reason` opcional (máx. 255), validando con `subscriptionCancelSchema`
 * e impidiendo el envío si hay errores (Req. 16.1-16.3). La ejecución real
 * (`POST /subscriptions/cancel`) y el feedback los maneja el padre.
 */
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/utils";
import {
  subscriptionCancelSchema,
  type SubscriptionCancelInput,
} from "@/features/subscriptions/schema";
import { LIMITS } from "@/lib/validation";

export interface CancelSubscriptionFormProps {
  /** `subscription_id` que se va a cancelar (sólo informativo en la UI). */
  readonly subscriptionId?: string;
  /** Se invoca con el payload validado al confirmar. */
  readonly onConfirm: (values: SubscriptionCancelInput) => void;
  /** Cierra la confirmación sin ejecutar la cancelación. */
  readonly onCancel: () => void;
  /** Indica una cancelación en curso. */
  readonly isSubmitting?: boolean;
  readonly className?: string;
}

/**
 * Forma del formulario. `cancel_reason` es opcional para coincidir con el tipo
 * inferido por `subscriptionCancelSchema` (y así satisfacer el `Resolver` de
 * RHF). Los inputs nativos siempre devuelven `string`; los `defaultValues`
 * garantizan que ambos campos arranquen definidos.
 */
interface CancelFormFields {
  customer_id: string;
  cancel_reason?: string;
}

export function CancelSubscriptionForm({
  subscriptionId,
  onConfirm,
  onCancel,
  isSubmitting = false,
  className,
}: CancelSubscriptionFormProps) {
  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<CancelFormFields>({
    resolver: zodResolver(subscriptionCancelSchema),
    defaultValues: { customer_id: "", cancel_reason: "" },
    mode: "onSubmit",
  });

  const submit = handleSubmit((fields) => {
    const reason = (fields.cancel_reason ?? "").trim();
    onConfirm({
      customer_id: fields.customer_id.trim(),
      cancel_reason: reason === "" ? undefined : reason,
    });
  });

  return (
    <form
      onSubmit={submit}
      className={cn("space-y-4", className)}
      noValidate
    >
      <p className="text-sm text-muted-foreground">
        Vas a cancelar la suscripción
        {subscriptionId ? (
          <>
            {" "}
            <span className="font-mono text-foreground">{subscriptionId}</span>
          </>
        ) : null}
        . Esta acción da de baja la suscripción del cliente.
      </p>

      <div className="space-y-1.5">
        <Label htmlFor="cancel_customer_id">
          customer_id<span className="ml-0.5 text-destructive">*</span>
        </Label>
        <Input
          id="cancel_customer_id"
          placeholder="00000000-0000-0000-0000-000000000000"
          autoComplete="off"
          disabled={isSubmitting}
          aria-invalid={Boolean(errors.customer_id)}
          {...register("customer_id")}
        />
        {errors.customer_id ? (
          <p className="text-xs text-destructive" role="alert">
            {errors.customer_id.message}
          </p>
        ) : (
          <p className="text-xs text-muted-foreground">UUID del cliente</p>
        )}
      </div>

      <div className="space-y-1.5">
        <Label htmlFor="cancel_reason">cancel_reason</Label>
        <Input
          id="cancel_reason"
          placeholder="Opcional"
          maxLength={LIMITS.cancelReason}
          autoComplete="off"
          disabled={isSubmitting}
          aria-invalid={Boolean(errors.cancel_reason)}
          {...register("cancel_reason")}
        />
        {errors.cancel_reason ? (
          <p className="text-xs text-destructive" role="alert">
            {errors.cancel_reason.message}
          </p>
        ) : (
          <p className="text-xs text-muted-foreground">
            Motivo opcional, máx. {LIMITS.cancelReason} caracteres
          </p>
        )}
      </div>

      <div className="flex items-center justify-end gap-2 pt-2">
        <Button
          type="button"
          variant="outline"
          onClick={onCancel}
          disabled={isSubmitting}
        >
          Volver
        </Button>
        <Button type="submit" variant="destructive" disabled={isSubmitting}>
          {isSubmitting ? "Cancelando..." : "Confirmar cancelación"}
        </Button>
      </div>
    </form>
  );
}

export default CancelSubscriptionForm;
