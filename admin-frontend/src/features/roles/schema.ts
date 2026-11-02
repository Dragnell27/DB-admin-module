/**
 * Schemas Zod del recurso Rol (crear/editar) con mensajes en español.
 *
 * Modelo (rediseño): un permiso de ROL es la dupla Recurso × Acción (SIN país).
 * El país es scope del usuario (ver features/users). Replican la validación
 * server-side del admin-backend (roles_handler.go: roleCreateBody/roleEditBody
 * con rolePermissionDTO {resource, action}).
 *
 * Contrato del backend:
 *   - POST /roles → { name, permissions: [{ resource, action }] }
 *   - PUT  /roles → { id, name, permissions: [...] }
 */
import { z } from "zod";

import type { Resource } from "@/lib/permissions";

/** Longitud máxima del nombre de un rol. */
const ROLE_NAME_MAX = 100;

/** Literales de recursos del catálogo cerrado (espejo de `Resource`). */
const RESOURCE_VALUES = [
  "customers",
  "plans",
  "subscriptions",
  "cards",
  "users",
  "roles",
  "audit",
  // Constructor de módulos: recurso del catálogo cerrado que el backend acepta
  // y persiste vía auth.ValidateRolePermission. Debe estar aquí para que
  // permissionSchema no rechace { resource: "module-builder", action }.
  "module-builder",
  // Plantillas de conexión (connection-templates): recurso del catálogo cerrado
  // que el backend acepta vía auth.ValidateRolePermission. Debe estar aquí para
  // que permissionSchema no rechace { resource: "connections", action }.
  "connections",
] as const satisfies readonly Resource[];

/**
 * Permiso atómico de un rol: la dupla Recurso × Acción (espejo de
 * rolePermissionDTO del backend). El país NO forma parte del permiso de rol.
 */
export const permissionSchema = z.object({
  // El recurso es o bien uno del catálogo cerrado, o bien un recurso de módulo
  // dinámico `mod:<slug>` (el regex del slug es espejo de auth.IsValidSlug).
  resource: z.union([
    z.enum(RESOURCE_VALUES),
    z
      .string()
      .regex(/^mod:[a-z0-9]+(?:-[a-z0-9]+)*$/, "El recurso de módulo no es válido"),
  ]),
  // La acción se relaja a texto libre: los módulos declaran acciones propias
  // (incluidas custom:<x>) que no están en el catálogo cerrado.
  action: z.string().min(1, "La acción es requerida"),
});

/** Rol — crear (POST /roles). */
export const roleCreateSchema = z.object({
  name: z
    .string({ required_error: "El nombre del rol es requerido" })
    .trim()
    .min(1, "El nombre del rol es requerido")
    .max(ROLE_NAME_MAX, `El nombre del rol no puede exceder ${ROLE_NAME_MAX} caracteres`),
  permissions: z.array(permissionSchema),
});

/** Rol — editar (PUT /roles). */
export const roleEditSchema = z.object({
  id: z
    .string({ required_error: "El id del rol es requerido" })
    .min(1, "El id del rol es requerido"),
  name: z
    .string({ required_error: "El nombre del rol es requerido" })
    .trim()
    .min(1, "El nombre del rol es requerido")
    .max(ROLE_NAME_MAX, `El nombre del rol no puede exceder ${ROLE_NAME_MAX} caracteres`),
  permissions: z.array(permissionSchema),
});

export type PermissionInput = z.infer<typeof permissionSchema>;
export type RoleCreateInput = z.infer<typeof roleCreateSchema>;
export type RoleEditInput = z.infer<typeof roleEditSchema>;
