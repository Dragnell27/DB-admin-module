// @vitest-environment jsdom
// Pruebas puras del estado de carpetas del menú lateral (collapsible folders).
// Cubren el helper puro `folderPath` (construcción de la ruta completa estable)
// e `isExpanded` (contraído por defecto, expandido/contraído explícito, unicidad
// por ruta anidada y auto-expandido de los ancestros de la ruta activa, con el
// contraído explícito ganando siempre), además del ciclo de persistencia en
// localStorage de setFolderExpanded/toggleFolder/getExpandedFolders.

import { afterEach, beforeEach, describe, expect, it } from "vitest";

import {
  __resetFoldersCacheForTests,
  folderPath,
  getExpandedFolders,
  isExpanded,
  setFolderExpanded,
  SIDEBAR_FOLDERS_KEY,
  toggleFolder,
} from "@/lib/sidebar-folders";

describe("folderPath", () => {
  it("usa la etiqueta tal cual cuando el padre es la raíz (\"\")", () => {
    expect(folderPath("", "CENAM")).toBe("CENAM");
  });

  it("une padre e hijo con «/» formando la ruta completa", () => {
    expect(folderPath("CENAM", "Recaudo")).toBe("CENAM/Recaudo");
    expect(folderPath("CENAM/Recaudo", "GT")).toBe("CENAM/Recaudo/GT");
  });
});

describe("isExpanded", () => {
  it("por defecto (sin estado) una carpeta está contraída", () => {
    expect(isExpanded({}, "CENAM")).toBe(false);
  });

  it("respeta el expandido explícito (true)", () => {
    expect(isExpanded({ CENAM: true }, "CENAM")).toBe(true);
  });

  it("respeta el contraído explícito (false)", () => {
    expect(isExpanded({ CENAM: false }, "CENAM")).toBe(false);
  });

  it("distingue carpetas homónimas por su ruta completa", () => {
    const map = { "CENAM/Recaudo": true };
    // Misma etiqueta «Recaudo» en otra rama: no comparte estado.
    expect(isExpanded(map, "CENAM/Recaudo")).toBe(true);
    expect(isExpanded(map, "OTRO/Recaudo")).toBe(false);
  });

  it("auto-expande los ancestros de la ruta activa si no hay estado explícito", () => {
    const ancestors = new Set(["CENAM", "CENAM/Recaudo"]);
    expect(isExpanded({}, "CENAM", ancestors)).toBe(true);
    expect(isExpanded({}, "CENAM/Recaudo", ancestors)).toBe(true);
    expect(isExpanded({}, "CENAM/Otro", ancestors)).toBe(false);
  });

  it("el contraído explícito gana sobre el auto-expandido de la ruta activa", () => {
    const ancestors = new Set(["CENAM"]);
    expect(isExpanded({ CENAM: false }, "CENAM", ancestors)).toBe(false);
  });
});

describe("persistencia en localStorage", () => {
  beforeEach(() => {
    window.localStorage.clear();
    __resetFoldersCacheForTests();
  });
  afterEach(() => {
    window.localStorage.clear();
    __resetFoldersCacheForTests();
  });

  it("arranca vacío (todo contraído) sin estado almacenado", () => {
    expect(getExpandedFolders()).toEqual({});
  });

  it("setFolderExpanded persiste el estado expandido y lo relee", () => {
    setFolderExpanded("CENAM/Recaudo", true);
    expect(getExpandedFolders()).toEqual({ "CENAM/Recaudo": true });
    expect(window.localStorage.getItem(SIDEBAR_FOLDERS_KEY)).toBe(
      JSON.stringify({ "CENAM/Recaudo": true }),
    );
  });

  it("setFolderExpanded(false) guarda el contraído explícito", () => {
    setFolderExpanded("CENAM", false);
    expect(getExpandedFolders()).toEqual({ CENAM: false });
  });

  it("toggleFolder alterna entre expandido y contraído", () => {
    toggleFolder("CENAM");
    expect(getExpandedFolders()).toEqual({ CENAM: true });
    toggleFolder("CENAM");
    expect(getExpandedFolders()).toEqual({ CENAM: false });
  });

  it("ignora un valor almacenado inválido degradando a vacío", () => {
    window.localStorage.setItem(SIDEBAR_FOLDERS_KEY, "no-es-json");
    __resetFoldersCacheForTests();
    expect(getExpandedFolders()).toEqual({});
  });
});
