// @vitest-environment jsdom

// Pruebas de componente de `CardForm` (tarea 16.5, Req. 18.2-18.7, 19.1).
// `CardForm` usa internamente el hook `useCreateCard` (TanStack Query); para
// determinismo y evitar red, se mockea el módulo de hooks: la mutación expone
// un `mutate` espía. Así podemos verificar que:
// - Un envío inválido muestra errores y NO llama a `mutate` (Req. 18.2-18.7).
// - Un envío válido llama a `mutate` con los valores capturados.

import "@testing-library/jest-dom/vitest";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

// Espía de la mutación. Se define antes del vi.mock (hoisted) usando vi.hoisted.
const { mutateMock } = vi.hoisted(() => ({ mutateMock: vi.fn() }));

vi.mock("@/features/cards/hooks", () => ({
  useCreateCard: () => ({ mutate: mutateMock, isPending: false }),
}));

import { CardForm } from "@/app/(protected)/cards/components/card-form";

const UUID_CUSTOMER = "55555555-5555-5555-5555-555555555555";
const UUID_TOKEN = "66666666-6666-6666-6666-666666666666";

beforeEach(() => {
  mutateMock.mockReset();
});

afterEach(() => {
  cleanup();
});

describe("CardForm", () => {
  it("no llama a mutate y muestra errores cuando los campos son inválidos", async () => {
    const user = userEvent.setup({ delay: null });

    render(<CardForm />);

    await user.click(screen.getByRole("button", { name: "Crear tarjeta" }));

    await waitFor(() => {
      expect(screen.getAllByRole("alert").length).toBeGreaterThan(0);
    });
    expect(mutateMock).not.toHaveBeenCalled();
  });

  it("muestra error de UUID inválido en customer_id y no envía", async () => {
    const user = userEvent.setup({ delay: null });

    render(<CardForm />);

    await user.type(screen.getByLabelText("ID de cliente"), "no-es-uuid");
    await user.type(screen.getByLabelText("Número de tarjeta"), "4111111111111111");
    await user.type(screen.getByLabelText("Expiración (MM/YY)"), "12/30");
    await user.type(screen.getByLabelText("Token de tarjeta"), UUID_TOKEN);
    await user.type(screen.getByLabelText("Método de pago"), "Visa débito");
    await user.type(screen.getByLabelText("Franquicia"), "VISA");

    await user.click(screen.getByRole("button", { name: "Crear tarjeta" }));

    await waitFor(() => {
      expect(
        screen.getByText("customer_id debe ser un UUID válido"),
      ).toBeInTheDocument();
    });
    expect(mutateMock).not.toHaveBeenCalled();
  });

  it("llama a mutate con los valores cuando el formulario es válido", async () => {
    const user = userEvent.setup({ delay: null });

    render(<CardForm defaultCustomerId={UUID_CUSTOMER} />);

    await user.type(screen.getByLabelText("Número de tarjeta"), "4111111111111111");
    await user.type(screen.getByLabelText("Expiración (MM/YY)"), "12/30");
    await user.type(screen.getByLabelText("Token de tarjeta"), UUID_TOKEN);
    await user.type(screen.getByLabelText("Método de pago"), "Visa débito");
    await user.type(screen.getByLabelText("Franquicia"), "VISA");

    await user.click(screen.getByRole("button", { name: "Crear tarjeta" }));

    await waitFor(() => {
      expect(mutateMock).toHaveBeenCalledTimes(1);
    });
    expect(mutateMock.mock.calls[0][0]).toMatchObject({
      customer_id: UUID_CUSTOMER,
      card_number: "4111111111111111",
      expiration_date: "12/30",
      card_token: UUID_TOKEN,
      payment_method_name: "Visa débito",
      card_franchise: "VISA",
    });
  });

  it("invoca onCancel al pulsar \"Cancelar\"", async () => {
    const user = userEvent.setup({ delay: null });
    const onCancel = vi.fn();

    render(<CardForm onCancel={onCancel} />);

    await user.click(screen.getByRole("button", { name: "Cancelar" }));

    expect(onCancel).toHaveBeenCalledTimes(1);
    expect(mutateMock).not.toHaveBeenCalled();
  });
});
