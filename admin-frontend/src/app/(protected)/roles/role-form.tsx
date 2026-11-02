"use client";

/**
 * Formulario de Rol compartido por las vistas de crear y editar.
 *
 * Modelo (rediseño): un permiso de ROL es la dupla Recurso × Acción, SIN país.
 * El país es scope del usuario. Por eso la matriz es una ÚNICA tabla recurso ×
 * acción (ya no una tabla por país). Cada checkbox marcado se traduce en un
 * permiso { resource, action }.
 *
 * El catálogo recurso×acción fijo se define en `role-permissions.ts` (espejo de
 * ActionsForResource del backend). Además, los permisos de módulos dinámicos
 * `mod:<slug>` se pintan en una segunda tabla alimentada por el catálogo que
 * expone el BFF (GET /roles/module-catalog, vía `useModuleCatalog`).
 */
import * as React from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { Loader2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import { FeedbackBanner } from "@/components/feedback-banner";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import type { Action, Resource } from "@/lib/permissions";
import { useModuleCatalog } from "@/features/roles/hooks";
import {
  roleCreateSchema,
  type PermissionInput,
  type RoleCreateInput,
} from "@/features/roles/schema";
import {
  RESOURCE_ACTIONS,
  cellKey,
  moduleResource,
  selectedFromPermissions,
  selectedToPermissions,
  type ModuleCatalogEntry,
} from "@/app/(protected)/roles/role-permissions";

/** Etiquetas legibles de cada recurso. */
const RESOURCE_LABELS: Readonly<Record<Resource, string>> = {
  customers: "Clientes",
  plans: "Planes",
  users: "Usuarios",
  roles: "Roles",
  cards: "Tarjetas",
  subscriptions: "Suscripciones",
  audit: "Auditoría",
  "module-builder": "Constructor de módulos",
  connections: "Conexiones",
};

/** Columnas de acción (catálogo fijo) en orden estable. */
const ACTION_COLUMNS: ReadonlyArray<Action> = [
  "ver",
  "crear",
  "editar",
  "eliminar",
  "cambiar",
  "cancelar",
  "recargar",
];

/** Etiquetas legibles de cada acción del catálogo fijo. */
const ACTION_LABELS: Readonly<Record<Action, string>> = {
  ver: "Ver",
  crear: "Crear",
  editar: "Editar",
  eliminar: "Eliminar",
  cambiar: "Cambiar",
  cancelar: "Cancelar",
  recargar: "Recargar",
};

/**
 * Etiqueta legible de una acción de módulo. Las acciones del catálogo fijo
 * reutilizan `ACTION_LABELS`; las `custom:<x>` usan el label real declarado por
 * el módulo cuando está disponible en `customLabels` (p. ej. "Buscar" para la
 * clave "buscar"); si no, se degrada al fallback de capitalizar la clave. El
 * resto se capitaliza tal cual.
 */
function moduleActionLabel(
  action: string,
  customLabels: Record<string, string>,
): string {
  if (action in ACTION_LABELS) {
    return ACTION_LABELS[action as Action];
  }
  if (action.startsWith("custom:")) {
    const real = customLabels[action];
    if (real) {
      return real;
    }
    const suffix = action.slice("custom:".length);
    return capitalize(suffix);
  }
  return capitalize(action);
}

/** Capitaliza la primera letra de un texto. */
function capitalize(text: string): string {
  return text.length > 0 ? text.charAt(0).toUpperCase() + text.slice(1) : text;
}

/** Valores de envío del formulario de rol. */
export interface RoleFormValues {
  readonly name: string;
  readonly permissions: ReadonlyArray<PermissionInput>;
}

export interface RoleFormProps {
  readonly defaultName?: string;
  readonly defaultPermissions?: ReadonlyArray<{
    resource: string;
    action: string;
  }>;
  readonly isSubmitting?: boolean;
  readonly submitLabel: string;
  readonly onCancel: () => void;
  readonly onSubmit: (values: RoleFormValues) => void;
}

/** Sólo el nombre se valida con react-hook-form; los permisos son estado local. */
type NameOnly = Pick<RoleCreateInput, "name">;

export function RoleForm({
  defaultName,
  defaultPermissions,
  isSubmitting = false,
  submitLabel,
  onCancel,
  onSubmit,
}: RoleFormProps) {
  const nameSchema = React.useMemo(
    () => roleCreateSchema.pick({ name: true }),
    [],
  );

  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<NameOnly>({
    resolver: zodResolver(nameSchema),
    defaultValues: { name: defaultName ?? "" },
  });

  // Catálogo de módulos dinámicos (degradación no bloqueante: si falla, se
  // editan solo los permisos fijos; si está vacío, no se pinta su tabla).
  const {
    data: moduleCatalog = [],
    isError: catalogError,
  } = useModuleCatalog();

  // Vista estable del catálogo para los helpers puros (slug + acciones).
  const catalogEntries = React.useMemo<ReadonlyArray<ModuleCatalogEntry>>(
    () =>
      moduleCatalog.map((m) => ({
        slug: m.slug,
        actions: m.actions,
        actionLabels: m.actionLabels,
      })),
    [moduleCatalog],
  );

  // Mapa plano permAction custom (`custom:<x>`) -> label legible, mergeando los
  // actionLabels de todos los módulos. Si una misma permAction aparece en varios
  // módulos con labels distintos, GANA el primero encontrado (determinista por
  // el orden del catálogo). Tolerante: módulos sin actionLabels se omiten.
  const customActionLabels = React.useMemo<Record<string, string>>(() => {
    const merged: Record<string, string> = {};
    for (const m of moduleCatalog) {
      if (!m.actionLabels) {
        continue;
      }
      for (const [permAction, label] of Object.entries(m.actionLabels)) {
        if (!(permAction in merged)) {
          merged[permAction] = label;
        }
      }
    }
    return merged;
  }, [moduleCatalog]);

  const [selected, setSelected] = React.useState<Set<string>>(() =>
    defaultPermissions
      ? selectedFromPermissions(defaultPermissions, catalogEntries)
      : new Set(),
  );

  // Hidratación de la matriz cuando el detalle (edición) y/o el catálogo llegan
  // después del primer render.
  //
  // El reto es la CARRERA entre `useRoleDetail` y `useModuleCatalog`: si el
  // detalle llega antes que el catálogo (o el catálogo falla y luego se
  // recupera), la primera hidratación se hace con catálogo vacío. Con el guard
  // de un solo disparo anterior, esa hidratación temprana quedaba congelada y
  // podía reenviar una dupla colgante en el payload. Ahora, además de hidratar
  // una vez cuando llega `defaultPermissions`, se RE-HIDRATA cuando el catálogo
  // pasa de vacío a cargado, SIEMPRE que el usuario aún no haya tocado la matriz
  // (si ya la tocó se respeta su estado para no descartar sus selecciones).
  const hydratedRef = React.useRef(false);
  const hydratedWithCatalogRef = React.useRef(false);
  const userTouchedRef = React.useRef(false);
  React.useEffect(() => {
    if (!defaultPermissions) {
      return;
    }
    const hasCatalog = catalogEntries.length > 0;
    const firstHydration = !hydratedRef.current;
    // Re-hidratar solo si: es la primera vez, o el catálogo acaba de pasar de
    // vacío a cargado y el usuario no ha tocado la matriz todavía.
    const catalogJustArrived =
      !hydratedWithCatalogRef.current && hasCatalog && !userTouchedRef.current;
    if (firstHydration || catalogJustArrived) {
      hydratedRef.current = true;
      hydratedWithCatalogRef.current = hasCatalog;
      setSelected(selectedFromPermissions(defaultPermissions, catalogEntries));
    }
  }, [defaultPermissions, catalogEntries]);

  const toggleCell = React.useCallback((resource: string, action: string) => {
    // Marca que el usuario interactuó: a partir de aquí no se re-hidrata al
    // llegar el catálogo, para no pisar sus selecciones.
    userTouchedRef.current = true;
    setSelected((prev) => {
      const next = new Set(prev);
      const key = cellKey(resource, action);
      if (next.has(key)) {
        next.delete(key);
      } else {
        next.add(key);
      }
      return next;
    });
  }, []);

  const submit = handleSubmit((values) => {
    onSubmit({
      name: values.name.trim(),
      permissions: selectedToPermissions(selected, catalogEntries),
    });
  });

  // Columnas de la tabla de módulos: unión de todas las acciones declaradas por
  // los módulos del catálogo, en orden estable (acciones del catálogo fijo
  // primero según ACTION_COLUMNS, luego el resto alfabético).
  const moduleActionColumns = React.useMemo<ReadonlyArray<string>>(() => {
    const all = new Set<string>();
    for (const m of moduleCatalog) {
      for (const a of m.actions) {
        all.add(a);
      }
    }
    // Incluye también las acciones de los permisos colgantes para que sus
    // celdas marcadas siempre tengan columna donde pintarse/des-marcarse.
    for (const key of selected) {
      const sep = key.indexOf("::");
      if (sep === -1) {
        continue;
      }
      if (key.slice(0, sep).startsWith("mod:")) {
        all.add(key.slice(sep + 2));
      }
    }
    const order = (a: string): number => {
      const idx = ACTION_COLUMNS.indexOf(a as Action);
      return idx === -1 ? ACTION_COLUMNS.length : idx;
    };
    return [...all].sort((a, b) => {
      const oa = order(a);
      const ob = order(b);
      return oa !== ob ? oa - ob : a.localeCompare(b);
    });
  }, [moduleCatalog, selected]);

  // Permisos de módulo COLGANTES: `mod:<slug>` cuyo slug no está en el catálogo.
  // Se agrupan por slug para pintarlos como filas atenuadas des-marcables.
  const danglingModules = React.useMemo(() => {
    const known = new Set(moduleCatalog.map((m) => m.slug));
    const bySlug = new Map<string, Set<string>>();
    for (const key of selected) {
      const sep = key.indexOf("::");
      if (sep === -1) {
        continue;
      }
      const resource = key.slice(0, sep);
      const action = key.slice(sep + 2);
      if (!resource.startsWith("mod:")) {
        continue;
      }
      const slug = resource.slice("mod:".length);
      if (known.has(slug)) {
        continue;
      }
      const actions = bySlug.get(slug) ?? new Set<string>();
      actions.add(action);
      bySlug.set(slug, actions);
    }
    return [...bySlug.entries()].map(([slug, actions]) => ({ slug, actions }));
  }, [selected, moduleCatalog]);

  const showModulesTable =
    moduleCatalog.length > 0 || danglingModules.length > 0;

  return (
    <form onSubmit={submit} className="space-y-6" noValidate>
      <div className="space-y-2">
        <Label htmlFor="role-name">Nombre del rol</Label>
        <Input
          id="role-name"
          autoComplete="off"
          aria-invalid={errors.name ? true : undefined}
          aria-describedby={errors.name ? "role-name-error" : undefined}
          placeholder="p. ej. Operador de clientes"
          {...register("name")}
        />
        {errors.name ? (
          <p id="role-name-error" className="text-sm text-destructive">
            {errors.name.message}
          </p>
        ) : null}
      </div>

      <fieldset className="space-y-4">
        <legend className="text-sm font-medium text-foreground">
          Permisos (recurso × acción)
        </legend>
        <p className="text-sm text-muted-foreground">
          Marca qué puede hacer el rol. Los países en los que aplica se definen
          por usuario, no por rol.
        </p>

        <div className="rounded-md border border-border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead scope="col">Recurso</TableHead>
                {ACTION_COLUMNS.map((action) => (
                  <TableHead key={action} scope="col" className="text-center">
                    {ACTION_LABELS[action]}
                  </TableHead>
                ))}
              </TableRow>
            </TableHeader>
            <TableBody>
              {RESOURCE_ACTIONS.map(([resource, actions]) => (
                <TableRow key={resource}>
                  <TableCell className="font-medium">
                    {RESOURCE_LABELS[resource]}
                  </TableCell>
                  {ACTION_COLUMNS.map((action) => {
                    const allowed = actions.includes(action);
                    if (!allowed) {
                      return (
                        <TableCell
                          key={action}
                          className="text-center text-muted-foreground"
                          aria-hidden="true"
                        >
                          —
                        </TableCell>
                      );
                    }
                    const key = cellKey(resource, action);
                    const checkboxId = `perm-${key}`;
                    const checkboxLabel = `${ACTION_LABELS[action]} ${RESOURCE_LABELS[resource]}`;
                    return (
                      <TableCell key={action} className="text-center">
                        <input
                          id={checkboxId}
                          type="checkbox"
                          className="size-4 cursor-pointer accent-primary"
                          checked={selected.has(key)}
                          onChange={() => toggleCell(resource, action)}
                          aria-label={checkboxLabel}
                        />
                      </TableCell>
                    );
                  })}
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      </fieldset>

      {catalogError ? (
        <FeedbackBanner
          tone="warning"
          message="No se pudieron cargar los módulos; puedes editar los permisos fijos"
        />
      ) : null}

      {showModulesTable ? (
        <fieldset className="space-y-4">
          <legend className="text-sm font-medium text-foreground">
            Permisos de módulos
          </legend>
          <p className="text-sm text-muted-foreground">
            Permisos sobre los módulos del constructor. Cada módulo declara sus
            propias acciones.
          </p>

          <div className="rounded-md border border-border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead scope="col">Módulo</TableHead>
                  {moduleActionColumns.map((action) => (
                    <TableHead
                      key={action}
                      scope="col"
                      className="text-center"
                    >
                      {moduleActionLabel(action, customActionLabels)}
                    </TableHead>
                  ))}
                </TableRow>
              </TableHeader>
              <TableBody>
                {moduleCatalog.map((mod) => {
                  const resource = moduleResource(mod.slug);
                  return (
                    <TableRow key={mod.slug}>
                      <TableCell className="font-medium">{mod.name}</TableCell>
                      {moduleActionColumns.map((action) => {
                        const key = cellKey(resource, action);
                        const allowed = mod.actions.includes(action);
                        // Acción colgante de un módulo conocido: el módulo NO la
                        // declara pero el rol la tiene seleccionada (p. ej. se
                        // borró la acción del módulo tras concederla). En vez de
                        // pintar "—" se muestra un checkbox marcado y
                        // des-marcable, atenuado y rotulado "acción inexistente"
                        // para que el usuario pueda quitarla (R3.4,
                        // acción-no-declarada). No bloquea render ni guardado.
                        if (!allowed) {
                          if (!selected.has(key)) {
                            return (
                              <TableCell
                                key={action}
                                className="text-center text-muted-foreground"
                                aria-hidden="true"
                              >
                                —
                              </TableCell>
                            );
                          }
                          const danglingId = `perm-${key}`;
                          const danglingLabel = `${moduleActionLabel(action, customActionLabels)} ${mod.name} (acción inexistente)`;
                          return (
                            <TableCell
                              key={action}
                              className="text-center opacity-60"
                              title="Acción inexistente en el módulo"
                            >
                              <input
                                id={danglingId}
                                type="checkbox"
                                className="size-4 cursor-pointer accent-primary"
                                checked={selected.has(key)}
                                onChange={() => toggleCell(resource, action)}
                                aria-label={danglingLabel}
                              />
                            </TableCell>
                          );
                        }
                        const checkboxId = `perm-${key}`;
                        const checkboxLabel = `${moduleActionLabel(action, customActionLabels)} ${mod.name}`;
                        return (
                          <TableCell key={action} className="text-center">
                            <input
                              id={checkboxId}
                              type="checkbox"
                              className="size-4 cursor-pointer accent-primary"
                              checked={selected.has(key)}
                              onChange={() => toggleCell(resource, action)}
                              aria-label={checkboxLabel}
                            />
                          </TableCell>
                        );
                      })}
                    </TableRow>
                  );
                })}

                {/* Colgantes: módulos cuyo slug no está en el catálogo. Fila
                    atenuada con sus acciones marcadas y des-marcables. */}
                {danglingModules.map(({ slug, actions }) => {
                  const resource = moduleResource(slug);
                  return (
                    <TableRow key={`dangling-${slug}`} className="opacity-60">
                      <TableCell className="font-medium">
                        {slug} (módulo inexistente)
                      </TableCell>
                      {moduleActionColumns.map((action) => {
                        if (!actions.has(action)) {
                          return (
                            <TableCell
                              key={action}
                              className="text-center text-muted-foreground"
                              aria-hidden="true"
                            >
                              —
                            </TableCell>
                          );
                        }
                        const key = cellKey(resource, action);
                        const checkboxId = `perm-${key}`;
                        const checkboxLabel = `${moduleActionLabel(action, customActionLabels)} ${slug}`;
                        return (
                          <TableCell key={action} className="text-center">
                            <input
                              id={checkboxId}
                              type="checkbox"
                              className="size-4 cursor-pointer accent-primary"
                              checked={selected.has(key)}
                              onChange={() => toggleCell(resource, action)}
                              aria-label={checkboxLabel}
                            />
                          </TableCell>
                        );
                      })}
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          </div>
        </fieldset>
      ) : null}

      <div className="flex items-center justify-end gap-2">
        <Button
          type="button"
          variant="ghost"
          onClick={onCancel}
          disabled={isSubmitting}
        >
          Cancelar
        </Button>
        <Button type="submit" disabled={isSubmitting}>
          {isSubmitting ? (
            <>
              <Loader2 className="size-4 animate-spin" aria-hidden="true" />
              Guardando...
            </>
          ) : (
            submitLabel
          )}
        </Button>
      </div>
    </form>
  );
}

export default RoleForm;
