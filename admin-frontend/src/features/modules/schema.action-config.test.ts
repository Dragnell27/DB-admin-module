// Bug #1 (persistencia extremo a extremo): toDefinitionInput debe EMITIR
// targetTable y uniqueness de la acción create JUNTOS, y fromDefinition debe
// RE-HIDRATARLOS. Es el espejo en frontend del round-trip de backend
// (encode/decodeActionConfig). Entorno node (sin jsdom).

import { describe, expect, it } from "vitest";

import {
  emptyUniquenessFormState,
  emptyWizardState,
  fromDefinition,
  toDefinitionInput,
  type ActionFormState,
  type WizardState,
} from "@/features/modules/schema";
import type { ModuleDefinitionInput } from "@/features/modules/types";

/** Acción create del caso real "activar medio de pago para un comercio". */
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

/** Estado multi-tabla mínimo (principal distinta de la tabla objetivo). */
function stateWithAction(action: ActionFormState): WizardState {
  return {
    ...emptyWizardState(),
    slug: "qa-medios-pago-x-comercio",
    name: "Medios de pago por comercio",
    sourceType: "postgres",
    pgTable: "gt_valores",
    pgWhitelist: {
      gt_valores: ["valo_numero"],
      gt_valores_hijo: ["padr_numero", "valor_lista", "valor_objeto"],
    },
    foreignTables: ["gt_valores_hijo"],
    actions: [action],
  };
}

describe("Bug #1 — targetTable + uniqueness persisten extremo a extremo", () => {
  it("toDefinitionInput emite targetTable y uniqueness juntos", () => {
    const input = toDefinitionInput(
      stateWithAction(
        createAction({
          targetTable: "gt_valores_hijo",
          uniqueness: {
            columns: ["padr_numero", "valor_lista", "valor_objeto"],
            message: "Ya existe este medio de pago para el comercio",
          },
        }),
      ),
    );
    expect(input.actions[0].targetTable).toBe("gt_valores_hijo");
    expect(input.actions[0].uniqueness).toEqual({
      columns: ["padr_numero", "valor_lista", "valor_objeto"],
      message: "Ya existe este medio de pago para el comercio",
    });
  });

  it("fromDefinition re-hidrata targetTable y uniqueness tras el round-trip", () => {
    const input = toDefinitionInput(
      stateWithAction(
        createAction({
          targetTable: "gt_valores_hijo",
          uniqueness: {
            columns: ["padr_numero", "valor_lista", "valor_objeto"],
            message: "Ya existe este medio de pago para el comercio",
          },
        }),
      ),
    );
    const rt = fromDefinition(input as ModuleDefinitionInput);
    expect(rt.actions[0].targetTable).toBe("gt_valores_hijo");
    expect(rt.actions[0].uniqueness).toEqual({
      columns: ["padr_numero", "valor_lista", "valor_objeto"],
      message: "Ya existe este medio de pago para el comercio",
    });
  });
});
