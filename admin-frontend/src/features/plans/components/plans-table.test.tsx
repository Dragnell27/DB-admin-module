// @vitest-environment jsdom

// Pruebas del botón flotante "Ir al inicio" en `PlansTable`.
// Validan que:
// - Se conserva el <h1>Plans</h1> interno.
// - Se renderiza el botón flotante "Ir al inicio".
// - "Ir al inicio" invoca `scrollIntoView` sobre el ancla del tope.
//
// jsdom no implementa `Element.prototype.scrollIntoView` ni `window.matchMedia`,
// así que se stubean.

import "@testing-library/jest-dom/vitest";
import type * as React from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
  QueryClient,
  QueryClientProvider,
} from "@tanstack/react-query";

const usePlansMock = vi.fn();

vi.mock("@/features/plans/hooks", () => ({
  // `planId` real-ish: resuelve id/plan_id.
  planId: (plan: { id?: string; plan_id?: string }) =>
    plan.id ?? plan.plan_id ?? "",
  usePlans: (...args: unknown[]) => usePlansMock(...args),
}));

vi.mock("@/lib/use-permissions", () => ({
  usePermissions: () => [],
}));

vi.mock("@/lib/country", () => ({
  getCountry: () => "gt",
  subscribeCountry: () => () => {},
}));

import { PlansTable } from "@/features/plans/components/plans-table";

const scrollIntoViewMock = vi.fn();

/** Renderiza con un `QueryClientProvider` (el `ReloadButton` lo requiere). */
function renderWithClient(ui: React.ReactElement) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>{ui}</QueryClientProvider>,
  );
}

beforeEach(() => {
  usePlansMock.mockReset();
  usePlansMock.mockReturnValue({
    data: {
      items: [],
      pagination: { totalRecords: 0, page: 1, recordsPerPage: 10, totalPages: 0 },
    },
    isLoading: false,
    isFetching: false,
  });

  scrollIntoViewMock.mockReset();
  Element.prototype.scrollIntoView = scrollIntoViewMock;
  vi.stubGlobal(
    "matchMedia",
    vi.fn().mockReturnValue({
      matches: false,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    }),
  );
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("PlansTable — botón flotante Ir al inicio", () => {
  it("conserva el <h1>Plans</h1> interno", () => {
    renderWithClient(<PlansTable onCreate={vi.fn()} />);
    expect(
      screen.getByRole("heading", { level: 1, name: "Plans" }),
    ).toBeInTheDocument();
  });

  it("renderiza el botón flotante Ir al inicio y el filtro Aplicar", () => {
    renderWithClient(<PlansTable onCreate={vi.fn()} />);
    expect(
      screen.getByRole("button", { name: "Aplicar filtros" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Ir al inicio de la vista" }),
    ).toBeInTheDocument();
  });

  it("invoca scrollIntoView al pulsar Ir al inicio", async () => {
    const user = userEvent.setup({ delay: null });
    renderWithClient(<PlansTable onCreate={vi.fn()} />);

    await user.click(
      screen.getByRole("button", { name: "Ir al inicio de la vista" }),
    );

    expect(scrollIntoViewMock).toHaveBeenCalled();
  });

  it("Aplicar filtros dispara el filtrado con el valor del campo", async () => {
    const user = userEvent.setup({ delay: null });
    renderWithClient(<PlansTable onCreate={vi.fn()} />);

    // Criterio por defecto: "name".
    await user.type(screen.getByLabelText("Nombre"), "Oro");

    await user.click(screen.getByRole("button", { name: "Aplicar filtros" }));

    const lastCall =
      usePlansMock.mock.calls[usePlansMock.mock.calls.length - 1];
    expect(lastCall[1]).toEqual({ name: "Oro" });
  });
});
