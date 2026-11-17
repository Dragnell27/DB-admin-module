import { describe, expect, it } from "vitest";
import {
  OPERATOR_OPTIONS,
  isNullOperator,
  splitValues,
  toOperator,
  validateOperatorValues,
} from "./operators";

describe("OPERATOR_OPTIONS", () => {
  it("expone los 9 operadores con etiquetas en español", () => {
    expect(OPERATOR_OPTIONS.map((o) => o.label)).toEqual([
      "igual a",
      "distinto de",
      "está en",
      "no está en",
      "contiene",
      "empieza con",
      "termina con",
      "es vacío",
      "no es vacío",
    ]);
  });
});

describe("toOperator", () => {
  it("acepta operadores conocidos y descarta el resto", () => {
    expect(toOperator("not_in")).toBe("not_in");
    expect(toOperator("drop")).toBeUndefined();
    expect(toOperator(5)).toBeUndefined();
  });
});

describe("splitValues", () => {
  it("in/not_in parten por comas, recortan y descartan vacíos", () => {
    expect(splitValues("in", " a, b ,, c ")).toEqual(["a", "b", "c"]);
    expect(splitValues("not_in", "x,")).toEqual(["x"]);

  });
  it("eq/neq/LIKE no parten por comas", () => {
    expect(splitValues("eq", " a,b ")).toEqual(["a,b"]);
    expect(splitValues("contains", "x")).toEqual(["x"]);
    expect(splitValues("eq", "  ")).toEqual([]);
  });
  it("nulidad no usa valores", () => {
    expect(splitValues("is_null", "algo")).toEqual([]);
    expect(isNullOperator("is_not_null")).toBe(true);
    expect(isNullOperator("eq")).toBe(false);
  });
});

describe("validateOperatorValues", () => {
  it("rechaza valores faltantes con mensajes en español", () => {
    expect(validateOperatorValues("in", [])).toBe(
      "el operador «está en» necesita al menos un valor",
    );
    expect(validateOperatorValues("eq", [])).toBe(
      "el operador «igual a» necesita un valor",
    );
    expect(validateOperatorValues("contains", [])).toBe(
      "el operador «contiene» necesita un valor",
    );
  });
  it("rechaza valores en operadores de nulidad", () => {
    expect(validateOperatorValues("is_null", ["x"])).toBe(
      "el operador «es vacío» no admite valores",
    );
  });
  it("acepta aridades válidas", () => {
    expect(validateOperatorValues("in", ["a", "b"])).toBeNull();
    expect(validateOperatorValues("starts_with", ["a"])).toBeNull();
    expect(validateOperatorValues("is_not_null", [])).toBeNull();
  });
});
