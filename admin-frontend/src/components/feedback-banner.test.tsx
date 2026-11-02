// @vitest-environment jsdom

// Pruebas de componente del `FeedbackBanner` de Customers (tarea 16.5,
// Req. 7.12). Validan el render del mensaje, el tono (role="alert") y el botón
// de descarte opcional.

import "@testing-library/jest-dom/vitest";
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { FeedbackBanner } from "@/components/feedback-banner";

afterEach(() => {
  cleanup();
});

describe("Customers FeedbackBanner", () => {
  it("renderiza el mensaje de éxito con role=alert", () => {
    render(<FeedbackBanner tone="success" message="Cliente creado" />);

    const banner = screen.getByRole("alert");
    expect(banner).toHaveTextContent("Cliente creado");
  });

  it("renderiza el mensaje de error (p. ej. del OTT)", () => {
    render(<FeedbackBanner tone="error" message="Email ya registrado" />);

    expect(screen.getByRole("alert")).toHaveTextContent("Email ya registrado");
  });

  it("aplica el borde izquierdo rojo grueso en la variante error", () => {
    render(<FeedbackBanner tone="error" message="Email ya registrado" />);

    const banner = screen.getByRole("alert");
    expect(banner).toHaveClass("border-l-4", "border-l-destructive");
  });

  it("renderiza el mensaje de advertencia (tone='warning')", () => {
    render(
      <FeedbackBanner tone="warning" message="No se pudieron cargar los módulos" />,
    );

    const banner = screen.getByRole("alert");
    expect(banner).toHaveTextContent("No se pudieron cargar los módulos");
    expect(banner.className).toContain("text-warn-text");
  });

  it("no muestra el botón de cierre si no se provee onDismiss", () => {
    render(<FeedbackBanner tone="success" message="Listo" />);

    expect(
      screen.queryByRole("button", { name: "Cerrar notificación" }),
    ).not.toBeInTheDocument();
  });

  it("invoca onDismiss al pulsar el botón de cierre", async () => {
    const user = userEvent.setup({ delay: null });
    const onDismiss = vi.fn();

    render(
      <FeedbackBanner tone="error" message="Error" onDismiss={onDismiss} />,
    );

    await user.click(
      screen.getByRole("button", { name: "Cerrar notificación" }),
    );

    expect(onDismiss).toHaveBeenCalledTimes(1);
  });
});
