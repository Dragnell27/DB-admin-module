/**
 * Schema Zod del recurso Card (crear) con mensajes en español.
 *
 * Replica la validación server-side espejo del admin-backend
 * (`admin-backend/internal/validation/resources.go`: CardInput,
 * ValidateCardCreate) para que cliente y servidor coincidan (Req. 18.2-18.7).
 *
 * El tipo inferido se exporta para su uso por las mutations de TanStack Query
 * (tarea 15.2) y el formulario (tarea 16.4).
 */
import { z } from "zod";

import {
  isFutureExpiration,
  LIMITS,
  requiredString,
  uuidSchema,
} from "@/lib/validation";

/**
 * Card — crear (Req. 18.2-18.7).
 *
 * - customer_id: UUID requerido
 * - card_number: req, máx 50
 * - expiration_date: MM/YY, futura, máx 10
 * - card_token: UUID requerido, máx 255
 * - payment_method_name: req, máx 100
 * - card_franchise: req, máx 50
 */
export const cardCreateSchema = z.object({
  customer_id: uuidSchema("customer_id"),
  card_number: requiredString("card_number", LIMITS.cardNumber),
  expiration_date: z
    .string({ required_error: "expiration_date es requerido" })
    .min(1, "expiration_date es requerido")
    .max(
      LIMITS.expirationDate,
      `expiration_date no puede exceder ${LIMITS.expirationDate} caracteres`,
    )
    .regex(/^\d{2}\/\d{2}$/, "expiration_date debe tener formato MM/YY")
    .refine(
      (v) => isFutureExpiration(v),
      "La fecha de expiración debe ser futura",
    ),
  card_token: uuidSchema("card_token").refine(
    (v) => v.length <= LIMITS.cardToken,
    `card_token no puede exceder ${LIMITS.cardToken} caracteres`,
  ),
  payment_method_name: requiredString(
    "payment_method_name",
    LIMITS.paymentMethodName,
  ),
  card_franchise: requiredString("card_franchise", LIMITS.cardFranchise),
});

export type CardCreateInput = z.infer<typeof cardCreateSchema>;
