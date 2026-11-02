/**
 * Lógica PURA de `ActionFormDialog` (module-builder), extraída del componente
 * para poder cubrirla en entorno node sin montar React/DOM.
 *
 * El montaje del componente bajo jsdom/happy-dom cuelga el worker fork de Vitest
 * 5.0.3 en Windows (ver `.agents/tasks/fix-hanging-test/spike.json`). Para no
 * perder cobertura de la semántica sin DOM (coerción de valores, construcción de
 * valores iniciales, validación de obligatoriedad y armado del input de envío),
 * esas piezas viven aquí como funciones puras y el componente las consume. Este
 * módulo NO importa React ni JSX: se importa en entorno node sin arrastrar jsdom.
 *
 * Precedente en el repo: `deriveSaveErrorMessage` (module-wizard) se extrajo del
 * mismo modo y se testea en `derive-save-error-message.test.ts`.
 */
import type { FieldSpec, FieldType, SelectOption } from "@/features/modules/types";

/**
 * Campo de un formulario GENERADO DESDE COLUMNAS (acciones de editar, Opción B).
 * Cuando `ActionFormDialog` recibe `columnFields`, construye el formulario a
 * partir de estas entradas en vez de `action.inputFields`. Un campo `readOnly`
 * (columna llave primaria o no editable) se muestra precargado y deshabilitado.
 *
 * Fuente única del tipo: el componente lo reexporta desde aquí.
 */
export interface ColumnField {
  readonly field: string;
  readonly label: string;
  readonly type: FieldType;
  readonly readOnly: boolean;
  readonly required: boolean;
  /**
   * Si se provee, la columna objetivo tiene origen LISTA: el control se renderiza
   * como desplegable con estas opciones (etiqueta/valor), en vez de un input.
   */
  readonly options?: ReadonlyArray<SelectOption>;
  /** Valor preseleccionado del desplegable (origen lista). */
  readonly defaultValue?: string;
  /**
   * Origen DINÁMICO: las opciones se están cargando del endpoint. Puramente de
   * presentación (deshabilita el desplegable y muestra "Cargando…").
   */
  readonly optionsLoading?: boolean;
  /**
   * Origen DINÁMICO: la carga de opciones falló (502). Puramente de presentación
   * (muestra "No se pudieron cargar las opciones").
   */
  readonly optionsError?: boolean;
}

/** Convierte el valor crudo de un control al tipo declarado del campo. */
export function coerce(field: FieldSpec, raw: string | boolean): unknown {
  // Un campo de origen lista (ESTÁTICA "select" o DINÁMICA "dynamic") envía el
  // VALUE elegido (string) tal cual, sin coaccionar por `type`: en select el
  // backend compara contra opt.Value (string) y en dynamic el value es el id FK
  // que viaja verbatim como bound param (Postgres lo coacciona). Así un value
  // "1" viaja como "1" y no como el número 1.
  if (field.source === "select" || field.source === "dynamic") {
    return raw;
  }
  if (field.type === "boolean") {
    return Boolean(raw);
  }
  if (field.type === "number") {
    if (raw === "" || raw === null) {
      return undefined;
    }
    const n = Number(raw);
    return Number.isNaN(n) ? raw : n;
  }
  return raw;
}

/**
 * Campos visibles del formulario (modo inputFields): los constantes NO se
 * muestran, inicializan ni validan (su valor literal lo resuelve el backend
 * desde constValue).
 */
export function getVisibleFields(inputFields: FieldSpec[]): FieldSpec[] {
  return inputFields.filter((f) => f.source !== "constant");
}

/** Argumentos para construir el estado inicial del formulario. */
export interface BuildInitialValuesArgs {
  readonly columnMode: boolean;
  readonly columnFields?: ReadonlyArray<ColumnField>;
  readonly visibleFields: FieldSpec[];
  readonly initialValues: Record<string, unknown>;
}

/**
 * Construye el `Record` inicial de valores del formulario para AMBAS ramas
 * (columnMode e inputFields), con la precedencia: valor precargado explícito →
 * defaultValue del campo → "" (placeholder). Boolean nunca viaja como "".
 */
export function buildInitialValues({
  columnMode,
  columnFields,
  visibleFields,
  initialValues,
}: BuildInitialValuesArgs): Record<string, string | boolean> {
  const init: Record<string, string | boolean> = {};
  if (columnMode) {
    for (const cf of columnFields ?? []) {
      const v = initialValues[cf.field];
      if (cf.options) {
        // Origen lista: precarga valor de la fila, o la preselección, o vacío.
        init[cf.field] =
          v != null && String(v) !== "" ? String(v) : (cf.defaultValue ?? "");
      } else {
        init[cf.field] = v === undefined || v === null ? "" : String(v);
      }
    }
    return init;
  }
  for (const f of visibleFields) {
    // Precarga por nombre exacto del parámetro: los valores que vienen de la
    // fila ya llegan en initialValues bajo el nombre del campo (resueltos por
    // el mapeo explícito "Se llena con" del usuario, sin adivinanzas).
    // Rama PREVIA por origen lista ("select" ESTÁTICA o "dynamic" DINÁMICA):
    // precedencia determinista valor precargado explícito → defaultValue del
    // campo → "" (placeholder). En dinámico el valor precargado es el id actual
    // de la fila (al editar), que preselecciona la opción cargada.
    if (f.source === "select" || f.source === "dynamic") {
      const pre = initialValues[f.name];
      init[f.name] =
        pre != null && String(pre) !== "" ? String(pre) : (f.defaultValue ?? "");
      continue;
    }
    const v = initialValues[f.name];
    if (f.type === "boolean") {
      init[f.name] = Boolean(v);
    } else {
      init[f.name] = v === undefined || v === null ? "" : String(v);
    }
  }
  return init;
}

/** Argumentos para validar la obligatoriedad del formulario. */
export interface ValidateRequiredArgs {
  readonly columnMode: boolean;
  readonly columnFields?: ReadonlyArray<ColumnField>;
  readonly visibleFields: FieldSpec[];
  readonly values: Record<string, string | boolean>;
}

/**
 * Calcula el mapa de errores de obligatoriedad para ambas ramas. En columnMode
 * solo valida las columnas EDITABLES (no readOnly). En inputFields un boolean
 * nunca se considera vacío. Un mapa vacío significa "sin errores".
 */
export function validateRequired({
  columnMode,
  columnFields,
  visibleFields,
  values,
}: ValidateRequiredArgs): Record<string, string> {
  const nextErrors: Record<string, string> = {};
  if (columnMode) {
    const editable = (columnFields ?? []).filter((cf) => !cf.readOnly);
    for (const cf of editable) {
      const empty = String(values[cf.field] ?? "").trim() === "";
      if (cf.required && empty) {
        nextErrors[cf.field] = "Este campo es obligatorio";
      }
    }
    return nextErrors;
  }
  for (const f of visibleFields) {
    const v = values[f.name];
    const empty = f.type === "boolean" ? false : String(v ?? "").trim() === "";
    if (f.required && empty) {
      nextErrors[f.name] = "Este campo es obligatorio";
    }
  }
  return nextErrors;
}

/** Argumentos para construir el input de envío. */
export interface BuildSubmitInputArgs {
  readonly columnMode: boolean;
  readonly columnFields?: ReadonlyArray<ColumnField>;
  readonly visibleFields: FieldSpec[];
  readonly values: Record<string, string | boolean>;
}

/**
 * Construye el `Record` de envío para ambas ramas. En columnMode solo viajan las
 * columnas editables; se coaccionan a número solo las no-lista numéricas y se
 * excluyen los valores vacíos/undefined. En inputFields usa `coerce` y excluye
 * undefined/"" (los campos constantes ya no están en visibleFields).
 */
export function buildSubmitInput({
  columnMode,
  columnFields,
  visibleFields,
  values,
}: BuildSubmitInputArgs): Record<string, unknown> {
  const input: Record<string, unknown> = {};
  if (columnMode) {
    // Solo las columnas EDITABLES (no readOnly). Las readOnly (PK/no editables)
    // se muestran precargadas pero su valor NO viaja desde aquí: module-view
    // reañade el valor de la PK desde la fila.
    const editable = (columnFields ?? []).filter((cf) => !cf.readOnly);
    for (const cf of editable) {
      const raw = values[cf.field] ?? "";
      // Origen lista: el value elegido viaja tal cual (string); el backend lo
      // compara contra opt.value. Solo los campos numéricos no-lista se coaccionan.
      const v = !cf.options && cf.type === "number" && raw !== "" ? Number(raw) : raw;
      if (v !== undefined && v !== "") {
        input[cf.field] = v;
      }
    }
    return input;
  }
  // No se incluyen los campos constantes en el input: el backend los resuelve
  // desde constValue (ya excluidos de visibleFields).
  for (const f of visibleFields) {
    const coerced = coerce(f, values[f.name] ?? "");
    if (coerced !== undefined && coerced !== "") {
      input[f.name] = coerced;
    }
  }
  return input;
}
