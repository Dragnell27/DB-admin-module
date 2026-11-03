/**
 * Esquema y helpers del asistente de creación/edición de módulos
 * (module-builder, tarea 13). Define la forma del estado del formulario (más
 * rica que la Definition del backend porque incluye las credenciales en claro)
 * y las conversiones a la carga útil del BFF (ModuleDefinitionInput).
 *
 * La validación fina por paso vive en el componente del asistente; aquí se
 * centralizan las reglas puras (slug kebab-case, requeridos) y la construcción
 * del cuerpo de la petición.
 */
import { z } from "zod";

import type {
  ActionKind,
  AttributeMap,
  ColumnValueSource,
  DynamicSource,
  FieldSpec,
  FieldType,
  FillStrategy,
  FillStrategyKind,
  FkJump,
  JoinConstant,
  JoinType,
  ListCondition,
  ModuleAction,
  ModuleDefinitionInput,
  OrderSpec,
  Placement,
  SelectOption,
  SourceType,
  UniquenessCheck,
} from "@/features/modules/types";

/** slug kebab-case: minúsculas y dígitos separados por guiones simples. */
export const SLUG_PATTERN = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;

/**
 * Deriva la `actionKey` (snake_case) a partir de la etiqueta del botón. Mapea
 * los acentos a su letra base (NFD + descarte de diacríticos: 'á'→'a', 'ñ'→'n'),
 * pasa a minúsculas, convierte espacios y símbolos en '_', colapsa los '_'
 * repetidos y recorta los de los bordes. Ej: 'Buscar cuenta'→'buscar_cuenta',
 * 'Añadir Item'→'anadir_item', ''→''. Función pura.
 */
export function slugifyActionKey(label: string): string {
  return label
    .normalize("NFD")
    .replace(/[\u0300-\u036f]/g, "")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "_")
    .replace(/_+/g, "_")
    .replace(/^_+|_+$/g, "");
}

/** Valida el nombre y el slug base del módulo (paso 1). */
export const moduleBaseSchema = z.object({
  slug: z
    .string()
    .min(1, "El identificador es obligatorio")
    .regex(SLUG_PATTERN, "Usa minúsculas, dígitos y guiones (kebab-case)"),
  name: z.string().trim().min(1, "El nombre es obligatorio"),
});

/**
 * Estado de formulario de una estrategia de llenado (source === "fillStrategy").
 * Más rico que `FillStrategy` del BFF: el catálogo de `capture-from-catalog` usa
 * el MISMO shape de formulario que un campo dinámico (`dynamicSource` plano +
 * `dynamicFilters` por comas), convertido a `strategy.catalog` en
 * `toDefinitionInput`. Las claves de todos los `kind` coexisten para no perder lo
 * capturado al cambiar de estrategia; `toStrategy` emite solo las del `kind`.
 */
export interface StrategyFormState {
  kind: FillStrategyKind;
  /** capture-from-catalog: catálogo (tabla/id/label/orden/límite) del selector. */
  catalog: DynamicSource;
  /** capture-from-catalog: filtros del catálogo (shape de formulario, por comas). */
  catalogFilters: ConditionFormState[];
  /** capture-from-catalog: columna de la fila elegida que se guarda (def. idColumn). */
  storeColumn: string;
  /** capture-from-catalog: identificador lógico de la captura. */
  captureKey: string;
  /** derive-from-option: captureKey de la captura de la que se relee la fila. */
  sourceCaptureKey: string;
  /** derive-from-option: atributo de la fila del catálogo a escribir. */
  attribute: string;
  /** derive-via-fk: definición del salto FK 1-a-1. */
  jump: FkJump;
  /** derive-via-fk: mapeo atributo→columna (una entrada; targetColumn == name). */
  mapping: AttributeMap[];
}

/** Estado inicial de una estrategia (kind "constant": sin sub-campos propios). */
export function emptyStrategyFormState(): StrategyFormState {
  return {
    kind: "constant",
    catalog: { table: "", idColumn: "", labelColumn: "", order: { column: "", direction: "asc" } },
    catalogFilters: [],
    storeColumn: "",
    captureKey: "",
    sourceCaptureKey: "",
    attribute: "",
    jump: { relatedTable: "", localCaptureKey: "", localAttribute: "", relatedColumn: "" },
    mapping: [],
  };
}

/** Estado de un campo de entrada en el formulario del asistente. */
export interface FieldFormState {
  name: string;
  type: FieldType;
  required: boolean;
  /**
   * Origen del valor: "input" (lo escribe el usuario), "row" (de la fila),
   * "constant" (valor literal fijo en `constValue`), "select" (lista estática
   * de opciones en `options`, con `defaultValue` preseleccionado opcional) o
   * "dynamic" (lista dinámica cargada de una tabla de catálogo en ejecución,
   * definida en `dynamicSource` con filtros en `dynamicFilters`).
   */
  source: "input" | "row" | "constant" | "select" | "dynamic" | "fillStrategy";
  /** Si source === "row": campo de la fila del que se toma el valor. */
  rowField: string;
  /** Si source === "constant": valor literal que se envía siempre. */
  constValue: string;
  /** Si source === "select": opciones estáticas (etiqueta/valor) a elegir. */
  options: SelectOption[];
  /** Si source === "select": value de la opción preseleccionada por defecto. */
  defaultValue: string;
  /**
   * Si source === "dynamic": definición de la tabla de catálogo (tabla, columna
   * id, columna etiqueta, orden y límite). Los filtros NO van aquí: viven en
   * `dynamicFilters` como `ConditionFormState[]` (shape de formulario) y se
   * convierten a `dynamicSource.filters` en `toDefinitionInput`.
   */
  dynamicSource: DynamicSource;
  /**
   * Si source === "dynamic": filtros del WHERE de la lista dinámica, en el MISMO
   * shape de formulario que las condiciones de listado (`valuesText` por comas).
   * `schema.ts` hace el split/join hacia/desde `dynamicSource.filters`
   * (`ListCondition[]`) EXACTAMENTE igual que `listConditions`.
   */
  dynamicFilters: ConditionFormState[];
  /**
   * Solo para source === "constant" en acciones de escritura: oculta la columna
   * objetivo del formulario (flujo "Inactivar"). El backend la ignora.
   */
  hidden: boolean;
  /**
   * Si source === "fillStrategy": estado de formulario de la estrategia de
   * llenado. Siempre presente (default `emptyStrategyFormState()`) para que
   * cambiar de origen no pierda lo capturado; `toDefinitionInput` emite
   * `strategy` SOLO cuando source === "fillStrategy".
   */
  strategy: StrategyFormState;
  /**
   * MULTI-TABLA: tabla de origen de la columna objetivo cuando es homónima. Solo
   * se emite en toDefinitionInput si es no vacía. Ausente ⇒ principal.
   */
  sourceTable?: string;
  /**
   * SOLO acciones SELECT (filtro de listado): tipo de coincidencia del predicado.
   * "exact" (default) ⇒ igualdad; "partial" ⇒ LIKE contiene. El backend es la
   * autoridad de la semántica SQL; se ignora en INSERT/UPDATE/DELETE.
   */
  matchType?: "exact" | "partial";
}

/**
 * Estado de formulario de la comprobación de unicidad de una acción create.
 * `columns` son columnas de la tabla objetivo; `message` el aviso de negocio al
 * chocar. `toDefinitionInput` emite `uniqueness` solo si `columns` no está vacío.
 */
export interface UniquenessFormState {
  columns: string[];
  message: string;
}

/** Estado inicial de la comprobación de unicidad (sin columnas ⇒ no se emite). */
export function emptyUniquenessFormState(): UniquenessFormState {
  return { columns: [], message: "" };
}

/** Estado de una acción en el formulario del asistente. */
export interface ActionFormState {
  actionKey: string;
  kind: ActionKind;
  label: string;
  placement: Placement;
  destructive: boolean;
  permAction: string;
  isWrite: boolean;
  sqlOp: string;
  inputFields: FieldFormState[];
  /** MULTI-TABLA (delete): tabla tope del borrado. Vacío ⇒ principal. */
  deleteTopTable?: string;
  /** MULTI-TABLA (create): tabla objetivo del INSERT. Vacío ⇒ principal. */
  targetTable?: string;
  /**
   * MULTI-TABLA (create): comprobación de unicidad. Siempre presente en el estado
   * (default `{ columns: [], message: "" }`); `toDefinitionInput` emite
   * `uniqueness` SOLO cuando hay al menos una columna (patrón omitempty).
   */
  uniqueness: UniquenessFormState;
  /**
   * EFÍMERO (solo estado del asistente): el usuario ya editó la `actionKey` a
   * mano, por lo que deja de autoderivarse del label. No viaja al backend
   * (`toDefinitionInput` no lo emite) ni se hidrata (`fromDefinition` lo deja
   * undefined).
   */
  keyTouched?: boolean;
}

/**
 * Claves de acción candidatas para el `<datalist>` del campo "Clave": las
 * `actionKey` distintas (trim no vacío, sin duplicados) de TODAS las acciones
 * salvo la de índice `selfIndex`. Orden determinista por primera aparición.
 * Sirve para sugerir reutilizar la clave de otra acción y agrupar permisos.
 * Función pura.
 */
export function datalistActionKeys(
  actions: ActionFormState[],
  selfIndex: number,
): string[] {
  const seen = new Set<string>();
  const result: string[] = [];
  actions.forEach((a, i) => {
    if (i === selfIndex) {
      return;
    }
    const key = a.actionKey.trim();
    if (key !== "" && !seen.has(key)) {
      seen.add(key);
      result.push(key);
    }
  });
  return result;
}

/**
 * Estado de la FUENTE DE VALORES de una columna en el asistente. `kind` "none"
 * (por defecto) no emite `valueSource` (JSONB idéntico al histórico); "static"
 * usa `options`; "dynamic" usa `dynamicSource` + `dynamicFilters` (shape de
 * formulario, EXACTAMENTE igual que los campos de acción). Las cuatro claves
 * coexisten para que cambiar de tipo no pierda lo ya capturado en el otro.
 */
export interface ColumnValueSourceFormState {
  kind: "none" | "static" | "dynamic";
  options: SelectOption[];
  dynamicSource: DynamicSource;
  dynamicFilters: ConditionFormState[];
}

/** Estado de una columna de visualización en el asistente. */
export interface ColumnFormState {
  field: string;
  label: string;
  order: number;
  format: string;
  /** Se muestra como columna al listar (R3). */
  visible: boolean;
  /** Parte de la clave del WHERE de UPDATE/DELETE (R4). PK ⇒ no editable. */
  isPrimaryKey: boolean;
  /** Modificable: entra en el SET del UPDATE (R5). */
  editable: boolean;
  /**
   * MULTI-TABLA: tabla a la que pertenece la columna. En N=1 es la tabla
   * principal. Desambigua columnas homónimas por (sourceTable, field).
   */
  sourceTable: string;
  /**
   * Fuente de valores de la columna (ninguna/estática/dinámica). Se conserva
   * entera en syncColumnsWithWhitelist/syncColumnsMultiTable (preservan la fila
   * completa de las columnas que persisten), por lo que no necesita rehidratarse
   * al sincronizar la lista blanca.
   */
  valueSource: ColumnValueSourceFormState;
}

/**
 * Etiqueta "tabla.columna" de una columna definida. Si `sourceTable` es vacía
 * (N=1 / principal sin cualificar) cae a solo la columna, sin punto inicial.
 * Fuente única de verdad de la etiqueta de TODA opción de columna definida.
 */
export function columnOptionLabel(field: string, sourceTable?: string): string {
  const t = (sourceTable ?? "").trim();
  return t !== "" ? `${t}.${field}` : field;
}

/**
 * Parsea una lista de columnas escrita por el usuario como texto separado por
 * comas a un array normalizado: corta por comas, recorta espacios y descarta
 * las entradas vacías. Es la ÚNICA fuente de verdad de este parseo (lo usan el
 * input de unicidad y la fila de tabla foránea), de modo que escribir comas y
 * espacios no colapse el texto. Función pura.
 */
export function parseColumnList(text: string): string[] {
  return text
    .split(",")
    .map((c) => c.trim())
    .filter((c) => c !== "");
}

/** Separador NUL de la clave compuesta (no aparece en nombres SQL). */
const COL_SEP = "\u0000";

/**
 * Clave estable de una columna definida para el `value` del `<option>`. Codifica
 * `sourceTable` y `field` juntos para distinguir homónimos entre tablas.
 */
export function columnOptionValue(field: string, sourceTable?: string): string {
  return `${(sourceTable ?? "").trim()}${COL_SEP}${field}`;
}

/**
 * Inverso de `columnOptionValue`: separa el `value` del `<option>` en
 * `{ field, sourceTable }`. Tolerante: si no hay separador, trata todo como
 * `field` y `sourceTable = ""`. No puede lanzar.
 */
export function parseColumnOptionValue(value: string): {
  field: string;
  sourceTable: string;
} {
  const i = value.indexOf(COL_SEP);
  if (i < 0) {
    return { field: value, sourceTable: "" };
  }
  return { sourceTable: value.slice(0, i), field: value.slice(i + 1) };
}

/** Opción de un `<select>` de columnas definidas. */
export interface ColumnOption {
  /** `columnOptionValue(field, sourceTable)`. */
  value: string;
  /** `columnOptionLabel(field, sourceTable)`, con sufijo si es huérfana. */
  label: string;
  /** La opción no corresponde a ninguna columna definida actual. */
  orphan: boolean;
}

/**
 * Opciones de un `<select>` de columnas definidas. `columns` ya viene filtrada
 * por el llamador (p. ej. `writableColumns` para escritura, `state.columns` para
 * el resto). Si `(selField, selSource)` no está entre `columns`, se añade como
 * opción huérfana seleccionable marcada "(columna inexistente)" para que las
 * definiciones viejas/copiadas carguen sin romperse y el usuario pueda
 * corregirlas. Nunca descarta ni lanza. Función pura.
 */
export function buildColumnOptions(
  columns: ColumnFormState[],
  selField: string,
  selSource: string,
): ColumnOption[] {
  const opts: ColumnOption[] = columns.map((c) => ({
    value: columnOptionValue(c.field, c.sourceTable),
    label: columnOptionLabel(c.field, c.sourceTable),
    orphan: false,
  }));
  const selValue = columnOptionValue(selField, selSource);
  const known = selField !== "" && opts.some((o) => o.value === selValue);
  if (selField !== "" && !known) {
    opts.push({
      value: selValue,
      label: `${columnOptionLabel(selField, selSource)} (columna inexistente)`,
      orphan: true,
    });
  }
  return opts;
}

/** Columna por defecto creada por precarga/sincronización desde la lista blanca. */
export function defaultColumn(
  field: string,
  order: number,
  sourceTable = "",
): ColumnFormState {
  return {
    field,
    label: field,
    order,
    format: "",
    visible: true,
    isPrimaryKey: false,
    editable: true,
    sourceTable,
    valueSource: {
      kind: "none",
      options: [],
      dynamicSource: { table: "", idColumn: "", labelColumn: "", order: { column: "", direction: "asc" } },
      dynamicFilters: [],
    },
  };
}

/** Estado de una relación FK en el formulario del asistente. */
export interface RelationshipFormState {
  childTable: string;
  parentTable: string;
  on: { child: string; parent: string }[];
  /** Condiciones constantes del ON (tabla.col = valor). Siempre array en el estado. */
  onConstants: JoinConstant[];
  joinType: JoinType;
}

/**
 * Sincroniza las columnas del asistente con las columnas de la lista blanca
 * (R1, R2). Reglas: preserva los ajustes de las columnas que siguen existiendo
 * (por `field`); añade una fila por defecto por cada columna nueva de la lista
 * blanca; descarta las columnas que ya no están en la lista blanca. Mantiene el
 * orden relativo de las preservadas y anexa las nuevas en la posición que ocupan
 * en la lista blanca. Función pura.
 */
export function syncColumnsWithWhitelist(
  current: ColumnFormState[],
  whitelistCols: string[],
): ColumnFormState[] {
  const byField = new Map(current.map((c) => [c.field, c]));
  const result: ColumnFormState[] = [];
  whitelistCols.forEach((name, i) => {
    const existing = byField.get(name);
    result.push(existing ?? defaultColumn(name, i));
  });
  return result;
}

/**
 * Sincroniza las columnas del asistente con la lista blanca MULTI-TABLA: para
 * CADA tabla (principal + foráneas) y cada columna de su lista blanca, preserva
 * la columna existente por (sourceTable, field) o crea una por defecto con su
 * sourceTable. Descarta las columnas de tablas/columnas que ya no están. El
 * orden resultante agrupa por tabla (principal primero). Función pura.
 */
export function syncColumnsMultiTable(
  current: ColumnFormState[],
  principal: string,
  whitelist: Record<string, string[]>,
  tables: string[],
): ColumnFormState[] {
  const byKey = new Map(current.map((c) => [`${c.sourceTable || principal}::${c.field}`, c]));
  const result: ColumnFormState[] = [];
  let order = 0;
  for (const table of tables) {
    const cols = whitelist[table] ?? [];
    for (const name of cols) {
      const existing = byKey.get(`${table}::${name}`);
      if (existing) {
        result.push({ ...existing, order });
      } else {
        result.push({ ...defaultColumn(name, order, table) });
      }
      order += 1;
    }
  }
  return result;
}

/**
 * Indica si la definición exige una columna llave primaria y no la tiene: hay al
 * menos una acción de editar/eliminar y ninguna columna con `isPrimaryKey` (R10.3).
 */
export function missingPrimaryKey(state: WizardState): boolean {
  const hasWrite = state.actions.some(
    (a) => a.kind === "edit" || a.kind === "delete",
  );
  if (!hasWrite) {
    return false;
  }
  if (!isMultiTableState(state)) {
    return !state.columns.some((c) => c.isPrimaryKey);
  }
  return tablesMissingPrimaryKey(state).length > 0;
}

/**
 * Tablas que, exigiendo PK (hay alguna acción edit/delete), NO tienen ninguna
 * columna marcada como llave primaria. En single-table devuelve [principal] si
 * falta la PK, o [] si está. En multi-tabla aplica la misma regla conservadora
 * que `missingPrimaryKey`: toda tabla declarada del módulo debe tener PK. El
 * orden es estable: principal primero, luego las foráneas en orden de
 * declaración. Función pura; sirve para nombrar en el mensaje de validación
 * EXACTAMENTE en qué tabla falta definir la llave.
 */
export function tablesMissingPrimaryKey(state: WizardState): string[] {
  const hasWrite = state.actions.some(
    (a) => a.kind === "edit" || a.kind === "delete",
  );
  if (!hasWrite) {
    return [];
  }
  const principal = state.pgTable;
  if (!isMultiTableState(state)) {
    return state.columns.some((c) => c.isPrimaryKey) ? [] : [principal];
  }
  // Orden estable: principal primero, luego las foráneas declaradas.
  const tables = [principal, ...state.foreignTables];
  const pkTables = new Set(
    state.columns.filter((c) => c.isPrimaryKey).map((c) => c.sourceTable || principal),
  );
  return tables.filter((t) => t !== "" && !pkTables.has(t));
}

/**
 * Valida una relación FK del asistente. Devuelve el mensaje de error (español)
 * de la primera regla incumplida, o null si es válida: tabla hija y padre
 * declaradas y distintas, y al menos un par ON con columnas no vacías.
 */
export function relationshipError(
  rel: RelationshipFormState,
  declaredTables: string[],
): string | null {
  const declared = new Set(declaredTables);
  if (rel.childTable.trim() === "" || rel.parentTable.trim() === "") {
    return "La relación necesita una tabla hija y una tabla padre";
  }
  if (rel.childTable === rel.parentTable) {
    return "La tabla hija y la tabla padre deben ser distintas";
  }
  if (!declared.has(rel.childTable) || !declared.has(rel.parentTable)) {
    return "La relación referencia una tabla no declarada";
  }
  if (rel.on.length === 0 || rel.on.some((p) => p.child.trim() === "" || p.parent.trim() === "")) {
    return "La relación necesita al menos un par de columnas de unión (ON)";
  }
  // Condiciones constantes del ON: tabla ∈ {hija, padre}, columna y valor no vacíos.
  for (const c of rel.onConstants ?? []) {
    if (c.table !== rel.childTable && c.table !== rel.parentTable) {
      return "La condición constante del ON debe referenciar la tabla hija o padre";
    }
    if (c.column.trim() === "" || c.value.trim() === "") {
      return "La condición constante del ON necesita columna y valor";
    }
  }
  return null;
}

/**
 * Aviso temprano espejo de BuildRelationTree (backend): detecta un ciclo FK o
 * una tabla declarada no enlazada con la principal. Devuelve el mensaje de error
 * (español) o null si el árbol es válido. El backend mantiene la autoridad.
 */
export function treeError(state: WizardState): string | null {
  if (!isMultiTableState(state)) {
    return null;
  }
  const principal = state.pgTable;
  const tables = [principal, ...state.foreignTables].filter((t) => t !== "");
  const declared = new Set(tables);

  // Valida cada relación primero.
  for (const r of state.relationships) {
    const err = relationshipError(r, tables);
    if (err) {
      return err;
    }
  }

  // Conectividad (aristas no dirigidas) desde la principal.
  const undirected = new Map<string, Set<string>>();
  const link = (a: string, b: string) => {
    if (!undirected.has(a)) undirected.set(a, new Set());
    if (!undirected.has(b)) undirected.set(b, new Set());
    undirected.get(a)!.add(b);
    undirected.get(b)!.add(a);
  };
  for (const r of state.relationships) {
    link(r.childTable, r.parentTable);
  }
  if (tables.length > 1) {
    const reached = new Set<string>();
    const stack = [principal];
    while (stack.length > 0) {
      const cur = stack.pop()!;
      if (reached.has(cur)) continue;
      reached.add(cur);
      for (const n of undirected.get(cur) ?? []) {
        stack.push(n);
      }
    }
    for (const t of tables) {
      if (!reached.has(t)) {
        return `La tabla "${t}" no está enlazada con la principal`;
      }
    }
  }

  // Ciclo FK siguiendo child→parent.
  const fkParent = new Map<string, string[]>();
  for (const r of state.relationships) {
    fkParent.set(r.childTable, [...(fkParent.get(r.childTable) ?? []), r.parentTable]);
  }
  const color = new Map<string, number>(); // 0 blanco, 1 gris, 2 negro
  let cyclic = false;
  const visit = (node: string) => {
    color.set(node, 1);
    for (const p of fkParent.get(node) ?? []) {
      const c = color.get(p) ?? 0;
      if (c === 1) {
        cyclic = true;
        return;
      }
      if (c === 0) {
        visit(p);
        if (cyclic) return;
      }
    }
    color.set(node, 2);
  };
  for (const t of tables) {
    if ((color.get(t) ?? 0) === 0) {
      visit(t);
      if (cyclic) {
        return "Las relaciones FK forman un ciclo";
      }
    }
  }
  if (!declared.has(principal) && tables.length > 0) {
    return "La tabla principal no está declarada";
  }
  return null;
}

/**
 * Reimplementación en TS de DeleteOrder (backend) para mostrar en SOLO LECTURA
 * el conjunto ordenado hijo→padre que borrará una acción delete. Devuelve el
 * conjunto = `top` + sus descendientes-hijo FK, en orden topológico (hijos
 * primero, `top` al final). Los padres FK de `top` NO se incluyen. Si el árbol
 * es inconsistente devuelve solo [top].
 */
export function deleteOrder(
  relationships: RelationshipFormState[],
  top: string,
): string[] {
  const fkChildren = new Map<string, string[]>();
  for (const r of relationships) {
    fkChildren.set(r.parentTable, [...(fkChildren.get(r.parentTable) ?? []), r.childTable]);
  }
  // Conjunto: top + descendientes-hijo.
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
  // Orden topológico: hijos antes que padres (post-orden DFS).
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
 * Estado de una condición de listado en el asistente. `valuesText` es lo que el
 * usuario escribe (valores separados por comas); se parte en la lista al
 * convertir a la definición.
 */
export interface ConditionFormState {
  field: string;
  valuesText: string;
  /** MULTI-TABLA: tabla de la columna cuando es homónima. Vacío ⇒ principal. */
  sourceTable?: string;
}

/** Estado completo del asistente. */
export interface WizardState {
  slug: string;
  name: string;
  sourceType: SourceType;
  /** Ruta jerárquica libre del módulo en el menú (organizativa, Req. 6). */
  menuPath: string;
  /** Plantilla de conexión seleccionada (obligatoria, Req. 3.1). */
  connectionTemplateId: string;
  /** Valores VARIABLE en claro (solo se envían si se rellenan). */
  variableValues: Record<string, string>;
  // Mapeos
  pgTable: string; // Postgres: tabla principal
  pgWhitelist: Record<string, string[]>;
  listParams: Record<string, unknown>;
  columns: ColumnFormState[];
  conditions: ConditionFormState[];
  actions: ActionFormState[];
  /** MULTI-TABLA: nombres de las tablas foráneas declaradas. */
  foreignTables: string[];
  /** MULTI-TABLA: relaciones FK dirigidas entre tablas. */
  relationships: RelationshipFormState[];
}

/** Estado inicial de un asistente nuevo. */
export function emptyWizardState(): WizardState {
  return {
    slug: "",
    name: "",
    sourceType: "postgres",
    menuPath: "",
    connectionTemplateId: "",
    variableValues: {},
    pgTable: "",
    pgWhitelist: {},
    listParams: {},
    columns: [],
    conditions: [],
    actions: [],
    foreignTables: [],
    relationships: [],
  };
}

/** Indica si el estado del asistente es multi-tabla (tiene foráneas o relaciones). */
export function isMultiTableState(state: WizardState): boolean {
  return state.foreignTables.length > 0 || state.relationships.length > 0;
}

/** Deriva la permAction de una acción a partir de su tipo. */
export function permActionForKind(kind: ActionKind, actionKey: string): string {
  switch (kind) {
    case "list":
    case "search": // buscar también es permiso de lectura "ver"
      return "ver";
    case "create":
      return "crear";
    case "edit":
      return "editar";
    case "delete":
      return "eliminar";
    default:
      return `custom:${actionKey}`;
  }
}

/**
 * Deriva la "forma" SQL de una acción a partir de su TIPO (fuente única de
 * verdad). Para todo kind ≠ custom fija `sqlOp`/`isWrite` según la tabla del
 * diseño, ignorando cualquier `sqlOp` previo. Para `custom` respeta el `sqlOp`
 * de entrada tal cual (incluido el string vacío de una acción recién creada) y
 * deriva `isWrite`: solo SELECT ⇒ false; "", INSERT, UPDATE, DELETE ⇒ true.
 * Función pura.
 */
export function deriveActionShape(
  kind: ActionKind,
  sqlOp: string,
): { sqlOp: string; isWrite: boolean } {
  switch (kind) {
    case "list":
    case "search":
      return { sqlOp: "SELECT", isWrite: false };
    case "create":
      return { sqlOp: "INSERT", isWrite: true };
    case "edit":
      return { sqlOp: "UPDATE", isWrite: true };
    case "delete":
      return { sqlOp: "DELETE", isWrite: true };
    case "custom":
    default:
      // custom: se respeta el sqlOp tal cual (incluido ""), isWrite derivado.
      return {
        sqlOp,
        isWrite: sqlOp.trim().toUpperCase() !== "SELECT",
      };
  }
}

/**
 * `sqlOp` EFECTIVO de una acción para decidir render/emisión del bloque
 * matchType. En `custom` manda el usuario (normalizado a mayúsculas); en el
 * resto se usa el derivado del tipo, ignorando el `a.sqlOp` crudo. NO muta el
 * estado. Función pura.
 */
export function effectiveSqlOp(a: ActionFormState): string {
  return a.kind === "custom"
    ? a.sqlOp.trim().toUpperCase()
    : deriveActionShape(a.kind, a.sqlOp).sqlOp;
}

/**
 * Placement por defecto de una acción según su tipo: "header" para
 * list/search/create, "row" para edit/delete/custom. Función pura.
 */
export function defaultPlacementForKind(kind: ActionKind): Placement {
  switch (kind) {
    case "list":
    case "search":
    case "create":
      return "header";
    case "edit":
    case "delete":
    case "custom":
    default:
      return "row";
  }
}

/**
 * Construye la `DynamicSource` del BFF a partir del estado de un campo
 * source==="dynamic". Los filtros se derivan de `dynamicFilters`
 * (`ConditionFormState[]`) EXACTAMENTE igual que `listConditions` (split por
 * comas, trim, descarta campos/valores vacíos). El orden solo se emite si hay
 * columna; el límite solo si es > 0 (el backend acota el techo a 500).
 */
function toDynamicSourceCore(input: {
  dynamicSource: DynamicSource;
  dynamicFilters: ConditionFormState[];
}): DynamicSource {
  const ds = input.dynamicSource;
  const filters: ListCondition[] = input.dynamicFilters
    .map((c) => ({
      field: c.field.trim(),
      values: c.valuesText
        .split(",")
        .map((v) => v.trim())
        .filter((v) => v !== ""),
    }))
    .filter((c) => c.field !== "" && c.values.length > 0);
  const order: OrderSpec | undefined =
    ds.order && ds.order.column.trim() !== ""
      ? { column: ds.order.column.trim(), direction: ds.order.direction }
      : undefined;
  return {
    table: ds.table.trim(),
    idColumn: ds.idColumn.trim(),
    labelColumn: ds.labelColumn.trim(),
    filters: filters.length > 0 ? filters : undefined,
    order,
    limit: typeof ds.limit === "number" && ds.limit > 0 ? ds.limit : undefined,
  };
}

/** Construye la `DynamicSource` del BFF desde el estado de un CAMPO dinámico. */
function toDynamicSource(f: FieldFormState): DynamicSource {
  return toDynamicSourceCore({
    dynamicSource: f.dynamicSource,
    dynamicFilters: f.dynamicFilters,
  });
}

/**
 * Construye la `FillStrategy` del BFF a partir del estado de formulario de un
 * campo source === "fillStrategy". Emite SOLO los sub-campos propios del `kind`
 * (patrón omitempty, espejo de los `omitempty` del backend), de modo que una
 * estrategia `constant` o `autogenerated` produce `{ kind }` sin ruido:
 *   - capture-from-catalog: `catalog` (reutiliza el núcleo plano de dynamic) +
 *     `storeColumn`/`captureKey` solo si no están vacíos.
 *   - derive-from-option: `sourceCaptureKey` + `attribute`.
 *   - derive-via-fk: `jump` + `mapping`.
 *   - constant/autogenerated: sin sub-campos.
 */
function toStrategy(s: StrategyFormState): FillStrategy {
  switch (s.kind) {
    case "capture-from-catalog":
      return {
        kind: s.kind,
        catalog: toDynamicSourceCore({
          dynamicSource: s.catalog,
          dynamicFilters: s.catalogFilters,
        }),
        storeColumn: s.storeColumn.trim() !== "" ? s.storeColumn.trim() : undefined,
        captureKey: s.captureKey.trim() !== "" ? s.captureKey.trim() : undefined,
      };
    case "derive-from-option":
      return {
        kind: s.kind,
        sourceCaptureKey:
          s.sourceCaptureKey.trim() !== "" ? s.sourceCaptureKey.trim() : undefined,
        attribute: s.attribute.trim() !== "" ? s.attribute.trim() : undefined,
      };
    case "derive-via-fk":
      return {
        kind: s.kind,
        jump: {
          relatedTable: s.jump.relatedTable.trim(),
          localCaptureKey: s.jump.localCaptureKey.trim(),
          localAttribute: s.jump.localAttribute.trim(),
          relatedColumn: s.jump.relatedColumn.trim(),
        },
        mapping:
          s.mapping.length > 0
            ? s.mapping.map((m) => ({
                attribute: m.attribute.trim(),
                targetColumn: m.targetColumn.trim(),
              }))
            : undefined,
      };
    case "constant":
    case "autogenerated":
    default:
      return { kind: s.kind };
  }
}

/**
 * Construye el `UniquenessCheck` del BFF a partir del estado de formulario. Se
 * OMITE (undefined) cuando no hay columnas (patrón omitempty, JSONB idéntico al
 * histórico); si las hay, emite `columns` (trim, sin vacíos) y `message` solo si
 * no está vacío.
 */
function toUniqueness(u: UniquenessFormState): UniquenessCheck | undefined {
  const columns = u.columns.map((c) => c.trim()).filter((c) => c !== "");
  if (columns.length === 0) {
    return undefined;
  }
  return {
    columns,
    message: u.message.trim() !== "" ? u.message.trim() : undefined,
  };
}

/**
 * Construye el `valueSource` del BFF a partir del estado de fuente de valores
 * de una columna. Devuelve undefined cuando `kind === "none"` (se OMITE el
 * campo => JSONB idéntico al histórico, retrocompat byte a byte); `static`
 * emite `{kind, options}` y `dynamic` emite `{kind, dynamicSource}` reutilizando
 * el mismo núcleo plano que los campos de acción.
 */
function toColumnValueSource(
  vs: ColumnValueSourceFormState,
): ColumnValueSource | undefined {
  if (vs.kind === "static") {
    return { kind: "static", options: vs.options };
  }
  if (vs.kind === "dynamic") {
    return {
      kind: "dynamic",
      dynamicSource: toDynamicSourceCore({
        dynamicSource: vs.dynamicSource,
        dynamicFilters: vs.dynamicFilters,
      }),
    };
  }
  return undefined;
}

/** Convierte el estado del asistente a la carga útil del BFF (crear/editar). */
export function toDefinitionInput(state: WizardState): ModuleDefinitionInput {
  const multi = isMultiTableState(state);
  const principal = state.pgTable;

  const actions: ModuleAction[] = state.actions.map((a) => {
    // Normalización al guardar (borde 4): sqlOp/isWrite SIEMPRE coherentes con
    // el tipo. Para kinds no-custom se derivan de la tabla; para custom se
    // respeta la operación elegida (y su isWrite derivado).
    const shape = deriveActionShape(a.kind, a.sqlOp);
    return {
    actionKey: a.actionKey,
    kind: a.kind,
    label: a.label,
    placement: a.placement,
    destructive: a.destructive,
    permAction: a.permAction || permActionForKind(a.kind, a.actionKey),
    isWrite: shape.isWrite,
    sqlOp: shape.sqlOp,
    // MULTI-TABLA: tabla tope / objetivo solo si difieren de la principal.
    deleteTopTable:
      a.kind === "delete" && a.deleteTopTable && a.deleteTopTable !== principal
        ? a.deleteTopTable
        : undefined,
    targetTable:
      a.kind === "create" && a.targetTable && a.targetTable !== principal
        ? a.targetTable
        : undefined,
    // uniqueness solo en create y solo si hay columnas declaradas (omitempty).
    uniqueness: a.kind === "create" ? toUniqueness(a.uniqueness) : undefined,
    inputFields: a.inputFields.map(
      (f): FieldSpec => ({
        name: f.name,
        type: f.type,
        required: f.required,
        source: f.source,
        rowField: f.source === "row" ? f.rowField : undefined,
        constValue: f.source === "constant" ? f.constValue : undefined,
        options: f.source === "select" ? f.options : undefined,
        defaultValue: f.source === "select" ? (f.defaultValue || undefined) : undefined,
        dynamicSource: f.source === "dynamic" ? toDynamicSource(f) : undefined,
        // strategy solo cuando source === "fillStrategy" (patrón omitempty,
        // idéntico a dynamicSource); el resto de orígenes lo omite.
        strategy: f.source === "fillStrategy" ? toStrategy(f.strategy) : undefined,
        hidden: f.source === "constant" ? f.hidden || undefined : undefined,
        // sourceTable solo se emite si la columna es homónima (no es la principal).
        sourceTable:
          f.sourceTable && f.sourceTable !== principal ? f.sourceTable : undefined,
        // matchType solo tiene sentido en acciones SELECT (filtro de listado) y
        // con semántica omitempty: se emite SOLO cuando es "partial" ("exact" es
        // el default del backend). Se ignora en escrituras.
        matchType:
          effectiveSqlOp(a) === "SELECT" && f.matchType === "partial"
            ? "partial"
            : undefined,
      }),
    ),
    };
  });

  const fetchMapping: Record<string, unknown> = { table: state.pgTable };
  if (state.foreignTables.length > 0) {
    fetchMapping.foreignTables = state.foreignTables.map((t) => ({ table: t }));
  }
  if (state.relationships.length > 0) {
    fetchMapping.relationships = state.relationships.map((r) => ({
      childTable: r.childTable,
      parentTable: r.parentTable,
      on: r.on.map((p) => ({ child: p.child, parent: p.parent })),
      // onConstants solo se emite si hay condiciones constantes (omitempty):
      // una relación sin ellas produce el JSONB idéntico al de hoy.
      onConstants:
        r.onConstants.length > 0
          ? r.onConstants.map((c) => ({ table: c.table, column: c.column, value: c.value }))
          : undefined,
      joinType: r.joinType,
    }));
  }

  const displayMapping: Record<string, unknown> = {
    columns: state.columns.map((c) => ({
      field: c.field,
      label: c.label || c.field,
      order: c.order,
      format: c.format || undefined,
      visible: c.visible,
      isPrimaryKey: c.isPrimaryKey,
      // Refuerza la invariante PK ⇒ no editable por si la UI no lo hizo.
      editable: c.isPrimaryKey ? false : c.editable,
      // sourceTable solo se emite en multi-tabla y si difiere de la principal.
      sourceTable:
        multi && c.sourceTable && c.sourceTable !== principal
          ? c.sourceTable
          : undefined,
      // valueSource solo se emite si kind es static/dynamic; "none" => undefined
      // (omitido), produciendo el MISMO JSONB que una columna sin fuente.
      valueSource: toColumnValueSource(c.valueSource),
    })),
  };

  // Condiciones de listado: columna + valores (texto por comas → lista). Se
  // descartan las que no tengan campo o no tengan al menos un valor.
  const listConditions: ListCondition[] = state.conditions
    .map((c) => ({
      field: c.field.trim(),
      values: c.valuesText
        .split(",")
        .map((v) => v.trim())
        .filter((v) => v !== ""),
      sourceTable:
        c.sourceTable && c.sourceTable !== principal ? c.sourceTable : undefined,
    }))
    .filter((c) => c.field !== "" && c.values.length > 0);

  // Solo se envían las variables con valor (vacío = conservar).
  const variableValues: Record<string, string> = {};
  for (const [k, v] of Object.entries(state.variableValues)) {
    if (v.trim() !== "") {
      variableValues[k] = v;
    }
  }
  const secrets = {
    variableValues: Object.keys(variableValues).length > 0 ? variableValues : undefined,
  };

  return {
    slug: state.slug,
    name: state.name,
    sourceType: state.sourceType,
    // El ámbito de país se eliminó del asistente: todos los módulos de BD se
    // comportan como país '*' (acceso independiente de país). Se FIJA siempre a
    // 'none' en el DTO; el backend sigue aceptando el campo y su lógica de
    // permisos (country_scope) permanece intacta (autorización viva).
    countryScope: "none",
    menuPath: state.menuPath,
    fetchMapping,
    displayMapping,
    listParams: state.listParams,
    listConditions,
    dataSource: {
      sourceType: state.sourceType,
      pgWhitelist: state.pgWhitelist,
      connectionTemplateId: state.connectionTemplateId,
      hasSecrets: false,
    },
    actions,
    secrets,
  };
}

/**
 * Convierte una Definición cargada del backend al estado del asistente para
 * editar. Las credenciales llegan vacías (el backend nunca las devuelve): el
 * usuario las deja vacías para conservarlas o las reescribe para cambiarlas.
 */
/**
 * Reconstruye el estado de formulario de la fuente de valores de una columna a
 * partir del `valueSource` JSON (tolerante). Inverso de `toColumnValueSource`:
 *   - ausente / kind no en {static,dynamic} ⇒ kind "none" (y estructuras vacías
 *     por defecto, listas para capturar).
 *   - "static" ⇒ options tal cual.
 *   - "dynamic" ⇒ dynamicSource con order por defecto {column:"",direction:"asc"}
 *     y dynamicFilters desde dynamicSource.filters (join por comas), igual que
 *     la hidratación de los filtros de un campo dinámico.
 * Función pura; nunca lanza.
 */
function columnValueSourceFromDefinition(
  raw: unknown,
): ColumnValueSourceFormState {
  const empty: ColumnValueSourceFormState = {
    kind: "none",
    options: [],
    dynamicSource: { table: "", idColumn: "", labelColumn: "", order: { column: "", direction: "asc" } },
    dynamicFilters: [],
  };
  if (!raw || typeof raw !== "object") {
    return empty;
  }
  const vs = raw as {
    kind?: unknown;
    options?: unknown;
    dynamicSource?: {
      table?: unknown;
      idColumn?: unknown;
      labelColumn?: unknown;
      order?: OrderSpec;
      limit?: unknown;
      filters?: Array<{ field?: unknown; values?: unknown }>;
    };
  };
  if (vs.kind === "static") {
    return {
      ...empty,
      kind: "static",
      options: Array.isArray(vs.options) ? (vs.options as SelectOption[]) : [],
    };
  }
  if (vs.kind === "dynamic") {
    const ds = vs.dynamicSource ?? {};
    return {
      ...empty,
      kind: "dynamic",
      dynamicSource: {
        table: typeof ds.table === "string" ? ds.table : "",
        idColumn: typeof ds.idColumn === "string" ? ds.idColumn : "",
        labelColumn: typeof ds.labelColumn === "string" ? ds.labelColumn : "",
        order: ds.order ?? { column: "", direction: "asc" },
        limit: typeof ds.limit === "number" ? ds.limit : undefined,
      },
      dynamicFilters: (ds.filters ?? []).map((cond) => ({
        field: String(cond.field ?? ""),
        valuesText: (Array.isArray(cond.values) ? (cond.values as unknown[]) : [])
          .map((v) => String(v))
          .join(", "),
      })),
    };
  }
  return empty;
}

/**
 * Reconstruye el estado de formulario de una estrategia de llenado a partir del
 * `strategy` JSON (tolerante). Inverso de `toStrategy`:
 *   - ausente / kind no reconocido ⇒ `emptyStrategyFormState()` (kind "constant").
 *   - capture-from-catalog ⇒ catálogo + filtros (igual que un campo dinámico) +
 *     storeColumn/captureKey.
 *   - derive-from-option ⇒ sourceCaptureKey + attribute.
 *   - derive-via-fk ⇒ jump + mapping.
 * Función pura; nunca lanza. Las claves de los demás `kind` quedan por defecto.
 */
function strategyFromDefinition(raw: unknown): StrategyFormState {
  const empty = emptyStrategyFormState();
  if (!raw || typeof raw !== "object") {
    return empty;
  }
  const s = raw as {
    kind?: unknown;
    catalog?: {
      table?: unknown;
      idColumn?: unknown;
      labelColumn?: unknown;
      order?: OrderSpec;
      limit?: unknown;
      filters?: Array<{ field?: unknown; values?: unknown }>;
    };
    storeColumn?: unknown;
    captureKey?: unknown;
    sourceCaptureKey?: unknown;
    attribute?: unknown;
    jump?: {
      relatedTable?: unknown;
      localCaptureKey?: unknown;
      localAttribute?: unknown;
      relatedColumn?: unknown;
    };
    mapping?: Array<{ attribute?: unknown; targetColumn?: unknown }>;
  };
  const kind = s.kind;
  if (kind === "capture-from-catalog") {
    const cat = s.catalog ?? {};
    return {
      ...empty,
      kind,
      catalog: {
        table: typeof cat.table === "string" ? cat.table : "",
        idColumn: typeof cat.idColumn === "string" ? cat.idColumn : "",
        labelColumn: typeof cat.labelColumn === "string" ? cat.labelColumn : "",
        order: cat.order ?? { column: "", direction: "asc" },
        limit: typeof cat.limit === "number" ? cat.limit : undefined,
      },
      catalogFilters: (cat.filters ?? []).map((cond) => ({
        field: String(cond.field ?? ""),
        valuesText: (Array.isArray(cond.values) ? (cond.values as unknown[]) : [])
          .map((v) => String(v))
          .join(", "),
      })),
      storeColumn: typeof s.storeColumn === "string" ? s.storeColumn : "",
      captureKey: typeof s.captureKey === "string" ? s.captureKey : "",
    };
  }
  if (kind === "derive-from-option") {
    return {
      ...empty,
      kind,
      sourceCaptureKey: typeof s.sourceCaptureKey === "string" ? s.sourceCaptureKey : "",
      attribute: typeof s.attribute === "string" ? s.attribute : "",
    };
  }
  if (kind === "derive-via-fk") {
    const j = s.jump ?? {};
    return {
      ...empty,
      kind,
      jump: {
        relatedTable: typeof j.relatedTable === "string" ? j.relatedTable : "",
        localCaptureKey: typeof j.localCaptureKey === "string" ? j.localCaptureKey : "",
        localAttribute: typeof j.localAttribute === "string" ? j.localAttribute : "",
        relatedColumn: typeof j.relatedColumn === "string" ? j.relatedColumn : "",
      },
      mapping: (s.mapping ?? []).map((m) => ({
        attribute: String(m.attribute ?? ""),
        targetColumn: String(m.targetColumn ?? ""),
      })),
    };
  }
  if (kind === "autogenerated") {
    return { ...empty, kind };
  }
  // "constant" o kind ausente/no reconocido ⇒ estado por defecto (constant).
  return empty;
}

/**
 * Reconstruye el estado de formulario de la comprobación de unicidad a partir del
 * `uniqueness` JSON (tolerante). Inverso de `toUniqueness`: ausente ⇒ estado
 * vacío (sin columnas); presente ⇒ columnas (como strings) y mensaje. Función
 * pura; nunca lanza.
 */
function uniquenessFromDefinition(raw: unknown): UniquenessFormState {
  if (!raw || typeof raw !== "object") {
    return emptyUniquenessFormState();
  }
  const u = raw as { columns?: unknown; message?: unknown };
  return {
    columns: Array.isArray(u.columns) ? (u.columns as unknown[]).map((c) => String(c)) : [],
    message: typeof u.message === "string" ? u.message : "",
  };
}

export function fromDefinition(def: ModuleDefinitionInput): WizardState {
  const base = emptyWizardState();
  const fetch = def.fetchMapping ?? {};
  const display = def.displayMapping ?? {};
  const rawColumns = Array.isArray((display as { columns?: unknown }).columns)
    ? ((display as { columns: unknown[] }).columns as Array<Record<string, unknown>>)
    : [];
  const pgWhitelist = def.dataSource?.pgWhitelist ?? {};
  const firstTable = Object.keys(pgWhitelist)[0] ?? "";
  const principal =
    typeof (fetch as { table?: unknown }).table === "string"
      ? String((fetch as { table?: unknown }).table)
      : firstTable;

  // MULTI-TABLA: foráneas y relaciones desde fetchMapping (tolerante).
  const rawForeign = Array.isArray((fetch as { foreignTables?: unknown }).foreignTables)
    ? ((fetch as { foreignTables: unknown[] }).foreignTables as Array<Record<string, unknown>>)
    : [];
  const foreignTables = rawForeign
    .map((t) => String(t.table ?? ""))
    .filter((t) => t !== "");
  const rawRels = Array.isArray((fetch as { relationships?: unknown }).relationships)
    ? ((fetch as { relationships: unknown[] }).relationships as Array<Record<string, unknown>>)
    : [];
  const relationships: RelationshipFormState[] = rawRels.map((r) => ({
    childTable: String(r.childTable ?? ""),
    parentTable: String(r.parentTable ?? ""),
    on: Array.isArray(r.on)
      ? (r.on as Array<Record<string, unknown>>).map((p) => ({
          child: String(p.child ?? ""),
          parent: String(p.parent ?? ""),
        }))
      : [],
    // onConstants ausente ⇒ [] (inverso exacto de toDefinitionInput, retrocompat).
    onConstants: Array.isArray(r.onConstants)
      ? (r.onConstants as Array<Record<string, unknown>>).map((c) => ({
          table: String(c.table ?? ""),
          column: String(c.column ?? ""),
          value: String(c.value ?? ""),
        }))
      : [],
    joinType: r.joinType === "left" ? "left" : "inner",
  }));

  return {
    ...base,
    foreignTables,
    relationships,
    slug: def.slug,
    name: def.name,
    // El único origen soportado es PostgreSQL. Las definiciones antiguas de
    // origen "api" ya no pueden editarse (el backend las rechaza al guardar),
    // por lo que el asistente siempre trabaja en modo postgres.
    sourceType: "postgres",
    // countryScope ya no forma parte del estado del asistente (se eliminó el
    // control). El backend conserva el valor del módulo; al reguardar,
    // toDefinitionInput vuelve a fijarlo en 'none'.
    menuPath: def.menuPath ?? "",
    connectionTemplateId: def.dataSource?.connectionTemplateId ?? "",
    // Los valores variables llegan vacíos (el backend nunca los devuelve).
    variableValues: {},
    pgTable: principal,
    pgWhitelist: pgWhitelist as Record<string, string[]>,
    listParams: def.listParams ?? {},
    columns: rawColumns.map((c, i) => {
      const isPrimaryKey = Boolean(c.isPrimaryKey);
      return {
        field: String(c.field ?? ""),
        label: String(c.label ?? c.field ?? ""),
        order: typeof c.order === "number" ? c.order : i,
        format: typeof c.format === "string" ? c.format : "",
        // Defaults de compatibilidad (R8): ausente ⇒ visible true, editable true
        // salvo PK; isPrimaryKey ausente ⇒ false.
        visible: c.visible === undefined ? true : Boolean(c.visible),
        isPrimaryKey,
        editable: isPrimaryKey ? false : c.editable === undefined ? true : Boolean(c.editable),
        // sourceTable ausente ⇒ principal (compatibilidad N=1).
        sourceTable: typeof c.sourceTable === "string" && c.sourceTable !== ""
          ? String(c.sourceTable)
          : principal,
        // Fuente de valores tolerante: ausente ⇒ kind "none" (no emite nada al
        // reguardar); inverso de toColumnValueSource.
        valueSource: columnValueSourceFromDefinition(c.valueSource),
      };
    }),
    conditions: (def.listConditions ?? []).map((cond) => ({
      field: cond.field,
      valuesText: (cond.values ?? []).join(", "),
      sourceTable: cond.sourceTable ?? "",
    })),
    actions: (def.actions ?? []).map((a) => ({
      actionKey: a.actionKey,
      kind: a.kind,
      label: a.label,
      placement: a.placement,
      destructive: a.destructive,
      permAction: a.permAction,
      isWrite: a.isWrite,
      sqlOp: a.sqlOp ?? "",
      deleteTopTable: a.deleteTopTable ?? "",
      targetTable: a.targetTable ?? "",
      // uniqueness tolerante: ausente ⇒ estado vacío (no emite nada al reguardar).
      uniqueness: uniquenessFromDefinition(a.uniqueness),
      inputFields: (a.inputFields ?? []).map((f) => ({
        name: f.name,
        type: f.type,
        required: f.required,
        source: (f.source ?? "input") as FieldFormState["source"],
        rowField: f.rowField ?? "",
        constValue: f.constValue ?? "",
        options: f.options ?? [],
        defaultValue: f.defaultValue ?? "",
        // sourceTable ausente ⇒ principal, SIMÉTRICO con la hidratación de
        // `columns` (arriba). toDefinitionInput omite el sourceTable cuando es
        // igual a la principal; si aquí lo dejáramos en "" el emparejamiento
        // (field, sourceTable) de buildColumnOptions no casaría con la columna
        // (que sí trae principal) y la marcaría "(columna inexistente)" al
        // reeditar. Rellenarlo con principal cierra el round-trip.
        sourceTable: f.sourceTable && f.sourceTable !== "" ? f.sourceTable : principal,
        // matchType de acción SELECT: "partial" se conserva, cualquier otro
        // (incluido ausente) se normaliza a "exact" (default del backend).
        matchType: f.matchType === "partial" ? "partial" : "exact",
        // Fuente dinámica: se reconstruye la definición de la tabla de catálogo
        // y sus filtros al shape de formulario (valuesText por comas), inverso
        // exacto de toDynamicSource (igual que listConditions).
        dynamicSource: {
          table: f.dynamicSource?.table ?? "",
          idColumn: f.dynamicSource?.idColumn ?? "",
          labelColumn: f.dynamicSource?.labelColumn ?? "",
          order: f.dynamicSource?.order ?? { column: "", direction: "asc" },
          limit: f.dynamicSource?.limit,
        },
        dynamicFilters: (f.dynamicSource?.filters ?? []).map((cond) => ({
          field: cond.field,
          valuesText: (cond.values ?? []).join(", "),
        })),
        hidden: Boolean(f.hidden),
        // Estrategia de llenado tolerante: ausente ⇒ estado por defecto; inverso
        // de toStrategy (retrocompat, INV-9).
        strategy: strategyFromDefinition(f.strategy),
      })),
    })),
  };
}

/**
 * Convierte la Definición de un módulo EXISTENTE al estado del asistente para
 * COPIAR su configuración al crear uno nuevo (module-builder). Reutiliza
 * `fromDefinition` (que ya copia campos/acciones/mapeos/listParams/condiciones/
 * menuPath y nunca trae secretos) y luego VACÍA lo que debe ser único o quedar
 * sin definir en el módulo nuevo:
 *   - slug y name: deben ser nuevos y únicos → se exigen al usuario.
 *   - connectionTemplateId y variableValues: la CONEXIÓN no se copia (ni la
 *     plantilla ni los valores variables); el usuario elige la conexión del
 *     módulo nuevo. Los permisos RBAC no son parte de la Definición y se
 *     conceden al crear, como siempre.
 * Helper puro, testeable de forma aislada.
 */
export function fromCopyableDefinition(def: ModuleDefinitionInput): WizardState {
  return {
    ...fromDefinition(def),
    slug: "",
    name: "",
    connectionTemplateId: "",
    variableValues: {},
  };
}

/** Valida que haya al menos un campo de obtención/columna (reglas de paso). */
export function hasAtLeastOneColumn(state: WizardState): boolean {
  return state.columns.length > 0;
}

/**
 * Valida un campo de origen "select" antes de guardar. Devuelve el mensaje de
 * error (en español) de la PRIMERA regla incumplida, o null si es válido. Las
 * reglas, en orden:
 *   1. Debe tener al menos una opción.
 *   2. Ninguna opción puede tener etiqueta o valor vacíos (tras trim).
 *   3. Los valores (`value`) de las opciones deben ser únicos: el <select> de
 *      opción por defecto los usa como `value`, y repetirlos lo deja ambiguo.
 *   4. Si hay opción por defecto (`defaultValue !== ""`), debe coincidir con el
 *      `value` de alguna opción.
 */
export function selectFieldError(field: FieldFormState): string | null {
  const label = field.name || "(sin nombre)";
  return selectOptionsError(field.options, label, field.defaultValue);
}

/**
 * Núcleo PLANO de validación de una lista estática de opciones etiqueta/valor,
 * reutilizable por campos de acción (`selectFieldError`) y por columnas con
 * fuente estática (`ColumnValueSourceFormState`). Aplica las 4 reglas en orden
 * y devuelve el mensaje (español) de la PRIMERA incumplida, o null si es
 * válida. `label` es el nombre legible (campo o columna) que se interpola en el
 * mensaje; `defaultValue` (opcional) activa la 4ª regla.
 */
export function selectOptionsError(
  options: ReadonlyArray<SelectOption>,
  label: string,
  defaultValue?: string,
): string | null {
  if (options.length === 0) {
    return `El campo "${label}" de tipo selección necesita al menos una opción`;
  }
  if (options.some((o) => o.label.trim() === "" || o.value.trim() === "")) {
    return `Las opciones del campo "${label}" no pueden tener etiqueta o valor vacíos`;
  }
  const values = options.map((o) => o.value);
  if (new Set(values).size !== values.length) {
    return `El campo "${label}" tiene valores de opción repetidos`;
  }
  if (defaultValue !== undefined && defaultValue !== "" && !values.includes(defaultValue)) {
    return `La opción por defecto del campo "${label}" ya no existe`;
  }
  return null;
}

/**
 * Valida un campo de origen "dynamic" antes de guardar. Exige que la fuente
 * declare tabla, columna id y columna etiqueta no vacías (R5.5), análogo a
 * `selectFieldError`. Devuelve el mensaje de error (en español) de la PRIMERA
 * regla incumplida, o null si es válido. NO sustituye la validación del backend
 * (que vuelve a validar tabla/columnas contra la lista blanca en
 * `buildOptionsSQL`).
 */
export function dynamicFieldError(field: FieldFormState): string | null {
  const label = field.name || "(sin nombre)";
  return dynamicSourceError(field.dynamicSource, label);
}

/**
 * Núcleo PLANO de validación de una fuente dinámica (tabla/id/etiqueta no
 * vacías), reutilizable por campos de acción (`dynamicFieldError`) y por
 * columnas con fuente dinámica. Aplica las 3 reglas en orden y devuelve el
 * mensaje (español) de la PRIMERA incumplida, o null si es válida. `label` es
 * el nombre legible (campo o columna) que se interpola en el mensaje. NO
 * sustituye la validación del backend (vuelve a validar contra la lista
 * blanca).
 */
export function dynamicSourceError(
  ds: { table: string; idColumn: string; labelColumn: string },
  label: string,
): string | null {
  if (!ds || ds.table.trim() === "") {
    return `El campo "${label}" de lista dinámica necesita una tabla de origen`;
  }
  if (ds.idColumn.trim() === "") {
    return `El campo "${label}" de lista dinámica necesita una columna ID`;
  }
  if (ds.labelColumn.trim() === "") {
    return `El campo "${label}" de lista dinámica necesita una columna etiqueta`;
  }
  return null;
}

/**
 * Valida una estrategia de llenado (source === "fillStrategy") antes de guardar,
 * análogo a `dynamicFieldError`/`selectFieldError`. Devuelve el mensaje de error
 * (español) de la PRIMERA regla incumplida, o null si es válida. Las capturas
 * declaradas de la acción (`captureKeys`) se usan para comprobar que las
 * derivaciones referencian una captura existente. NO sustituye la validación del
 * backend (autoridad final). Reglas por `kind`:
 *   - capture-from-catalog: catálogo completo (tabla/id/label) y clave de captura
 *     no vacía.
 *   - derive-from-option: `sourceCaptureKey` existente entre las capturas y
 *     `attribute` no vacío.
 *   - derive-via-fk: tabla/columna relacionadas, captura local existente,
 *     atributo local, y al menos un mapeo con atributo y columna destino.
 *   - constant / autogenerated: sin parámetros, siempre válidas.
 */
export function strategyError(
  field: FieldFormState,
  captureKeys: ReadonlyArray<string>,
): string | null {
  const label = field.name || "(sin nombre)";
  const s = field.strategy;
  const known = new Set(captureKeys);
  switch (s.kind) {
    case "capture-from-catalog": {
      const dsErr = dynamicSourceError(s.catalog, label);
      if (dsErr) {
        return dsErr;
      }
      if (s.captureKey.trim() === "") {
        return `La captura "${label}" necesita una clave de captura`;
      }
      return null;
    }
    case "derive-from-option": {
      if (s.sourceCaptureKey.trim() === "" || !known.has(s.sourceCaptureKey.trim())) {
        return `El campo "${label}" referencia una captura de origen inexistente`;
      }
      if (s.attribute.trim() === "") {
        return `El campo "${label}" necesita un atributo de la opción`;
      }
      return null;
    }
    case "derive-via-fk": {
      if (s.jump.relatedTable.trim() === "" || s.jump.relatedColumn.trim() === "") {
        return `El campo "${label}" necesita la tabla y columna relacionadas`;
      }
      if (
        s.jump.localCaptureKey.trim() === "" ||
        !known.has(s.jump.localCaptureKey.trim())
      ) {
        return `El campo "${label}" referencia una captura local inexistente`;
      }
      if (s.jump.localAttribute.trim() === "") {
        return `El campo "${label}" necesita un atributo local del salto`;
      }
      const maps = s.mapping.filter(
        (m) => m.attribute.trim() !== "" && m.targetColumn.trim() !== "",
      );
      if (maps.length === 0) {
        return `El campo "${label}" necesita al menos un mapeo atributo→columna`;
      }
      return null;
    }
    case "constant":
    case "autogenerated":
    default:
      return null;
  }
}

/**
 * Valida que una acción esté completa (Req. 7.7). Con `sqlOp` derivado del tipo,
 * toda acción NO-custom queda completa por construcción; solo `custom` sigue
 * exigiendo que el usuario declare la operación SQL.
 */
export function actionIsComplete(a: ActionFormState): boolean {
  if (a.kind === "custom") {
    return a.sqlOp.trim() !== "";
  }
  return true;
}
