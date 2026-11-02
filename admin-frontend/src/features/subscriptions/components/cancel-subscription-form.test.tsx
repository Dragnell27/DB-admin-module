// @vitest-environment jsdom

// Pruebas de componente de `CancelSubscriptionForm` (tarea 16.5, Req. 16.1).
// Validan que:
// - "Volver" invoca `onCancel` y nunca `onConfirm` (cancelar sin ejecutar).
// - Un customer_id inválido/vacío bloquea la confirmación (no invoca onConfirm).
// - Un customer_id UUID válido invoca `onConfirm` con el payload normalizado.

import "@testing-library/jest-dom/vitest";
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { CancelSubscriptionForm } from "@/features/subscriptions/components/cancel-subscription-form";

const UUID = "44444444-4444-4444-4444-444444444444";

afterEach(() => {
  cleanup();
});

describe("CancelSubscriptionForm", () => {
  it("invoca onCancel al pulsar \"Volver\" sin ejecutar onConfirm", async () => {
    const user = userEvent.setup({ delay: null });
    const onCancel = vi.fn();
    const onConfirm = vi.fn();

    render(
      <CancelSubscriptionForm onCancel={onCancel} onConfirm={onConfirm} />,
    );

    await user.click(screen.getByRole("button", { name: "Volver" }));

    expect(onCancel).toHaveBeenCalledTimes(1);
    expect(onConfirm).not.toHaveBeenCalled();
  });

  it("no invoca onConfirm cuando customer_id es inválido", async () => {
    const user = userEvent.setup({ delay: null });
    const onConfirm = vi.fn();

    render(
      <CancelSubscriptionForm onCancel={vi.fn()} onConfirm={onConfirm} />,
    );

    await user.type(screen.getByLabelText(/customer_id/), "no-es-uuid");
    await user.click(
      screen.getByRole("button", { name: "Confirmar cancelación" }),
    );

    await waitFor(() => {
      expect(
        screen.getByText("customer_id debe ser un UUID válido"),
      ).toBeInTheDocument();
    });
    expect(onConfirm).not.toHaveBeenCalled();
  });

  it("invoca onConfirm con el customer_id y motivo cuando es válido", async () => {
    const user = userEvent.setup({ delay: null });
    const onConfirm = vi.fn();

    render(
      <CancelSubscriptionForm onCancel={vi.fn()} onConfirm={onConfirm} />,
    );

    await user.type(screen.getByLabelText(/customer_id/), UUID);
    await user.type(screen.getByLabelText("cancel_reason"), "Ya no lo usa");
    await user.click(
      screen.getByRole("button", { name: "Confirmar cancelación" }),
    );

    await waitFor(() => {
      expect(onConfirm).toHaveBeenCalledTimes(1);
    });
    expect(onConfirm.mock.calls[0][0]).toEqual({
      customer_id: UUID,
      cancel_reason: "Ya no lo usa",
    });
  });

  // Nota de comportamiento: el schema `subscriptionCancelSchema` define
  // `cancel_reason` como `requiredString(...).optional()`; `.optional()` sólo
  // admite `undefined`, no cadena vacía. Como los `defaultValues` inicializan
  // `cancel_reason` en "", dejar el motivo en blanco dispara el error "requerido"
  // y bloquea la confirmación. Se documenta el comportamiento real tal cual.
  it("bloquea la confirmación si cancel_reason queda vacío", async () => {
    const user = userEvent.setup({ delay: null });
    const onConfirm = vi.fn();

    render(
      <CancelSubscriptionForm onCancel={vi.fn()} onConfirm={onConfirm} />,
    );

    await user.type(screen.getByLabelText(/customer_id/), UUID);
    await user.click(
      screen.getByRole("button", { name: "Confirmar cancelación" }),
    );

    await waitFor(() => {
      expect(
        screen.getByText("cancel_reason es requerido"),
      ).toBeInTheDocument();
    });
    expect(onConfirm).not.toHaveBeenCalled();
  });
});
