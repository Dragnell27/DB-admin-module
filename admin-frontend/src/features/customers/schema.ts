/**
 * Schemas Zod del recurso Customer (crear/editar) con mensajes en español.
 *
 * Replican la validación server-side espejo del admin-backend
 * (`admin-backend/internal/validation/resources.go`: CustomerInput,
 * ValidateCustomerCreate, ValidateCustomerEdit) para que cliente y servidor
 * coincidan (Req. 7.2-7.9, 8.1-8.3).
 *
 * Los tipos inferidos se exportan para su uso por las mutations de TanStack
 * Query (tarea 15.2) y los formularios (tarea 16.1).
 */
import { z } from "zod";

import {
  LIMITS,
  birthDateSchema,
  emailSchema,
  GENDERS,
  optionalString,
  phoneSchema,
  requiredString,
  rfc3339Schema,
} from "@/lib/validation";

/**
 * Customer — crear (Req. 7.2-7.9).
 *
 * - external_id: req, máx 100
 * - first_name / last_name: req, máx 100
 * - email: req, formato válido, máx 255 (no editable después)
 * - phone: req, '+' + 8..15 dígitos
 * - birth_date: YYYY-MM-DD, no futura, ≥18 años
 * - external_created: req, RFC3339
 * - second_last_name / gender (M|F|O) / address (máx 255) / device_fingerprint: opc
 *
 * `second_last_name` se acepta SOLO en creación (así lo admite el API OTT al
 * crear); en edición no se incluye porque el API no lo devuelve ni lo edita.
 */
export const customerCreateSchema = z.object({
  external_id: requiredString("external_id", LIMITS.externalId),
  first_name: requiredString("first_name", LIMITS.name),
  last_name: requiredString("last_name", LIMITS.name),
  email: emailSchema,
  phone: phoneSchema,
  birth_date: birthDateSchema,
  external_created: rfc3339Schema,
  second_last_name: optionalString("second_last_name", LIMITS.name),
  gender: z
    .enum(GENDERS, { errorMap: () => ({ message: "gender debe ser M, F u O" }) })
    .optional(),
  address: optionalString("address", LIMITS.address),
  device_fingerprint: z.string().optional(),
});

/**
 * Customer — editar (Req. 8.1-8.3).
 *
 * Solo son editables first_name, last_name, phone, gender, address,
 * device_fingerprint y birth_date. `external_id` y `email` NO son editables
 * (Req. 8.2) y por eso no forman parte de este schema; se muestran
 * deshabilitados en la UI y se usan solo para localizar al cliente.
 *
 * Todos los campos editables son opcionales en edición; si se envían, cumplen
 * las mismas reglas de formato/longitud que en creación. El API OTT real no
 * maneja `second_last_name`, por lo que no se incluye.
 */
export const customerEditSchema = z.object({
  first_name: requiredString("first_name", LIMITS.name).optional(),
  last_name: requiredString("last_name", LIMITS.name).optional(),
  phone: phoneSchema.optional(),
  birth_date: birthDateSchema.optional(),
  gender: z
    .enum(GENDERS, { errorMap: () => ({ message: "gender debe ser M, F u O" }) })
    .optional(),
  address: optionalString("address", LIMITS.address),
  device_fingerprint: z.string().optional(),
});

export type CustomerCreateInput = z.infer<typeof customerCreateSchema>;
export type CustomerEditInput = z.infer<typeof customerEditSchema>;
