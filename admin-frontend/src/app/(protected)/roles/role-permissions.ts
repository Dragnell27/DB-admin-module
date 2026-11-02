/**
 * Helpers puros de la matriz de permisos del formulario de Rol, extraídos de
 * `role-form.tsx` para poder probarlos de forma aislada (FEAT-002).
 *
 * La selección de la UI se modela como un `Set<string>` de claves de celda
 * `cellKey = `${resource}::${action}``. Estos helpers convierten entre ese Set
 * y la lista de permisos `{ resource, action }` que consume el schema/BFF,
 * cubriendo tanto el catálogo fijo (recurso × acción cerrado) como los permisos
 * de módulos dinámicos `mod:<slug>`.
 */
import type { Action, Resource } from "@/lib/permissions";
import type { PermissionInput } from "@/features/roles/schema";

/** Catálogo recurso → acciones válidas (espejo de ActionsForResource). */
export const RESOURCE_ACTIONS: ReadonlyArray<
  readonly [Resource, readonly Action[]]
> = [
  // customers/plans/subscriptions son cacheables: añaden "recargar" (forzar el
  // refresco del caché del BFF).
  ["customers", ["ver", "crear", "editar", "eliminar", "recargar"]],
  ["plans", ["ver", "crear", "editar", "eliminar", "recargar"]],
  ["users", ["ver", "crear", "editar", "eliminar"]],
  ["roles", ["ver", "crear", "editar", "eliminar"]],
  ["cards", ["ver", "crear", "eliminar"]], // cards NO tiene "editar" ni es cacheable
  ["subscriptions", ["ver", "crear", "cambiar", "cancelar", "recargar"]],
  ["audit", ["ver"]], // la bitácora solo se consulta
  // Constructor de módulos: recurso del catálogo cerrado (espejo de
  // auth.ActionsForResource(ResourceModuleBuilder) = ver/crear/editar/eliminar).
  // El backend ya lo acepta vía auth.ValidateRolePermission; viaja en el payload
  // como { resource: "module-builder", action } (sin prefijo mod:).
  ["module-builder", ["ver", "crear", "editar", "eliminar"]],
  // Plantillas de conexión (connection-templates): recurso del catálogo cerrado
  // (espejo de auth.ActionsForResource(ResourceConnections) = ver/crear/editar/
  // eliminar). Viaja en el payload como { resource: "connections", action }.
  ["connections", ["ver", "crear", "editar", "eliminar"]],
];

/**
 * Un módulo del catálogo dinámico, tal como lo necesitan estos helpers: su
 * `slug` y las `actions` que declara (p. ej. "ver", "custom:activar").
 */
export interface ModuleCatalogEntry {
  readonly slug: string;
  readonly actions: ReadonlyArray<string>;
  /**
   * Mapa opcional de acciones custom (`custom:<x>`) a su etiqueta legible. No lo
   * usan los helpers puros de este archivo; se declara para que el catálogo que
   * construye `role-form.tsx` pueda transportarlo y rotular las columnas custom.
   */
  readonly actionLabels?: Readonly<Record<string, string>>;
}

/** Clave única de una celda de la matriz (recurso + acción). */
export function cellKey(resource: string, action: string): string {
  return `${resource}::${action}`;
}

/** `true` si `resource` admite `action` según el catálogo fijo. */
export function isValidAction(resource: string, action: string): boolean {
  const entry = RESOURCE_ACTIONS.find(([r]) => r === resource);
  return entry ? (entry[1] as readonly string[]).includes(action) : false;
}

/** Clave del recurso de módulo dinámico (`mod:<slug>`). */
export function moduleResource(slug: string): string {
  return `mod:${slug}`;
}

/** Estado de la matriz: conjunto de celdas (duplas) seleccionadas. */
export type Selected = ReadonlySet<string>;

/**
 * Construye el conjunto seleccionado a partir de permisos existentes (edición).
 *
 * - Permisos fijos: se conservan si la dupla recurso × acción es válida.
 * - Permisos de módulo `mod:<slug>`: se conservan SIEMPRE, exista o no la acción
 *   en la declaración del módulo. Si el módulo conocido declara la acción es un
 *   permiso normal; si NO la declara (acción colgante) también se preserva para
 *   que la vista pueda pintarla marcada y des-marcable ("acción inexistente") y
 *   el usuario pueda quitarla (R3.4, caso acción-no-declarada).
 * - Colgantes (`mod:<slug>` con slug fuera del catálogo): también se preservan
 *   para no perder permisos de un módulo que la UI aún no conoce; la vista los
 *   pinta como fila atenuada des-marcable ("módulo inexistente").
 */
export function selectedFromPermissions(
  permissions: ReadonlyArray<{ resource: string; action: string }>,
  // El catálogo ya no se usa para decidir qué preservar (todas las duplas
  // `mod:<slug>` se conservan independientemente de si el módulo/acción existe
  // en el catálogo), pero se mantiene en la firma por compatibilidad con los
  // llamadores y para dejar explícito el contexto del helper.
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  catalog: ReadonlyArray<ModuleCatalogEntry> = [],
): Set<string> {
  const set = new Set<string>();
  for (const p of permissions) {
    if (p.resource.startsWith("mod:")) {
      // Se preserva la dupla exista o no el módulo/acción en el catálogo: un
      // módulo conocido que declara la acción es un permiso normal; uno que no
      // la declara (acción colgante) o un slug fuera del catálogo (módulo
      // colgante) se conservan igualmente para que la vista los muestre
      // des-marcables en vez de descartarlos en silencio.
      set.add(cellKey(p.resource, p.action));
      continue;
    }
    if (isValidAction(p.resource, p.action)) {
      set.add(cellKey(p.resource, p.action));
    }
  }
  return set;
}

/**
 * Aplana el conjunto seleccionado a la lista de permisos del schema. Recorre
 * tanto el catálogo fijo (recurso × acción) como el catálogo de módulos
 * (emitiendo `{ resource: 'mod:'+slug, action }` por celda marcada). Las claves
 * colgantes (slug fuera del catálogo pero presente en el Set) también se
 * emiten, para no perderlas en un round-trip.
 */
export function selectedToPermissions(
  selected: Selected,
  catalog: ReadonlyArray<ModuleCatalogEntry> = [],
): PermissionInput[] {
  const permissions: PermissionInput[] = [];
  const emitted = new Set<string>();

  const push = (resource: string, action: string): void => {
    const key = cellKey(resource, action);
    if (selected.has(key) && !emitted.has(key)) {
      emitted.add(key);
      permissions.push({ resource, action });
    }
  };

  // 1) Catálogo fijo.
  for (const [resource, actions] of RESOURCE_ACTIONS) {
    for (const action of actions) {
      push(resource, action);
    }
  }

  // 2) Catálogo de módulos conocidos.
  for (const mod of catalog) {
    for (const action of mod.actions) {
      push(moduleResource(mod.slug), action);
    }
  }

  // 3) Colgantes: cualquier clave `mod:<slug>::<acción>` del Set que no haya
  //    sido emitida (slug fuera del catálogo) se preserva.
  for (const key of selected) {
    if (emitted.has(key)) {
      continue;
    }
    const sep = key.indexOf("::");
    if (sep === -1) {
      continue;
    }
    const resource = key.slice(0, sep);
    const action = key.slice(sep + 2);
    if (resource.startsWith("mod:")) {
      emitted.add(key);
      permissions.push({ resource, action });
    }
  }

  return permissions;
}
