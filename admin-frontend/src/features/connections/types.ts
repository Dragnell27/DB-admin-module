/**
 * Tipos de las plantillas de conexión (connection-templates) en el frontend.
 * Espejo del dominio del backend (internal/connections): una plantilla define
 * campos marcados FIJO o VARIABLE. Los valores FIJOS NUNCA llegan de vuelta (el
 * backend los omite): solo se envían al crear/editar bajo `fixedValues`.
 */

/** Marca de un campo de la plantilla: FIJO (valor cifrado en la plantilla) o VARIABLE (lo aporta cada módulo). */
export type FieldKind = "fixed" | "variable";

/** Rol de un campo en el DSN PostgreSQL (conjunto cerrado; "" = option con nombre = key). */
export type FieldRole =
  | ""
  | "host"
  | "port"
  | "user"
  | "password"
  | "dbname"
  | "sslmode"
  | "option";

/** Conjunto cerrado de roles ofrecidos en el editor (espejo del backend). */
export const FIELD_ROLES: ReadonlyArray<FieldRole> = [
  "host",
  "port",
  "user",
  "password",
  "dbname",
  "sslmode",
  "option",
];

/** Estructura de un campo de la plantilla (sin su valor). */
export interface TemplateField {
  key: string;
  kind: FieldKind;
  role?: FieldRole;
  /** Solo variable: obligatorio por defecto (true). */
  required?: boolean;
  /** Solo variable: valor sugerido NO sensible (prellenado editable). */
  suggested?: string;
}

/** Vista pública de una plantilla (nunca incluye los valores FIJOS). */
export interface ConnectionTemplate {
  id: string;
  name: string;
  fields: TemplateField[];
  /** Indica si hay valores FIJOS cifrados, sin revelarlos. */
  hasFixedValues: boolean;
  /** Número de módulos que la usan (radio de impacto, D-4). */
  usedByModules: number;
  createdAt?: string;
  updatedAt?: string;
}

/** Cuerpo de crear/editar: estructura + valores FIJOS en claro (solo al enviar). */
export interface ConnectionTemplateInput {
  name: string;
  fields: TemplateField[];
  /** Valores FIJOS en claro (clave -> valor). Vacío = conservar previo (edición). */
  fixedValues: Record<string, string>;
}
