// Pruebas puras de la validación EN VIVO de la "columna objetivo" vacía del
// constructor de módulos (Arreglo 2): fieldTargetColumnError (campo sin columna)
// y actionsSectionError (agregador por acción/campo que localiza acción +
// posición y respeta el ORDEN DE PRIORIDAD declarado: acción incompleta →
// columna objetivo vacía → errores de source). Entorno node (NO monta React).

import { describe, expect, it } from "vitest";

import {
  actionsSectionError,
  defaultColumn,
  emptyStrategyFormState,
  emptyUniquenessFormState,
  emptyWizardState,
  fieldTargetColumnError,
  type ActionFormState,
  type FieldFormState,
  type WizardState,
} from "@/features/modules/schema";

/** Campo de formulario base (source input por defecto). */
function baseField(overrides: Partial<FieldFormState> = {}): FieldFormState {
  return {
    name: "valor_objeto",
    type: "string",
    required: false,
    source: "input",
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

/** Acción create con los campos dados (lista con una sola acción). */
function createAction(fields: FieldFormState[], overrides: Partial<ActionFormState> = {}): ActionFormState {
  return {
    actionKey: "crear",
    kind: "create",
    label: "Crear registro",
    placement: "header",
    destructive: false,
    permAction: "crear",
    isWrite: true,
    sqlOp: "INSERT",
    inputFields: fields,
    uniqueness: emptyUniquenessFormState(),
    ...overrides,
  };
}

/** Estado del asistente con PK resuelta y las acciones dadas. */
function stateWith(actions: ActionFormState[]): WizardState {
  return {
    ...emptyWizardState(),
    slug: "demo",
    name: "Demo",
    pgTable: "t",
    pgWhitelist: { t: ["id", "valor_objeto"] },
    columns: [
      { ...defaultColumn("id", 0, "t"), label: "ID", isPrimaryKey: true, editable: false },
      { ...defaultColumn("valor_objeto", 1, "t"), label: "Valor" },
    ],
    actions,
  };
}

describe("fieldTargetColumnError", () => {
  it("name vacío ⇒ mensaje no nulo", () => {
    expect(fieldTargetColumnError(baseField({ name: "" }))).not.toBeNull();
  });

  it("name solo espacios (trim) ⇒ mensaje no nulo", () => {
    expect(fieldTargetColumnError(baseField({ name: "  " }))).not.toBeNull();
  });

  it("name con columna ⇒ null", () => {
    expect(fieldTargetColumnError(baseField({ name: "valor_objeto" }))).toBeNull();
  });

  it("MEJORA 4: captureOnly ⇒ null aunque name esté vacío", () => {
    expect(
      fieldTargetColumnError(baseField({ name: "", captureOnly: true })),
    ).toBeNull();
  });
});

describe("actionsSectionError", () => {
  it("localiza la acción y la posición #N del campo sin columna objetivo", () => {
    const state = stateWith([
      createAction([
        baseField({ name: "valor_objeto" }),
        baseField({ name: "" }), // 2º campo sin columna
      ]),
    ]);
    const err = actionsSectionError(state);
    expect(err).not.toBeNull();
    expect(err).toContain("Crear registro");
    expect(err).toContain("#2");
  });

  it("prioridad: columna objetivo vacía gana sobre el error de source (select)", () => {
    // 1er campo: columna vacía Y además select sin opciones (dos errores).
    const state = stateWith([
      createAction([
        baseField({ name: "", source: "select", options: [] }),
      ]),
    ]);
    const err = actionsSectionError(state);
    expect(err).not.toBeNull();
    // Debe ser el mensaje de columna objetivo, NO el de select.
    expect(err).toContain("sin columna objetivo");
    expect(err).not.toContain("selección");
  });

  it("estado válido ⇒ null (incluye captura definida DESPUÉS de la derivación)", () => {
    // derive-from-option (campo 0) referencia una captura declarada DESPUÉS
    // (campo 1). declaredCaptureKeys(a, -1) la recoge sin importar el orden.
    const derive = baseField({
      name: "valor_derivado",
      source: "fillStrategy",
      strategy: {
        ...emptyStrategyFormState(),
        kind: "derive-from-option",
        sourceCaptureKey: "medio_pago",
        attribute: "codigo",
      },
    });
    const capture = baseField({
      name: "valor_objeto",
      source: "fillStrategy",
      strategy: {
        ...emptyStrategyFormState(),
        kind: "capture-from-catalog",
        catalog: { table: "cat", idColumn: "id", labelColumn: "nombre", order: { column: "", direction: "asc" } },
        captureKey: "medio_pago",
      },
    });
    const state = stateWith([createAction([derive, capture])]);
    expect(actionsSectionError(state)).toBeNull();
  });
});
