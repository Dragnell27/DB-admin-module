// @vitest-environment jsdom

// Pruebas de componente de `SubscriptionForm` tras la mejora de UX (selectores
// de búsqueda + precarga). El formulario ahora usa hooks de TanStack Query
// (usePlans/useCustomers/useCards), por lo que se mockean esos hooks y se provee
// un QueryClientProvider. Lo esencial que se valida: en modo creación, un envío
// con los campos requeridos vacíos muestra errores y NO invoca onSubmit.

import "@testing-library/jest-dom/vitest";
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

import { SubscriptionForm } from "@/features/subscriptions/components/subscription-form";

// Hooks de datos usados por los selectores: devuelven listas vacías para que el
// formulario monte sin red. Los selectores quedan vacíos (sin opciones), lo que
// es suficiente para probar el bloqueo de envío por validación.
vi.mock("@/features/plans/hooks", async (orig) => {
  const actual = await orig<typeof import("@/features/plans/hooks")>();
  return {
    ...actual,
    usePlans: () => ({ data: { items: [] }, isLoading: false }),
  };
});
vi.mock("@/features/customers/hooks", async (orig) => {
  const actual = await orig<typeof import("@/features/customers/hooks")>();
  return {
    ...actual,
    useCustomers: () => ({ data: { items: [] }, isLoading: false }),
    useCustomer: () => ({ data: undefined, isLoading: false }),
  };
});
vi.mock("@/features/cards/hooks", async (orig) => {
  const actual = await orig<typeof import("@/features/cards/hooks")>();
  return {
    ...actual,
    useCards: () => ({ data: { items: [] }, isLoading: false }),
  };
});

function renderWithClient(ui: React.ReactElement) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>{ui}</QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
});

describe("SubscriptionForm — creación: validación bloquea el envío", () => {
  it("no invoca onSubmit cuando los campos requeridos están vacíos", async () => {
    const user = userEvent.setup({ delay: null });
    const onSubmit = vi.fn();

    renderWithClient(<SubscriptionForm mode="create" onSubmit={onSubmit} />);

    await user.click(screen.getByRole("button", { name: "Crear suscripción" }));

    await waitFor(() => {
      expect(screen.getAllByRole("alert").length).toBeGreaterThan(0);
    });
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("muestra el selector de plan y el cliente seleccionable en creación", () => {
    renderWithClient(<SubscriptionForm mode="create" onSubmit={vi.fn()} />);

    // El selector de método de pago está deshabilitado hasta elegir cliente.
    expect(
      screen.getByText("Selecciona primero un cliente."),
    ).toBeInTheDocument();
  });
});

describe("SubscriptionForm — cambio: cliente fijo (no editable)", () => {
  it("muestra el cliente como solo lectura cuando hay lockedCustomerId", () => {
    renderWithClient(
      <SubscriptionForm
        mode="change"
        lockedCustomerId="643a6e57-d7bf-464e-9e43-836d48061dc0"
        onSubmit={vi.fn()}
      />,
    );

    // El campo de cliente existe como input de solo lectura (no selector).
    const customerField = screen.getByLabelText("Cliente") as HTMLInputElement;
    expect(customerField).toHaveAttribute("readonly");
  });
});
