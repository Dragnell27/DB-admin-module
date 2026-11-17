import { describe, expect, it } from "vitest";

import {
  conditionsError,
  dynamicFieldError,
  fromDefinition,
  relationshipError,
  strategyError,
  toDefinitionInput,
  type ConditionFormState,
  type FieldFormState,
  type RelationshipFormState,
} from "@/features/modules/schema";
import type { ModuleDefinitionInput } from "@/features/modules/types";

/** Definición mínima con los sitios que llevan operador. */
function baseDef(over: Partial<ModuleDefinitionInput> = {}): ModuleDefinitionInput {
  return {
    slug: "demo",
    name: "Demo",
    sourceType: "postgres",
    countryScope: "none",
    fetchMapping: { table: "pagos" },
    displayMapping: { columns: [] },
    listParams: {},
    dataSource: {
      sourceType: "postgres",
      pgWhitelist: { pagos: ["id", "estado"] },
      connectionTemplateId: "tpl",
      hasSecrets: false,
    },
    actions: [],
    ...over,
  };
}

function roundTrip(def: ModuleDefinitionInput) {
  return toDefinitionInput(fromDefinition(def));
}

describe("round-trip de operadores en condiciones de listado", () => {
  it("sin operator no añade operator al reserializar", () => {
    const out = roundTrip(
      baseDef({ listConditions: [{ field: "estado", values: ["A", "B"] }] }),
    );
    expect(out.listConditions).toEqual([{ field: "estado", values: ["A", "B"] }]);
    expect(out.listConditions?.[0].operator).toBeUndefined();
  });

  it("'in' explícito se omite (default) y los demás operadores se conservan", () => {
    const out = roundTrip(
      baseDef({
        listConditions: [
          { field: "estado", values: ["A"], operator: "in" },
          { field: "estado", values: ["x, y"], operator: "contains" },
          { field: "estado", values: ["A", "B"], operator: "not_in" },
        ],
      }),
    );
    expect(out.listConditions?.[0].operator).toBeUndefined();
    expect(out.listConditions?.[1]).toMatchObject({ values: ["x, y"], operator: "contains" });
    expect(out.listConditions?.[2]).toMatchObject({ values: ["A", "B"], operator: "not_in" });
  });

  it("is_null sin valores no se descarta", () => {
    const out = roundTrip(
      baseDef({ listConditions: [{ field: "estado", values: [], operator: "is_null" }] }),
    );
    expect(out.listConditions).toEqual([{ field: "estado", values: [], operator: "is_null" }]);
  });

  it("operador desconocido se lee como el default", () => {
    const st = fromDefinition(
      baseDef({
        listConditions: [
          { field: "estado", values: ["A"], operator: "drop" as never },
        ],
      }),
    );
    expect(st.conditions[0].operator).toBeUndefined();
  });
});

describe("round-trip de operadores en filtros de catálogo", () => {
  it("filtros de campo dinámico y de estrategia conservan el operador", () => {
    const def = baseDef({
      actions: [
        {
          actionKey: "crear",
          kind: "create",
          label: "Crear",
          placement: "header",
          destructive: false,
          permAction: "create",
          isWrite: true,
          sqlOp: "INSERT",
          inputFields: [
            {
              name: "estado",
              type: "string",
              required: false,
              source: "dynamic",
              dynamicSource: {
                table: "cat",
                idColumn: "id",
                labelColumn: "nombre",
                filters: [{ field: "tipo", values: ["a"], operator: "starts_with" }],
              },
            },
            {
              name: "__capture_x",
              type: "string",
              required: false,
              source: "fillStrategy",
              strategy: {
                kind: "capture-from-catalog",
                captureKey: "x",
                catalog: {
                  table: "cat",
                  idColumn: "id",
                  labelColumn: "nombre",
                  filters: [{ field: "tipo", values: [], operator: "is_not_null" }],
                },
              },
            },
          ],
        },
      ],
    });
    const st = fromDefinition(def);
    expect(st.actions[0].inputFields[0].dynamicFilters[0].operator).toBe("starts_with");
    expect(st.actions[0].inputFields[1].strategy.catalogFilters[0].operator).toBe("is_not_null");
    const out = toDefinitionInput(st);
    expect(out.actions[0].inputFields[0].dynamicSource?.filters?.[0].operator).toBe("starts_with");
    expect(out.actions[0].inputFields[1].strategy?.catalog?.filters?.[0]).toMatchObject({
      operator: "is_not_null",
      values: [],
    });
  });

  it("filtros de la fuente dinámica de una columna conservan el operador", () => {
    const def = baseDef({
      displayMapping: {
        columns: [
          {
            field: "estado",
            label: "Estado",
            order: 0,
            visible: true,
            valueSource: {
              kind: "dynamic",
              dynamicSource: {
                table: "cat",
                idColumn: "id",
                labelColumn: "nombre",
                filters: [{ field: "tipo", values: ["a"], operator: "neq" }],
              },
            },
          },
        ],
      },
    });
    const out = roundTrip(def);
    const cols = (
      out.displayMapping as {
        columns: Array<{ valueSource: { dynamicSource: { filters: unknown[] } } }>;
      }
    ).columns;
    expect(cols[0].valueSource.dynamicSource.filters[0]).toMatchObject({
      field: "tipo",
      operator: "neq",
    });
  });
});

describe("round-trip de operadores en filtros de acción", () => {
  function actionDef(field: Record<string, unknown>, sqlOp = "SELECT"): ModuleDefinitionInput {
    return baseDef({
      actions: [
        {
          actionKey: "buscar",
          kind: "custom",
          label: "Buscar",
          placement: "header",
          destructive: false,
          permAction: "custom:buscar",
          isWrite: false,
          sqlOp,
          inputFields: [{ name: "estado", type: "string", required: false, source: "input", ...field }],
        } as never,
      ],
    });
  }

  it("matchType 'partial' legado se lee como contains y nunca se emite matchType", () => {
    const st = fromDefinition(actionDef({ matchType: "partial" }));
    expect(st.actions[0].inputFields[0].operator).toBe("contains");
    const out = toDefinitionInput(st);
    expect(out.actions[0].inputFields[0].operator).toBe("contains");
    expect(out.actions[0].inputFields[0].matchType).toBeUndefined();
  });

  it("sin operator ni matchType no se emite operator", () => {
    const out = roundTrip(actionDef({}));
    expect(out.actions[0].inputFields[0].operator).toBeUndefined();
    expect(out.actions[0].inputFields[0].matchType).toBeUndefined();
  });

  it("las escrituras no emiten operator", () => {
    const st = fromDefinition(actionDef({ operator: "contains" }, "UPDATE"));
    expect(toDefinitionInput(st).actions[0].inputFields[0].operator).toBeUndefined();
  });
});

describe("round-trip de onConstants", () => {
  function relDef(onConstants: unknown[]): ModuleDefinitionInput {
    return baseDef({
      fetchMapping: {
        table: "pagos",
        foreignTables: [{ table: "listas" }],
        relationships: [
          {
            childTable: "pagos",
            parentTable: "listas",
            on: [{ child: "lista_id", parent: "id" }],
            onConstants,
            joinType: "left",
          },
        ],
      },
      dataSource: {
        sourceType: "postgres",
        pgWhitelist: { pagos: ["id", "lista_id"], listas: ["id", "num"] },
        connectionTemplateId: "tpl",
        hasSecrets: false,
      },
    });
  }
  const consts = (def: ModuleDefinitionInput) =>
    (def.fetchMapping.relationships as Array<{ onConstants?: unknown[] }>)[0].onConstants;

  it("in: 'a, b' <-> values ['a','b']", () => {
    const def = relDef([{ table: "listas", column: "num", value: "", operator: "in", values: ["a", "b"] }]);
    const st = fromDefinition(def);
    expect(st.relationships[0].onConstants[0].value).toBe("a, b");
    expect(consts(toDefinitionInput(st))).toEqual([
      { table: "listas", column: "num", value: "", operator: "in", values: ["a", "b"] },
    ]);
  });

  it("legado sin operator se reserializa sin operator", () => {
    const def = relDef([{ table: "listas", column: "num", value: "10001" }]);
    const out = consts(roundTrip(def));
    expect(out).toEqual([{ table: "listas", column: "num", value: "10001" }]);
  });

  it("nulidad emite value vacío y operador", () => {
    const def = relDef([{ table: "listas", column: "num", value: "", operator: "is_null" }]);
    expect(consts(roundTrip(def))).toEqual([
      { table: "listas", column: "num", value: "", operator: "is_null" },
    ]);
  });
});

describe("conditionsError", () => {
  const cond = (over: Partial<ConditionFormState>): ConditionFormState => ({
    field: "estado",
    valuesText: "",
    ...over,
  });

  it("rechaza in/not_in sin valores", () => {
    expect(conditionsError([cond({ operator: "in" })], "listado")).toBe(
      "La condición sobre «estado» (listado): el operador «está en» necesita al menos un valor",
    );
    expect(conditionsError([cond({ operator: "not_in", valuesText: " , " })], "listado")).toContain(
      "necesita al menos un valor",
    );
  });

  it("rechaza eq/neq/contains/starts_with/ends_with con valor vacío", () => {
    for (const operator of ["eq", "neq", "contains", "starts_with", "ends_with"] as const) {
      expect(conditionsError([cond({ operator })], "listado")).toContain("necesita un valor");
    }
  });

  it("acepta nulidad sin valor y el legado sin operador", () => {
    expect(conditionsError([cond({ operator: "is_null" })], "listado")).toBeNull();
    expect(conditionsError([cond({})], "listado")).toBeNull();
    expect(conditionsError([cond({ field: "", operator: "eq" })], "listado")).toBeNull();
  });

  it("se engancha en campos dinámicos, estrategias y constantes del ON", () => {
    const field = {
      name: "estado",
      dynamicSource: { table: "t", idColumn: "id", labelColumn: "n" },
      dynamicFilters: [cond({ operator: "eq" })],
      strategy: {
        kind: "capture-from-catalog",
        catalog: { table: "t", idColumn: "id", labelColumn: "n" },
        catalogFilters: [cond({ operator: "contains" })],
        captureKey: "k",
      },
    } as unknown as FieldFormState;
    expect(dynamicFieldError(field)).toContain("«igual a»");
    expect(strategyError(field, [])).toContain("«contiene»");

    const rel: RelationshipFormState = {
      childTable: "a",
      parentTable: "b",
      on: [{ child: "x", parent: "y" }],
      onConstants: [{ table: "b", column: "num", value: "", operator: "in" }],
      joinType: "left",
    };
    expect(relationshipError(rel, ["a", "b"])).toContain("«num»");
    rel.onConstants = [{ table: "b", column: "num", value: "", operator: "is_null" }];
    expect(relationshipError(rel, ["a", "b"])).toBeNull();
  });
});
