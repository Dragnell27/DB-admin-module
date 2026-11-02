/**
 * Schemas Zod del recurso Subscription (crear/cambiar/cancelar) con mensajes en
 * español.
 *
 * Replican la validación server-side espejo del admin-backend
 * (`admin-backend/internal/validation/resources.go`: SubscriptionInput,
 * SubscriptionChangeInput, SubscriptionCancelInput y sus validadores) para que
 * cliente y servidor coincidan (Req. 14.2-14.7, 15.1-15.3, 16.1-16.3).
 *
 * Los tipos inferidos se exportan para su uso por las mutations de TanStack
 * Query (tarea 15.2) y los formularios/diálogos (tarea 16.3).
 */
import { z } from "zod";

import {
  LIMITS,
  positiveNumber,
  requiredString,
  rfc3339Schema,
  uuidSchema,
} from "@/lib/validation";

/**
 * Subscription — crear (Req. 14.2-14.7).
 *
 * - plan_id / customer_id / payment_method_token: UUID requerido
 * - payment_method: requerido
 * - amount: opcional, > 0 si se envía
 * - start_date: opcional, RFC3339 si se envía
 */
export const subscriptionCreateSchema = z.object({
  plan_id: uuidSchema("plan_id"),
  customer_id: uuidSchema("customer_id"),
  payment_method: z
    .string({ required_error: "payment_method es requerido" })
    .min(1, "payment_method es requerido"),
  payment_method_token: uuidSchema("payment_method_token"),
  amount: positiveNumber("amount").optional(),
  start_date: rfc3339Schema.optional(),
});

/**
 * Subscription — cambiar (Req. 15.1-15.3). Todos los campos son opcionales; se
 * valida UUID/formato cuando se envían. Espejo de ValidateSubscriptionChange.
 */
export const subscriptionChangeSchema = z.object({
  // `subscription_id` identifica la suscripción a cambiar; el OTT lo exige en el
  // cuerpo del cambio. Es opcional en el schema (el formulario lo inyecta), pero
  // en la práctica siempre viaja en el modo "change".
  subscription_id: uuidSchema("subscription_id").optional(),
  plan_id: uuidSchema("plan_id").optional(),
  customer_id: uuidSchema("customer_id").optional(),
  payment_method_token: uuidSchema("payment_method_token").optional(),
  amount: positiveNumber("amount").optional(),
  start_date: rfc3339Schema.optional(),
});

/**
 * Subscription — cancelar (Req. 16.1-16.3). `customer_id` UUID requerido;
 * `cancel_reason` opcional con máx 255. Espejo de ValidateSubscriptionCancel.
 */
export const subscriptionCancelSchema = z.object({
  customer_id: uuidSchema("customer_id"),
  cancel_reason: requiredString("cancel_reason", LIMITS.cancelReason).optional(),
});

export type SubscriptionCreateInput = z.infer<typeof subscriptionCreateSchema>;
export type SubscriptionChangeInput = z.infer<typeof subscriptionChangeSchema>;
export type SubscriptionCancelInput = z.infer<typeof subscriptionCancelSchema>;
