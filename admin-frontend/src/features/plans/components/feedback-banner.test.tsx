// @vitest-environment jsdom

// Pruebas de componente del `FeedbackBanner` de Plans (tarea 16.5, Req. 10.15,
// 12.4, 12.5). Validan el render del mensaje, el tono (role="status" en éxito y
// role="alert" en error) y el botón de descarte opcional.

import "@testing-library/jest-dom/vitest";
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { FeedbackBanner } from "@/features/plans/components/feedback-banner";

afterEach(() => {
  cleanup();
});

describe("Plans FeedbackBanner", () => {
  it("usa role=status para el tono de éxito", () => {
    render(<FeedbackBanner tone="success" message="Plan creado" />);

    expect(screen.getByRole("status")).toHaveTextContent("Plan creado");
  });

  it("usa role=alert para el tono de error", () => {
    render(<FeedbackBanner tone="error" message="No se pudo eliminar" />);

    expect(screen.getByRole("alert")).toHaveTextContent("No se pudo eliminar");
  });

  it("invoca onDismiss al pulsar el botón de descarte", async () => {
    const user = userEvent.setup({ delay: null });
    const onDismiss = vi.fn();

    render(
      <FeedbackBanner
        tone="success"
        message="Plan actualizado"
        onDismiss={onDismiss}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Descartar mensaje" }));

    expect(onDismiss).toHaveBeenCalledTimes(1);
  });

  it("no muestra botón de descarte sin onDismiss", () => {
    render(<FeedbackBanner tone="error" message="Error" />);

    expect(
      screen.queryByRole("button", { name: "Descartar mensaje" }),
    ).not.toBeInTheDocument();
  });
});
