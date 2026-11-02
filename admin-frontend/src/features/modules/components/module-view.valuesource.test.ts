// Pruebas puras de la resolución de la FUENTE DE VALORES por columna en la vista
// dinámica (FEAT-004): precedencia columna > FieldSpec de acción, resolución del
// grid (columnValueSourceBindings) y del desplegable de crear/editar
// (resolveColumnOptionSource). No renderiza el componente (entorno node, sin
// jsdom): evita el hang jsdom/userEvent en Windows y aísla la lógica pura.
import { describe, expect, it } from "vitest";

import {
  columnValueSourceBindings,
  resolveColumnOptionSource,
} from "@/features/modules/components/module-view";
import type {
  DisplayColumn,
  ModuleAction,
  ModuleDefinition,
  SelectOption,
} from "@/features/modules/types";

/** Columna visible mínima; acepta una fuente de valores opcional. */
function col(
  field: string,
  extra: Partial<DisplayColumn> = {},
): DisplayColumn {
  return {
    field,
    label: field,
    order: 0,
    visible: true,
    isPrimaryKey: false,
    editable: true,
    ...extra,
  };
}

/** Acción con los inputFields dados. */
function action(
  actionKey: string,
  kind: ModuleAction["kind"],
  inputFields: ModuleAction["inputFields"],
): ModuleAction {
  return {
    actionKey,
    kind,
    label: actionKey,
    placement: "header",
    destructive: false,
    permAction: "crear",
    isWrite: true,
    inputFields,
  };
}

/** Definición con las acciones dadas y una tabla principal. */
function defWith(actions: ModuleAction[], table = "planes"): ModuleDefinition {
  return {
    slug: "planes",
    name: "Planes",
    sourceType: "postgres",
    countryScope: "none",
    fetchMapping: { table },
    displayMapping: {},
    listParams: {},
    dataSource: { sourceType: "postgres", connectionTemplateId: "t", hasSecrets: false },
    actions,
  };
}

const staticOptions: SelectOption[] = [
  { value: "1", label: "Activo" },
  { value: "2", label: "Inactivo" },
];

describe("columnValueSourceBindings", () => {
  it("(a) la columna con valueSource DINÁMICO gana sobre el FieldSpec de acción (precedencia)", () => {
    // La columna declara fuente dinámica propia Y hay un FieldSpec dinámico de
    // acción para el mismo campo: debe mandar la COLUMNA (column-dynamic), no la
    // acción (action-dynamic).
    const def = defWith([
      action("crear", "create", [
        {
          name: "estado",
          type: "string",
          required: true,
          source: "dynamic",
          dynamicSource: { table: "estados_accion", idColumn: "id", labelColumn: "nombre" },
        },
      ]),
    ]);
    const cols = [
      col("estado", {
        valueSource: {
          kind: "dynamic",
          dynamicSource: { table: "estados_col", idColumn: "id", labelColumn: "nombre" },
        },
      }),
    ];

    const bindings = columnValueSourceBindings(def, cols, false, "planes");

    expect(bindings).toEqual([
      { renderKey: "estado", field: "estado", sourceTable: undefined, origin: { kind: "column-dynamic" } },
    ]);
  });

  it("(b) sin valueSource cae al FieldSpec dinámico de la acción (retrocompat)", () => {
    const def = defWith([
      action("crear", "create", [
        {
          name: "forma_pago",
          type: "string",
          required: true,
          source: "dynamic",
          dynamicSource: { table: "formas_pago", idColumn: "id", labelColumn: "nombre" },
        },
      ]),
    ]);
    const cols = [col("nombre"), col("forma_pago")];

    const bindings = columnValueSourceBindings(def, cols, false, "planes");

    expect(bindings).toEqual([
      {
        renderKey: "forma_pago",
        field: "forma_pago",
        sourceTable: undefined,
        origin: { kind: "action-dynamic", actionKey: "crear" },
      },
    ]);
  });

  it("(c) multi-tabla empareja por (field, sourceTable) y la clave es renderKey, no f.name (homónimas no colisionan)", () => {
    // Dos columnas homónimas "estado" en tablas distintas, cada una con su propia
    // fuente dinámica. La clave de cada binding debe ser el alias <tabla>__estado,
    // no "estado", de modo que NO colisionen.
    const def = defWith([], "planes");
    const cols = [
      col("estado", {
        sourceTable: "planes",
        valueSource: {
          kind: "dynamic",
          dynamicSource: { table: "estados_plan", idColumn: "id", labelColumn: "nombre" },
        },
      }),
      col("estado", {
        sourceTable: "pagos",
        valueSource: {
          kind: "dynamic",
          dynamicSource: { table: "estados_pago", idColumn: "id", labelColumn: "nombre" },
        },
      }),
    ];

    const bindings = columnValueSourceBindings(def, cols, true, "planes");

    expect(bindings).toEqual([
      {
        renderKey: "planes__estado",
        field: "estado",
        sourceTable: "planes",
        origin: { kind: "column-dynamic" },
      },
      {
        renderKey: "pagos__estado",
        field: "estado",
        sourceTable: "pagos",
        origin: { kind: "column-dynamic" },
      },
    ]);
  });

  it("(d) column-static produce el origen con las opciones en memoria (sin fetch)", () => {
    const def = defWith([]);
    const cols = [col("estado", { valueSource: { kind: "static", options: staticOptions } })];

    const bindings = columnValueSourceBindings(def, cols, false, "planes");

    expect(bindings).toEqual([
      {
        renderKey: "estado",
        field: "estado",
        sourceTable: undefined,
        origin: { kind: "column-static", options: staticOptions },
      },
    ]);
  });

  it("(e) columna sin fuente ni FieldSpec dinámico => sin binding", () => {
    const def = defWith([
      action("crear", "create", [
        { name: "nombre", type: "string", required: true, source: "input" },
      ]),
    ]);
    const cols = [col("nombre"), col("descripcion")];

    expect(columnValueSourceBindings(def, cols, false, "planes")).toEqual([]);
  });

  it("static vacío no emite column-static; cae al fallback de acción o a nada", () => {
    // valueSource static con options vacío no es utilizable => sin binding (no hay
    // FieldSpec dinámico que lo respalde).
    const def = defWith([]);
    const cols = [col("estado", { valueSource: { kind: "static", options: [] } })];
    expect(columnValueSourceBindings(def, cols, false, "planes")).toEqual([]);
  });
});

describe("resolveColumnOptionSource", () => {
  it("(a) columna estática manda sobre el FieldSpec (mode static con opciones)", () => {
    const act = action("crear", "create", [
      { name: "estado", type: "string", required: true, source: "input" },
    ]);
    const c = col("estado", { valueSource: { kind: "static", options: staticOptions } });

    expect(resolveColumnOptionSource(c, act, "planes")).toEqual({
      mode: "static",
      options: staticOptions,
    });
  });

  it("(a') columna dinámica manda sobre el FieldSpec dinámico (mode dynamic por columna)", () => {
    const act = action("crear", "create", [
      {
        name: "estado",
        type: "string",
        required: true,
        source: "dynamic",
        dynamicSource: { table: "estados_accion", idColumn: "id", labelColumn: "nombre" },
      },
    ]);
    const c = col("estado", {
      sourceTable: "pagos",
      valueSource: {
        kind: "dynamic",
        dynamicSource: { table: "estados_col", idColumn: "id", labelColumn: "nombre" },
      },
    });

    expect(resolveColumnOptionSource(c, act, "planes")).toEqual({
      mode: "dynamic",
      sourceTable: "pagos",
    });
  });

  it("(b) sin valueSource: FieldSpec dinámico => action-dynamic (retrocompat)", () => {
    const act = action("crear", "create", [
      {
        name: "forma_pago",
        type: "string",
        required: true,
        source: "dynamic",
        dynamicSource: { table: "formas_pago", idColumn: "id", labelColumn: "nombre" },
      },
    ]);
    const c = col("forma_pago");

    expect(resolveColumnOptionSource(c, act, "planes")).toEqual({
      mode: "action-dynamic",
      field: "forma_pago",
    });
  });

  it("sin valueSource: FieldSpec select => action-select", () => {
    const act = action("crear", "create", [
      {
        name: "tipo",
        type: "string",
        required: false,
        source: "select",
        options: [{ value: "a", label: "A" }],
      },
    ]);
    const c = col("tipo");

    expect(resolveColumnOptionSource(c, act, "planes")).toEqual({
      mode: "action-select",
      field: "tipo",
    });
  });

  it("(e) sin fuente ni FieldSpec emparejable => plain", () => {
    const act = action("crear", "create", [
      { name: "nombre", type: "string", required: true, source: "input" },
    ]);
    expect(resolveColumnOptionSource(col("nombre"), act, "planes")).toEqual({ mode: "plain" });
  });

  it("(c) multi-tabla: no empareja un FieldSpec de OTRA tabla con la columna homónima", () => {
    // El FieldSpec dinámico pertenece a la tabla "pagos"; la columna "estado" es
    // de la principal. No deben emparejar => plain (no action-dynamic).
    const act = action("crear", "create", [
      {
        name: "estado",
        type: "string",
        required: true,
        source: "dynamic",
        sourceTable: "pagos",
        dynamicSource: { table: "estados_pago", idColumn: "id", labelColumn: "nombre" },
      },
    ]);
    const c = col("estado", { sourceTable: "planes" });

    expect(resolveColumnOptionSource(c, act, "planes")).toEqual({ mode: "plain" });
  });
});
