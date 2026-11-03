// Pruebas puras del esquema del constructor de módulos para el origen "select"
// (FEAT-002): selectFieldError (sus 4 reglas) y el round-trip de
// toDefinitionInput/fromDefinition de un campo source:"select", además de la
// hidratación retrocompatible de una definición SIN campos select.

import { describe, expect, it } from "vitest";

import {
  buildColumnOptions,
  columnOptionValue,
  defaultColumn,
  dynamicFieldError,
  emptyWizardState,
  fromCopyableDefinition,
  fromDefinition,
  missingPrimaryKey,
  parseColumnList,
  selectFieldError,
  syncColumnsMultiTable,
  syncColumnsWithWhitelist,
  writableColumns,
  toDefinitionInput,
  type ColumnFormState,
  type FieldFormState,
  type WizardState,
} from "@/features/modules/schema";
import type { ModuleDefinitionInput } from "@/features/modules/types";

/** Campo de formulario base para construir casos de selectFieldError. */
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
    ...overrides,
  };
}

/** Estado del asistente con una sola acción que contiene `fields`. */
function stateWithFields(fields: FieldFormState[]): WizardState {
  return {
    ...emptyWizardState(),
    slug: "afiliaciones",
    name: "Afiliaciones",
    sourceType: "postgres",
    pgTable: "afiliaciones",
    pgWhitelist: { afiliaciones: ["id"] },
    columns: [
      { ...defaultColumn("id", 0, "afiliaciones"), label: "ID" },
    ],
    actions: [
      {
        actionKey: "list",
        kind: "list",
        label: "Listar",
        placement: "header",
        destructive: false,
        permAction: "ver",
        isWrite: false,
        sqlOp: "SELECT",
        inputFields: fields,
      },
    ],
  };
}

describe("selectFieldError", () => {
  it("exige al menos una opción", () => {
    const err = selectFieldError(baseField({ options: [] }));
    expect(err).toBe('El campo "country" de tipo selección necesita al menos una opción');
  });

  it("rechaza etiquetas o valores vacíos (tras trim)", () => {
    const err = selectFieldError(
      baseField({ options: [{ label: "  ", value: "gt" }] }),
    );
    expect(err).toBe('Las opciones del campo "country" no pueden tener etiqueta o valor vacíos');
  });

  it("rechaza valores de opción repetidos", () => {
    const err = selectFieldError(
      baseField({
        options: [
          { label: "Guatemala", value: "gt" },
          { label: "Guate", value: "gt" },
        ],
      }),
    );
    expect(err).toBe('El campo "country" tiene valores de opción repetidos');
  });

  it("rechaza una opción por defecto inexistente", () => {
    const err = selectFieldError(
      baseField({
        options: [{ label: "Guatemala", value: "gt" }],
        defaultValue: "sv",
      }),
    );
    expect(err).toBe('La opción por defecto del campo "country" ya no existe');
  });

  it("devuelve null cuando el campo es válido", () => {
    const err = selectFieldError(
      baseField({
        options: [
          { label: "Guatemala", value: "gt" },
          { label: "El Salvador", value: "sv" },
        ],
        defaultValue: "gt",
      }),
    );
    expect(err).toBeNull();
  });

  it("usa (sin nombre) cuando el campo no tiene nombre", () => {
    const err = selectFieldError(baseField({ name: "", options: [] }));
    expect(err).toBe('El campo "(sin nombre)" de tipo selección necesita al menos una opción');
  });
});

describe("round-trip toDefinitionInput / fromDefinition de un campo select", () => {
  it("preserva options y defaultValue de un campo source:'select'", () => {
    const state = stateWithFields([
      baseField({
        name: "country",
        options: [
          { label: "Guatemala", value: "gt" },
          { label: "El Salvador", value: "sv" },
        ],
        defaultValue: "gt",
      }),
    ]);

    const def = toDefinitionInput(state);
    const field = def.actions[0].inputFields[0];
    expect(field.source).toBe("select");
    expect(field.options).toEqual([
      { label: "Guatemala", value: "gt" },
      { label: "El Salvador", value: "sv" },
    ]);
    expect(field.defaultValue).toBe("gt");

    const back = fromDefinition(def);
    const hydrated = back.actions[0].inputFields[0];
    expect(hydrated.source).toBe("select");
    expect(hydrated.options).toEqual([
      { label: "Guatemala", value: "gt" },
      { label: "El Salvador", value: "sv" },
    ]);
    expect(hydrated.defaultValue).toBe("gt");
  });

  it("un campo no-select NO emite options ni defaultValue", () => {
    const state = stateWithFields([
      baseField({ name: "plan_id", source: "input", options: [], defaultValue: "" }),
    ]);

    const def = toDefinitionInput(state);
    const field = def.actions[0].inputFields[0];
    expect(field.source).toBe("input");
    expect(field.options).toBeUndefined();
    expect(field.defaultValue).toBeUndefined();
  });
});

describe("matchType de acción SELECT (omitempty) y retiro del buscador por columna", () => {
  it("emite matchType SOLO cuando es 'partial' en una acción SELECT", () => {
    // stateWithFields crea una acción con sqlOp "SELECT".
    const state = stateWithFields([
      baseField({ name: "nombre", source: "input", matchType: "partial" }),
      baseField({ name: "estado", source: "input", matchType: "exact" }),
    ]);

    const def = toDefinitionInput(state);
    const [partial, exact] = def.actions[0].inputFields;
    expect(partial.matchType).toBe("partial");
    // "exact" es el default del backend ⇒ no se emite (omitempty).
    expect(exact.matchType).toBeUndefined();
  });

  it("round-trip: fromDefinition hidrata 'partial' y normaliza el resto a 'exact'", () => {
    const state = stateWithFields([
      baseField({ name: "nombre", source: "input", matchType: "partial" }),
      baseField({ name: "estado", source: "input", matchType: "exact" }),
    ]);

    const back = fromDefinition(toDefinitionInput(state));
    const [nombre, estado] = back.actions[0].inputFields;
    expect(nombre.matchType).toBe("partial");
    // El que no se emitió se hidrata al default "exact".
    expect(estado.matchType).toBe("exact");
  });

  it("una acción NO-SELECT nunca emite matchType", () => {
    const state: WizardState = {
      ...emptyWizardState(),
      slug: "m",
      name: "M",
      pgTable: "t",
      pgWhitelist: { t: ["nombre"] },
      columns: [
        { ...defaultColumn("nombre", 0, "t"), label: "Nombre" },
      ],
      actions: [
        {
          actionKey: "editar",
          kind: "edit",
          label: "Editar",
          placement: "row",
          destructive: false,
          permAction: "editar",
          isWrite: true,
          sqlOp: "UPDATE",
          inputFields: [baseField({ name: "nombre", source: "input", matchType: "partial" })],
        },
      ],
    };
    const def = toDefinitionInput(state);
    expect(def.actions[0].inputFields[0].matchType).toBeUndefined();
  });

  it("toDefinitionInput ya NO emite banderas de búsqueda de columna", () => {
    const state = stateWithFields([]);
    const def = toDefinitionInput(state);
    const cols = (def.displayMapping as { columns: Array<Record<string, unknown>> }).columns;
    expect(cols[0].searchable).toBeUndefined();
    expect(cols[0].searchMatch).toBeUndefined();
    expect(cols[0].searchRequired).toBeUndefined();
  });
});

describe("plantilla de conexión + valores variables (connection-templates)", () => {
  it("toDefinitionInput emite connectionTemplateId y variableValues (sin dsn)", () => {
    const state: WizardState = {
      ...stateWithFields([]),
      connectionTemplateId: "tmpl-9",
      variableValues: { dbname: "recaudo_gt", vacio: "" },
      menuPath: "CENAM/Cuentas de recaudo/GT",
    };
    const def = toDefinitionInput(state);
    expect(def.dataSource.connectionTemplateId).toBe("tmpl-9");
    // Solo las variables con valor viajan (vacío = conservar).
    expect(def.secrets?.variableValues).toEqual({ dbname: "recaudo_gt" });
    // No existe ya un campo dsn en el payload.
    expect((def.secrets as Record<string, unknown>).dsn).toBeUndefined();
    expect(def.menuPath).toBe("CENAM/Cuentas de recaudo/GT");
  });

  it("toDefinitionInput omite variableValues cuando todas están vacías", () => {
    const state: WizardState = {
      ...stateWithFields([]),
      connectionTemplateId: "tmpl-9",
      variableValues: { dbname: "" },
    };
    const def = toDefinitionInput(state);
    expect(def.secrets?.variableValues).toBeUndefined();
  });

  it("fromDefinition lee connectionTemplateId y menuPath, deja variables vacías", () => {
    const def: ModuleDefinitionInput = {
      slug: "recaudo-gt",
      name: "Recaudo GT",
      sourceType: "postgres",
      countryScope: "none",
      fetchMapping: { table: "t" },
      displayMapping: { columns: [] },
      listParams: {},
      menuPath: "CENAM/GT",
      dataSource: {
        sourceType: "postgres",
        pgWhitelist: { t: ["id"] },
        connectionTemplateId: "tmpl-42",
        hasSecrets: true,
      },
      actions: [],
    };
    const state = fromDefinition(def);
    expect(state.connectionTemplateId).toBe("tmpl-42");
    expect(state.menuPath).toBe("CENAM/GT");
    expect(state.variableValues).toEqual({});
  });
});

describe("fromDefinition con una definición SIN campos select", () => {
  it("hidrata options:[] y defaultValue:'' sin romper", () => {
    const def: ModuleDefinitionInput = {
      slug: "afiliaciones",
      name: "Afiliaciones",
      sourceType: "postgres",
      countryScope: "none",
      fetchMapping: { table: "afiliaciones" },
      displayMapping: { columns: [] },
      listParams: {},
      dataSource: {
        sourceType: "postgres",
        pgWhitelist: { afiliaciones: ["plan_id"] },
        connectionTemplateId: "tmpl-1",
        hasSecrets: false,
      },
      actions: [
        {
          actionKey: "edit",
          kind: "edit",
          label: "Editar",
          placement: "row",
          destructive: false,
          permAction: "editar",
          isWrite: true,
          sqlOp: "UPDATE",
          inputFields: [
            {
              name: "plan_id",
              type: "string",
              required: true,
              source: "input",
            },
          ],
        },
      ],
    };

    const state = fromDefinition(def);
    const field = state.actions[0].inputFields[0];
    expect(field.source).toBe("input");
    expect(field.options).toEqual([]);
    expect(field.defaultValue).toBe("");
  });
});

describe("fromCopyableDefinition (copiar configuración al crear)", () => {
  /** Definición completa de un módulo origen para copiar. */
  const sourceDef: ModuleDefinitionInput = {
    slug: "recaudo-gt",
    name: "Recaudo GT",
    sourceType: "postgres",
    countryScope: "multi",
    fetchMapping: { table: "recaudo" },
    displayMapping: {
      columns: [{ field: "monto", label: "Monto", order: 0, format: "" }],
    },
    listParams: { page_size: 25 },
    listConditions: [{ field: "status", values: ["active", "pending"] }],
    menuPath: "CENAM/Recaudo/GT",
    dataSource: {
      sourceType: "postgres",
      pgWhitelist: { recaudo: ["id", "monto", "status"] },
      connectionTemplateId: "tmpl-42",
      hasSecrets: true,
    },
    actions: [
      {
        actionKey: "activar",
        kind: "custom",
        label: "Activar",
        placement: "row",
        destructive: false,
        permAction: "custom:activar",
        isWrite: true,
        sqlOp: "UPDATE",
        inputFields: [
          {
            name: "estado",
            type: "string",
            required: true,
            source: "select",
            options: [
              { label: "Activo", value: "active" },
              { label: "Pendiente", value: "pending" },
            ],
            defaultValue: "active",
            isKey: false,
          },
          {
            name: "id",
            type: "number",
            required: true,
            source: "row",
            rowField: "id",
            isKey: true,
          },
        ],
      },
    ],
  };

  it("COPIA la configuración/estructura igual que fromDefinition", () => {
    const copy = fromCopyableDefinition(sourceDef);
    const edit = fromDefinition(sourceDef);

    // Todo lo que se debe copiar coincide con el estado de edición.
    // (countryScope ya no forma parte del estado del asistente: se eliminó el
    // control; el DTO conserva el campo y al guardar se fija a 'none'.)
    expect(copy.sourceType).toBe(edit.sourceType);
    expect(copy.menuPath).toBe(edit.menuPath);
    expect(copy.pgTable).toBe(edit.pgTable);
    expect(copy.pgWhitelist).toEqual(edit.pgWhitelist);
    expect(copy.listParams).toEqual(edit.listParams);
    expect(copy.columns).toEqual(edit.columns);
    expect(copy.conditions).toEqual(edit.conditions);
    expect(copy.actions).toEqual(edit.actions);
  });

  it("copia campos select/row con options, defaultValue y rowField", () => {
    const copy = fromCopyableDefinition(sourceDef);
    const [selectField, rowField] = copy.actions[0].inputFields;
    expect(selectField.source).toBe("select");
    expect(selectField.options).toEqual([
      { label: "Activo", value: "active" },
      { label: "Pendiente", value: "pending" },
    ]);
    expect(selectField.defaultValue).toBe("active");
    expect(rowField.source).toBe("row");
    expect(rowField.rowField).toBe("id");
    // La llave primaria ya NO se declara por campo (isKey DEPRECADO): ahora es
    // una bandera de columna (DisplayColumn.isPrimaryKey).
    expect("isKey" in rowField).toBe(false);
  });

  it("NO copia la conexión (ni plantilla ni valores variables)", () => {
    const copy = fromCopyableDefinition(sourceDef);
    expect(copy.connectionTemplateId).toBe("");
    expect(copy.variableValues).toEqual({});
  });

  it("NO copia slug ni name (quedan vacíos para exigir valores nuevos)", () => {
    const copy = fromCopyableDefinition(sourceDef);
    expect(copy.slug).toBe("");
    expect(copy.name).toBe("");
  });
});

/** Columna de formulario base para las pruebas de banderas. */
function col(overrides: Partial<ColumnFormState> = {}): ColumnFormState {
  return { ...defaultColumn("c", 0), ...overrides };
}

describe("columnas: banderas visible/isPrimaryKey/editable", () => {
  it("toDefinitionInput emite las tres banderas y fuerza PK ⇒ no editable", () => {
    const state: WizardState = {
      ...emptyWizardState(),
      slug: "m",
      name: "M",
      pgTable: "t",
      pgWhitelist: { t: ["id", "monto"] },
      columns: [
        col({ field: "id", isPrimaryKey: true, editable: true, visible: false }),
        col({ field: "monto", editable: true, visible: true }),
      ],
    };
    const input = toDefinitionInput(state);
    const cols = (input.displayMapping as { columns: Array<Record<string, unknown>> }).columns;
    expect(cols[0]).toMatchObject({ field: "id", isPrimaryKey: true, editable: false, visible: false });
    expect(cols[1]).toMatchObject({ field: "monto", isPrimaryKey: false, editable: true, visible: true });
  });

  it("fromDefinition aplica defaults de compatibilidad (R8)", () => {
    const def = {
      slug: "m",
      name: "M",
      sourceType: "postgres",
      countryScope: "none",
      fetchMapping: { table: "t" },
      displayMapping: {
        columns: [
          { field: "id", isPrimaryKey: true }, // sin visible/editable
          { field: "monto" }, // sin banderas
        ],
      },
      listParams: {},
      dataSource: { sourceType: "postgres", pgWhitelist: { t: ["id", "monto"] }, connectionTemplateId: "x", hasSecrets: false },
      actions: [],
    } as unknown as ModuleDefinitionInput;

    const state = fromDefinition(def);
    expect(state.columns[0]).toMatchObject({ field: "id", visible: true, isPrimaryKey: true, editable: false });
    expect(state.columns[1]).toMatchObject({ field: "monto", visible: true, isPrimaryKey: false, editable: true });
  });

  it("missingPrimaryKey: true si hay acción edit/delete y ninguna PK", () => {
    const withEdit: WizardState = {
      ...emptyWizardState(),
      columns: [col({ field: "id", isPrimaryKey: false })],
      actions: [{ actionKey: "edit", kind: "edit", label: "Editar", placement: "row", destructive: false, permAction: "editar", isWrite: true, sqlOp: "UPDATE", inputFields: [] }],
    };
    expect(missingPrimaryKey(withEdit)).toBe(true);

    const withPK: WizardState = {
      ...withEdit,
      columns: [col({ field: "id", isPrimaryKey: true })],
    };
    expect(missingPrimaryKey(withPK)).toBe(false);

    const listOnly: WizardState = {
      ...emptyWizardState(),
      columns: [col({ field: "id" })],
      actions: [{ actionKey: "list", kind: "list", label: "Listar", placement: "header", destructive: false, permAction: "ver", isWrite: false, sqlOp: "SELECT", inputFields: [] }],
    };
    expect(missingPrimaryKey(listOnly)).toBe(false);
  });
});

describe("syncColumnsWithWhitelist (precarga y sincronización)", () => {
  it("precarga una fila por columna de la lista blanca con defaults", () => {
    const result = syncColumnsWithWhitelist([], ["id", "monto"]);
    expect(result).toHaveLength(2);
    expect(result[0]).toEqual(defaultColumn("id", 0));
    expect(result[1]).toEqual(defaultColumn("monto", 1));
  });

  it("preserva ajustes de columnas que persisten y añade/quita según la lista blanca", () => {
    const current: ColumnFormState[] = [
      col({ field: "id", label: "Identificador", isPrimaryKey: true, editable: false, order: 0 }),
      col({ field: "viejo", label: "Viejo", order: 1 }),
    ];
    const result = syncColumnsWithWhitelist(current, ["id", "nuevo"]);
    expect(result).toHaveLength(2);
    // "id" preservada con sus ajustes.
    expect(result[0]).toMatchObject({ field: "id", label: "Identificador", isPrimaryKey: true, editable: false });
    // "viejo" eliminada (ya no está en la lista blanca); "nuevo" añadida por defecto.
    expect(result[1]).toEqual(defaultColumn("nuevo", 1));
  });

  it("lista blanca vacía ⇒ sin columnas", () => {
    expect(syncColumnsWithWhitelist([col({ field: "id" })], [])).toEqual([]);
  });
});

// BUG 1 — el input de unicidad descartaba comas/espacios. `parseColumnList` es
// la función pura que ahora parsea el TEXTO CRUDO a array (el input mantiene el
// texto tal cual y solo parsea en un efecto), de modo que escribir varias
// columnas separadas por comas no colapse el texto.
describe("parseColumnList (lista de columnas por comas)", () => {
  it("'a, b, c' ⇒ ['a','b','c'] sin colapsar", () => {
    expect(parseColumnList("a, b, c")).toEqual(["a", "b", "c"]);
  });

  it("texto vacío ⇒ [] (omitempty: no se emite unicidad)", () => {
    expect(parseColumnList("")).toEqual([]);
  });

  it("solo espacios ⇒ []", () => {
    expect(parseColumnList("  ")).toEqual([]);
  });

  it("tolera comas sobrantes y espacios", () => {
    expect(parseColumnList("a,,b, ")).toEqual(["a", "b"]);
  });

  it("recorta espacios alrededor de cada nombre", () => {
    expect(parseColumnList(" padr_numero , depe_numero ")).toEqual([
      "padr_numero",
      "depe_numero",
    ]);
  });

  it("caso real de 3 columnas del usuario", () => {
    expect(parseColumnList("padr_numero, valor_lista, valor_objeto")).toEqual([
      "padr_numero",
      "valor_lista",
      "valor_objeto",
    ]);
  });
});

// BUG 2 — los selectores de columna de ActionsSection («Columna objetivo» y el
// selector de columnas de estrategias) deben reflejar las columnas recién
// añadidas a la lista blanca. La derivación es pura: sincronizar `state.columns`
// con la lista blanca (como hace ahora el efecto global de ModuleWizard), luego
// filtrar con `writableColumns` y construir opciones con `buildColumnOptions`.
// Este test reproduce esa cadena SIN montar ColumnsSection.
describe("derivación de columnas disponibles para selectores de acciones", () => {
  it("single-table: nuevas columnas de la lista blanca aparecen en las opciones", () => {
    const current: ColumnFormState[] = [
      col({ field: "id", isPrimaryKey: true, editable: false, order: 0 }),
      col({ field: "nombre", order: 1 }),
    ];
    // La lista blanca gana "monto" sin pasar por «Columnas».
    const synced = syncColumnsWithWhitelist(current, ["id", "nombre", "monto"]);
    const state: WizardState = { ...emptyWizardState(), pgTable: "t", columns: synced };

    const writable = writableColumns(state);
    // "id" es PK no editable ⇒ fuera; "nombre" y "monto" ⇒ dentro.
    expect(writable.map((c) => c.field)).toEqual(["nombre", "monto"]);

    const opts = buildColumnOptions(writable, "", "");
    expect(opts.map((o) => o.value)).toContain(columnOptionValue("monto"));
    expect(opts.map((o) => o.value)).toContain(columnOptionValue("nombre"));
  });

  it("multi-tabla: nuevas columnas de una tabla foránea aparecen y se preservan ajustes", () => {
    // Estado previo: solo una columna por tabla.
    const current: ColumnFormState[] = [
      col({ field: "padr_numero", sourceTable: "gt_valores_padre", label: "Padre", order: 0 }),
      col({ field: "hijo_numero", sourceTable: "gt_valores_hijo", isPrimaryKey: true, editable: false, order: 1 }),
    ];
    const whitelist: Record<string, string[]> = {
      gt_valores_padre: ["padr_numero"],
      // La lista blanca de la tabla hija se amplía con tres columnas nuevas.
      gt_valores_hijo: ["hijo_numero", "list_numero_h", "valor_objeto", "obje_numero_h"],
    };
    const tables = ["gt_valores_padre", "gt_valores_hijo"];
    const synced = syncColumnsMultiTable(current, "gt_valores_padre", whitelist, tables);

    // Se preserva la etiqueta de "padr_numero" y la PK de "hijo_numero".
    expect(synced.find((c) => c.field === "padr_numero")?.label).toBe("Padre");
    expect(synced.find((c) => c.field === "hijo_numero")?.isPrimaryKey).toBe(true);
    // Las nuevas columnas de la tabla hija están presentes.
    expect(synced.map((c) => c.field)).toEqual(
      expect.arrayContaining(["list_numero_h", "valor_objeto", "obje_numero_h"]),
    );

    const state: WizardState = {
      ...emptyWizardState(),
      pgTable: "gt_valores_padre",
      foreignTables: ["gt_valores_hijo"],
      columns: synced,
    };
    const writable = writableColumns(state);
    // "hijo_numero" es PK no editable ⇒ fuera; las nuevas editables ⇒ dentro.
    expect(writable.map((c) => c.field)).not.toContain("hijo_numero");

    const opts = buildColumnOptions(writable, "", "");
    const values = opts.map((o) => o.value);
    expect(values).toContain(columnOptionValue("list_numero_h", "gt_valores_hijo"));
    expect(values).toContain(columnOptionValue("valor_objeto", "gt_valores_hijo"));
    expect(values).toContain(columnOptionValue("obje_numero_h", "gt_valores_hijo"));
  });
});

describe("round-trip de `hidden` en un campo constante de escritura", () => {
  it("toDefinitionInput emite hidden solo para source:'constant'", () => {
    const state = stateWithFields([
      baseField({ name: "estado", source: "constant", constValue: "2", hidden: true }),
    ]);
    const def = toDefinitionInput(state);
    const field = def.actions[0].inputFields[0];
    expect(field.source).toBe("constant");
    expect(field.constValue).toBe("2");
    expect(field.hidden).toBe(true);
  });

  it("un campo no-constante NO emite hidden", () => {
    const state = stateWithFields([
      baseField({ name: "nombre", source: "input", hidden: true }),
    ]);
    const def = toDefinitionInput(state);
    expect(def.actions[0].inputFields[0].hidden).toBeUndefined();
  });

  it("fromDefinition hidrata hidden (ausente ⇒ false)", () => {
    const def: ModuleDefinitionInput = {
      slug: "m",
      name: "M",
      sourceType: "postgres",
      countryScope: "none",
      fetchMapping: { table: "t" },
      displayMapping: { columns: [] },
      listParams: {},
      dataSource: {
        sourceType: "postgres",
        pgWhitelist: { t: ["estado"] },
        connectionTemplateId: "tmpl-1",
        hasSecrets: false,
      },
      actions: [
        {
          actionKey: "inactivar",
          kind: "edit",
          label: "Inactivar",
          placement: "row",
          destructive: false,
          permAction: "editar",
          isWrite: true,
          sqlOp: "UPDATE",
          inputFields: [
            { name: "estado", type: "string", required: false, source: "constant", constValue: "2", hidden: true },
            { name: "otro", type: "string", required: false, source: "input" },
          ],
        },
      ],
    };
    const state = fromDefinition(def);
    expect(state.actions[0].inputFields[0].hidden).toBe(true);
    expect(state.actions[0].inputFields[1].hidden).toBe(false);
  });
});

describe("dynamicFieldError", () => {
  it("exige una tabla de origen", () => {
    const err = dynamicFieldError(
      baseField({ source: "dynamic", dynamicSource: { table: "", idColumn: "id", labelColumn: "nombre" } }),
    );
    expect(err).toBe('El campo "country" de lista dinámica necesita una tabla de origen');
  });

  it("exige una columna ID", () => {
    const err = dynamicFieldError(
      baseField({ source: "dynamic", dynamicSource: { table: "planes", idColumn: "  ", labelColumn: "nombre" } }),
    );
    expect(err).toBe('El campo "country" de lista dinámica necesita una columna ID');
  });

  it("exige una columna etiqueta", () => {
    const err = dynamicFieldError(
      baseField({ source: "dynamic", dynamicSource: { table: "planes", idColumn: "id", labelColumn: "" } }),
    );
    expect(err).toBe('El campo "country" de lista dinámica necesita una columna etiqueta');
  });

  it("devuelve null cuando tabla/id/etiqueta están completos", () => {
    const err = dynamicFieldError(
      baseField({ source: "dynamic", dynamicSource: { table: "planes", idColumn: "id", labelColumn: "nombre" } }),
    );
    expect(err).toBeNull();
  });
});

describe("round-trip toDefinitionInput / fromDefinition de un campo dynamic", () => {
  it("emite dynamicSource (tabla/id/etiqueta/orden/límite) y filtros desde dynamicFilters", () => {
    const state = stateWithFields([
      baseField({
        name: "plan_id",
        source: "dynamic",
        dynamicSource: {
          table: "planes",
          idColumn: "id",
          labelColumn: "nombre",
          order: { column: "nombre", direction: "desc" },
          limit: 100,
        },
        dynamicFilters: [
          { field: "estado", valuesText: "activo, pendiente" },
          // Se descarta: campo vacío.
          { field: "  ", valuesText: "x" },
          // Se descarta: sin valores.
          { field: "pais", valuesText: " , " },
        ],
      }),
    ]);

    const def = toDefinitionInput(state);
    const field = def.actions[0].inputFields[0];
    expect(field.source).toBe("dynamic");
    expect(field.dynamicSource).toEqual({
      table: "planes",
      idColumn: "id",
      labelColumn: "nombre",
      order: { column: "nombre", direction: "desc" },
      limit: 100,
      filters: [{ field: "estado", values: ["activo", "pendiente"] }],
    });

    const back = fromDefinition(def);
    const hydrated = back.actions[0].inputFields[0];
    expect(hydrated.source).toBe("dynamic");
    expect(hydrated.dynamicSource).toMatchObject({
      table: "planes",
      idColumn: "id",
      labelColumn: "nombre",
      order: { column: "nombre", direction: "desc" },
      limit: 100,
    });
    // Los filtros vuelven al shape de formulario (valuesText por comas).
    expect(hydrated.dynamicFilters).toEqual([
      { field: "estado", valuesText: "activo, pendiente" },
    ]);
  });

  it("un campo no-dynamic NO emite dynamicSource", () => {
    const state = stateWithFields([baseField({ name: "x", source: "input" })]);
    const def = toDefinitionInput(state);
    expect(def.actions[0].inputFields[0].dynamicSource).toBeUndefined();
  });

  it("omite filtros/orden/límite cuando están vacíos", () => {
    const state = stateWithFields([
      baseField({
        name: "plan_id",
        source: "dynamic",
        dynamicSource: { table: "planes", idColumn: "id", labelColumn: "nombre", order: { column: "", direction: "asc" } },
        dynamicFilters: [],
      }),
    ]);
    const def = toDefinitionInput(state);
    const ds = def.actions[0].inputFields[0].dynamicSource;
    expect(ds).toEqual({ table: "planes", idColumn: "id", labelColumn: "nombre" });
  });

  it("fromDefinition hidrata un dynamicSource ausente a un shape vacío sin romper", () => {
    const def: ModuleDefinitionInput = {
      slug: "m",
      name: "M",
      sourceType: "postgres",
      countryScope: "none",
      fetchMapping: { table: "t" },
      displayMapping: { columns: [] },
      listParams: {},
      dataSource: { sourceType: "postgres", pgWhitelist: { t: ["id"] }, connectionTemplateId: "x", hasSecrets: false },
      actions: [
        {
          actionKey: "list",
          kind: "list",
          label: "Listar",
          placement: "header",
          destructive: false,
          permAction: "ver",
          isWrite: false,
          sqlOp: "SELECT",
          inputFields: [{ name: "x", type: "string", required: false, source: "input" }],
        },
      ],
    };
    const state = fromDefinition(def);
    const f = state.actions[0].inputFields[0];
    expect(f.dynamicSource).toEqual({ table: "", idColumn: "", labelColumn: "", order: { column: "", direction: "asc" }, limit: undefined });
    expect(f.dynamicFilters).toEqual([]);
  });
});
