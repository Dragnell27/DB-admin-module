// @vitest-environment jsdom
// Pruebas del estado contraído del menú lateral global (FEAT-002). Cubren el
// valor por defecto (expandido = false sin estado almacenado), el ciclo de
// persistencia en localStorage de setCollapsed/toggleCollapsed/getCollapsed y
// la degradación a expandido ante un valor almacenado inválido.

import { afterEach, beforeEach, describe, expect, it } from "vitest";

import {
  __resetCollapseCacheForTests,
  getCollapsed,
  setCollapsed,
  SIDEBAR_COLLAPSE_KEY,
  toggleCollapsed,
} from "@/lib/sidebar-collapse";

describe("persistencia del estado contraído en localStorage", () => {
  beforeEach(() => {
    window.localStorage.clear();
    __resetCollapseCacheForTests();
  });
  afterEach(() => {
    window.localStorage.clear();
    __resetCollapseCacheForTests();
  });

  it("por defecto (sin estado) el menú está expandido (false)", () => {
    expect(getCollapsed()).toBe(false);
  });

  it("setCollapsed(true) persiste el estado contraído y lo relee", () => {
    setCollapsed(true);
    expect(getCollapsed()).toBe(true);
    expect(window.localStorage.getItem(SIDEBAR_COLLAPSE_KEY)).toBe(
      JSON.stringify(true),
    );
  });

  it("relee un valor escrito directamente en localStorage tras invalidar la caché", () => {
    window.localStorage.setItem(SIDEBAR_COLLAPSE_KEY, JSON.stringify(true));
    __resetCollapseCacheForTests();
    expect(getCollapsed()).toBe(true);
  });

  it("toggleCollapsed alterna entre contraído y expandido y persiste", () => {
    toggleCollapsed();
    expect(getCollapsed()).toBe(true);
    expect(window.localStorage.getItem(SIDEBAR_COLLAPSE_KEY)).toBe(
      JSON.stringify(true),
    );
    toggleCollapsed();
    expect(getCollapsed()).toBe(false);
    expect(window.localStorage.getItem(SIDEBAR_COLLAPSE_KEY)).toBe(
      JSON.stringify(false),
    );
  });

  it("ignora un valor almacenado inválido degradando a expandido (false)", () => {
    window.localStorage.setItem(SIDEBAR_COLLAPSE_KEY, "no-es-json");
    __resetCollapseCacheForTests();
    expect(getCollapsed()).toBe(false);
  });
});
