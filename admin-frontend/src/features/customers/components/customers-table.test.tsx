// @vitest-environment jsdom

// Pruebas del botón flotante "Ir al inicio" en `CustomersTable`.
// Validan que:
// - Se renderiza el botón flotante "Ir al inicio" (esquina inferior derecha).
// - "Ir al inicio" invoca `scrollIntoView` sobre el ancla del tope.
// - "Aplicar filtros" de la Card dispara el filtrado (reutiliza `applyFilters`):
//   al buscar por un valor, la query se vuelve a ejecutar con el filtro aplicado.
//
// jsdom no implementa `Element.prototype.scrollIntoView` ni `window.matchMedia`,
// así que se stubean (de lo contrario estos tests fallarían).

import "@testing-library/jest-dom/vitest";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

// Mock del hook de datos: captura los `filters` con los que se consulta.
const useCustomersMock = vi.fn();

vi.mock("@/features/customers/hooks", () => ({
  useCustomers: (...args: unknown[]) => useCustomersMock(...args),
}));

import { CustomersTable } from "@/features/customers/components/customers-table";

const scrollIntoViewMock = vi.fn();

beforeEach(() => {
  useCustomersMock.mockReset();
  useCustomersMock.mockReturnValue({
    data: {
      items: [],
      pagination: { totalRecords: 0, page: 1, recordsPerPage: 10, totalPages: 0 },
    },
    isLoading: false,
    isError: false,
    error: null,
  });

  scrollIntoViewMock.mockReset();
  // jsdom no implementa estos; se stubean para los tests.
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

describe("CustomersTable — botón flotante Ir al inicio", () => {
  it("renderiza el botón flotante Ir al inicio y los filtros Aplicar/Limpiar", () => {
    render(<CustomersTable />);

    expect(
      screen.getByRole("button", { name: "Ir al inicio de la vista" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Aplicar filtros" }),
    ).toBeInTheDocument();
  });

  it("invoca scrollIntoView al pulsar Ir al inicio", async () => {
    const user = userEvent.setup({ delay: null });
    render(<CustomersTable />);

    await user.click(
      screen.getByRole("button", { name: "Ir al inicio de la vista" }),
    );

    expect(scrollIntoViewMock).toHaveBeenCalled();
  });

  it("Aplicar filtros dispara el filtrado con el valor del campo", async () => {
    const user = userEvent.setup({ delay: null });
    render(<CustomersTable />);

    // Escribe un valor en el campo de la Card de filtros.
    await user.type(screen.getByLabelText("Customer ID"), "cust-123");

    await user.click(screen.getByRole("button", { name: "Aplicar filtros" }));

    // La última consulta se hizo con el filtro aplicado.
    const lastCall =
      useCustomersMock.mock.calls[useCustomersMock.mock.calls.length - 1];
    expect(lastCall[1]).toEqual({ customer_id: "cust-123" });
  });

  it("Limpiar restablece el valor del campo", async () => {
    const user = userEvent.setup({ delay: null });
    render(<CustomersTable />);

    const input = screen.getByLabelText("Customer ID") as HTMLInputElement;
    await user.type(input, "cust-123");
    expect(input.value).toBe("cust-123");

    await user.click(screen.getByRole("button", { name: "Limpiar" }));

    expect(input.value).toBe("");
  });

  it("renderiza el ancla invisible al inicio del contenedor", () => {
    const { container } = render(<CustomersTable />);
    const root = container.firstElementChild as HTMLElement;
    const anchor = root.firstElementChild as HTMLElement;
    expect(anchor).toHaveAttribute("aria-hidden", "true");
    void within(root);
  });
});
