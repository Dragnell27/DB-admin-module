import { describe, expect, it } from "vitest";

import { showsCountrySelector } from "@/lib/ott-routes";

describe("showsCountrySelector", () => {
  it("muestra el selector en los recursos OTT por país", () => {
    for (const path of [
      "/customers",
      "/customers/new",
      "/customers/edit",
      "/cards",
      "/cards/new",
      "/plans",
      "/plans/detail",
      "/subscriptions",
      "/subscriptions/cancel",
    ]) {
      expect(showsCountrySelector(path)).toBe(true);
    }
  });

  it("NO muestra el selector fuera de los recursos OTT", () => {
    for (const path of [
      "/",
      "/module-defs",
      "/module-defs/new",
      "/module-defs/facturas/edit",
      "/connection-templates",
      "/roles",
      "/users",
      "/audit",
      "/modules/algun-modulo",
    ]) {
      expect(showsCountrySelector(path)).toBe(false);
    }
  });

  it("no confunde prefijos textuales que no son segmentos de ruta", () => {
    // `/customers-extra` empieza por «/customers» pero NO es el recurso OTT.
    expect(showsCountrySelector("/customers-extra")).toBe(false);
    expect(showsCountrySelector("/cards-archive")).toBe(false);
  });

  it("devuelve false con pathname nulo o vacío", () => {
    expect(showsCountrySelector(null)).toBe(false);
    expect(showsCountrySelector("")).toBe(false);
  });
});
