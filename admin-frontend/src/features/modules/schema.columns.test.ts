// Pruebas puras de los helpers de desplegables de columnas definidas del
// constructor de módulos: etiqueta tabla.columna (+ fallback N=1), codificación
// y parseo del value homónimo, construcción de opciones con tolerancia a
// huérfanos, y un round-trip ligero de toDefinitionInput/fromDefinition que
// confirma que el cambio de UI no altera el contrato del backend.
import { describe, expect, it } from "vitest";

import {
  buildColumnOptions,
  columnOptionLabel,
  columnOptionValue,
  defaultColumn,
  emptyWizardState,
  fromDefinition,
  parseColumnOptionValue,
  toDefinitionInput,
  type ColumnFormState,
  type WizardState,
} from "@/features/modules/schema";

/** Columna definida de prueba. */
function col(field: string, sourceTable: string): ColumnFormState {
  return {
    ...defaultColumn(field, 0, sourceTable),
  };
}

describe("columnOptionLabel", () => {
  it("devuelve tabla.columna cuando hay sourceTable", () => {
    expect(columnOptionLabel("valor_lista", "gt_valores")).toBe("gt_valores.valor_lista");
  });

  it("cae a solo la columna (sin punto inicial) cuando sourceTable es vacía", () => {
    expect(columnOptionLabel("valor_lista", "")).toBe("valor_lista");
    expect(columnOptionLabel("valor_lista", undefined)).toBe("valor_lista");
  });

  it("recorta espacios de sourceTable", () => {
    expect(columnOptionLabel("valor_lista", "  gt_valores  ")).toBe("gt_valores.valor_lista");
    expect(columnOptionLabel("valor_lista", "   ")).toBe("valor_lista");
  });
});

describe("columnOptionValue / parseColumnOptionValue", () => {
  it("round-trip con sourceTable", () => {
    const v = columnOptionValue("valor_lista", "gt_valores");
    expect(parseColumnOptionValue(v)).toEqual({ field: "valor_lista", sourceTable: "gt_valores" });
  });

  it("round-trip sin sourceTable (N=1)", () => {
    const v = columnOptionValue("valor_lista", "");
    expect(parseColumnOptionValue(v)).toEqual({ field: "valor_lista", sourceTable: "" });
  });

  it("parseo tolerante de un value sin separador", () => {
    expect(parseColumnOptionValue("valor_lista")).toEqual({ field: "valor_lista", sourceTable: "" });
  });

  it("distingue homónimos entre tablas", () => {
    const hijo = columnOptionValue("padr_numero", "gt_valores_hijo");
    const padre = columnOptionValue("padr_numero", "gt_valores_padre");
    expect(hijo).not.toBe(padre);
    expect(parseColumnOptionValue(hijo).sourceTable).toBe("gt_valores_hijo");
    expect(parseColumnOptionValue(padre).sourceTable).toBe("gt_valores_padre");
  });
});

describe("buildColumnOptions", () => {
  const columns = [
    col("valor_lista", "gt_valores"),
    col("padr_numero", "gt_valores_hijo"),
    col("padr_numero", "gt_valores_padre"),
  ];

  it("una opción por columna, sin huérfanas, labels tabla.columna", () => {
    const opts = buildColumnOptions(columns, "valor_lista", "gt_valores");
    expect(opts).toHaveLength(3);
    expect(opts.every((o) => !o.orphan)).toBe(true);
    expect(opts.map((o) => o.label)).toEqual([
      "gt_valores.valor_lista",
      "gt_valores_hijo.padr_numero",
      "gt_valores_padre.padr_numero",
    ]);
  });

  it("no inyecta huérfana cuando el seleccionado está presente", () => {
    const opts = buildColumnOptions(columns, "padr_numero", "gt_valores_hijo");
    expect(opts.some((o) => o.orphan)).toBe(false);
    expect(opts).toHaveLength(3);
  });

  it("inyecta opción huérfana marcada cuando el seleccionado no está definido", () => {
    const opts = buildColumnOptions(columns, "columna_vieja", "tabla_x");
    const orphan = opts.find((o) => o.orphan);
    expect(orphan).toBeDefined();
    expect(orphan?.label).toBe("tabla_x.columna_vieja (columna inexistente)");
    // El value de la huérfana round-trip-ea al par original.
    expect(parseColumnOptionValue(orphan!.value)).toEqual({
      field: "columna_vieja",
      sourceTable: "tabla_x",
    });
  });

  it("huérfana por homónimo con sourceTable distinta a las definidas", () => {
    const opts = buildColumnOptions(columns, "padr_numero", "otra_tabla");
    const orphan = opts.find((o) => o.orphan);
    expect(orphan).toBeDefined();
    expect(orphan?.label).toBe("otra_tabla.padr_numero (columna inexistente)");
  });

  it("no inyecta huérfana cuando el campo seleccionado está vacío", () => {
    const opts = buildColumnOptions(columns, "", "");
    expect(opts.some((o) => o.orphan)).toBe(false);
    expect(opts).toHaveLength(3);
  });

  it("N=1: labels sin punto cuando las columnas no tienen sourceTable", () => {
    const n1 = [col("valor_lista", ""), col("codigo", "")];
    const opts = buildColumnOptions(n1, "valor_lista", "");
    expect(opts.map((o) => o.label)).toEqual(["valor_lista", "codigo"]);
  });
});

describe("round-trip de contrato tras el cambio de UI", () => {
  /** Estado con un campo de acción no-escritura y una condición de listado, ambos homónimos. */
  function state(): WizardState {
    return {
      ...emptyWizardState(),
      slug: "catalogo",
      name: "Catálogo",
      pgTable: "gt_valores",
      foreignTables: ["gt_valores_hijo"],
      relationships: [
        {
          childTable: "gt_valores_hijo",
          parentTable: "gt_valores",
          on: [{ child: "padr_numero", parent: "padr_numero" }],
          onConstants: [],
          joinType: "left",
        },
      ],
      pgWhitelist: {
        gt_valores: ["padr_numero", "valor_lista"],
        gt_valores_hijo: ["padr_numero", "valor_lista"],
      },
      columns: [
        col("valor_lista", "gt_valores"),
        col("valor_lista", "gt_valores_hijo"),
      ],
      conditions: [{ field: "valor_lista", valuesText: "A, B", sourceTable: "gt_valores_hijo" }],
      actions: [
        {
          actionKey: "custom",
          kind: "custom",
          label: "Marcar",
          placement: "row",
          destructive: false,
          permAction: "custom:custom",
          isWrite: false,
          sqlOp: "",
          inputFields: [
            {
              name: "valor_lista",
              type: "string",
              required: true,
              source: "input",
              rowField: "",
              constValue: "",
              options: [],
              defaultValue: "",
              dynamicSource: { table: "", idColumn: "", labelColumn: "", order: { column: "", direction: "asc" }, limit: undefined },
              dynamicFilters: [],
              hidden: false,
              sourceTable: "gt_valores_hijo",
            },
          ],
        },
      ],
    };
  }

  it("emite sourceTable del campo y condición homónimos y los restaura", () => {
    const input = toDefinitionInput(state());

    // El campo de acción no-escritura emite sourceTable (difiere de la principal).
    expect(input.actions[0].inputFields[0].sourceTable).toBe("gt_valores_hijo");
    // La condición de listado emite sourceTable.
    expect(input.listConditions?.[0].sourceTable).toBe("gt_valores_hijo");

    // fromDefinition restaura ambos sourceTable.
    const back = fromDefinition(input);
    expect(back.actions[0].inputFields[0].sourceTable).toBe("gt_valores_hijo");
    expect(back.conditions[0].sourceTable).toBe("gt_valores_hijo");

    // El value del <select> reconstruye la clave homónima correcta.
    const fieldValue = columnOptionValue(
      back.actions[0].inputFields[0].name,
      back.actions[0].inputFields[0].sourceTable ?? "",
    );
    expect(parseColumnOptionValue(fieldValue)).toEqual({
      field: "valor_lista",
      sourceTable: "gt_valores_hijo",
    });
  });
});

describe("regresión: campo de la tabla PRINCIPAL no queda huérfano al reeditar", () => {
  /**
   * Caso reportado: al guardar y reabrir, un campo de acción cuya columna es de
   * la tabla PRINCIPAL salía marcado "(columna inexistente)". Causa: al guardar,
   * toDefinitionInput OMITE el sourceTable cuando es igual a la principal; al
   * rehidratar, las columnas se rellenaban a `principal` pero los campos de
   * acción quedaban en "", de modo que el emparejamiento (field, sourceTable) de
   * buildColumnOptions no casaba y la opción se inyectaba como huérfana. El fix
   * rehidrata el sourceTable del campo a `principal` cuando viene ausente/vacío,
   * simétrico con las columnas.
   */
  function stateMultiTablePrincipalField(): WizardState {
    return {
      ...emptyWizardState(),
      slug: "comercios",
      name: "Comercios",
      pgTable: "comercio",
      foreignTables: ["pagos"],
      relationships: [
        {
          childTable: "pagos",
          parentTable: "comercio",
          on: [{ child: "comercio_numero", parent: "comercio_numero" }],
          onConstants: [],
          joinType: "left",
        },
      ],
      pgWhitelist: {
        comercio: ["comercio_numero", "nombre"],
        pagos: ["comercio_numero", "monto"],
      },
      // comercio_numero es columna de la tabla PRINCIPAL (comercio).
      columns: [col("comercio_numero", "comercio"), col("nombre", "comercio")],
      actions: [
        {
          actionKey: "buscar",
          kind: "search",
          label: "Buscar",
          placement: "header",
          destructive: false,
          permAction: "ver",
          isWrite: false,
          sqlOp: "SELECT",
          inputFields: [
            {
              name: "comercio_numero",
              type: "string",
              required: false,
              source: "input",
              rowField: "",
              constValue: "",
              options: [],
              defaultValue: "",
              dynamicSource: { table: "", idColumn: "", labelColumn: "", order: { column: "", direction: "asc" }, limit: undefined },
              dynamicFilters: [],
              hidden: false,
              matchType: "exact",
              // sourceTable apunta a la PRINCIPAL ⇒ toDefinitionInput lo omitirá.
              sourceTable: "comercio",
            },
          ],
        },
      ],
    };
  }

  it("omite el sourceTable de la principal al guardar y lo restaura al reeditar", () => {
    const input = toDefinitionInput(stateMultiTablePrincipalField());

    // Al guardar: el sourceTable igual a la principal NO se emite.
    expect(input.actions[0].inputFields[0].sourceTable).toBeUndefined();

    // Al reeditar: el campo recupera el sourceTable de la principal (no "").
    const back = fromDefinition(input);
    const field = back.actions[0].inputFields[0];
    expect(field.sourceTable).toBe("comercio");

    // Y el emparejamiento con las columnas ya NO produce huérfana.
    const opts = buildColumnOptions(back.columns, field.name, field.sourceTable ?? "");
    expect(opts.some((o) => o.orphan)).toBe(false);
    expect(opts.some((o) => o.value === columnOptionValue("comercio_numero", "comercio"))).toBe(true);
  });
});
