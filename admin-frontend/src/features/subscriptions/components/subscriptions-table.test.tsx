// @vitest-environment jsdom

// Pruebas del botón flotante "Ir al inicio" en `SubscriptionsTable`.
// Validan que:
// - Se renderiza el botón flotante "Ir al inicio".
// - "Ir al inicio" invoca `scrollIntoView` sobre el ancla del tope.
// - "Aplicar filtros" de la Card reutiliza el mismo filtrado (callback común).
//
// jsdom no implementa `Element.prototype.scrollIntoView` ni `window.matchMedia`,
// así que se stubean.

import "@testing-library/jest-dom/vitest";
import type * as React from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const useSubscriptionsMock = vi.fn();

vi.mock("@/features/subscriptions/hooks", () => ({
  useSubscriptions: (...args: unknown[]) => useSubscriptionsMock(...args),
}));

vi.mock("@/lib/use-permissions", () => ({
  usePermissions: () => [],
}));

vi.mock("@/lib/country", () => ({
  getCountry: () => "gt",
  subscribeCountry: () => () => {},
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn() }),
}));

import { SubscriptionsTable } from "@/features/subscriptions/components/subscriptions-table";

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
  useSubscriptionsMock.mockReset();
  useSubscriptionsMock.mockReturnValue({
    data: {
      items: [],
      pagination: { totalRecords: 0, page: 1, recordsPerPage: 10, totalPages: 0 },
    },
    isLoading: false,
    isError: false,
    error: null,
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

describe("SubscriptionsTable — botón flotante Ir al inicio", () => {
  it("renderiza el botón flotante Ir al inicio y el filtro Aplicar", () => {
    renderWithClient(<SubscriptionsTable canWrite={false} />);
    expect(
      screen.getByRole("button", { name: "Aplicar filtros" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Ir al inicio de la vista" }),
    ).toBeInTheDocument();
  });

  it("invoca scrollIntoView al pulsar Ir al inicio", async () => {
    const user = userEvent.setup({ delay: null });
    renderWithClient(<SubscriptionsTable canWrite={false} />);

    await user.click(
      screen.getByRole("button", { name: "Ir al inicio de la vista" }),
    );

    expect(scrollIntoViewMock).toHaveBeenCalled();
  });

  it("Aplicar filtros dispara el filtrado con el valor del campo", async () => {
    const user = userEvent.setup({ delay: null });
    renderWithClient(<SubscriptionsTable canWrite={false} />);

    // Criterio por defecto: "customer_id".
    await user.type(screen.getByLabelText("Customer ID"), "cust-9");

    await user.click(screen.getByRole("button", { name: "Aplicar filtros" }));

    const lastCall =
      useSubscriptionsMock.mock.calls[
        useSubscriptionsMock.mock.calls.length - 1
      ];
    expect(lastCall[1]).toEqual({ customer_id: "cust-9" });
  });
});
