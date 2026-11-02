"use client";

/**
 * Render dinámico de un módulo del constructor (module-builder, tareas 14.1-14.3).
 *
 * Dado el `slug` de un módulo, carga su Definición (useModuleDef), genera las
 * columnas de la tabla desde el `displayMapping`, lista las filas con
 * `useModuleList` sobre `ResourceTable`, y materializa las acciones del módulo
 * como botones de cabecera (crear/global) y de fila (editar/eliminar/custom).
 * Las acciones con campos de entrada abren un formulario autogenerado; las
 * destructivas piden confirmación; las que no tienen campos ni son destructivas
 * se ejecutan de un clic (Req. 9, 10). La visibilidad de cada botón respeta
 * `canModule` (Req. 11.4); la seguridad real la aplica el backend.
 */
import * as React from "react";
import { Loader2, Plus, RefreshCw } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  ResourceTable,
  type PaginationMeta,
  type ResourceColumn,
  type SortState,
} from "@/components/resource-table";
import { FeedbackBanner } from "@/components/feedback-banner";
import { toast } from "@/lib/use-toast";
import { ScrollToTopButton } from "@/components/scroll-to-top-button";
import { useScrollToTop } from "@/lib/use-scroll-to-top";
import { getCountry, subscribeCountry } from "@/lib/country";
import { canModule } from "@/lib/permissions";
import { useModulePermissions } from "@/lib/use-permissions";
import type { ListParams } from "@/types/api";
import {
  useModuleAction,
  useModuleActionList,
  useModuleColumnOptions,
  useModuleDef,
  useModuleFieldOptions,
  useModuleList,
  useRefreshPermissions,
  type ModuleRow,
} from "@/features/modules/hooks";
import type {
  DisplayColumn,
  FieldSpec,
  ModuleAction,
  ModuleDefinition,
  SelectOption,
} from "@/features/modules/types";
import {
  ActionFormDialog,
  type ColumnField,
} from "@/features/modules/components/action-form-dialog";
import { ConfirmActionDialog } from "@/features/modules/components/confirm-action-dialog";

const DEFAULT_RECORDS_PER_PAGE = 25;

/** Lee el país seleccionado de forma reactiva y SSR-safe. */
function useSelectedCountry(): string {
  return React.useSyncExternalStore(
    subscribeCountry,
    () => getCountry() ?? "",
    () => "",
  );
}

/**
 * Extrae TODAS las columnas declaradas del display_mapping, ordenadas, con sus
 * banderas normalizadas con los defaults de compatibilidad (R8): visible ausente
 * ⇒ true, isPrimaryKey ausente ⇒ false, editable ausente ⇒ true salvo PK. Incluye
 * las no visibles (se usan para edición/PK); el filtrado a visibles para la tabla
 * se hace aparte.
 */
function columnsFromMapping(def: ModuleDefinition | undefined): DisplayColumn[] {
  if (!def) {
    return [];
  }
  const raw = (def.displayMapping as { columns?: unknown }).columns;
  if (!Array.isArray(raw)) {
    return [];
  }
  const cols = raw
    .filter((c): c is DisplayColumn => Boolean(c) && typeof c === "object" && "field" in c)
    .map((c) => {
      const isPrimaryKey = Boolean(c.isPrimaryKey);
      return {
        field: String(c.field),
        label: String(c.label ?? c.field),
        order: typeof c.order === "number" ? c.order : 0,
        format: typeof c.format === "string" ? c.format : undefined,
        visible: c.visible === undefined ? true : Boolean(c.visible),
        isPrimaryKey,
        editable: isPrimaryKey ? false : c.editable === undefined ? true : Boolean(c.editable),
        sourceTable: typeof c.sourceTable === "string" ? c.sourceTable : undefined,
        // Fuente de valores de la columna (FEAT-001/003): se conserva tal cual
        // para que la precedencia columna > acción pueda leerla al resolver
        // id→etiqueta en el grid y poblar el desplegable en crear/editar.
        // Ausente ⇒ undefined ⇒ comportamiento histórico idéntico.
        valueSource: c.valueSource,
      } satisfies DisplayColumn;
    });
  cols.sort((a, b) => a.order - b.order);
  return cols;
}

/**
 * Indica si un módulo es multi-tabla (tiene foreignTables o relationships en su
 * fetchMapping). Predicado espejo del backend (MultiTableSpec.IsMultiTable).
 */
export function isMultiTableDef(def: ModuleDefinition | undefined): boolean {
  if (!def) {
    return false;
  }
  const fm = def.fetchMapping as { foreignTables?: unknown; relationships?: unknown };
  const ft = Array.isArray(fm.foreignTables) ? fm.foreignTables.length : 0;
  const rel = Array.isArray(fm.relationships) ? fm.relationships.length : 0;
  return ft > 0 || rel > 0;
}

/**
 * Conjunto ordenado hijo→padre que una acción delete multi-tabla borrará:
 * `top` + sus descendientes-hijo FK (los padres FK de `top` NO se incluyen).
 * Reimplementación de DeleteOrder del backend para la confirmación (§6.4). Lee
 * las relaciones de def.fetchMapping. `top` = action.deleteTopTable o principal.
 */
export function deleteSetForAction(
  def: ModuleDefinition | undefined,
  action: ModuleAction,
  principal: string,
): string[] {
  if (!def) {
    return [];
  }
  const rawRels = (def.fetchMapping as { relationships?: unknown }).relationships;
  if (!Array.isArray(rawRels)) {
    return [];
  }
  const fkChildren = new Map<string, string[]>();
  for (const r of rawRels as Array<Record<string, unknown>>) {
    const child = String(r.childTable ?? "");
    const parent = String(r.parentTable ?? "");
    if (child === "" || parent === "") {
      continue;
    }
    fkChildren.set(parent, [...(fkChildren.get(parent) ?? []), child]);
  }
  const top = action.deleteTopTable && action.deleteTopTable !== "" ? action.deleteTopTable : principal;
  const inSet = new Set<string>();
  const stack = [top];
  while (stack.length > 0) {
    const cur = stack.pop()!;
    if (inSet.has(cur)) continue;
    inSet.add(cur);
    for (const ch of (fkChildren.get(cur) ?? []).slice().sort()) {
      stack.push(ch);
    }
  }
  const visited = new Set<string>();
  const out: string[] = [];
  const emit = (node: string) => {
    if (visited.has(node)) return;
    visited.add(node);
    const children = (fkChildren.get(node) ?? []).filter((c) => inSet.has(c)).sort();
    for (const ch of children) emit(ch);
    out.push(node);
  };
  for (const t of Array.from(inSet).sort()) {
    emit(t);
  }
  return out;
}

/**
 * Clave con la que una columna viaja en la fila devuelta por el backend: en
 * multi-tabla es el alias "<sourceTable>__<field>" (coherente con el SELECT y el
 * motor de escritura); en N=1 es el nombre pelado "<field>". `principal` resuelve
 * el default de sourceTable (columnas sin sourceTable ⇒ principal).
 */
export function renderKeyFor(
  col: { field: string; sourceTable?: string },
  multiTable: boolean,
  principal: string,
): string {
  if (!multiTable) {
    return col.field;
  }
  const table = col.sourceTable && col.sourceTable !== "" ? col.sourceTable : principal;
  return `${table}__${col.field}`;
}

/**
 * Emparejamiento columna de LISTAR → campo+acción de una fuente DINÁMICA. Una
 * columna del grid (`DisplayColumn`) solo conoce `field`/`sourceTable`, pero la
 * fuente dinámica vive en un `FieldSpec` (`source === "dynamic"`) dentro de los
 * `inputFields` de alguna acción. Para resolver id→etiqueta en la celda hay que
 * emparejar ambos por identidad de columna, igual que `buildColumnFields`:
 *   - `renderKey`: clave con la que la celda viaja en la fila.
 *   - `field`: `FieldSpec.name` (== `col.field`) → va al endpoint /options/:field.
 *   - `actionKey`: `ModuleAction.actionKey` → va como ?action= (misma queryKey
 *     que crear/editar, de modo que TanStack comparte/deduplica la respuesta).
 */
export interface DynamicColumnBinding {
  readonly renderKey: string;
  readonly field: string;
  readonly actionKey: string;
}

/**
 * Deriva los bindings dinámicos de las columnas VISIBLES del grid. Recorre
 * `def.actions`; por cada `FieldSpec` dinámico (`source === "dynamic"` y
 * `dynamicSource != null`) busca la `displayColumn` visible que empate por
 * `(field, sourceTable)` —misma relación que usa `buildColumnFields`— y emite un
 * binding con el `actionKey` de esa acción. Reglas:
 *   - Solo columnas visibles (`displayColumns`).
 *   - Dedupe por `renderKey`: como máximo UN binding por columna, prefiriendo la
 *     primera acción `create`/`edit` que la declare (orden determinista de
 *     `def.actions`); el catálogo id→label no depende de la acción.
 *   - Columnas sin `FieldSpec` dinámico NO entran ⇒ se renderizan igual que hoy.
 * Si la lista queda vacía, el padre no monta ningún fetcher ⇒ cero peticiones.
 */
export function dynamicColumnBindings(
  def: ModuleDefinition | undefined,
  displayColumns: DisplayColumn[],
  multiTable: boolean,
  principal: string,
): DynamicColumnBinding[] {
  if (!def) {
    return [];
  }
  const actions = Array.isArray(def.actions) ? def.actions : [];
  // Orden determinista: primero create/edit, luego el resto (estable por índice).
  const preferScore = (a: ModuleAction): number =>
    a.kind === "create" || a.kind === "edit" ? 0 : 1;
  const ordered = actions
    .map((a, i) => ({ a, i }))
    .sort((x, y) => preferScore(x.a) - preferScore(y.a) || x.i - y.i)
    .map((e) => e.a);

  const byRenderKey = new Map<string, DynamicColumnBinding>();
  for (const action of ordered) {
    for (const f of action.inputFields ?? []) {
      if (f.source !== "dynamic" || f.dynamicSource == null) {
        continue;
      }
      const col = displayColumns.find(
        (c) =>
          c.field === f.name &&
          (f.sourceTable ?? principal) === (c.sourceTable ?? principal),
      );
      if (!col) {
        continue;
      }
      const renderKey = renderKeyFor(col, multiTable, principal);
      if (byRenderKey.has(renderKey)) {
        continue; // dedupe: una columna no se resuelve dos veces.
      }
      byRenderKey.set(renderKey, { renderKey, field: f.name, actionKey: action.actionKey });
    }
  }
  return Array.from(byRenderKey.values());
}

/**
 * Origen resuelto de los valores de UNA columna visible del grid, aplicando la
 * precedencia columna > campo de acción (FEAT-004):
 *   - "column-static": la columna trae una lista ESTÁTICA (valueSource.kind ===
 *     "static" con opciones); el mapa id→etiqueta se arma en memoria SIN fetch.
 *   - "column-dynamic": la columna trae una lista DINÁMICA (valueSource.kind ===
 *     "dynamic"); se resuelve con UN fetch por columna (useModuleColumnOptions).
 *   - "action-dynamic": la columna NO declara fuente propia pero empareja con un
 *     FieldSpec dinámico de alguna acción (comportamiento histórico de
 *     dynamicColumnBindings); se resuelve con useModuleFieldOptions.
 */
export type ColumnSourceOrigin =
  | { readonly kind: "column-static"; readonly options: ReadonlyArray<SelectOption> }
  | { readonly kind: "column-dynamic" }
  | { readonly kind: "action-dynamic"; readonly actionKey: string };

/**
 * Binding de una columna visible del grid a su fuente de valores resuelta. La
 * clave `renderKey` es con la que la celda viaja en la fila (alias
 * `<tabla>__<col>` en multi-tabla, nombre pelado en N=1), de modo que columnas
 * HOMÓNIMAS de tablas distintas no colisionan. `field`/`sourceTable` identifican
 * la columna para el endpoint de opciones por columna.
 */
export interface ColumnSourceBinding {
  readonly renderKey: string;
  readonly field: string;
  readonly sourceTable?: string;
  readonly origin: ColumnSourceOrigin;
}

/**
 * Deriva, por cada columna VISIBLE, el binding de su fuente de valores aplicando
 * la precedencia columna > campo de acción (FEAT-004). Generaliza
 * `dynamicColumnBindings`: ahora la fuente puede venir de la propia columna
 * (`valueSource`) y manda sobre el FieldSpec dinámico de una acción. Reglas por
 * columna:
 *   - valueSource.kind === "static" con opciones ⇒ "column-static" (mapa en
 *     memoria, sin fetch).
 *   - valueSource.kind === "dynamic" ⇒ "column-dynamic" (1 fetch por columna).
 *   - si no hay valueSource utilizable y existe un FieldSpec dinámico emparejable
 *     por (field, sourceTable) ⇒ "action-dynamic" (lógica histórica).
 *   - si no ⇒ no se emite binding (la columna se renderiza igual que hoy).
 * Dedupe por `renderKey`: como máximo UN binding por columna.
 */
export function columnValueSourceBindings(
  def: ModuleDefinition | undefined,
  displayColumns: DisplayColumn[],
  multiTable: boolean,
  principal: string,
): ColumnSourceBinding[] {
  if (!def) {
    return [];
  }
  // Fallback de acción (precedencia más baja): reutiliza la lógica histórica de
  // emparejamiento columna↔FieldSpec dinámico, keyeada por renderKey.
  const actionBindings = new Map(
    dynamicColumnBindings(def, displayColumns, multiTable, principal).map(
      (b) => [b.renderKey, b] as const,
    ),
  );

  const byRenderKey = new Map<string, ColumnSourceBinding>();
  for (const col of displayColumns) {
    const renderKey = renderKeyFor(col, multiTable, principal);
    if (byRenderKey.has(renderKey)) {
      continue; // dedupe: una columna no se resuelve dos veces.
    }
    const vs = col.valueSource;
    // Precedencia 1: lista ESTÁTICA de la columna (sin fetch).
    if (vs && vs.kind === "static" && vs.options && vs.options.length > 0) {
      byRenderKey.set(renderKey, {
        renderKey,
        field: col.field,
        sourceTable: col.sourceTable,
        origin: { kind: "column-static", options: vs.options },
      });
      continue;
    }
    // Precedencia 2: lista DINÁMICA de la columna (1 fetch por columna).
    if (vs && vs.kind === "dynamic" && vs.dynamicSource != null) {
      byRenderKey.set(renderKey, {
        renderKey,
        field: col.field,
        sourceTable: col.sourceTable,
        origin: { kind: "column-dynamic" },
      });
      continue;
    }
    // Precedencia 3: FieldSpec dinámico de una acción (comportamiento histórico).
    const ab = actionBindings.get(renderKey);
    if (ab) {
      byRenderKey.set(renderKey, {
        renderKey,
        field: col.field,
        sourceTable: col.sourceTable,
        origin: { kind: "action-dynamic", actionKey: ab.actionKey },
      });
      continue;
    }
    // Sin fuente ni FieldSpec emparejable ⇒ no se emite binding.
  }
  return Array.from(byRenderKey.values());
}

/**
 * Mapa nombre-de-campo → etiqueta legible para el formulario autogenerado desde
 * inputFields (crear / acciones custom / SELECT). Empareja cada FieldSpec con su
 * DisplayColumn por el MISMO criterio que buildColumnFields/dynamicColumnBindings:
 * (field === name) y, en multi-tabla, (f.sourceTable ?? principal) === (c.sourceTable ?? principal).
 * La etiqueta = col.label (que ya trae el fallback a field vía columnsFromMapping);
 * si ningún campo empareja con una columna, ese name NO entra en el mapa y el
 * diálogo degrada a mostrar f.name (el fallback vive en un único sitio: el diálogo).
 * Clave del mapa = f.name.
 */
export function fieldLabelsForAction(
  fields: ReadonlyArray<FieldSpec>,
  columns: ReadonlyArray<DisplayColumn>,
  principal: string,
): Record<string, string> {
  const out: Record<string, string> = {};
  for (const f of fields) {
    const col = columns.find(
      (c) =>
        c.field === f.name &&
        (f.sourceTable ?? principal) === (c.sourceTable ?? principal),
    );
    if (col) {
      out[f.name] = col.label;
    }
  }
  return out;
}

/**
 * Origen de opciones de una columna al CREAR/EDITAR, aplicando la precedencia
 * columna > FieldSpec de acción (FEAT-004). Devuelve:
 *   - "static": lista estática (de la columna o de un FieldSpec select); el
 *     desplegable se puebla con `options` y opcionalmente `defaultValue`.
 *   - "dynamic": lista DINÁMICA de la COLUMNA (valueSource.kind === "dynamic");
 *     se carga por columna con useModuleColumnOptions (desambigua por
 *     `sourceTable`).
 *   - "action-dynamic": lista DINÁMICA de un FieldSpec de acción (ruta histórica
 *     con useModuleFieldOptions sobre `field`).
 *   - "action-select": lista ESTÁTICA de un FieldSpec select (opciones directas).
 *   - "plain": control editable normal (sin opciones).
 * La precedencia de la columna se evalúa PRIMERO; solo si la columna no declara
 * una fuente utilizable se mira el FieldSpec emparejado por
 * (f.name === col.field) && ((f.sourceTable ?? principal) === (col.sourceTable ?? principal)).
 */
export type ColumnOptionSource =
  | { readonly mode: "static"; readonly options: ReadonlyArray<SelectOption>; readonly defaultValue?: string }
  | { readonly mode: "dynamic"; readonly sourceTable?: string }
  | { readonly mode: "action-dynamic"; readonly field: string }
  | { readonly mode: "action-select"; readonly field: string }
  | { readonly mode: "plain" };

export function resolveColumnOptionSource(
  col: DisplayColumn,
  action: ModuleAction,
  principal: string,
): ColumnOptionSource {
  const vs = col.valueSource;
  // Precedencia columna: estática en memoria o dinámica por columna.
  if (vs && vs.kind === "static" && vs.options && vs.options.length > 0) {
    return { mode: "static", options: vs.options };
  }
  if (vs && vs.kind === "dynamic" && vs.dynamicSource != null) {
    return { mode: "dynamic", sourceTable: col.sourceTable };
  }
  // Fallback: FieldSpec de la acción emparejado por (name, sourceTable).
  const spec = (action.inputFields ?? []).find(
    (f) =>
      f.name === col.field &&
      (f.sourceTable ?? principal) === (col.sourceTable ?? principal),
  );
  if (spec?.source === "dynamic" && spec.dynamicSource != null) {
    return { mode: "action-dynamic", field: spec.name };
  }
  if (spec?.source === "select") {
    return { mode: "action-select", field: spec.name };
  }
  return { mode: "plain" };
}

/** Tipo de campo de entrada inferido del formato de una columna. */
function fieldTypeFromFormat(format?: string): "string" | "number" | "boolean" | "date" {
  return format === "date" || format === "datetime" ? "date" : "string";
}

/** Formatea un valor de celda según el formato declarado de la columna. */
function formatValue(value: unknown, format?: string): React.ReactNode {
  if (value === null || value === undefined || value === "") {
    return "—";
  }
  if (format === "datetime" || format === "date") {
    const d = new Date(String(value));
    if (!Number.isNaN(d.getTime())) {
      return format === "date" ? d.toLocaleDateString("es-MX") : d.toLocaleString("es-MX");
    }
  }
  if (typeof value === "boolean") {
    return value ? "Sí" : "No";
  }
  if (typeof value === "object") {
    return JSON.stringify(value);
  }
  return String(value);
}

/**
 * Botón "Recargar permisos": reemite el JWT con los permisos actuales. Es
 * SIEMPRE visible (no gated por canModule) y NO es la acción de permiso
 * "Recargar" (que recarga la caché de datos). Se extrae para reutilizar la MISMA
 * marca en la cabecera del camino feliz y en el estado de error (defError/!def),
 * de modo que el usuario siempre pueda recuperar permisos desde esta pantalla.
 */
function RefreshPermissionsButton({
  isPending,
  onClick,
}: {
  readonly isPending: boolean;
  readonly onClick: () => void;
}) {
  return (
    <Button
      type="button"
      variant="outline"
      size="sm"
      onClick={onClick}
      disabled={isPending}
    >
      {isPending ? (
        <>
          <Loader2 className="size-4 animate-spin" aria-hidden="true" />
          Recargando permisos…
        </>
      ) : (
        <>
          <RefreshCw className="size-4" aria-hidden="true" />
          Recargar permisos
        </>
      )}
    </Button>
  );
}

/** Estado de carga de las opciones de UN campo dinámico. */
interface DynamicOptionsEntry {
  readonly options: ReadonlyArray<SelectOption>;
  readonly loading: boolean;
  readonly error: boolean;
}

/** Mapa nombre de campo → estado de sus opciones dinámicas. */
type DynamicOptionsMap = Record<string, DynamicOptionsEntry>;

/**
 * Carga las opciones de UN campo dinámico (un hook por campo, componente aparte
 * para respetar las reglas de hooks con una cantidad variable de campos) y las
 * reporta al padre vía `onLoaded`. No renderiza nada. El padre inyecta las
 * opciones resueltas en el FieldSpec/ColumnField que dibuja el formulario.
 */
function FieldOptionsFetcher({
  slug,
  field,
  actionKey,
  onLoaded,
}: {
  readonly slug: string;
  readonly field: string;
  readonly actionKey: string;
  readonly onLoaded: (field: string, entry: DynamicOptionsEntry) => void;
}): null {
  const { data, isLoading, isError } = useModuleFieldOptions(slug, field, actionKey);
  React.useEffect(() => {
    // Invariante id SIEMPRE string: value = String(id), label = String(label).
    const options: SelectOption[] = (data ?? []).map((opt) => ({
      value: String(opt.id),
      label: String(opt.label),
    }));
    onLoaded(field, { options, loading: isLoading, error: isError });
    // Se reporta en cada cambio de estado de la query.
  }, [field, data, isLoading, isError, onLoaded]);
  return null;
}

/**
 * Carga las opciones de UNA columna DINÁMICA por su propia fuente de valores
 * (valueSource.kind === "dynamic") con `useModuleColumnOptions` y las reporta al
 * padre vía `onLoaded` KEYEADAS POR renderKey (no por f.name), para poblar el
 * desplegable de crear/editar sin colisionar columnas homónimas. Paralelo a
 * `FieldOptionsFetcher` (que usa la queryKey de acción). No renderiza nada.
 */
function ColumnOptionsFetcher({
  slug,
  field,
  sourceTable,
  renderKey,
  onLoaded,
}: {
  readonly slug: string;
  readonly field: string;
  readonly sourceTable?: string;
  readonly renderKey: string;
  readonly onLoaded: (renderKey: string, entry: DynamicOptionsEntry) => void;
}): null {
  const { data, isLoading, isError } = useModuleColumnOptions(slug, field, sourceTable);
  React.useEffect(() => {
    // Invariante id SIEMPRE string: value = String(id), label = String(label).
    const options: SelectOption[] = (data ?? []).map((opt) => ({
      value: String(opt.id),
      label: String(opt.label),
    }));
    onLoaded(renderKey, { options, loading: isLoading, error: isError });
  }, [renderKey, data, isLoading, isError, onLoaded]);
  return null;
}

/**
 * Reporta al padre un mapa id→etiqueta armado EN MEMORIA a partir de una lista
 * ESTÁTICA (columna con valueSource.kind === "static"). No hace fetch: el mapa se
 * deriva de `options` en un `useEffect` y se reporta vía `onLoaded`. La identidad
 * de `options` es estable (sale de la definición), por lo que el efecto no se
 * redispara en cada render. No renderiza nada, igual que los demás fetchers.
 */
function ColumnStaticLabelFetcher({
  options,
  renderKey,
  onLoaded,
}: {
  readonly options: ReadonlyArray<SelectOption>;
  readonly renderKey: string;
  readonly onLoaded: (renderKey: string, map: ReadonlyMap<string, string>) => void;
}): null {
  React.useEffect(() => {
    // Invariante id SIEMPRE string: clave normalizada con String(...).
    const map = new Map(options.map((o) => [String(o.value), String(o.label)]));
    onLoaded(renderKey, map);
  }, [renderKey, options, onLoaded]);
  return null;
}

/**
 * Carga las opciones de UNA columna DINÁMICA del grid por su propia fuente de
 * valores (valueSource.kind === "dynamic") con `useModuleColumnOptions` y reporta
 * al padre un mapa id→etiqueta. Un hook por columna; comparte caché (misma
 * queryKey `moduleKeys.columnOptions(slug, field, sourceTable)`) con el
 * desplegable de crear/editar. No renderiza nada.
 */
function ColumnDynamicLabelFetcher({
  slug,
  field,
  sourceTable,
  renderKey,
  onLoaded,
}: {
  readonly slug: string;
  readonly field: string;
  readonly sourceTable?: string;
  readonly renderKey: string;
  readonly onLoaded: (renderKey: string, map: ReadonlyMap<string, string>) => void;
}): null {
  const { data } = useModuleColumnOptions(slug, field, sourceTable);
  React.useEffect(() => {
    // Invariante id SIEMPRE string: clave y valor normalizados con String(...).
    const map = new Map<string, string>();
    for (const opt of data ?? []) {
      map.set(String(opt.id), String(opt.label));
    }
    onLoaded(renderKey, map);
  }, [renderKey, data, onLoaded]);
  return null;
}

/**
 * Carga las opciones de UNA columna del grid resuelta por un FieldSpec DINÁMICO
 * de acción (ruta histórica) con `useModuleFieldOptions` y reporta al padre un
 * mapa id→etiqueta. Un hook por columna; comparte caché (misma queryKey
 * `moduleKeys.options(slug, field, actionKey)`) con el desplegable de
 * crear/editar. No renderiza nada.
 */
function ColumnActionLabelFetcher({
  slug,
  field,
  actionKey,
  renderKey,
  onLoaded,
}: {
  readonly slug: string;
  readonly field: string;
  readonly actionKey: string;
  readonly renderKey: string;
  readonly onLoaded: (renderKey: string, map: ReadonlyMap<string, string>) => void;
}): null {
  const { data } = useModuleFieldOptions(slug, field, actionKey);
  React.useEffect(() => {
    // Invariante id SIEMPRE string: clave y valor normalizados con String(...).
    const map = new Map<string, string>();
    for (const opt of data ?? []) {
      map.set(String(opt.id), String(opt.label));
    }
    onLoaded(renderKey, map);
  }, [renderKey, data, onLoaded]);
  return null;
}

/**
 * Despacha la carga del mapa id→etiqueta de UNA columna del grid según su
 * `ColumnSourceBinding.origin` (FEAT-004), aplicando la precedencia ya resuelta
 * en `columnValueSourceBindings`:
 *   - "column-static": mapa en memoria sin fetch.
 *   - "column-dynamic": 1 fetch por columna (useModuleColumnOptions).
 *   - "action-dynamic": fetch por el FieldSpec de acción (useModuleFieldOptions).
 * Cada rama monta un componente leaf ESTABLE (un hook fijo por rama), de modo que
 * el `kind` del binding de una columna no cambia entre renders y las reglas de
 * hooks se respetan. No renderiza nada.
 */
function ColumnSourceLabelFetcher({
  slug,
  binding,
  onLoaded,
}: {
  readonly slug: string;
  readonly binding: ColumnSourceBinding;
  readonly onLoaded: (renderKey: string, map: ReadonlyMap<string, string>) => void;
}): React.ReactElement {
  const { origin, renderKey, field, sourceTable } = binding;
  if (origin.kind === "column-static") {
    return (
      <ColumnStaticLabelFetcher
        options={origin.options}
        renderKey={renderKey}
        onLoaded={onLoaded}
      />
    );
  }
  if (origin.kind === "column-dynamic") {
    return (
      <ColumnDynamicLabelFetcher
        slug={slug}
        field={field}
        sourceTable={sourceTable}
        renderKey={renderKey}
        onLoaded={onLoaded}
      />
    );
  }
  return (
    <ColumnActionLabelFetcher
      slug={slug}
      field={field}
      actionKey={origin.actionKey}
      renderKey={renderKey}
      onLoaded={onLoaded}
    />
  );
}

export interface ModuleViewProps {
  readonly slug: string;
}

export function ModuleView({ slug }: ModuleViewProps) {
  const perms = useModulePermissions();
  const country = useSelectedCountry();
  const { topRef, scrollToTop } = useScrollToTop();

  const [page, setPage] = React.useState(1);
  const [recordsPerPage, setRecordsPerPage] = React.useState(DEFAULT_RECORDS_PER_PAGE);
  const [sort, setSort] = React.useState<SortState | null>(null);

  // Filtro de listado por acción SELECT: cuando no es null, el grid se alimenta
  // de la ruta dedicada (useModuleActionList) con los valores capturados por el
  // usuario al confirmar el formulario de la acción; cuando es null, del listado
  // base (useModuleList). "Limpiar filtro" o un "Listar" base vuelven a null.
  const [activeFilter, setActiveFilter] = React.useState<{
    actionKey: string;
    input: Record<string, unknown>;
  } | null>(null);

  // Acción en curso (formulario o confirmación) y la fila sobre la que opera.
  const [activeAction, setActiveAction] = React.useState<ModuleAction | null>(null);
  const [activeRow, setActiveRow] = React.useState<ModuleRow | null>(null);
  const [mode, setMode] = React.useState<"form" | "confirm" | null>(null);
  // El resultado de las acciones (éxito/error) y del botón "Recargar permisos"
  // se muestra ahora como TOAST flotante (siempre visible sin importar el
  // scroll del grid), no como banner inline. Ver `toast.*` más abajo.
  // Opciones cargadas de los campos DINÁMICOS de la acción activa (nombre de
  // campo → {options, loading, error}). Lo llenan los FieldOptionsFetcher.
  const [dynamicOptions, setDynamicOptions] = React.useState<DynamicOptionsMap>({});

  const onDynamicOptionsLoaded = React.useCallback(
    (field: string, entry: DynamicOptionsEntry) => {
      setDynamicOptions((prev) => {
        const curr = prev[field];
        if (
          curr &&
          curr.loading === entry.loading &&
          curr.error === entry.error &&
          curr.options === entry.options
        ) {
          return prev;
        }
        return { ...prev, [field]: entry };
      });
    },
    [],
  );

  // Mapas id→etiqueta de las columnas DINÁMICAS del grid (renderKey → mapa). Los
  // llenan los ColumnLabelFetcher con las opciones del endpoint (misma caché que
  // crear/editar). La celda lee de aquí para mostrar la etiqueta en vez del id.
  const [dynamicLabels, setDynamicLabels] = React.useState<
    Record<string, ReadonlyMap<string, string>>
  >({});

  const onColumnLabelsLoaded = React.useCallback(
    (renderKey: string, map: ReadonlyMap<string, string>) => {
      setDynamicLabels((prev) => {
        if (prev[renderKey] === map) {
          return prev;
        }
        return { ...prev, [renderKey]: map };
      });
    },
    [],
  );

  // Opciones cargadas de las columnas DINÁMICAS por su propia fuente de valores
  // (valueSource.kind === "dynamic"), keyeadas por renderKey para NO colisionar
  // columnas homónimas (a diferencia de dynamicOptions, keyeado por f.name que sí
  // colisiona entre tablas). Lo llena un fetcher por columna (useModuleColumnOptions)
  // montado SOLO con el formulario de crear/editar abierto. Paralelo a dynamicOptions.
  const [columnOptionsByRenderKey, setColumnOptionsByRenderKey] =
    React.useState<DynamicOptionsMap>({});

  const onColumnOptionsLoaded = React.useCallback(
    (renderKey: string, entry: DynamicOptionsEntry) => {
      setColumnOptionsByRenderKey((prev) => {
        const curr = prev[renderKey];
        if (
          curr &&
          curr.loading === entry.loading &&
          curr.error === entry.error &&
          curr.options === entry.options
        ) {
          return prev;
        }
        return { ...prev, [renderKey]: entry };
      });
    },
    [],
  );

  const { data: def, isLoading: defLoading, isError: defError } = useModuleDef(slug);

  const params: ListParams = React.useMemo(
    () => ({ page, recordsPerPage, orderBy: sort?.field, order: sort?.order }),
    [page, recordsPerPage, sort],
  );
  // Listado base: activo cuando NO hay un filtro de acción SELECT en curso.
  const baseList = useModuleList(slug, params, {
    placeholderData: (prev) => prev,
    enabled: Boolean(def) && activeFilter == null,
  });
  // Listado filtrado por acción SELECT (ruta dedicada): activo cuando hay un
  // activeFilter. La paginación/orden viaja en `params`; los valores de filtro
  // en el cuerpo (activeFilter.input).
  const actionList = useModuleActionList(
    slug,
    activeFilter?.actionKey ?? "",
    activeFilter?.input ?? {},
    params,
    {
      placeholderData: (prev) => prev,
      enabled: Boolean(def) && activeFilter != null,
    },
  );
  // Deriva el estado del grid de la query activa.
  const { data, isLoading, isError, error } =
    activeFilter != null ? actionList : baseList;

  const actionMutation = useModuleAction(slug, activeAction?.actionKey ?? "");

  // Reemisión del token con los permisos actuales (botón "Recargar permisos").
  const refreshPermissions = useRefreshPermissions({
    onSuccess: () => {
      toast.success("Permisos actualizados");
    },
    onError: (err) => {
      toast.error(err?.message ?? "No se pudieron recargar los permisos.");
    },
  });

  const onRefreshPermissions = React.useCallback(() => {
    refreshPermissions.mutate();
  }, [refreshPermissions]);

  // Todas las columnas de la definición (para edición/PK) y, aparte, solo las
  // visibles al listar (para la tabla, R3.3/R3.5).
  const allColumns = React.useMemo(() => columnsFromMapping(def), [def]);
  const displayColumns = React.useMemo(
    () => allColumns.filter((c) => c.visible !== false),
    [allColumns],
  );

  // MULTI-TABLA: discriminante y tabla principal (para resolver los alias
  // <tabla>__<col> con los que viajan las celdas y las PK).
  const multiTable = React.useMemo(() => isMultiTableDef(def), [def]);
  const principalTable = React.useMemo(() => {
    const t = (def?.fetchMapping as { table?: unknown } | undefined)?.table;
    return typeof t === "string" ? t : "";
  }, [def]);

  // Bindings de fuente de valores de las columnas VISIBLES del grid (renderKey →
  // origen resuelto con precedencia columna > acción). Si la lista queda vacía no
  // se monta ningún fetcher ⇒ cero fetches; las columnas "column-static" no hacen
  // fetch (mapa en memoria). Reemplaza al antiguo dynamicColumnBindings.
  const columnBindings = React.useMemo(
    () => columnValueSourceBindings(def, displayColumns, multiTable, principalTable),
    [def, displayColumns, multiTable, principalTable],
  );
  const bindingByRenderKey = React.useMemo(
    () => new Map(columnBindings.map((b) => [b.renderKey, b] as const)),
    [columnBindings],
  );

  // Columnas que forman la llave primaria (para el WHERE de editar/eliminar).
  // En multi-tabla la clave de cada PK es su alias <tabla>__<col>.
  const pkFields = React.useMemo(
    () =>
      allColumns
        .filter((c) => c.isPrimaryKey)
        .map((c) => renderKeyFor(c, multiTable, principalTable)),
    [allColumns, multiTable, principalTable],
  );

  // Limpia el filtro de acción SELECT y vuelve al listado base.
  const onClearFilter = React.useCallback(() => {
    setActiveFilter(null);
    setPage(1);
  }, []);

  // Construye los campos del formulario de EDITAR COMBINANDO columnas + la
  // configuración de la acción (acciones especializadas):
  //   - columnas PK: siempre, de solo lectura, precargadas de la fila (WHERE).
  //   - columnas OBJETIVO de la acción (sus inputFields cuyo name es una columna
  //     editable no-PK): control según su origen —input (editable), select
  //     (desplegable), constant visible (solo lectura), constant oculto (no se
  //     muestra; su valor lo resuelve el backend).
  //   - si la acción NO declara inputFields: todas las editables como control
  //     capturado (compatibilidad con el comportamiento actual).
  const buildColumnFields = React.useCallback(
    (action: ModuleAction): ColumnField[] => {
      const byName = new Map(action.inputFields.map((f) => [f.name, f]));
      const hasConfig = action.inputFields.length > 0;
      const out: ColumnField[] = [];
      for (const c of allColumns) {
        const isPK = Boolean(c.isPrimaryKey);
        const isEditable = !isPK && c.editable !== false;
        // Llave primaria: NO se muestra en el formulario (evita exponer cómo se
        // construye la tabla). Su valor viaja "por debajo": composeEditInput
        // reañade cada PK desde la fila seleccionada para el WHERE del UPDATE, de
        // modo que ocultar el campo no cambia el comportamiento de escritura.
        if (isPK) {
          continue;
        }
        // Columna no editable (no PK): solo lectura, precargada de la fila.
        if (!isEditable) {
          out.push({
            field: c.field,
            label: c.label,
            type: fieldTypeFromFormat(c.format),
            readOnly: true,
            required: false,
          });
          continue;
        }
        // Columna editable: ¿es objetivo de la acción?
        const spec = byName.get(c.field);
        if (hasConfig && !spec) {
          // La acción no toca esta columna: no se muestra ni se envía.
          continue;
        }
        if (spec?.source === "constant") {
          if (spec.hidden) {
            continue; // oculta: el backend resuelve su valor desde constValue
          }
          // Constante visible: solo lectura.
          out.push({
            field: c.field,
            label: c.label,
            type: fieldTypeFromFormat(c.format),
            readOnly: true,
            required: false,
          });
          continue;
        }
        // Precedencia columna > acción (FEAT-004): la fuente de valores de la
        // COLUMNA manda sobre el FieldSpec de la acción para poblar el desplegable.
        const optionSource = resolveColumnOptionSource(c, action, principalTable);
        if (optionSource.mode === "static") {
          // Lista estática de la columna: opciones directas (sin fetch).
          out.push({
            field: c.field,
            label: c.label,
            type: fieldTypeFromFormat(c.format),
            readOnly: false,
            required: spec?.required ?? false,
            options: optionSource.options,
            defaultValue: optionSource.defaultValue,
          });
          continue;
        }
        if (optionSource.mode === "action-select") {
          out.push({
            field: c.field,
            label: c.label,
            type: fieldTypeFromFormat(c.format),
            readOnly: false,
            required: spec?.required ?? false,
            options: spec?.options ?? [],
            defaultValue: spec?.defaultValue,
          });
          continue;
        }
        if (optionSource.mode === "dynamic" || optionSource.mode === "action-dynamic") {
          // Lista dinámica (de la columna o del FieldSpec de acción): se renderiza
          // como desplegable. Las opciones (y el estado de carga/error) los inyecta
          // el padre tras cargarlas del endpoint; aquí se declara `options: []`
          // para forzar el <select>.
          out.push({
            field: c.field,
            label: c.label,
            type: fieldTypeFromFormat(c.format),
            readOnly: false,
            required: spec?.required ?? false,
            options: [],
          });
          continue;
        }
        // Capturado (input) o sin config: control editable normal.
        out.push({
          field: c.field,
          label: c.label,
          type: fieldTypeFromFormat(c.format),
          readOnly: false,
          required: spec?.required ?? false,
        });
      }
      return out;
    },
    // Vuelve a depender de principalTable: resolveColumnOptionSource empareja el
    // FieldSpec por (name, sourceTable) usando la tabla principal, de modo que el
    // emparejamiento cambia si cambia la principal (que a su vez deriva del modo
    // tabla). No se incluye multiTable porque principalTable ya refleja ese cambio.
    [allColumns, principalTable],
  );

  // Compone el input final de una acción de editar: valores editables del
  // formulario + valores de las columnas PK tomados de la fila (garantiza el
  // valor del WHERE, R6.6). Las columnas no editables no-PK no se incluyen.
  const composeEditInput = React.useCallback(
    (formInput: Record<string, unknown>, row: ModuleRow | null): Record<string, unknown> => {
      const out: Record<string, unknown> = { ...formInput };
      if (row) {
        for (const pk of pkFields) {
          out[pk] = row[pk];
        }
      }
      return out;
    },
    [pkFields],
  );
  const rowActions = React.useMemo(
    () => (def?.actions ?? []).filter((a) => a.placement === "row"),
    [def],
  );
  const headerActions = React.useMemo(
    () => (def?.actions ?? []).filter((a) => a.placement === "header"),
    [def],
  );

  const closeDialogs = React.useCallback(() => {
    setActiveAction(null);
    setActiveRow(null);
    setMode(null);
    setDynamicOptions({});
    setColumnOptionsByRenderKey({});
    actionMutation.reset();
  }, [actionMutation]);

  // Tras ejecutar una acción con éxito: cierra el diálogo y muestra un TOAST de
  // confirmación con la etiqueta de la acción ejecutada.
  const onActionSuccess = React.useCallback(
    (action: ModuleAction) => {
      toast.success(`La acción "${action.label}" se ejecutó correctamente.`);
      closeDialogs();
    },
    [closeDialogs],
  );

  // Resuelve los valores que vienen de la fila (campos con source "row"): cada
  // uno toma el valor de la columna `rowField` declarada EXPLÍCITAMENTE por el
  // usuario (p. ej. el parámetro "plan_id" se llena con la columna "id"). No se
  // adivina nada: si no hay rowField declarado, el campo queda sin valor.
  const resolveRowValues = React.useCallback(
    (action: ModuleAction, row: ModuleRow | null): Record<string, unknown> => {
      const out: Record<string, unknown> = {};
      if (!row) {
        return out;
      }
      for (const f of action.inputFields) {
        if (f.source === "row" && f.rowField) {
          out[f.name] = row[f.rowField];
        }
      }
      return out;
    },
    [],
  );

  // Una acción SELECT (o la acción de listar) NO escribe: FILTRA el listado. Al
  // confirmar su formulario, sus campos llenos alimentan la ruta dedicada de
  // lectura en lugar de la mutación de escritura.
  const isSelectAction = React.useCallback(
    (action: ModuleAction): boolean =>
      action.sqlOp === "SELECT" || action.kind === "list",
    [],
  );

  // Aplica (o limpia) el filtro de una acción SELECT: si no hay ningún valor
  // efectivo (todos vacíos/espacios) vuelve al listado base; si hay al menos uno,
  // alimenta el grid con la ruta dedicada. Siempre cierra el diálogo y reinicia a
  // la página 1.
  const applySelectFilter = React.useCallback(
    (action: ModuleAction, input: Record<string, unknown>) => {
      const hasValue = Object.values(input).some(
        (v) => v != null && String(v).trim() !== "",
      );
      setActiveFilter(hasValue ? { actionKey: action.actionKey, input } : null);
      setPage(1);
      closeDialogs();
    },
    [closeDialogs],
  );

  // Lanza una acción decidiendo entre ejecución directa, formulario o confirmación:
  //
  // - Campos "row" se resuelven desde la fila por el mapeo explícito del usuario.
  // - Si quedan campos "input" (que el usuario debe escribir): abre el formulario.
  // - Si todos los campos se resolvieron (o no hay campos) y es destructiva:
  //   confirmación simple mostrando los valores a enviar.
  // - Si todo resuelto y NO destructiva: ejecución directa de un clic.
  const hasColumnPK = pkFields.length > 0;

  const launchAction = React.useCallback(
    (action: ModuleAction, row: ModuleRow | null) => {
      setActiveAction(action);
      setActiveRow(row);

      // Acción SELECT (incl. "Listar"): FILTRA el listado, no escribe. Si tiene
      // campos capturables abre el formulario (al confirmar se setea el filtro);
      // si no, aplica de inmediato (sin campos ⇒ input vacío ⇒ listado base).
      if (isSelectAction(action)) {
        const needsInput = action.inputFields.some((f) => {
          const s = f.source ?? "input";
          return s === "input" || s === "select" || s === "dynamic";
        });
        if (needsInput) {
          setMode("form");
        } else {
          setMode(null);
          applySelectFilter(action, resolveRowValues(action, row));
        }
        return;
      }

      // EDITAR con PK de columna (Opción B): el formulario se genera desde las
      // columnas (editables modificables; PK/no editables de solo lectura).
      if (action.kind === "edit" && hasColumnPK) {
        setMode("form");
        return;
      }

      // ELIMINAR con PK de columna: el WHERE se arma con las columnas PK leídas de
      // la fila; se confirma (destructiva) o se ejecuta directo.
      if (action.kind === "delete" && hasColumnPK) {
        const input: Record<string, unknown> = {};
        if (row) {
          for (const pk of pkFields) {
            input[pk] = row[pk];
          }
        }
        if (action.destructive) {
          setMode("confirm");
        } else {
          setMode(null);
          actionMutation.mutate(input, { onSuccess: () => onActionSuccess(action) });
        }
        return;
      }

      // Resto de acciones (listar/crear/custom, o edit/delete de definiciones
      // antiguas sin PK de columna): comportamiento basado en inputFields.
      // El usuario interactúa con los campos "input" (los escribe) y "select"
      // (elige una opción). Los "row" se resuelven de la fila y los "constant"
      // los fija el backend desde constValue, por lo que una acción con todos
      // sus campos constantes/row NO abre el diálogo. Una acción destructiva CON
      // campos abre el formulario (que asume el rol de confirmación vía la prop
      // `destructive`); ConfirmActionDialog queda para destructivas SIN campos.
      const needsUserInput = action.inputFields.some((f) => {
        const s = f.source ?? "input";
        return s === "input" || s === "select";
      });

      if (needsUserInput) {
        setMode("form");
        return;
      }

      // No hay nada que capturar: los valores salen de la fila (o no hay campos).
      const input = resolveRowValues(action, row);
      if (action.destructive) {
        setMode("confirm");
      } else {
        setMode(null);
        actionMutation.mutate(input, { onSuccess: () => onActionSuccess(action) });
      }
    },
    [
      actionMutation,
      onActionSuccess,
      resolveRowValues,
      hasColumnPK,
      pkFields,
      isSelectAction,
      applySelectFilter,
    ],
  );

  const columns = React.useMemo<ReadonlyArray<ResourceColumn<ModuleRow>>>(() => {
    const base: ResourceColumn<ModuleRow>[] = displayColumns.map((col) => {
      // En multi-tabla la celda viaja bajo el alias <tabla>__<col>; en N=1 bajo
      // el nombre pelado.
      const key = renderKeyFor(col, multiTable, principalTable);
      const binding = bindingByRenderKey.get(key);
      return {
        key,
        header: col.label,
        // Columna respaldada por una fuente DINÁMICA: se resuelve el id
        // almacenado a su etiqueta humana con el mapa id→label cargado del MISMO
        // endpoint de opciones que usa crear/editar (comparación SIEMPRE como
        // String). Si el mapa aún no llegó, falló, o el id no tiene opción
        // (catálogo cambiado / fuera del límite de 500), se muestra el id crudo
        // tal cual (nunca en blanco, sin bloquear el grid). Columnas no dinámicas
        // se renderizan idéntico a hoy.
        render: (row) => {
          const raw = row[key];
          if (binding) {
            const labels = dynamicLabels[key];
            if (labels && raw != null && raw !== "") {
              const hit = labels.get(String(raw));
              if (hit !== undefined) {
                return formatValue(hit, col.format);
              }
            }
            return formatValue(raw, col.format);
          }
          return formatValue(raw, col.format);
        },
      };
    });
    if (rowActions.length > 0) {
      base.push({
        key: "__actions",
        header: "Acciones",
        className: "text-right",
        render: (row) => (
          <div className="flex justify-end gap-2">
            {rowActions
              .filter((a) => canModule(perms, slug, a.permAction, country))
              .map((a) => (
                <Button
                  key={a.actionKey}
                  type="button"
                  variant={a.destructive ? "destructive" : "outline"}
                  size="sm"
                  onClick={() => launchAction(a, row)}
                >
                  {a.label}
                </Button>
              ))}
          </div>
        ),
      });
    }
    return base;
  }, [
    displayColumns,
    rowActions,
    perms,
    slug,
    country,
    launchAction,
    multiTable,
    principalTable,
    bindingByRenderKey,
    dynamicLabels,
  ]);

  // Campos DINÁMICOS de la acción activa (los que cargan opciones del endpoint).
  const dynamicFields = React.useMemo<ReadonlyArray<FieldSpec>>(
    () =>
      (activeAction?.inputFields ?? []).filter(
        (f) => f.source === "dynamic" && f.dynamicSource != null,
      ),
    [activeAction],
  );

  // Resolución de la fuente de opciones POR CAMPO para el formulario de la acción
  // activa, aplicando la precedencia columna > acción (FEAT-004). Para cada
  // columna que la acción activa toca y que resuelve a una lista DINÁMICA de la
  // propia COLUMNA (mode === "dynamic"), guarda { renderKey, field, sourceTable };
  // esas opciones se cargan por COLUMNA (columnOptionsByRenderKey) en vez de por
  // el FieldSpec de la acción. Las columnas que caen en "action-dynamic" siguen la
  // ruta histórica (dynamicOptions por f.name). Clave del mapa = nombre de columna
  // (== f.name del campo que la referencia).
  const columnDynamicByField = React.useMemo(() => {
    const out = new Map<string, { renderKey: string; field: string; sourceTable?: string }>();
    if (!activeAction) {
      return out;
    }
    for (const c of allColumns) {
      const src = resolveColumnOptionSource(c, activeAction, principalTable);
      if (src.mode === "dynamic") {
        out.set(c.field, {
          renderKey: renderKeyFor(c, multiTable, principalTable),
          field: c.field,
          sourceTable: c.sourceTable,
        });
      }
    }
    return out;
  }, [activeAction, allColumns, principalTable, multiTable]);

  // Bindings de columna-dinámica de la acción activa (para montar un
  // ColumnOptionsFetcher por columna SOLO con el formulario abierto).
  const activeColumnDynamicBindings = React.useMemo(
    () => Array.from(columnDynamicByField.values()),
    [columnDynamicByField],
  );

  // Construye la lista de opciones de un campo dinámico para el <select>,
  // aplicando la tolerancia a id COLGANTE al editar (R8): si el id actual de la
  // fila (rawId) no coincide (SIEMPRE comparado como string) con ninguna opción
  // cargada, se PREPENDE una opción de respaldo { value, label } = String(rawId)
  // y se preselecciona. Si la carga falló (502), se muestra solo esa opción.
  // La fuente de las opciones depende del mode resuelto: columna-dinámica lee de
  // columnOptionsByRenderKey (keyeado por renderKey, sin colisión de homónimas);
  // el resto (FieldSpec dinámico de acción) lee de dynamicOptions por f.name.
  const optionsForField = React.useCallback(
    (fieldName: string, rawId: unknown): ReadonlyArray<SelectOption> => {
      const colDyn = columnDynamicByField.get(fieldName);
      const entry = colDyn
        ? columnOptionsByRenderKey[colDyn.renderKey]
        : dynamicOptions[fieldName];
      const loaded = entry?.options ?? [];
      const hasRaw = rawId !== undefined && rawId !== null && String(rawId) !== "";
      if (!hasRaw) {
        return loaded;
      }
      const rawStr = String(rawId);
      const matches = loaded.some((o) => o.value === rawStr);
      if (matches) {
        return loaded;
      }
      // Id colgante (o carga fallida): opción de respaldo con el id crudo.
      return [{ value: rawStr, label: rawStr }, ...loaded];
    },
    [columnDynamicByField, columnOptionsByRenderKey, dynamicOptions],
  );

  // Devuelve el estado de carga/error de las opciones de un campo según su
  // fuente resuelta (columna-dinámica vs FieldSpec de acción).
  const optionsStatusForField = React.useCallback(
    (fieldName: string): { loading: boolean; error: boolean } => {
      const colDyn = columnDynamicByField.get(fieldName);
      const entry = colDyn
        ? columnOptionsByRenderKey[colDyn.renderKey]
        : dynamicOptions[fieldName];
      return { loading: entry?.loading ?? true, error: entry?.error ?? false };
    },
    [columnDynamicByField, columnOptionsByRenderKey, dynamicOptions],
  );

  // Nombres de campo que se renderizan como desplegable DINÁMICO (para marcar qué
  // inyectar): los del FieldSpec dinámico de la acción MÁS los de columnas con
  // fuente dinámica propia (precedencia columna, FEAT-004).
  const dynamicFieldNames = React.useMemo(() => {
    const names = new Set(dynamicFields.map((f) => f.name));
    for (const name of columnDynamicByField.keys()) {
      names.add(name);
    }
    return names;
  }, [dynamicFields, columnDynamicByField]);

  // ColumnFields del formulario de EDITAR (Opción B) con las opciones dinámicas
  // ya inyectadas (opciones cargadas + respaldo de id colgante + loading/error).
  const buildColumnFieldsWithOptions = React.useCallback(
    (action: ModuleAction): ColumnField[] => {
      const base = buildColumnFields(action);
      return base.map((cf) => {
        if (!dynamicFieldNames.has(cf.field)) {
          return cf;
        }
        const status = optionsStatusForField(cf.field);
        return {
          ...cf,
          options: optionsForField(cf.field, activeRow?.[cf.field]),
          optionsLoading: status.loading,
          optionsError: status.error,
        };
      });
    },
    [buildColumnFields, dynamicFieldNames, optionsStatusForField, optionsForField, activeRow],
  );

  // Acción con las opciones dinámicas inyectadas en sus inputFields (camino
  // crear/custom/editar-sin-PK). El id actual para el respaldo de id colgante
  // sale del valor resuelto de la fila por el mapeo del campo.
  const actionWithDynamicOptions = React.useMemo<ModuleAction | null>(() => {
    if (!activeAction) {
      return null;
    }
    // Sin campos dinámicos de acción NI columnas con fuente dinámica propia:
    // nada que inyectar.
    if (dynamicFields.length === 0 && columnDynamicByField.size === 0) {
      return activeAction;
    }
    return {
      ...activeAction,
      inputFields: activeAction.inputFields.map((f) => {
        // Se enriquece el campo si es dinámico por el FieldSpec o si su columna
        // homónima aporta una fuente dinámica propia (precedencia columna).
        if (f.source !== "dynamic" && !columnDynamicByField.has(f.name)) {
          return f;
        }
        const status = optionsStatusForField(f.name);
        const rawId = activeRow ? activeRow[f.name] : undefined;
        return {
          ...f,
          // Fuerza el <select> dinámico aunque el FieldSpec fuera input/select:
          // la fuente de valores la define ahora la columna.
          source: "dynamic",
          options: [...optionsForField(f.name, rawId)],
          optionsLoading: status.loading,
          optionsError: status.error,
        };
      }),
    };
  }, [
    activeAction,
    dynamicFields,
    columnDynamicByField,
    optionsStatusForField,
    optionsForField,
    activeRow,
  ]);

  // Etiquetas de columna para los campos de la rama inputFields (crear/custom/
  // SELECT). Se calcula de activeAction.inputFields (no del enriquecido: este
  // solo cambia options/loading/error, no name/sourceTable) contra allColumns
  // (incluye columnas no visibles que un campo de CREAR pudiera referenciar).
  const activeFieldLabels = React.useMemo(
    () =>
      activeAction
        ? fieldLabelsForAction(activeAction.inputFields, allColumns, principalTable)
        : {},
    [activeAction, allColumns, principalTable],
  );

  const pagination: PaginationMeta = data?.pagination ?? {
    totalRecords: 0,
    page,
    recordsPerPage,
    totalPages: 0,
  };
  const rows = (data?.items ?? []) as ModuleRow[];

  if (defLoading) {
    return (
      <div className="flex items-center gap-2 text-muted-foreground">
        <Loader2 className="size-4 animate-spin" aria-hidden="true" />
        Cargando módulo...
      </div>
    );
  }
  if (defError || !def) {
    // No se pudo cargar la definición (p. ej. 403 por permisos obsoletos). Aun
    // así renderizamos el botón "Recargar permisos" junto al banner de error:
    // es el control que existe precisamente para recuperar permisos obsoletos,
    // y debe seguir alcanzable desde este estado. Reutiliza el mismo hook/handler
    // que la cabecera del camino feliz. Tras un refresco con éxito el componente
    // re-renderiza y, si los permisos ya lo permiten, carga la definición.
    return (
      <div className="space-y-4">
        <div className="flex justify-end">
          <RefreshPermissionsButton
            isPending={refreshPermissions.isPending}
            onClick={onRefreshPermissions}
          />
        </div>

        {/* El error de "Recargar permisos" se muestra como TOAST flotante. El
            banner de abajo es la carga fallida de la DEFINICIÓN (contextual al
            cuerpo de la página): se queda inline, fuera del alcance de esta
            migración. */}
        <FeedbackBanner
          tone="error"
          message="No se pudo cargar la definición del módulo."
        />
      </div>
    );
  }

  const visibleHeaderActions = headerActions.filter((a) =>
    canModule(perms, slug, a.permAction, country),
  );

  return (
    <div className="space-y-4">
      <div ref={topRef} aria-hidden="true" />
      <ScrollToTopButton onClick={scrollToTop} />

      <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
        <div className="space-y-1">
          <h1 className="text-2xl font-semibold text-foreground">{def.name}</h1>
        </div>
        <div className="flex items-center gap-2">
          {/* "Recargar permisos": reemite el JWT con los permisos actuales. Es
              SIEMPRE visible (no gated por canModule) y NO es la acción de
              permiso "Recargar" (que recarga la caché de datos). */}
          <RefreshPermissionsButton
            isPending={refreshPermissions.isPending}
            onClick={onRefreshPermissions}
          />
          <div className="h-6 w-px bg-border" aria-hidden="true" />
          {visibleHeaderActions.map((a) => (
            <Button key={a.actionKey} type="button" onClick={() => launchAction(a, null)}>
              <Plus aria-hidden="true" />
              {a.label}
            </Button>
          ))}
        </div>
      </div>

      {/* El resultado de las acciones y de "Recargar permisos" (éxito/error)
          se muestra como TOAST flotante. El banner de abajo es la carga fallida
          de los DATOS del grid (contextual al cuerpo): se queda inline, fuera
          del alcance de esta migración. */}
      {isError ? (
        <FeedbackBanner
          tone="error"
          message={error?.message ?? "No se pudieron cargar los datos del módulo."}
        />
      ) : null}

      {/* Filtro de acción SELECT activo: aviso + botón para volver al listado
          base. Solo aparece cuando hay una acción SELECT alimentando el grid. */}
      {activeFilter != null ? (
        <div className="flex flex-wrap items-center justify-between gap-2 rounded-md border border-border bg-muted/30 px-3 py-2">
          <span className="text-sm text-muted-foreground">
            Mostrando resultados filtrados por una acción.
          </span>
          <Button type="button" variant="outline" size="sm" onClick={onClearFilter}>
            Limpiar filtro
          </Button>
        </div>
      ) : null}

      <ResourceTable<ModuleRow>
        columns={columns}
        rows={rows}
        rowKey={(row) => String(row.id ?? JSON.stringify(row))}
        pagination={pagination}
        isLoading={isLoading}
        sort={sort}
        onSortChange={(next) => {
          setSort(next);
          setPage(1);
        }}
        onPageChange={setPage}
        onRecordsPerPageChange={(v) => {
          setRecordsPerPage(v);
          setPage(1);
        }}
        pageSizeOptions={[25, 50, 100]}
        emptyMessage="Sin registros"
      />

      {/* Carga las opciones de las columnas del grid con fuente de valores (un
          fetcher por columna) para resolver id→etiqueta en la celda. Se montan
          SOLO si hay bindings; "column-static" arma el mapa en memoria sin fetch,
          "column-dynamic"/"action-dynamic" comparten caché con el desplegable de
          crear/editar. Si no hay bindings, el .map no monta nada ⇒ cero fetches. */}
      {columnBindings.map((b) => (
        <ColumnSourceLabelFetcher
          key={b.renderKey}
          slug={slug}
          binding={b}
          onLoaded={onColumnLabelsLoaded}
        />
      ))}

      {/* Carga las opciones de los campos dinámicos de la acción activa SOLO
          con el formulario abierto (un fetcher por campo, componentes aparte
          para respetar las reglas de hooks). Estos son los FieldSpec dinámicos
          (ruta de acción, keyeados por f.name). */}
      {mode === "form" && activeAction
        ? dynamicFields.map((f) => (
            <FieldOptionsFetcher
              key={f.name}
              slug={slug}
              field={f.name}
              actionKey={activeAction.actionKey}
              onLoaded={onDynamicOptionsLoaded}
            />
          ))
        : null}

      {/* Carga las opciones de las COLUMNAS con fuente dinámica propia que la
          acción activa toca (precedencia columna, FEAT-004), un fetcher por
          columna keyeado por renderKey para no colisionar homónimas. Solo con el
          formulario abierto. */}
      {mode === "form" && activeAction
        ? activeColumnDynamicBindings.map((b) => (
            <ColumnOptionsFetcher
              key={b.renderKey}
              slug={slug}
              field={b.field}
              sourceTable={b.sourceTable}
              renderKey={b.renderKey}
              onLoaded={onColumnOptionsLoaded}
            />
          ))
        : null}

      {activeAction && mode === "form" && activeAction.kind === "edit" && hasColumnPK ? (
        // EDITAR (Opción B): formulario generado desde columnas. Precarga TODAS
        // las columnas desde la fila; el diálogo deshabilita PK/no editables.
        <ActionFormDialog
          open
          action={activeAction}
          columnFields={buildColumnFieldsWithOptions(activeAction)}
          initialValues={activeRow ?? {}}
          isPending={actionMutation.isPending}
          errorMessage={actionMutation.isError ? actionMutation.error?.message ?? "Error al ejecutar" : null}
          destructive={activeAction.destructive}
          onSubmit={(input) => {
            // input = solo columnas editables; se reañaden las PK desde la fila
            // para el WHERE (el backend decide SET vs WHERE).
            const merged = composeEditInput(input, activeRow);
            actionMutation.mutate(merged, { onSuccess: () => onActionSuccess(activeAction) });
          }}
          onCancel={closeDialogs}
        />
      ) : activeAction && mode === "form" ? (
        <ActionFormDialog
          open
          // Acción con las opciones dinámicas ya inyectadas en sus inputFields.
          action={actionWithDynamicOptions ?? activeAction}
          // Etiqueta legible por campo (solo presentación; no viaja al input).
          fieldLabels={activeFieldLabels}
          // Precarga: valores resueltos de la fila (campos "row") por nombre del
          // parámetro. Los campos "input" los completa el usuario.
          initialValues={resolveRowValues(activeAction, activeRow)}
          isPending={isSelectAction(activeAction) ? false : actionMutation.isPending}
          errorMessage={
            isSelectAction(activeAction)
              ? null
              : actionMutation.isError
                ? actionMutation.error?.message ?? "Error al ejecutar"
                : null
          }
          destructive={activeAction.destructive}
          onSubmit={(input) => {
            // Combina lo capturado por el usuario con los valores "row"
            // resueltos (estos últimos no son editables en el form).
            const merged = { ...input, ...resolveRowValues(activeAction, activeRow) };
            // Acción SELECT: NO escribe; sus campos llenos filtran el listado vía
            // la ruta dedicada de lectura.
            if (isSelectAction(activeAction)) {
              applySelectFilter(activeAction, merged);
              return;
            }
            actionMutation.mutate(merged, { onSuccess: () => onActionSuccess(activeAction) });
          }}
          onCancel={closeDialogs}
        />
      ) : null}

      {activeAction && mode === "confirm" ? (
        <ConfirmActionDialog
          open
          label={activeAction.label}
          isPending={actionMutation.isPending}
          errorMessage={actionMutation.isError ? actionMutation.error?.message ?? "Error al ejecutar" : null}
          details={
            multiTable && activeAction.kind === "delete"
              ? (() => {
                  const order = deleteSetForAction(def, activeAction, principalTable);
                  if (order.length === 0) {
                    return null;
                  }
                  return (
                    <div className="rounded-md border border-border/60 bg-muted/30 px-3 py-2 text-sm">
                      <p className="mb-1 font-medium">
                        Se eliminarán estas tablas en orden (hijo → padre):
                      </p>
                      <ol className="list-decimal pl-5">
                        {order.map((t) => (
                          <li key={t}>{t}</li>
                        ))}
                      </ol>
                      <p className="mt-1 text-xs text-muted-foreground">
                        El borrado alcanza la tabla tope y lo que cuelga de ella por FK, no el módulo completo.
                      </p>
                    </div>
                  );
                })()
              : null
          }
          onConfirm={() => {
            // Para eliminar con PK de columna, el input son las columnas PK
            // tomadas de la fila (WHERE). En otros casos, los valores declarados
            // resueltos de la fila por el mapeo explícito (sin fila cruda).
            let input: Record<string, unknown>;
            if (activeAction.kind === "delete" && hasColumnPK && activeRow) {
              input = {};
              for (const pk of pkFields) {
                input[pk] = activeRow[pk];
              }
            } else {
              input = resolveRowValues(activeAction, activeRow);
            }
            actionMutation.mutate(input, { onSuccess: () => onActionSuccess(activeAction) });
          }}
          onCancel={closeDialogs}
        />
      ) : null}
    </div>
  );
}

export default ModuleView;
