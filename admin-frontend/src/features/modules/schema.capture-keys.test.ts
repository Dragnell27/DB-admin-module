// Pruebas puras de declaredCaptureKeys (Arreglo 3): la ÚNICA fuente de las
// claves de captura (`capture-from-catalog`) de una acción, independiente del
// ORDEN (recorre TODOS los campos), excluye `selfIndex`, descarta vacíos y
// colapsa duplicados. Blindaje anti-regresión del bug histórico "solo listaba
// las capturas definidas ANTES". Entorno node (NO monta React).

import { describe, expect, it } from "vitest";

import {
  declaredCaptureKeys,
  emptyStrategyFormState,
  emptyUniquenessFormState,
  type ActionFormState,
  type FieldFormState,
  type StrategyFormState,
} from "@/features/modules/schema";

/** Campo capture-from-catalog con la clave de captura dada. */
function capture(captureKey: string): FieldFormState {
  return field("fillStrategy", {
    ...emptyStrategyFormState(),
    kind: "capture-from-catalog",
    catalog: { table: "cat", idColumn: "id", labelColumn: "nombre", order: { column: "", direction: "asc" } },
    captureKey,
  });
}

/** Campo derive-from-option (no aporta captura; se usa como "self"). */
function derive(sourceCaptureKey: string): FieldFormState {
  return field("fillStrategy", {
    ...emptyStrategyFormState(),
    kind: "derive-from-option",
    sourceCaptureKey,
    attribute: "codigo",
  });
}

function field(source: FieldFormState["source"], strategy: StrategyFormState): FieldFormState {
  return {
    name: "campo",
    type: "string",
    required: false,
    source,
    rowField: "",
    constValue: "",
    options: [],
    defaultValue: "",
    dynamicSource: { table: "", idColumn: "", labelColumn: "", order: { column: "", direction: "asc" } },
    dynamicFilters: [],
    hidden: false,
    label: "",
    captureOnly: false,
    strategy,
  };
}

/** Acción con los campos dados. */
function action(fields: FieldFormState[]): ActionFormState {
  return {
    actionKey: "crear",
    kind: "create",
    label: "Crear",
    placement: "header",
    destructive: false,
    permAction: "crear",
    isWrite: true,
    sqlOp: "INSERT",
    inputFields: fields,
    uniqueness: emptyUniquenessFormState(),
  };
}

describe("declaredCaptureKeys", () => {
  it("es independiente del orden: recoge una captura definida DESPUÉS del self", () => {
    // Campo 0 = derivación (self); campo 1 = captura "medio_pago" (posterior).
    const a = action([derive("medio_pago"), capture("medio_pago")]);
    expect(declaredCaptureKeys(a, 0)).toContain("medio_pago");
  });

  it("recoge capturas antes y después del self, en orden de aparición, sin duplicados", () => {
    const a = action([
      capture("comercio"),
      derive("comercio"), // self (idx 1): se excluye, pero no aporta captura
      capture("medio_pago"),
      capture("comercio"), // duplicado: se colapsa
    ]);
    expect(declaredCaptureKeys(a, 1)).toEqual(["comercio", "medio_pago"]);
  });

  it("excluye el selfIndex y descarta claves vacías/solo-espacios", () => {
    const a = action([capture("comercio"), capture("   "), capture("medio_pago")]);
    // selfIndex 0 excluye "comercio"; "   " se descarta por trim.
    expect(declaredCaptureKeys(a, 0)).toEqual(["medio_pago"]);
  });

  it("selfIndex = -1 no excluye ninguno (uso del agregador)", () => {
    const a = action([capture("comercio"), capture("medio_pago")]);
    expect(declaredCaptureKeys(a, -1)).toEqual(["comercio", "medio_pago"]);
  });
});
