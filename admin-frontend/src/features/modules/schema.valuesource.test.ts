// Pruebas puras de la FUENTE DE VALORES por columna (FEAT-003): round-trip de
// toDefinitionInput/fromDefinition para columnas con fuente estática y dinámica,
// retrocompatibilidad byte a byte de una columna sin fuente (kind "none" no
// emite `valueSource`), tabla de casos de los núcleos planos selectOptionsError
// y dynamicSourceError, y regresión de que selectFieldError/dynamicFieldError
// siguen devolviendo los mismos mensajes tras delegar. Entorno node (sin jsdom).

import { describe, expect, it } from "vitest";

import {
  defaultColumn,
  dynamicFieldError,
  dynamicSourceError,
  emptyStrategyFormState,
  emptyWizardState,
  fromDefinition,
  selectFieldError,
  selectOptionsError,
  toDefinitionInput,
  type ColumnFormState,
  type FieldFormState,
  type WizardState,
} from "@/features/modules/schema";
import type { ModuleDefinitionInput } from "@/features/modules/types";

/** Columna de prueba (spread de defaultColumn + overrides). */
function col(overrides: Partial<ColumnFormState> = {}): ColumnFormState {
  return { ...defaultColumn("id", 0, "catalogo"), ...overrides };
}

/** Estado mínimo del asistente con una sola columna `column`. */
function stateWithColumn(column: ColumnFormState): WizardState {
  return {
    ...emptyWizardState(),
    slug: "catalogo",
    name: "Catálogo",
    sourceType: "postgres",
    pgTable: "catalogo",
    pgWhitelist: { catalogo: ["id"] },
    columns: [column],
  };
}

/** Campo base para las regresiones de selectFieldError/dynamicFieldError. */
function baseField(overrides: Partial<FieldFormState> = {}): FieldFormState {
  return {
    name: "country",
    type: "string",
    required: false,
    source: "select",
    rowField: "",
    constValue: "",
    options: [],
    defaultValue: "",
    dynamicSource: { table: "", idColumn: "", labelColumn: "", order: { column: "", direction: "asc" } },
    dynamicFilters: [],
    hidden: false,
    label: "",
    captureOnly: false,
    strategy: emptyStrategyFormState(),
    ...overrides,
  };
}

/** Extrae la primera columna del displayMapping emitido. */
function emittedColumn(state: WizardState): Record<string, unknown> {
  const input = toDefinitionInput(state);
  const cols = (input.displayMapping as { columns: Array<Record<string, unknown>> }).columns;
  return cols[0];
}

describe("valueSource — emisión en toDefinitionInput", () => {
  it("columna sin fuente (kind 'none') NO emite valueSource (undefined => omitido en JSON)", () => {
    const emitted = emittedColumn(stateWithColumn(col()));
    // El valor es undefined, de modo que JSON.stringify lo descarta (la clave no
    // viaja al backend). La prueba de retrocompat byte a byte lo confirma abajo.
    expect(emitted.valueSource).toBeUndefined();
    expect(JSON.stringify(emitted).includes("valueSource")).toBe(false);
  });

  it("columna estática emite { kind: 'static', options }", () => {
    const options = [
      { label: "Activo", value: "A" },
      { label: "Inactivo", value: "I" },
    ];
    const emitted = emittedColumn(
      stateWithColumn(
        col({
          valueSource: {
            kind: "static",
            options,
            dynamicSource: { table: "", idColumn: "", labelColumn: "", order: { column: "", direction: "asc" } },
            dynamicFilters: [],
          },
        }),
      ),
    );
    expect(emitted.valueSource).toEqual({ kind: "static", options });
  });

  it("columna dinámica emite { kind: 'dynamic', dynamicSource } con filtros de igualdad", () => {
    const emitted = emittedColumn(
      stateWithColumn(
        col({
          valueSource: {
            kind: "dynamic",
            options: [],
            dynamicSource: {
              table: "estados",
              idColumn: "id",
              labelColumn: "nombre",
              order: { column: "nombre", direction: "asc" },
            },
            dynamicFilters: [{ field: "pais", valuesText: "gt, sv" }],
          },
        }),
      ),
    );
    expect(emitted.valueSource).toEqual({
      kind: "dynamic",
      dynamicSource: {
        table: "estados",
        idColumn: "id",
        labelColumn: "nombre",
        filters: [{ field: "pais", values: ["gt", "sv"] }],
        order: { column: "nombre", direction: "asc" },
        limit: undefined,
      },
    });
  });
});

describe("valueSource — retrocompatibilidad byte a byte", () => {
  it("una columna sin fuente produce el MISMO JSONB que una definición sin la clave", () => {
    const emitted = emittedColumn(stateWithColumn(col({ label: "Identificador" })));
    // El objeto de columna debe tener EXACTAMENTE las claves históricas (ninguna
    // `valueSource`). sourceTable se omite porque es N=1 (igual a la principal).
    expect(Object.keys(emitted).sort()).toEqual(
      ["editable", "field", "format", "isPrimaryKey", "label", "order", "sourceTable", "valueSource", "visible"].sort(),
    );
    // Aunque la clave existe en el objeto literal, su valor es undefined, por lo
    // que JSON.stringify la descarta (JSONB idéntico al histórico).
    const historico = {
      editable: emitted.editable,
      field: emitted.field,
      format: emitted.format,
      isPrimaryKey: emitted.isPrimaryKey,
      label: emitted.label,
      order: emitted.order,
      sourceTable: emitted.sourceTable,
      visible: emitted.visible,
    };
    expect(JSON.parse(JSON.stringify(emitted))).toEqual(
      JSON.parse(JSON.stringify(historico)),
    );
  });
});

describe("valueSource — round-trip toDefinitionInput → fromDefinition", () => {
  it("estática: options estable tras el round-trip", () => {
    const options = [
      { label: "Activo", value: "A" },
      { label: "Inactivo", value: "I" },
    ];
    const input = toDefinitionInput(
      stateWithColumn(
        col({
          valueSource: {
            kind: "static",
            options,
            dynamicSource: { table: "", idColumn: "", labelColumn: "", order: { column: "", direction: "asc" } },
            dynamicFilters: [],
          },
        }),
      ),
    );
    const vs = fromDefinition(input as ModuleDefinitionInput).columns[0].valueSource;
    expect(vs.kind).toBe("static");
    expect(vs.options).toEqual(options);
  });

  it("dinámica con filtros de igualdad: estable tras el round-trip", () => {
    const input = toDefinitionInput(
      stateWithColumn(
        col({
          valueSource: {
            kind: "dynamic",
            options: [],
            dynamicSource: {
              table: "estados",
              idColumn: "id",
              labelColumn: "nombre",
              order: { column: "nombre", direction: "asc" },
            },
            dynamicFilters: [{ field: "pais", valuesText: "gt, sv" }],
          },
        }),
      ),
    );
    const vs = fromDefinition(input as ModuleDefinitionInput).columns[0].valueSource;
    expect(vs.kind).toBe("dynamic");
    expect(vs.dynamicSource.table).toBe("estados");
    expect(vs.dynamicSource.idColumn).toBe("id");
    expect(vs.dynamicSource.labelColumn).toBe("nombre");
    expect(vs.dynamicSource.order).toEqual({ column: "nombre", direction: "asc" });
    expect(vs.dynamicFilters).toEqual([{ field: "pais", valuesText: "gt, sv" }]);
  });

  it("ausente: se hidrata a kind 'none' con estructuras vacías por defecto", () => {
    const input = toDefinitionInput(stateWithColumn(col()));
    const vs = fromDefinition(input as ModuleDefinitionInput).columns[0].valueSource;
    expect(vs.kind).toBe("none");
    expect(vs.options).toEqual([]);
    expect(vs.dynamicFilters).toEqual([]);
    expect(vs.dynamicSource).toEqual({
      table: "",
      idColumn: "",
      labelColumn: "",
      order: { column: "", direction: "asc" },
    });
  });
});

describe("selectOptionsError (núcleo plano)", () => {
  it("exige al menos una opción", () => {
    expect(selectOptionsError([], "estado")).toBe(
      'El campo "estado" de tipo selección necesita al menos una opción',
    );
  });

  it("rechaza etiquetas o valores vacíos (tras trim)", () => {
    expect(selectOptionsError([{ label: "  ", value: "A" }], "estado")).toBe(
      'Las opciones del campo "estado" no pueden tener etiqueta o valor vacíos',
    );
  });

  it("rechaza valores repetidos", () => {
    expect(
      selectOptionsError(
        [
          { label: "Uno", value: "x" },
          { label: "Dos", value: "x" },
        ],
        "estado",
      ),
    ).toBe('El campo "estado" tiene valores de opción repetidos');
  });

  it("rechaza una opción por defecto inexistente", () => {
    expect(
      selectOptionsError([{ label: "Uno", value: "x" }], "estado", "zzz"),
    ).toBe('La opción por defecto del campo "estado" ya no existe');
  });

  it("devuelve null cuando es válido (sin y con defaultValue existente)", () => {
    expect(selectOptionsError([{ label: "Uno", value: "x" }], "estado")).toBeNull();
    expect(
      selectOptionsError([{ label: "Uno", value: "x" }], "estado", "x"),
    ).toBeNull();
  });
});

describe("dynamicSourceError (núcleo plano)", () => {
  it("exige una tabla de origen", () => {
    expect(dynamicSourceError({ table: "  ", idColumn: "id", labelColumn: "n" }, "estado")).toBe(
      'El campo "estado" de lista dinámica necesita una tabla de origen',
    );
  });

  it("exige una columna ID", () => {
    expect(dynamicSourceError({ table: "t", idColumn: "  ", labelColumn: "n" }, "estado")).toBe(
      'El campo "estado" de lista dinámica necesita una columna ID',
    );
  });

  it("exige una columna etiqueta", () => {
    expect(dynamicSourceError({ table: "t", idColumn: "id", labelColumn: "" }, "estado")).toBe(
      'El campo "estado" de lista dinámica necesita una columna etiqueta',
    );
  });

  it("devuelve null cuando tabla/id/etiqueta están completos", () => {
    expect(dynamicSourceError({ table: "t", idColumn: "id", labelColumn: "n" }, "estado")).toBeNull();
  });
});

describe("regresión: selectFieldError/dynamicFieldError delegan sin cambiar mensajes", () => {
  it("selectFieldError usa el nombre del campo como etiqueta", () => {
    expect(selectFieldError(baseField({ options: [] }))).toBe(
      'El campo "country" de tipo selección necesita al menos una opción',
    );
    expect(selectFieldError(baseField({ name: "", options: [] }))).toBe(
      'El campo "(sin nombre)" de tipo selección necesita al menos una opción',
    );
  });

  it("dynamicFieldError usa el nombre del campo como etiqueta", () => {
    expect(
      dynamicFieldError(
        baseField({ source: "dynamic", dynamicSource: { table: "", idColumn: "id", labelColumn: "n" } }),
      ),
    ).toBe('El campo "country" de lista dinámica necesita una tabla de origen');
    expect(
      dynamicFieldError(
        baseField({ source: "dynamic", dynamicSource: { table: "t", idColumn: "id", labelColumn: "n" } }),
      ),
    ).toBeNull();
  });
});
