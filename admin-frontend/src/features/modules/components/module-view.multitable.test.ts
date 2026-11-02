// Pruebas puras de los helpers multi-tabla de module-view: renderKeyFor,
// isMultiTableDef y deleteSetForAction (reimplementación de DeleteOrder). No
// renderiza el componente (evita el hang jsdom/userEvent en Windows).
import { describe, expect, it } from "vitest";

import {
  deleteSetForAction,
  isMultiTableDef,
  renderKeyFor,
} from "@/features/modules/components/module-view";
import type { ModuleAction, ModuleDefinition } from "@/features/modules/types";

function multiDef(): ModuleDefinition {
  return {
    slug: "pagos-clientes",
    name: "Pagos",
    sourceType: "postgres",
    countryScope: "none",
    fetchMapping: {
      table: "pagos",
      foreignTables: [{ table: "clientes" }],
      relationships: [
        {
          childTable: "pagos",
          parentTable: "clientes",
          on: [{ child: "cliente_id", parent: "id" }],
          joinType: "inner",
        },
      ],
    },
    displayMapping: {},
    listParams: {},
    dataSource: { sourceType: "postgres", connectionTemplateId: "t", hasSecrets: false },
    actions: [],
  };
}

describe("isMultiTableDef", () => {
  it("true con relaciones, false sin ellas", () => {
    expect(isMultiTableDef(multiDef())).toBe(true);
    expect(
      isMultiTableDef({ ...multiDef(), fetchMapping: { table: "pagos" } }),
    ).toBe(false);
  });
});

describe("renderKeyFor", () => {
  it("alias en multi-tabla, pelado en N=1", () => {
    expect(renderKeyFor({ field: "id", sourceTable: "clientes" }, true, "pagos")).toBe("clientes__id");
    // Sin sourceTable ⇒ principal.
    expect(renderKeyFor({ field: "id" }, true, "pagos")).toBe("pagos__id");
    // N=1 ⇒ nombre pelado.
    expect(renderKeyFor({ field: "id", sourceTable: "pagos" }, false, "pagos")).toBe("id");
  });
});

describe("deleteSetForAction", () => {
  it("delete hasta clientes borra hijo (pagos) antes que padre (clientes)", () => {
    const action: ModuleAction = {
      actionKey: "delete",
      kind: "delete",
      label: "Eliminar",
      placement: "row",
      destructive: true,
      permAction: "eliminar",
      isWrite: true,
      inputFields: [],
      deleteTopTable: "clientes",
    };
    expect(deleteSetForAction(multiDef(), action, "pagos")).toEqual(["pagos", "clientes"]);
  });

  it("delete por defecto (principal) solo borra la principal", () => {
    const action: ModuleAction = {
      actionKey: "delete",
      kind: "delete",
      label: "Eliminar",
      placement: "row",
      destructive: true,
      permAction: "eliminar",
      isWrite: true,
      inputFields: [],
    };
    expect(deleteSetForAction(multiDef(), action, "pagos")).toEqual(["pagos"]);
  });
});
