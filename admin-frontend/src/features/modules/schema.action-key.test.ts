// Pruebas puras del esquema del constructor de módulos para la UX de la clave
// de acción (FEAT-001): slugifyActionKey (acentos, ñ, símbolos, colapso y trim
// de '_', cadena vacía) y datalistActionKeys (claves distintas, dedupe, excluye
// vacíos/whitespace y la propia acción por selfIndex).

import { describe, expect, it } from "vitest";

import {
  datalistActionKeys,
  slugifyActionKey,
  type ActionFormState,
} from "@/features/modules/schema";

/** Acción de formulario mínima: solo importa la actionKey para el datalist. */
function action(actionKey: string): ActionFormState {
  return {
    actionKey,
    kind: "custom",
    label: "",
    placement: "row",
    destructive: false,
    permAction: `custom:${actionKey}`,
    isWrite: true,
    sqlOp: "",
    inputFields: [],
  };
}

describe("slugifyActionKey", () => {
  it("mapea vocales acentuadas a su letra base", () => {
    expect(slugifyActionKey("áéíóú")).toBe("aeiou");
    expect(slugifyActionKey("Buscar cuenta")).toBe("buscar_cuenta");
  });

  it("mapea 'ñ' a 'n'", () => {
    expect(slugifyActionKey("Añadir Item")).toBe("anadir_item");
    expect(slugifyActionKey("ñandú")).toBe("nandu");
  });

  it("convierte espacios y símbolos en '_'", () => {
    expect(slugifyActionKey("Ver detalle / editar")).toBe("ver_detalle_editar");
    expect(slugifyActionKey("precio ($)")).toBe("precio");
  });

  it("colapsa los '_' repetidos", () => {
    expect(slugifyActionKey("a   b")).toBe("a_b");
    expect(slugifyActionKey("a - - b")).toBe("a_b");
  });

  it("recorta los '_' de los bordes", () => {
    expect(slugifyActionKey("  hola  ")).toBe("hola");
    expect(slugifyActionKey("!activar!")).toBe("activar");
  });

  it("devuelve cadena vacía para entrada vacía o solo símbolos", () => {
    expect(slugifyActionKey("")).toBe("");
    expect(slugifyActionKey("   ")).toBe("");
    expect(slugifyActionKey("***")).toBe("");
  });
});

describe("datalistActionKeys", () => {
  it("devuelve las claves distintas de las otras acciones", () => {
    const actions = [action("buscar"), action("crear"), action("editar")];
    expect(datalistActionKeys(actions, 1)).toEqual(["buscar", "editar"]);
  });

  it("deduplica claves repetidas conservando el orden de aparición", () => {
    const actions = [action("buscar"), action("crear"), action("buscar")];
    expect(datalistActionKeys(actions, 1)).toEqual(["buscar"]);
  });

  it("excluye claves vacías o de solo espacios", () => {
    const actions = [action(""), action("   "), action("activar")];
    expect(datalistActionKeys(actions, 2)).toEqual([]);
  });

  it("excluye la propia acción por selfIndex", () => {
    const actions = [action("buscar"), action("crear")];
    expect(datalistActionKeys(actions, 0)).toEqual(["crear"]);
    expect(datalistActionKeys(actions, 1)).toEqual(["buscar"]);
  });
});
