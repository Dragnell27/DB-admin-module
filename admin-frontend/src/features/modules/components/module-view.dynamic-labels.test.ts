// Pruebas puras del helper dynamicColumnBindings de module-view: empareja una
// columna visible del grid con su FieldSpec dinámico (source === "dynamic") de
// alguna acción, deduplica por columna y deja fuera las columnas no dinámicas.
// No renderiza el componente (evita el hang jsdom/userEvent en Windows).
import { describe, expect, it } from "vitest";

import { dynamicColumnBindings } from "@/features/modules/components/module-view";
import type {
  DisplayColumn,
  ModuleAction,
  ModuleDefinition,
} from "@/features/modules/types";

/** Columna visible mínima. */
function col(field: string, sourceTable?: string): DisplayColumn {
  return {
    field,
    label: field,
    order: 0,
    visible: true,
    isPrimaryKey: false,
    editable: true,
    sourceTable,
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

describe("dynamicColumnBindings", () => {
  it("empareja una columna visible con un FieldSpec dinámico (N=1)", () => {
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

    const bindings = dynamicColumnBindings(def, cols, false, "planes");

    expect(bindings).toEqual([
      { renderKey: "forma_pago", field: "forma_pago", actionKey: "crear" },
    ]);
  });

  it("usa el alias <tabla>__<col> como renderKey en multi-tabla con sourceTable", () => {
    const def = defWith([
      action("editar", "edit", [
        {
          name: "forma_pago",
          type: "string",
          required: true,
          source: "dynamic",
          sourceTable: "pagos",
          dynamicSource: { table: "formas_pago", idColumn: "id", labelColumn: "nombre" },
        },
      ]),
    ]);
    const cols = [col("forma_pago", "pagos")];

    const bindings = dynamicColumnBindings(def, cols, true, "planes");

    expect(bindings).toEqual([
      { renderKey: "pagos__forma_pago", field: "forma_pago", actionKey: "editar" },
    ]);
  });

  it("no empareja columnas cuyo field es input/select/constant (no dinámico)", () => {
    const def = defWith([
      action("crear", "create", [
        { name: "nombre", type: "string", required: true, source: "input" },
        {
          name: "tipo",
          type: "string",
          required: false,
          source: "select",
          options: [{ value: "1", label: "A" }],
        },
        { name: "estado", type: "string", required: false, source: "constant" },
      ]),
    ]);
    const cols = [col("nombre"), col("tipo"), col("estado")];

    expect(dynamicColumnBindings(def, cols, false, "planes")).toEqual([]);
  });

  it("devuelve lista vacía para un def sin columnas dinámicas (garantía no-fetch)", () => {
    const def = defWith([action("crear", "create", [])]);
    expect(dynamicColumnBindings(def, [col("nombre")], false, "planes")).toEqual([]);
    expect(dynamicColumnBindings(undefined, [col("nombre")], false, "planes")).toEqual([]);
  });

  it("deduplica por columna cuando dos acciones la referencian y prefiere create/edit", () => {
    const dyn = {
      name: "forma_pago",
      type: "string" as const,
      required: true,
      source: "dynamic" as const,
      dynamicSource: { table: "formas_pago", idColumn: "id", labelColumn: "nombre" },
    };
    // Orden en def.actions: primero una custom, luego edit. El helper debe
    // preferir edit (create/edit) y emitir un único binding.
    const def = defWith([
      action("custom", "custom", [dyn]),
      action("editar", "edit", [dyn]),
    ]);
    const cols = [col("forma_pago")];

    const bindings = dynamicColumnBindings(def, cols, false, "planes");

    expect(bindings).toEqual([
      { renderKey: "forma_pago", field: "forma_pago", actionKey: "editar" },
    ]);
  });

  it("solo empareja columnas visibles provistas (no empareja si la columna no está)", () => {
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
    // La columna forma_pago NO está en las visibles ⇒ sin binding.
    expect(dynamicColumnBindings(def, [col("nombre")], false, "planes")).toEqual([]);
  });
});
