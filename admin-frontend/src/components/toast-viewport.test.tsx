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
      toast.success("Módulo guardado");
    });
    expect(screen.getByText("Módulo guardado")).toBeInTheDocument();

    act(() => {
      vi.advanceTimersByTime(4000);
    });
    expect(screen.queryByText("Módulo guardado")).not.toBeInTheDocument();
  });

  it("renderiza título y descripción cuando el toast trae ambos", () => {
    render(<ToastViewport />);
    act(() => {
      toast.success({ title: "Hecho", message: "Rol creado" });
    });
    expect(screen.getByText("Hecho")).toBeInTheDocument();
    expect(screen.getByText("Rol creado")).toBeInTheDocument();
  });

  it("pinta el enlace 'Ir a …' solo cuando hay `where` y ejecuta su onClick", () => {
    const onClick = vi.fn();
    render(<ToastViewport />);
    act(() => {
      toast.error({
        message: "Revisa el formulario",
        where: { label: "Nombre", onClick },
      });
    });

    const link = screen.getByRole("button", { name: "Ir a «Nombre»" });
    expect(link).toBeInTheDocument();
    fireEvent.click(link);
    expect(onClick).toHaveBeenCalledTimes(1);
  });

  it("no pinta el enlace 'Ir a …' cuando el toast no trae `where`", () => {
    render(<ToastViewport />);
    act(() => {
      toast.error("Sin enlace");
    });
    expect(
      screen.queryByRole("button", { name: /^Ir a /u }),
    ).not.toBeInTheDocument();
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
