// @vitest-environment jsdom
// Pruebas del store de tema CLARO/OSCURO (Entrega A). Espejo de
// `sidebar-collapse.test.ts`: cubren el defecto OSCURO sin estado, la lectura de
// un valor guardado, la normalización de valores inválidos, el ciclo de
// persistencia de setTheme/toggleTheme, el snapshot de servidor estable
// (hidratación SSR), la sincronización vía evento `storage` y la degradación
// silenciosa cuando `localStorage` lanza. Incluye además una comprobación del
// string del script anti-FOUC (sin ejecutarlo).

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  __resetThemeCacheForTests,
  DEFAULT_THEME,
  getServerTheme,
  getTheme,
  setTheme,
  subscribeTheme,
  THEME_KEY,
  toggleTheme,
} from "@/lib/theme";
import { THEME_INIT } from "@/lib/theme-script";

describe("store de tema (persistencia en localStorage)", () => {
  beforeEach(() => {
    window.localStorage.clear();
    document.documentElement.className = "";
    document.documentElement.style.colorScheme = "";
    __resetThemeCacheForTests();
  });
  afterEach(() => {
    window.localStorage.clear();
    document.documentElement.className = "";
    document.documentElement.style.colorScheme = "";
    __resetThemeCacheForTests();
    vi.restoreAllMocks();
  });

  it("por defecto (sin estado) el tema es OSCURO", () => {
    expect(getTheme()).toBe("dark");
    expect(DEFAULT_THEME).toBe("dark");
  });

  it("lee un valor guardado ('light') desde localStorage", () => {
    window.localStorage.setItem(THEME_KEY, "light");
    __resetThemeCacheForTests();
    expect(getTheme()).toBe("light");
  });

  it("normaliza un valor inválido degradando a OSCURO", () => {
    window.localStorage.setItem(THEME_KEY, "azul");
    __resetThemeCacheForTests();
    expect(getTheme()).toBe("dark");
  });

  it("setTheme persiste el tema, lo relee y aplica la clase en <html>", () => {
    setTheme("light");
    expect(getTheme()).toBe("light");
    expect(window.localStorage.getItem(THEME_KEY)).toBe("light");
    expect(document.documentElement.classList.contains("dark")).toBe(false);
    expect(document.documentElement.style.colorScheme).toBe("light");

    setTheme("dark");
    expect(getTheme()).toBe("dark");
    expect(window.localStorage.getItem(THEME_KEY)).toBe("dark");
    expect(document.documentElement.classList.contains("dark")).toBe(true);
    expect(document.documentElement.style.colorScheme).toBe("dark");
  });

  it("toggleTheme alterna entre oscuro y claro y persiste", () => {
    // Defecto oscuro → el primer toggle lleva a claro.
    toggleTheme();
    expect(getTheme()).toBe("light");
    expect(window.localStorage.getItem(THEME_KEY)).toBe("light");
    toggleTheme();
    expect(getTheme()).toBe("dark");
    expect(window.localStorage.getItem(THEME_KEY)).toBe("dark");
  });

  it("getServerTheme devuelve SIEMPRE 'dark' (snapshot SSR estable)", () => {
    expect(getServerTheme()).toBe("dark");
    window.localStorage.setItem(THEME_KEY, "light");
    __resetThemeCacheForTests();
    // El snapshot de servidor NO depende del almacenamiento.
    expect(getServerTheme()).toBe("dark");
  });

  it("sincroniza entre pestañas vía el evento 'storage'", () => {
    const listener = vi.fn();
    const unsubscribe = subscribeTheme(listener);

    window.localStorage.setItem(THEME_KEY, "light");
    window.dispatchEvent(new StorageEvent("storage", { key: THEME_KEY }));

    expect(listener).toHaveBeenCalledTimes(1);
    expect(getTheme()).toBe("light");
    expect(document.documentElement.classList.contains("dark")).toBe(false);

    unsubscribe();
  });

  it("degrada silenciosamente cuando localStorage.setItem lanza", () => {
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("almacenamiento bloqueado");
    });
    expect(() => setTheme("light")).not.toThrow();
    // Sin persistir, pero la clase en <html> sí se aplicó.
    expect(document.documentElement.classList.contains("dark")).toBe(false);
  });
});

describe("script anti-FOUC (THEME_INIT)", () => {
  it("contiene la clave de almacenamiento y el defecto oscuro", () => {
    expect(THEME_INIT).toContain("panel-admin-ott.theme");
    expect(THEME_INIT).toContain("'dark'");
    expect(THEME_INIT).toContain("classList.toggle('dark'");
  });
});
