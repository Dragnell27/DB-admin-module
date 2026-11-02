/**
 * Schemas Zod de las plantillas de conexión (connection-templates). Replican la
 * validación del backend (internal/connections): nombre no vacío, al menos un
 * campo, sin claves duplicadas, role del conjunto cerrado o vacío, y campos
 * FIJOS con valor al crear.
 */
import { z } from "zod";

import { FIELD_ROLES } from "@/features/connections/types";

/** Roles válidos (conjunto cerrado) + "" (que el backend trata como option). */
const roleSchema = z.union([z.literal(""), z.enum(FIELD_ROLES as [string, ...string[]])]);

/** Un campo de la plantilla (estructura, sin valor). */
export const templateFieldSchema = z.object({
  key: z.string().trim().min(1, "La clave del campo es obligatoria"),
  kind: z.enum(["fixed", "variable"]),
  role: roleSchema.optional(),
  required: z.boolean().optional(),
  suggested: z.string().optional(),
});

/**
 * Esquema de crear/editar. `forCreate` activa la regla de FIJO-con-valor: en
 * creación un campo FIJO necesita un valor en `fixedValues`; en edición un FIJO
 * vacío conserva el previo (validado server-side contra el bundle descifrado).
 */
export function connectionTemplateSchema(forCreate: boolean) {
  return z
    .object({
      name: z.string().trim().min(1, "El nombre de la plantilla es obligatorio"),
      fields: z.array(templateFieldSchema).min(1, "Agrega al menos un campo"),
      fixedValues: z.record(z.string()),
    })
    .superRefine((data, ctx) => {
      // Claves únicas.
      const seen = new Set<string>();
      data.fields.forEach((f, i) => {
        const key = f.key.trim();
        if (seen.has(key)) {
          ctx.addIssue({
            code: z.ZodIssueCode.custom,
            message: `Clave duplicada "${key}"`,
            path: ["fields", i, "key"],
          });
        }
        seen.add(key);
      });
      // En creación, cada FIJO necesita valor.
      if (forCreate) {
        data.fields.forEach((f, i) => {
          if (f.kind === "fixed" && (data.fixedValues[f.key] ?? "").trim() === "") {
            ctx.addIssue({
              code: z.ZodIssueCode.custom,
              message: `El campo fijo "${f.key}" requiere un valor`,
              path: ["fields", i, "key"],
            });
          }
        });
      }
    });
}

export type ConnectionTemplateFormInput = z.infer<ReturnType<typeof connectionTemplateSchema>>;
