/**
 * Schemas Zod del recurso Usuario_Admin (crear/editar) con mensajes en español.
 *
 * Replican la validación server-side espejo del admin-backend
 * (`admin-backend/internal/api/users_handler.go`: userCreateBody, userEditBody
 * y sus validaciones en Create/Edit) para que cliente y servidor coincidan
 * (Req. 16.1, 16.2).
 *
 * Contrato del backend reflejado aquí:
 *   - POST /users           → { username, roleIds: [] }  (al menos un rol)
 *   - PUT  /users           → { id, roleIds: [], isActive }
 *   - POST /users/reset-password → { id }  (no requiere schema de formulario)
 *
 * Los tipos inferidos se exportan para su uso por las mutations de TanStack
 * Query (tarea 18.2) y los formularios/páginas (tarea 19.1).
 */
import { z } from "zod";

/**
 * Longitudes del username (espejo de las validaciones del backend). El backend
 * exige un username no vacío; aquí fijamos un mínimo de 3 y un máximo de 64 para
 * dar retroalimentación temprana en el formulario.
 */
const USERNAME_MIN = 3;
const USERNAME_MAX = 64;

/**
 * Usuario — crear (POST /users, Req. 16.1).
 *
 * - username: string requerido, entre 3 y 64 caracteres
 * - roleIds: lista de ids de rol con al menos un elemento (el backend exige al
 *   menos un rol para crear el usuario)
 *
 * La contraseña NO forma parte del formulario: la genera el servidor y se
 * devuelve una sola vez al crear (ver handler Create del backend).
 */
export const userCreateSchema = z.object({
  username: z
    .string({ required_error: "El nombre de usuario es requerido" })
    .trim()
    .min(USERNAME_MIN, `El nombre de usuario debe tener al menos ${USERNAME_MIN} caracteres`)
    .max(USERNAME_MAX, `El nombre de usuario no puede exceder ${USERNAME_MAX} caracteres`),
  roleIds: z
    .array(z.string().min(1, "El id del rol es requerido"))
    .min(1, "Debe asignar al menos un rol al usuario"),
  countries: z
    .array(z.string().min(1, "El país es requerido"))
    .min(1, "Debe asignar al menos un país al usuario"),
});

/**
 * Usuario — editar (PUT /users, Req. 16.1).
 *
 * - id: string requerido (identifica el usuario a editar)
 * - roleIds: lista de ids de rol. Aunque el backend permite fijar un conjunto
 *   vacío de roles, en la edición desde el panel exigimos al menos uno como
 *   medida de auto-protección (evitar dejar a un usuario sin permisos por error).
 * - isActive: estado de activación del usuario
 */
export const userEditSchema = z.object({
  id: z
    .string({ required_error: "El id del usuario es requerido" })
    .min(1, "El id del usuario es requerido"),
  roleIds: z
    .array(z.string().min(1, "El id del rol es requerido"))
    .min(1, "Debe asignar al menos un rol al usuario"),
  countries: z
    .array(z.string().min(1, "El país es requerido"))
    .min(1, "Debe asignar al menos un país al usuario"),
  isActive: z.boolean(),
});

export type UserCreateInput = z.infer<typeof userCreateSchema>;
export type UserEditInput = z.infer<typeof userEditSchema>;
