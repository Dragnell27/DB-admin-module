// Pruebas puras del esquema del constructor de módulos para la UNIFICACIÓN del
// "Tipo" de acción (FEAT-001): el Tipo es la única fuente de verdad que deriva
// sqlOp/isWrite/permiso/placement, y se introduce el tipo "search" (Buscar).
// Cubre deriveActionShape (tabla borde 2), defaultPlacementForKind,
// effectiveSqlOp (incluido el caso clave del MEDIUM), actionIsComplete,
// permActionForKind('search'), y la normalización/retrocompat de
// toDefinitionInput/fromDefinition (bordes 3 y 4). Tests de función pura.

import { describe, expect, it } from "vitest";

import {
  actionIsComplete,
  defaultPlacementForKind,
  deriveActionShape,
  effectiveSqlOp,
  emptyStrategyFormState,
  emptyUniquenessFormState,
  emptyWizardState,
  fromDefinition,
  permActionForKind,
  toDefinitionInput,
  type ActionFormState,
  type WizardState,
} from "@/features/modules/schema";
import type { ModuleDefinitionInput } from "@/features/modules/types";

/** Acción de formulario mínima; se sobreescribe lo relevante por test. */
function action(overrides: Partial<ActionFormState> = {}): ActionFormState {
  return {
    actionKey: "acc",
    kind: "custom",
    label: "",
    placement: "row",
    destructive: false,
    permAction: "custom:acc",
    isWrite: true,
    sqlOp: "",
    inputFields: [],
    uniqueness: emptyUniquenessFormState(),
    ...overrides,
  };
}

/** Estado del asistente mínimo con una sola tabla y una lista de acciones. */
function stateWith(actions: ActionFormState[]): WizardState {
  return {
    ...emptyWizardState(),
    slug: "demo",
    name: "Demo",
    pgTable: "cuentas",
    pgWhitelist: { cuentas: ["id", "nombre"] },
    actions,
  };
}

describe("deriveActionShape", () => {
  it("list ⇒ SELECT, isWrite=false (ignora el sqlOp previo)", () => {
    expect(deriveActionShape("list", "INSERT")).toEqual({
      sqlOp: "SELECT",
      isWrite: false,
    });
  });

  it("search ⇒ SELECT, isWrite=false", () => {
    expect(deriveActionShape("search", "")).toEqual({
      sqlOp: "SELECT",
      isWrite: false,
    });
  });

  it("create ⇒ INSERT, isWrite=true", () => {
    expect(deriveActionShape("create", "")).toEqual({
      sqlOp: "INSERT",
      isWrite: true,
    });
  });

  it("edit ⇒ UPDATE, isWrite=true", () => {
    expect(deriveActionShape("edit", "")).toEqual({
      sqlOp: "UPDATE",
      isWrite: true,
    });
  });

  it("delete ⇒ DELETE, isWrite=true", () => {
    expect(deriveActionShape("delete", "")).toEqual({
      sqlOp: "DELETE",
      isWrite: true,
    });
  });

  it("custom con '' respeta sqlOp vacío e isWrite=true", () => {
    expect(deriveActionShape("custom", "")).toEqual({
      sqlOp: "",
      isWrite: true,
    });
  });

  it("custom con INSERT/UPDATE/DELETE ⇒ isWrite=true, sqlOp intacto", () => {
    expect(deriveActionShape("custom", "INSERT")).toEqual({
      sqlOp: "INSERT",
      isWrite: true,
    });
    expect(deriveActionShape("custom", "UPDATE")).toEqual({
      sqlOp: "UPDATE",
      isWrite: true,
    });
    expect(deriveActionShape("custom", "DELETE")).toEqual({
      sqlOp: "DELETE",
      isWrite: true,
    });
  });

  it("custom con SELECT ⇒ isWrite=false, sqlOp intacto", () => {
    expect(deriveActionShape("custom", "SELECT")).toEqual({
      sqlOp: "SELECT",
      isWrite: false,
    });
  });
});

describe("defaultPlacementForKind", () => {
  it("header para list/search/create", () => {
    expect(defaultPlacementForKind("list")).toBe("header");
    expect(defaultPlacementForKind("search")).toBe("header");
    expect(defaultPlacementForKind("create")).toBe("header");
  });

  it("row para edit/delete/custom", () => {
    expect(defaultPlacementForKind("edit")).toBe("row");
    expect(defaultPlacementForKind("delete")).toBe("row");
    expect(defaultPlacementForKind("custom")).toBe("row");
  });
});

describe("effectiveSqlOp", () => {
  it("no-custom devuelve el valor fijo del tipo", () => {
    expect(effectiveSqlOp(action({ kind: "list" }))).toBe("SELECT");
    expect(effectiveSqlOp(action({ kind: "search" }))).toBe("SELECT");
    expect(effectiveSqlOp(action({ kind: "create" }))).toBe("INSERT");
    expect(effectiveSqlOp(action({ kind: "edit" }))).toBe("UPDATE");
    expect(effectiveSqlOp(action({ kind: "delete" }))).toBe("DELETE");
  });

  it("custom normaliza el sqlOp del estado a mayúsculas", () => {
    expect(effectiveSqlOp(action({ kind: "custom", sqlOp: "select" }))).toBe(
      "SELECT",
    );
  });

  it("CASO CLAVE (MEDIUM): list con sqlOp crudo incoherente sigue SELECT", () => {
    expect(effectiveSqlOp(action({ kind: "list", sqlOp: "" }))).toBe("SELECT");
    expect(effectiveSqlOp(action({ kind: "list", sqlOp: "INSERT" }))).toBe(
      "SELECT",
    );
  });
});

describe("permActionForKind", () => {
  it("search ⇒ 'ver' (lectura)", () => {
    expect(permActionForKind("search", "lo_que_sea")).toBe("ver");
  });
});

describe("actionIsComplete", () => {
  it("custom sin operación ⇒ false", () => {
    expect(actionIsComplete(action({ kind: "custom", sqlOp: "" }))).toBe(false);
  });

  it("custom con operación ⇒ true", () => {
    expect(actionIsComplete(action({ kind: "custom", sqlOp: "SELECT" }))).toBe(
      true,
    );
  });

  it("search/list y demás no-custom ⇒ true aunque sqlOp esté vacío", () => {
    expect(actionIsComplete(action({ kind: "search", sqlOp: "" }))).toBe(true);
    expect(actionIsComplete(action({ kind: "list", sqlOp: "" }))).toBe(true);
    expect(actionIsComplete(action({ kind: "edit", sqlOp: "" }))).toBe(true);
  });
});

describe("toDefinitionInput (normalización, borde 4)", () => {
  it("list con sqlOp=INSERT ⇒ persiste SELECT e isWrite=false", () => {
    const def = toDefinitionInput(
      stateWith([action({ kind: "list", sqlOp: "INSERT", isWrite: true })]),
    );
    expect(def.actions[0].sqlOp).toBe("SELECT");
    expect(def.actions[0].isWrite).toBe(false);
  });

  it("search ⇒ persiste sqlOp=SELECT, isWrite=false", () => {
    const def = toDefinitionInput(
      stateWith([action({ kind: "search", sqlOp: "" })]),
    );
    expect(def.actions[0].kind).toBe("search");
    expect(def.actions[0].sqlOp).toBe("SELECT");
    expect(def.actions[0].isWrite).toBe(false);
  });

  it("custom+SELECT con matchType=partial ⇒ respeta SELECT y emite partial", () => {
    const def = toDefinitionInput(
      stateWith([
        action({
          kind: "custom",
          sqlOp: "SELECT",
          inputFields: [
            {
              name: "nombre",
              type: "string",
              required: false,
              source: "input",
              rowField: "",
              constValue: "",
              options: [],
              defaultValue: "",
              dynamicSource: {
                table: "",
                idColumn: "",
                labelColumn: "",
                order: { column: "", direction: "asc" },
              },
              dynamicFilters: [],
              hidden: false,
              label: "",
              captureOnly: false,
              strategy: emptyStrategyFormState(),
              matchType: "partial",
            },
          ],
        }),
      ]),
    );
    expect(def.actions[0].sqlOp).toBe("SELECT");
    expect(def.actions[0].inputFields[0].matchType).toBe("partial");
  });

  it("list con sqlOp crudo incoherente y campo partial ⇒ emite partial vía effectiveSqlOp", () => {
    const def = toDefinitionInput(
      stateWith([
        action({
          kind: "list",
          sqlOp: "INSERT",
          inputFields: [
            {
              name: "nombre",
              type: "string",
              required: false,
              source: "input",
              rowField: "",
              constValue: "",
              options: [],
              defaultValue: "",
              dynamicSource: {
                table: "",
                idColumn: "",
                labelColumn: "",
                order: { column: "", direction: "asc" },
              },
              dynamicFilters: [],
              hidden: false,
              label: "",
              captureOnly: false,
              strategy: emptyStrategyFormState(),
              matchType: "partial",
            },
          ],
        }),
      ]),
    );
    expect(def.actions[0].sqlOp).toBe("SELECT");
    expect(def.actions[0].inputFields[0].matchType).toBe("partial");
  });
});

describe("fromDefinition (retrocompat, borde 4)", () => {
  function defWithAction(
    a: Record<string, unknown>,
  ): ModuleDefinitionInput {
    return {
      slug: "demo",
      name: "Demo",
      sourceType: "postgres",
      countryScope: "none",
      fetchMapping: { table: "cuentas" },
      displayMapping: { columns: [] },
      listParams: {},
      dataSource: {
        sourceType: "postgres",
        pgWhitelist: { cuentas: ["id", "nombre"] },
        connectionTemplateId: "tpl",
        hasSecrets: false,
      },
      actions: [a as never],
    };
  }

  it("custom+SELECT con campos sigue custom/SELECT y preserva matchType", () => {
    const st = fromDefinition(
      defWithAction({
        actionKey: "buscar",
        kind: "custom",
        label: "Buscar",
        placement: "row",
        destructive: false,
        permAction: "custom:buscar",
        isWrite: false,
        sqlOp: "SELECT",
        inputFields: [
          {
            name: "nombre",
            type: "string",
            required: false,
            source: "input",
            matchType: "partial",
          },
        ],
      }),
    );
    expect(st.actions[0].kind).toBe("custom");
    expect(st.actions[0].sqlOp).toBe("SELECT");
    expect(st.actions[0].inputFields[0].matchType).toBe("partial");
  });

  it("edit con sqlOp='' hidrata sin romper (sqlOp respetado tal cual)", () => {
    const st = fromDefinition(
      defWithAction({
        actionKey: "editar",
        kind: "edit",
        label: "Editar",
        placement: "row",
        destructive: false,
        permAction: "editar",
        isWrite: true,
        sqlOp: "",
        inputFields: [],
      }),
    );
    expect(st.actions[0].kind).toBe("edit");
    expect(st.actions[0].sqlOp).toBe("");
  });
});
