// @vitest-environment jsdom

// Pruebas de componente del `ToastViewport` (tarea toast-notifications). Cubren
// el comportamiento de temporizadores (auto-descarte de éxito, persistencia de
// error + cierre manual) y el enrutado a regiones vivas (status vs alert).

import "@testing-library/jest-dom/vitest";
import {
  afterEach,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from "vitest";
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";

import { ToastViewport } from "@/components/toast-viewport";
import { __resetToastsForTests, toast } from "@/lib/toast-store";

beforeEach(() => {
  __resetToastsForTests();
  vi.useFakeTimers();
});

afterEach(() => {
  vi.runOnlyPendingTimers();
  vi.useRealTimers();
  cleanup();
  __resetToastsForTests();
});

describe("ToastViewport", () => {
  it("auto-descarta un toast de éxito tras 4000ms", () => {
    render(<ToastViewport />);
    act(() => {
      toast.success("Guardado");
    });
    expect(screen.getByText("Guardado")).toBeInTheDocument();

    act(() => {
      vi.advanceTimersByTime(4000);
    });
    expect(screen.queryByText("Guardado")).not.toBeInTheDocument();
  });

  it("un toast de error PERSISTE y se cierra solo con el botón", () => {
    render(<ToastViewport />);
    act(() => {
      toast.error("Falló");
    });

    // Aun tras 10s el error sigue presente (no se auto-descarta).
    act(() => {
      vi.advanceTimersByTime(10000);
    });
    expect(screen.getByText("Falló")).toBeInTheDocument();

    // El botón de cerrar lo quita.
    act(() => {
      fireEvent.click(
        screen.getByRole("button", { name: "Cerrar notificación" }),
      );
    });
    expect(screen.queryByText("Falló")).not.toBeInTheDocument();
  });

  it("enruta éxito a la región polite y error a la región assertive", () => {
    render(<ToastViewport />);
    act(() => {
      toast.success("OK");
      toast.error("Mal");
    });

    const statusRegion = screen.getByRole("status");
    const alertRegion = screen.getByRole("alert");
    expect(within(statusRegion).getByText("OK")).toBeInTheDocument();
    expect(within(alertRegion).getByText("Mal")).toBeInTheDocument();
  });
});
