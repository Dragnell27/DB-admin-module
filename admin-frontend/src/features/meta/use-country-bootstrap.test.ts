// @vitest-environment jsdom

// Pruebas de `useCountryBootstrap` (FEAT-001): la auto-selección de país que
// antes vivía en `CountrySelector`. Con `useCountriesMeta`/`usePermissions`
// mockeados y un espía sobre `setCountry` de `lib/country`, se valida que:
//  (a) sin selección previa fija `active` cuando está permitido;
//  (b) NO sobrescribe una selección válida existente;
//  (c) NO actúa cuando no hay países permitidos (countries vacío);
//  (d) cae al primer país permitido cuando `active` no está permitido.

import * as React from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { renderHook } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

import * as country from "@/lib/country";
import { COUNTRY_KEY } from "@/lib/country";
import { useCountriesMeta } from "@/features/meta/hooks";
import { usePermissions } from "@/lib/use-permissions";
import { useCountryBootstrap } from "@/features/meta/use-country-bootstrap";

vi.mock("@/features/meta/hooks", () => ({
  useCountriesMeta: vi.fn(),
}));

vi.mock("@/lib/use-permissions", () => ({
  usePermissions: vi.fn(),
}));

const useCountriesMetaMock = vi.mocked(useCountriesMeta);
const usePermissionsMock = vi.mocked(usePermissions);

/** Permisos efectivos de ejemplo (gt + ni) para habilitar ambos países. */
const PERMS_GT_NI = [
  { resource: "customers", action: "ver", country: "gt" },
  { resource: "customers", action: "ver", country: "ni" },
] as const;

/** Configura el mock de `useCountriesMeta` con los datos indicados. */
function mockMeta(countries: string[], active: string): void {
  useCountriesMetaMock.mockReturnValue({
    data: { countries, active },
  } as ReturnType<typeof useCountriesMeta>);
}

/** Provee un `QueryClientProvider` sin reintentos para el hook. */
function createWrapper() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return React.createElement(QueryClientProvider, { client }, children);
  };
}

beforeEach(() => {
  // Partimos siempre sin selección previa; cada caso ajusta localStorage.
  window.localStorage.clear();
  usePermissionsMock.mockReturnValue(PERMS_GT_NI);
});

afterEach(() => {
  // `restoreAllMocks` deshace los `vi.spyOn` (no deja espías anidados entre
  // casos); `clearAllMocks` resetea el historial del resto de mocks.
  vi.restoreAllMocks();
  vi.clearAllMocks();
  window.localStorage.clear();
});

describe("useCountryBootstrap", () => {
  it("(a) sin selección + active permitido → fija el país activo", () => {
    const setCountrySpy = vi.spyOn(country, "setCountry");
    mockMeta(["gt", "ni"], "gt");

    renderHook(() => useCountryBootstrap(), { wrapper: createWrapper() });

    expect(setCountrySpy).toHaveBeenCalledTimes(1);
    expect(setCountrySpy).toHaveBeenCalledWith("gt");
  });

  it("(b) con selección válida previa → NO sobrescribe", () => {
    // Sembramos la selección directamente en localStorage (no vía `setCountry`)
    // para que el espía solo registre lo que haga el hook.
    window.localStorage.setItem(COUNTRY_KEY, "ni");
    const setCountrySpy = vi.spyOn(country, "setCountry");
    mockMeta(["gt", "ni"], "gt");

    renderHook(() => useCountryBootstrap(), { wrapper: createWrapper() });

    expect(setCountrySpy).not.toHaveBeenCalled();
  });

  it("(c) sin países permitidos (countries vacío) → NO actúa", () => {
    const setCountrySpy = vi.spyOn(country, "setCountry");
    mockMeta([], "");

    renderHook(() => useCountryBootstrap(), { wrapper: createWrapper() });

    expect(setCountrySpy).not.toHaveBeenCalled();
  });

  it("(d) active no permitido → cae al primer país permitido", () => {
    const setCountrySpy = vi.spyOn(country, "setCountry");
    // `active` = "gt" pero el usuario solo tiene permiso sobre "ni".
    usePermissionsMock.mockReturnValue([
      { resource: "customers", action: "ver", country: "ni" },
    ]);
    mockMeta(["gt", "ni"], "gt");

    renderHook(() => useCountryBootstrap(), { wrapper: createWrapper() });

    expect(setCountrySpy).toHaveBeenCalledTimes(1);
    expect(setCountrySpy).toHaveBeenCalledWith("ni");
  });
});
