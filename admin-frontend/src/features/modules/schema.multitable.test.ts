// Pruebas puras multi-tabla del esquema del constructor: round-trip de
// foráneas/relaciones/sourceTable, treeError, missingPrimaryKey por tabla y
// deleteOrder (reimplementación en TS de DeleteOrder del backend).
import { describe, expect, it } from "vitest";

import {
  deleteOrder,
  emptyWizardState,
  fromDefinition,
  isMultiTableState,
  missingPrimaryKey,
  tablesMissingPrimaryKey,
  relationshipError,
  toDefinitionInput,
  treeError,
  type RelationshipFormState,
  type WizardState,
} from "@/features/modules/schema";
import type { ModuleDefinitionInput } from "@/features/modules/types";

/** Estado multi-tabla pagos ← clientes (pagos hijo FK de clientes). */
function multiState(): WizardState {
  return {
    ...emptyWizardState(),
    slug: "pagos-clientes",
    name: "Pagos",
    pgTable: "pagos",
    foreignTables: ["clientes"],
    relationships: [
      {
        childTable: "pagos",
        parentTable: "clientes",
        on: [{ child: "cliente_id", parent: "id" }],
        onConstants: [],
        joinType: "left",
      },
    ],
    pgWhitelist: {
      pagos: ["id", "cliente_id", "monto"],
      clientes: ["id", "nombre"],
    },
    columns: [
      { field: "id", label: "ID", order: 0, format: "", visible: false, isPrimaryKey: true, editable: false, sourceTable: "pagos" },
      { field: "monto", label: "Monto", order: 1, format: "", visible: true, isPrimaryKey: false, editable: true, sourceTable: "pagos" },
      { field: "id", label: "ID cliente", order: 2, format: "", visible: false, isPrimaryKey: true, editable: false, sourceTable: "clientes" },
      { field: "nombre", label: "Nombre", order: 3, format: "", visible: true, isPrimaryKey: false, editable: true, sourceTable: "clientes" },
    ],
    conditions: [{ field: "nombre", valuesText: "Ana", sourceTable: "clientes" }],
    actions: [
      {
        actionKey: "delete",
        kind: "delete",
        label: "Eliminar",
        placement: "row",
        destructive: true,
        permAction: "eliminar",
        isWrite: true,
        sqlOp: "DELETE",
        inputFields: [],
        deleteTopTable: "clientes",
      },
    ],
  };
}

describe("isMultiTableState", () => {
  it("true con foráneas/relaciones, false sin ellas", () => {
    expect(isMultiTableState(multiState())).toBe(true);
    expect(isMultiTableState(emptyWizardState())).toBe(false);
  });
});

describe("toDefinitionInput/fromDefinition round-trip multi-tabla", () => {
  it("preserva foráneas, relaciones, sourceTable y tope de borrado", () => {
    const input = toDefinitionInput(multiState());

    // fetchMapping lleva foreignTables + relationships.
    expect(input.fetchMapping.foreignTables).toEqual([{ table: "clientes" }]);
    expect(input.fetchMapping.relationships).toEqual([
      {
        childTable: "pagos",
        parentTable: "clientes",
        on: [{ child: "cliente_id", parent: "id" }],
        joinType: "left",
      },
    ]);

    // Columnas de la foránea llevan sourceTable; las de la principal NO (= principal).
    const cols = (input.displayMapping.columns as Array<Record<string, unknown>>);
    const clienteNombre = cols.find((c) => c.field === "nombre");
    const pagosMonto = cols.find((c) => c.field === "monto");
    expect(clienteNombre?.sourceTable).toBe("clientes");
    expect(pagosMonto?.sourceTable).toBeUndefined();

    // Condición cualificada.
    expect(input.listConditions?.[0].sourceTable).toBe("clientes");

    // Acción delete conserva deleteTopTable (distinto de la principal).
    expect(input.actions[0].deleteTopTable).toBe("clientes");

    // Round-trip inverso: fromDefinition reconstruye el estado equivalente.
    const back = fromDefinition(input as ModuleDefinitionInput);
    expect(back.foreignTables).toEqual(["clientes"]);
    expect(back.relationships).toEqual(multiState().relationships);
    expect(back.actions[0].deleteTopTable).toBe("clientes");
    // La columna de la principal recupera sourceTable = principal.
    const backMonto = back.columns.find((c) => c.field === "monto" && c.sourceTable === "pagos");
    expect(backMonto).toBeDefined();
  });

  it("preserva las condiciones constantes del ON en el round-trip", () => {
    const state = multiState();
    state.relationships = [
      {
        childTable: "pagos",
        parentTable: "clientes",
        on: [{ child: "cliente_id", parent: "id" }],
        onConstants: [{ table: "clientes", column: "estado", value: "1" }],
        joinType: "left",
      },
    ];
    const input = toDefinitionInput(state);
    expect(input.fetchMapping.relationships).toEqual([
      {
        childTable: "pagos",
        parentTable: "clientes",
        on: [{ child: "cliente_id", parent: "id" }],
        onConstants: [{ table: "clientes", column: "estado", value: "1" }],
        joinType: "left",
      },
    ]);
    // Inverso: fromDefinition reconstruye las constantes.
    const back = fromDefinition(input as ModuleDefinitionInput);
    expect(back.relationships[0].onConstants).toEqual([
      { table: "clientes", column: "estado", value: "1" },
    ]);
  });

  it("omite onConstants cuando está vacío (retrocompatibilidad JSONB)", () => {
    const input = toDefinitionInput(multiState());
    const rel = (input.fetchMapping.relationships as Array<Record<string, unknown>>)[0];
    // omitempty: el campo queda undefined (se descarta al serializar a JSONB).
    expect(rel.onConstants).toBeUndefined();
    // Una definición sin onConstants reconstruye [] en el estado.
    const back = fromDefinition(input as ModuleDefinitionInput);
    expect(back.relationships[0].onConstants).toEqual([]);
  });

  it("N=1 no emite foreignTables/relationships ni sourceTable", () => {
    const state: WizardState = {
      ...emptyWizardState(),
      slug: "planes",
      name: "Planes",
      pgTable: "planes",
      pgWhitelist: { planes: ["id", "monto"] },
      columns: [
        { field: "id", label: "ID", order: 0, format: "", visible: true, isPrimaryKey: true, editable: false, sourceTable: "planes" },
      ],
    };
    const input = toDefinitionInput(state);
    expect(input.fetchMapping.foreignTables).toBeUndefined();
    expect(input.fetchMapping.relationships).toBeUndefined();
    const cols = input.displayMapping.columns as Array<Record<string, unknown>>;
    expect(cols[0].sourceTable).toBeUndefined();
  });
});

describe("missingPrimaryKey multi-tabla", () => {
  it("true si una tabla escribible no tiene PK", () => {
    const state = multiState();
    // Quita la PK de clientes.
    state.columns = state.columns.map((c) =>
      c.sourceTable === "clientes" && c.field === "id"
        ? { ...c, isPrimaryKey: false }
        : c,
    );
    expect(missingPrimaryKey(state)).toBe(true);
  });

  it("false si todas las tablas tienen PK", () => {
    expect(missingPrimaryKey(multiState())).toBe(false);
  });

  it("tablesMissingPrimaryKey nombra la tabla concreta sin PK", () => {
    const state = multiState();
    state.columns = state.columns.map((c) =>
      c.sourceTable === "clientes" && c.field === "id"
        ? { ...c, isPrimaryKey: false }
        : c,
    );
    expect(tablesMissingPrimaryKey(state)).toEqual(["clientes"]);
  });

  it("tablesMissingPrimaryKey devuelve [] cuando todas tienen PK", () => {
    expect(tablesMissingPrimaryKey(multiState())).toEqual([]);
  });
});

describe("treeError / relationshipError", () => {
  it("válido no da error", () => {
    expect(treeError(multiState())).toBeNull();
  });

  it("tabla huérfana", () => {
    const state = multiState();
    state.foreignTables = ["clientes", "huerfana"];
    expect(treeError(state)).toMatch(/huerfana/);
  });

  it("ciclo FK", () => {
    const state = multiState();
    state.relationships = [
      { childTable: "pagos", parentTable: "clientes", on: [{ child: "cliente_id", parent: "id" }], onConstants: [], joinType: "inner" },
      { childTable: "clientes", parentTable: "pagos", on: [{ child: "id", parent: "id" }], onConstants: [], joinType: "inner" },
    ];
    expect(treeError(state)).toMatch(/ciclo/);
  });

  it("relationshipError detecta tablas iguales y ON vacío", () => {
    expect(
      relationshipError({ childTable: "a", parentTable: "a", on: [{ child: "x", parent: "y" }], onConstants: [], joinType: "inner" }, ["a"]),
    ).toMatch(/distintas/);
    expect(
      relationshipError({ childTable: "a", parentTable: "b", on: [], onConstants: [], joinType: "inner" }, ["a", "b"]),
    ).toMatch(/unión|union/);
  });

  it("relationshipError valida las condiciones constantes del ON", () => {
    // Tabla de la constante ajena a la relación.
    expect(
      relationshipError(
        {
          childTable: "pagos",
          parentTable: "clientes",
          on: [{ child: "cliente_id", parent: "id" }],
          onConstants: [{ table: "otra", column: "estado", value: "1" }],
          joinType: "inner",
        },
        ["pagos", "clientes"],
      ),
    ).toMatch(/hija o padre/);
    // Columna o valor vacíos.
    expect(
      relationshipError(
        {
          childTable: "pagos",
          parentTable: "clientes",
          on: [{ child: "cliente_id", parent: "id" }],
          onConstants: [{ table: "clientes", column: "", value: "1" }],
          joinType: "inner",
        },
        ["pagos", "clientes"],
      ),
    ).toMatch(/columna y valor/);
    // Constante válida ⇒ sin error.
    expect(
      relationshipError(
        {
          childTable: "pagos",
          parentTable: "clientes",
          on: [{ child: "cliente_id", parent: "id" }],
          onConstants: [{ table: "clientes", column: "estado", value: "1" }],
          joinType: "inner",
        },
        ["pagos", "clientes"],
      ),
    ).toBeNull();
  });
});

describe("deleteOrder", () => {
  const rels: RelationshipFormState[] = [
    { childTable: "A", parentTable: "P", on: [{ child: "p_id", parent: "id" }], onConstants: [], joinType: "inner" },
    { childTable: "B", parentTable: "A", on: [{ child: "a_id", parent: "id" }], onConstants: [], joinType: "inner" },
  ];
  it("P←A←B produce el orden hijo→padre esperado", () => {
    expect(deleteOrder(rels, "P")).toEqual(["B", "A", "P"]);
    expect(deleteOrder(rels, "A")).toEqual(["B", "A"]);
    expect(deleteOrder(rels, "B")).toEqual(["B"]);
  });
});
