// Pruebas puras de las TABLAS AUXILIARES (catálogos) del esquema del
// constructor: round-trip de fetchMapping.auxTables, fusión en pgWhitelist sin
// duplicados, que NO vuelven multi-tabla el estado ni generan columnas del grid,
// y que una «Lista dinámica» que apunta a una auxiliar queda en la lista blanca
// (sin error de whitelist en el asistente).
import { describe, expect, it } from "vitest";

import {
  defaultColumn,
  emptyWizardState,
  fromDefinition,
  isMultiTableState,
  syncColumnsMultiTable,
  toDefinitionInput,
  type ColumnFormState,
  type WizardState,
} from "@/features/modules/schema";
import type { ModuleDefinitionInput } from "@/features/modules/types";

/**
 * Estado con tabla principal `merchant` + tabla AUXILIAR `countries` (catálogo,
 * SIN relación) y una columna `country_id` con «Lista dinámica» apuntando a la
 * auxiliar. Reproduce el módulo qa-tabla-auxiliar de la verificación en vivo.
 */
function auxState(): WizardState {
  const countryIdCol: ColumnFormState = {
    ...defaultColumn("country_id", 2, "merchant"),
    label: "País",
    valueSource: {
      kind: "dynamic",
      options: [],
      dynamicSource: {
        table: "countries",
        idColumn: "id",
        labelColumn: "name",
        order: { column: "", direction: "asc" },
      },
      dynamicFilters: [],
    },
  };
  return {
    ...emptyWizardState(),
    slug: "qa-tabla-auxiliar",
    name: "QA Tabla Auxiliar",
    pgTable: "merchant",
    pgWhitelist: {
      merchant: ["uuid", "merchant_name", "country_id"],
    },
    auxTables: [{ table: "countries", columns: ["id", "name"] }],
    columns: [
      { ...defaultColumn("uuid", 0, "merchant"), label: "UUID", isPrimaryKey: true, editable: false },
      { ...defaultColumn("merchant_name", 1, "merchant"), label: "Comercio" },
      countryIdCol,
    ],
  };
}

describe("auxTables: estado y semántica multi-tabla", () => {
  it("una auxiliar NO vuelve multi-tabla el estado", () => {
    expect(isMultiTableState(auxState())).toBe(false);
  });

  it("una auxiliar NO entra a syncColumnsMultiTable (no genera columnas del grid)", () => {
    const state = auxState();
    // tables = [principal, ...foráneas]; la auxiliar NO está aquí.
    const tables = [state.pgTable, ...state.foreignTables].filter(Boolean);
    expect(tables).toEqual(["merchant"]);
    const synced = syncColumnsMultiTable(
      state.columns,
      state.pgTable,
      state.pgWhitelist,
      tables,
    );
    // Solo columnas de merchant; ninguna de countries.
    expect(synced.every((c) => (c.sourceTable || "merchant") === "merchant")).toBe(true);
    expect(synced.some((c) => c.sourceTable === "countries")).toBe(false);
  });
});

describe("auxTables: toDefinitionInput", () => {
  it("emite fetchMapping.auxTables y funde sus columnas en pgWhitelist", () => {
    const input = toDefinitionInput(auxState());
    expect(input.fetchMapping.auxTables).toEqual([
      { table: "countries", columns: ["id", "name"] },
    ]);
    // La auxiliar queda en la lista blanca efectiva (unión con lo existente).
    expect(input.dataSource.pgWhitelist?.countries).toEqual(["id", "name"]);
    // La principal se conserva intacta.
    expect(input.dataSource.pgWhitelist?.merchant).toEqual([
      "uuid",
      "merchant_name",
      "country_id",
    ]);
    // NO emite foreignTables/relationships (la auxiliar no es foránea).
    expect(input.fetchMapping.foreignTables).toBeUndefined();
    expect(input.fetchMapping.relationships).toBeUndefined();
  });

  it("funde sin duplicar cuando la auxiliar ya aporta columnas repetidas", () => {
    const state = auxState();
    // La lista blanca ya trae 'id' para countries; la auxiliar lo repite.
    state.pgWhitelist = { ...state.pgWhitelist, countries: ["id"] };
    const input = toDefinitionInput(state);
    expect(input.dataSource.pgWhitelist?.countries).toEqual(["id", "name"]);
  });

  it("omite fetchMapping.auxTables cuando no hay auxiliares (retrocompat JSONB)", () => {
    const state = auxState();
    state.auxTables = [];
    const input = toDefinitionInput(state);
    expect(input.fetchMapping.auxTables).toBeUndefined();
  });

  it("descarta auxiliares sin tabla y columnas vacías al emitir", () => {
    const state = auxState();
    state.auxTables = [
      { table: "", columns: ["x"] },
      { table: "countries", columns: ["id", "  ", "name"] },
    ];
    const input = toDefinitionInput(state);
    expect(input.fetchMapping.auxTables).toEqual([
      { table: "countries", columns: ["id", "name"] },
    ]);
  });
});

describe("auxTables: round-trip toDefinitionInput -> fromDefinition", () => {
  it("conserva {table, columns}, la mantiene en pgWhitelist y no genera columnas de grid", () => {
    const input = toDefinitionInput(auxState());
    const back = fromDefinition(input as ModuleDefinitionInput);

    // La auxiliar se hidrata idéntica.
    expect(back.auxTables).toEqual([{ table: "countries", columns: ["id", "name"] }]);
    // Sigue en la lista blanca.
    expect(back.pgWhitelist.countries).toEqual(["id", "name"]);
    // No se volvió multi-tabla ni aparecieron columnas de countries en el grid.
    expect(isMultiTableState(back)).toBe(false);
    expect(back.columns.some((c) => c.sourceTable === "countries")).toBe(false);

    // Idempotencia: re-emitir produce el MISMO contrato (merge estable).
    const input2 = toDefinitionInput(back);
    expect(input2.fetchMapping.auxTables).toEqual(input.fetchMapping.auxTables);
    expect(input2.dataSource.pgWhitelist).toEqual(input.dataSource.pgWhitelist);
  });

  it("la columna 'Lista dinámica' que apunta a la auxiliar queda whitelisteada (sin error de whitelist)", () => {
    const input = toDefinitionInput(auxState());
    const back = fromDefinition(input as ModuleDefinitionInput);
    // La columna country_id conserva su fuente dinámica a countries...
    const col = back.columns.find((c) => c.field === "country_id");
    expect(col?.valueSource.kind).toBe("dynamic");
    expect(col?.valueSource.dynamicSource.table).toBe("countries");
    // ...y la tabla destino está en la lista blanca efectiva (lo que el backend
    // exige para validar la fuente dinámica).
    const wl = input.dataSource.pgWhitelist ?? {};
    expect(wl.countries).toContain("id");
    expect(wl.countries).toContain("name");
  });
});
