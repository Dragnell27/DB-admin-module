// @vitest-environment jsdom

// Pruebas del botón flotante "Ir al inicio" + el enfoque B del payload en
// `AuditTable`. Validan que:
// - Se renderiza el botón flotante "Ir al inicio".
// - "Ir al inicio" invoca `scrollIntoView` sobre el ancla del tope.
// - El botón "Payload" conserva `aria-expanded` y alterna el detalle.
// - Al abrir el payload se renderiza "Payload registrado", se invoca
//   `scrollIntoView` sobre su Card y el foco va al encabezado.
// - "Volver a la fila" invoca `scrollIntoView` y devuelve el foco al botón
//   "Payload" que abrió el detalle.
//
// jsdom no implementa `Element.prototype.scrollIntoView` ni `window.matchMedia`,
// así que se stubean (de lo contrario estos tests fallarían).

import "@testing-library/jest-dom/vitest";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import type { AuditEntry } from "@/features/audit/hooks";

const useAuditLogsMock = vi.fn();

vi.mock("@/features/audit/hooks", () => ({
  useAuditLogs: (...args: unknown[]) => useAuditLogsMock(...args),
}));

import { AuditTable } from "@/features/audit/components/audit-table";

const scrollIntoViewMock = vi.fn();

const SAMPLE: AuditEntry = {
  id: "entry-1",
  actorName: "Ana",
  resource: "customers",
  action: "crear",
  objectId: "cust-1",
  country: "gt",
  outcome: "success",
  statusCode: 201,
  message: "Cliente creado",
  payload: { email: "a@b.com" },
  createdAt: "2026-10-07T12:00:00Z",
};

beforeEach(() => {
  useAuditLogsMock.mockReset();
  useAuditLogsMock.mockReturnValue({
    data: {
      items: [SAMPLE],
      pagination: { totalRecords: 1, page: 1, recordsPerPage: 25, totalPages: 1 },
    },
    isLoading: false,
    isFetching: false,
    isError: false,
    error: null,
    refetch: vi.fn(),
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

describe("AuditTable — botón flotante Ir al inicio", () => {
  it("renderiza el botón flotante Ir al inicio y el filtro Aplicar", () => {
    render(<AuditTable />);
    expect(
      screen.getByRole("button", { name: "Aplicar filtros" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Ir al inicio de la vista" }),
    ).toBeInTheDocument();
  });

  it("invoca scrollIntoView al pulsar Ir al inicio", async () => {
    const user = userEvent.setup({ delay: null });
    render(<AuditTable />);

    await user.click(
      screen.getByRole("button", { name: "Ir al inicio de la vista" }),
    );

    expect(scrollIntoViewMock).toHaveBeenCalled();
  });
});

describe("AuditTable — payload (enfoque B)", () => {
  it("el botón Payload conserva aria-expanded y alterna el detalle", async () => {
    const user = userEvent.setup({ delay: null });
    render(<AuditTable />);

    const payloadButton = screen.getByRole("button", { name: "Ver payload" });
    expect(payloadButton).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByText("Payload registrado")).not.toBeInTheDocument();

    await user.click(payloadButton);

    expect(
      screen.getByRole("button", { name: "Ocultar payload" }),
    ).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByText("Payload registrado")).toBeInTheDocument();
  });

  it("al abrir el payload hace scroll y enfoca el encabezado", async () => {
    const user = userEvent.setup({ delay: null });
    render(<AuditTable />);

    await user.click(screen.getByRole("button", { name: "Ver payload" }));

    expect(scrollIntoViewMock).toHaveBeenCalled();
    expect(screen.getByText("Payload registrado")).toHaveFocus();
  });

  it("'Volver a la fila' devuelve el foco al botón Payload que lo abrió", async () => {
    const user = userEvent.setup({ delay: null });
    render(<AuditTable />);

    const payloadButton = screen.getByRole("button", { name: "Ver payload" });
    await user.click(payloadButton);

    scrollIntoViewMock.mockClear();

    await user.click(
      screen.getByRole("button", { name: "Volver a la fila del registro" }),
    );

    expect(scrollIntoViewMock).toHaveBeenCalled();
    // El botón que abrió el detalle recupera el foco (ahora rotulado "Ocultar
    // payload" porque sigue expandido).
    expect(
      screen.getByRole("button", { name: "Ocultar payload" }),
    ).toHaveFocus();
  });
});
