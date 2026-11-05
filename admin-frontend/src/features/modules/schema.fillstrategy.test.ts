// Pruebas puras de las ESTRATEGIAS DE LLENADO y la UNICIDAD (FEAT-003): round-trip
// de toDefinitionInput/fromDefinition para campos source==="fillStrategy" y para
// Action.uniqueness, retrocompatibilidad byte a byte (sin estrategias ⇒ estado
// vacío y JSONB idéntico al histórico), y las validaciones del asistente
// (strategyError). Entorno node (sin jsdom), espejo del backend FEAT-001/002.

import { describe, expect, it } from "vitest";

import {
  emptyStrategyFormState,
  emptyUniquenessFormState,
  emptyWizardState,
  fromDefinition,
  strategyError,
  toDefinitionInput,
  type ActionFormState,
  type FieldFormState,
  type StrategyFormState,
  type WizardState,
} from "@/features/modules/schema";
import type { ModuleDefinitionInput } from "@/features/modules/types";

/** Campo base (todas las claves del FieldFormState con defaults). */
function field(overrides: Partial<FieldFormState> = {}): FieldFormState {
  return {
    name: "",
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

/** Estrategia base (spread de emptyStrategyFormState + overrides). */
function strat(overrides: Partial<StrategyFormState> = {}): StrategyFormState {
  return { ...emptyStrategyFormState(), ...overrides };
}

/** Acción create base con los campos indicados. */
function createAction(overrides: Partial<ActionFormState> = {}): ActionFormState {
  return {
    actionKey: "crear",
    kind: "create",
    label: "Crear",
    placement: "header",
    destructive: false,
    permAction: "crear",
    isWrite: true,
    sqlOp: "INSERT",
    inputFields: [],
    targetTable: "gt_valores_hijo",
    uniqueness: emptyUniquenessFormState(),
    ...overrides,
  };
}

/** Estado multi-tabla mínimo con una acción create. */
function stateWithAction(action: ActionFormState): WizardState {
  return {
    ...emptyWizardState(),
    slug: "catalogo",
    name: "Catálogo",
    sourceType: "postgres",
    pgTable: "gt_valores",
    pgWhitelist: {
      gt_valores: ["valo_numero", "list_numero"],
      gt_valores_hijo: ["padr_numero", "depe_numero"],
      gt_valores_padre: ["padr_numero", "depe_numero", "valor_objeto"],
    },
    foreignTables: ["gt_valores_hijo", "gt_valores_padre"],
    actions: [action],
  };
}

/** Extrae el primer FieldSpec emitido de la primera acción. */
function emittedField(state: WizardState): Record<string, unknown> {
  const input = toDefinitionInput(state);
  return input.actions[0].inputFields[0] as unknown as Record<string, unknown>;
}

describe("fillStrategy — emisión en toDefinitionInput (omitempty)", () => {
  it("un campo NO-fillStrategy no emite strategy", () => {
    const emitted = emittedField(
      stateWithAction(createAction({ inputFields: [field({ name: "list_numero", source: "input" })] })),
    );
    expect(emitted.strategy).toBeUndefined();
    expect(JSON.stringify(emitted).includes("strategy")).toBe(false);
  });

  it("capture-from-catalog emite catalog + storeColumn + captureKey", () => {
    const emitted = emittedField(
      stateWithAction(
        createAction({
          inputFields: [
            field({
              name: "__capture_comercio",
              source: "fillStrategy",
              strategy: strat({
                kind: "capture-from-catalog",
                catalog: { table: "gt_objetos", idColumn: "obje_numero", labelColumn: "nombre", order: { column: "nombre", direction: "asc" } },
                storeColumn: "obje_numero",
                captureKey: "comercio",
              }),
            }),
          ],
        }),
      ),
    );
    expect(emitted.strategy).toEqual({
      kind: "capture-from-catalog",
      catalog: {
        table: "gt_objetos",
        idColumn: "obje_numero",
        labelColumn: "nombre",
        filters: undefined,
        order: { column: "nombre", direction: "asc" },
        limit: undefined,
      },
      storeColumn: "obje_numero",
      captureKey: "comercio",
    });
  });

  it("derive-from-option emite solo sourceCaptureKey + attribute", () => {
    const emitted = emittedField(
      stateWithAction(
        createAction({
          inputFields: [
            field({
              name: "list_numero",
              source: "fillStrategy",
              strategy: strat({
                kind: "derive-from-option",
                sourceCaptureKey: "medio",
                attribute: "list_numero",
              }),
            }),
          ],
        }),
      ),
    );
    expect(emitted.strategy).toEqual({
      kind: "derive-from-option",
      sourceCaptureKey: "medio",
      attribute: "list_numero",
    });
  });

  it("derive-via-fk emite jump + mapping", () => {
    const emitted = emittedField(
      stateWithAction(
        createAction({
          inputFields: [
            field({
              name: "padr_numero",
              source: "fillStrategy",
              strategy: strat({
                kind: "derive-via-fk",
                jump: {
                  relatedTable: "gt_valores_padre",
                  localCaptureKey: "comercio",
                  localAttribute: "come_numero",
                  relatedColumn: "valor_objeto",
                },
                mapping: [{ attribute: "padr_numero", targetColumn: "padr_numero" }],
              }),
            }),
          ],
        }),
      ),
    );
    expect(emitted.strategy).toEqual({
      kind: "derive-via-fk",
      jump: {
        relatedTable: "gt_valores_padre",
        localCaptureKey: "comercio",
        localAttribute: "come_numero",
        relatedColumn: "valor_objeto",
      },
      mapping: [{ attribute: "padr_numero", targetColumn: "padr_numero" }],
    });
  });

  it("constant/autogenerated emiten solo { kind }", () => {
    const constEmitted = emittedField(
      stateWithAction(
        createAction({
          inputFields: [
            field({ name: "estado", source: "fillStrategy", strategy: strat({ kind: "constant" }) }),
          ],
        }),
      ),
    );
    expect(constEmitted.strategy).toEqual({ kind: "constant" });

    const autoEmitted = emittedField(
      stateWithAction(
        createAction({
          inputFields: [
            field({ name: "hijo_numero", source: "fillStrategy", strategy: strat({ kind: "autogenerated" }) }),
          ],
        }),
      ),
    );
    expect(autoEmitted.strategy).toEqual({ kind: "autogenerated" });
  });
});

describe("uniqueness — emisión en toDefinitionInput (omitempty)", () => {
  it("sin columnas NO emite uniqueness", () => {
    const input = toDefinitionInput(stateWithAction(createAction()));
    expect(input.actions[0].uniqueness).toBeUndefined();
    expect(JSON.stringify(input.actions[0]).includes("uniqueness")).toBe(false);
  });

  it("con columnas emite { columns, message } (trim, sin vacíos)", () => {
    const input = toDefinitionInput(
      stateWithAction(
        createAction({
          uniqueness: { columns: [" padr_numero ", "depe_numero", ""], message: " ya existe " },
        }),
      ),
    );
    expect(input.actions[0].uniqueness).toEqual({
      columns: ["padr_numero", "depe_numero"],
      message: "ya existe",
    });
  });

  it("columnas sin mensaje omiten message", () => {
    const input = toDefinitionInput(
      stateWithAction(createAction({ uniqueness: { columns: ["padr_numero"], message: "" } })),
    );
    expect(input.actions[0].uniqueness).toEqual({ columns: ["padr_numero"], message: undefined });
  });

  it("uniqueness solo se considera en create (una acción no-create lo omite)", () => {
    const input = toDefinitionInput(
      stateWithAction(
        createAction({
          kind: "custom",
          sqlOp: "INSERT",
          uniqueness: { columns: ["padr_numero"], message: "x" },
        }),
      ),
    );
    expect(input.actions[0].uniqueness).toBeUndefined();
  });
});

describe("fillStrategy/uniqueness — retrocompatibilidad", () => {
  it("una acción create sin estrategias ni unicidad produce JSONB sin strategy/uniqueness", () => {
    const input = toDefinitionInput(
      stateWithAction(createAction({ inputFields: [field({ name: "list_numero", source: "input" })] })),
    );
    const json = JSON.stringify(input.actions[0]);
    expect(json.includes("strategy")).toBe(false);
    expect(json.includes("uniqueness")).toBe(false);
  });
});

describe("fillStrategy/uniqueness — round-trip toDefinitionInput → fromDefinition", () => {
  it("capture-from-catalog estable tras el round-trip", () => {
    const original = createAction({
      inputFields: [
        field({
          name: "__capture_comercio",
          source: "fillStrategy",
          strategy: strat({
            kind: "capture-from-catalog",
            catalog: { table: "gt_objetos", idColumn: "obje_numero", labelColumn: "nombre", order: { column: "nombre", direction: "asc" } },
            catalogFilters: [{ field: "activo", valuesText: "si" }],
            storeColumn: "obje_numero",
            captureKey: "comercio",
          }),
        }),
      ],
    });
    const input = toDefinitionInput(stateWithAction(original));
    const rt = fromDefinition(input as ModuleDefinitionInput);
    const s = rt.actions[0].inputFields[0].strategy;
    expect(s.kind).toBe("capture-from-catalog");
    expect(s.catalog.table).toBe("gt_objetos");
    expect(s.catalog.idColumn).toBe("obje_numero");
    expect(s.catalog.labelColumn).toBe("nombre");
    expect(s.catalogFilters).toEqual([{ field: "activo", valuesText: "si" }]);
    expect(s.storeColumn).toBe("obje_numero");
    expect(s.captureKey).toBe("comercio");
  });

  it("derive-via-fk estable tras el round-trip", () => {
    const original = createAction({
      inputFields: [
        field({
          name: "padr_numero",
          source: "fillStrategy",
          strategy: strat({
            kind: "derive-via-fk",
            jump: { relatedTable: "gt_valores_padre", localCaptureKey: "comercio", localAttribute: "come_numero", relatedColumn: "valor_objeto" },
            mapping: [{ attribute: "padr_numero", targetColumn: "padr_numero" }],
          }),
        }),
      ],
    });
    const input = toDefinitionInput(stateWithAction(original));
    const s = fromDefinition(input as ModuleDefinitionInput).actions[0].inputFields[0].strategy;
    expect(s.kind).toBe("derive-via-fk");
    expect(s.jump).toEqual({
      relatedTable: "gt_valores_padre",
      localCaptureKey: "comercio",
      localAttribute: "come_numero",
      relatedColumn: "valor_objeto",
    });
    expect(s.mapping).toEqual([{ attribute: "padr_numero", targetColumn: "padr_numero" }]);
  });

  it("uniqueness estable tras el round-trip", () => {
    const input = toDefinitionInput(
      stateWithAction(createAction({ uniqueness: { columns: ["padr_numero", "depe_numero"], message: "ya existe" } })),
    );
    const u = fromDefinition(input as ModuleDefinitionInput).actions[0].uniqueness;
    expect(u).toEqual({ columns: ["padr_numero", "depe_numero"], message: "ya existe" });
  });

  it("definición sin estrategias se hidrata a estado vacío (strategy kind 'constant', uniqueness sin columnas)", () => {
    const input = toDefinitionInput(
      stateWithAction(createAction({ inputFields: [field({ name: "list_numero", source: "input" })] })),
    );
    const rt = fromDefinition(input as ModuleDefinitionInput);
    expect(rt.actions[0].inputFields[0].strategy).toEqual(emptyStrategyFormState());
    expect(rt.actions[0].uniqueness).toEqual(emptyUniquenessFormState());
  });
});

describe("strategyError — validaciones del asistente", () => {
  it("capture-from-catalog: exige catálogo completo y clave de captura", () => {
    const incompleto = field({
      name: "__capture_comercio",
      source: "fillStrategy",
      strategy: strat({ kind: "capture-from-catalog", catalog: { table: "", idColumn: "id", labelColumn: "n", order: { column: "", direction: "asc" } } }),
    });
    expect(strategyError(incompleto, [])).toBe(
      'El campo "__capture_comercio" de lista dinámica necesita una tabla de origen',
    );
    const sinClave = field({
      name: "__capture_comercio",
      source: "fillStrategy",
      strategy: strat({ kind: "capture-from-catalog", catalog: { table: "t", idColumn: "id", labelColumn: "n", order: { column: "", direction: "asc" } }, captureKey: "" }),
    });
    expect(strategyError(sinClave, [])).toBe(
      'La captura "__capture_comercio" necesita una clave de captura',
    );
    const ok = field({
      name: "__capture_comercio",
      source: "fillStrategy",
      strategy: strat({ kind: "capture-from-catalog", catalog: { table: "t", idColumn: "id", labelColumn: "n", order: { column: "", direction: "asc" } }, captureKey: "comercio" }),
    });
    expect(strategyError(ok, [])).toBeNull();
  });

  it("derive-from-option: exige sourceCaptureKey existente y attribute", () => {
    const sinCaptura = field({
      name: "list_numero",
      source: "fillStrategy",
      strategy: strat({ kind: "derive-from-option", sourceCaptureKey: "medio", attribute: "x" }),
    });
    expect(strategyError(sinCaptura, ["otra"])).toBe(
      'El campo "list_numero" referencia una captura de origen inexistente',
    );
    const sinAttr = field({
      name: "list_numero",
      source: "fillStrategy",
      strategy: strat({ kind: "derive-from-option", sourceCaptureKey: "medio", attribute: "" }),
    });
    expect(strategyError(sinAttr, ["medio"])).toBe(
      'El campo "list_numero" necesita un atributo de la opción',
    );
    const ok = field({
      name: "list_numero",
      source: "fillStrategy",
      strategy: strat({ kind: "derive-from-option", sourceCaptureKey: "medio", attribute: "list_numero" }),
    });
    expect(strategyError(ok, ["medio"])).toBeNull();
  });

  it("derive-via-fk: exige tabla/columna, captura local existente, atributo local y >=1 mapeo", () => {
    const base = (strategy: StrategyFormState) =>
      field({ name: "padr_numero", source: "fillStrategy", strategy });
    const sinTabla = base(strat({ kind: "derive-via-fk", jump: { relatedTable: "", localCaptureKey: "comercio", localAttribute: "x", relatedColumn: "y" } }));
    expect(strategyError(sinTabla, ["comercio"])).toBe(
      'El campo "padr_numero" necesita la tabla y columna relacionadas',
    );
    const capturaMala = base(strat({ kind: "derive-via-fk", jump: { relatedTable: "t", localCaptureKey: "comercio", localAttribute: "x", relatedColumn: "y" } }));
    expect(strategyError(capturaMala, ["otra"])).toBe(
      'El campo "padr_numero" referencia una captura local inexistente',
    );
    const sinMapeo = base(strat({ kind: "derive-via-fk", jump: { relatedTable: "t", localCaptureKey: "comercio", localAttribute: "x", relatedColumn: "y" }, mapping: [] }));
    expect(strategyError(sinMapeo, ["comercio"])).toBe(
      'El campo "padr_numero" necesita al menos un mapeo atributo→columna',
    );
    const ok = base(strat({ kind: "derive-via-fk", jump: { relatedTable: "t", localCaptureKey: "comercio", localAttribute: "x", relatedColumn: "y" }, mapping: [{ attribute: "padr_numero", targetColumn: "padr_numero" }] }));
    expect(strategyError(ok, ["comercio"])).toBeNull();
  });

  it("constant/autogenerated siempre válidas", () => {
    expect(strategyError(field({ source: "fillStrategy", strategy: strat({ kind: "constant" }) }), [])).toBeNull();
    expect(strategyError(field({ source: "fillStrategy", strategy: strat({ kind: "autogenerated" }) }), [])).toBeNull();
  });
});

describe("MEJORA 3/4 — label y captureOnly (omitempty + round-trip)", () => {
  it("un campo sin label ni captureOnly no emite esas claves", () => {
    const emitted = emittedField(
      stateWithAction(createAction({ inputFields: [field({ name: "padr_numero", source: "input" })] })),
    );
    expect(emitted.label).toBeUndefined();
    expect(emitted.captureOnly).toBeUndefined();
    const json = JSON.stringify(emitted);
    expect(json.includes("label")).toBe(false);
    expect(json.includes("captureOnly")).toBe(false);
  });

  it("label con contenido se emite (trim); captureOnly:true se emite", () => {
    const emitted = emittedField(
      stateWithAction(
        createAction({
          inputFields: [
            field({
              name: "__capture_comercio",
              source: "fillStrategy",
              label: "  Comercio  ",
              captureOnly: true,
              strategy: strat({
                kind: "capture-from-catalog",
                catalog: { table: "cl_comercios_sapp", idColumn: "come_numero", labelColumn: "nombre", order: { column: "", direction: "asc" } },
                captureKey: "comercio",
              }),
            }),
          ],
        }),
      ),
    );
    expect(emitted.label).toBe("Comercio");
    expect(emitted.captureOnly).toBe(true);
  });

  it("label solo espacios ⇒ se omite (omitempty)", () => {
    const emitted = emittedField(
      stateWithAction(createAction({ inputFields: [field({ name: "padr_numero", label: "   " })] })),
    );
    expect(emitted.label).toBeUndefined();
  });

  it("round-trip fromDefinition(toDefinitionInput) conserva label y captureOnly", () => {
    const state = stateWithAction(
      createAction({
        inputFields: [
          field({
            name: "__capture_comercio",
            source: "fillStrategy",
            label: "Comercio",
            captureOnly: true,
            strategy: strat({
              kind: "capture-from-catalog",
              catalog: { table: "cl_comercios_sapp", idColumn: "come_numero", labelColumn: "nombre", order: { column: "", direction: "asc" } },
              captureKey: "comercio",
            }),
          }),
        ],
      }),
    );
    const def = toDefinitionInput(state);
    const hydrated = fromDefinition(def);
    const f0 = hydrated.actions[0].inputFields[0];
    expect(f0.label).toBe("Comercio");
    expect(f0.captureOnly).toBe(true);
  });
});
