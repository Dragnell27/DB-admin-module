// @vitest-environment jsdom
// Pruebas de componente del botón de alternancia de tema (Entrega A). Mockean
// `@/lib/use-theme` (para controlar el estado del tema) y `@/lib/theme` (para
// espiar `toggleTheme`), igual que `app-sidebar.test.tsx` mockea sus hooks.
// Verifican el aria-label dinámico según el estado, la presencia del ícono
// sol/luna y que el clic invoque `toggleTheme`.

import "@testing-library/jest-dom/vitest";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";

import type { Theme } from "@/lib/theme";

const useThemeMock = vi.fn<() => Theme>();

vi.mock("@/lib/use-theme", () => ({
  useTheme: () => useThemeMock(),
}));

const toggleThemeMock = vi.fn();

vi.mock("@/lib/theme", () => ({
  toggleTheme: () => toggleThemeMock(),
}));

// Se importa DESPUÉS de declarar los mocks para que el componente los use.
import { ThemeToggle } from "@/components/theme-toggle";

describe("ThemeToggle", () => {
  beforeEach(() => {
    useThemeMock.mockReset();
    toggleThemeMock.mockReset();
  });
  afterEach(() => {
    cleanup();
  });

  it("en modo OSCURO muestra el ícono de sol y el aria-label para pasar a claro", () => {
    useThemeMock.mockReturnValue("dark");
    render(<ThemeToggle />);

    const button = screen.getByRole("button", {
      name: "Cambiar a modo claro",
    });
    expect(button).toBeInTheDocument();
    expect(button).toHaveAttribute("title", "Cambiar a modo claro");
    // El ícono sol de lucide-react expone la clase `lucide-sun`.
    expect(button.querySelector(".lucide-sun")).toBeTruthy();
  });

  it("en modo CLARO muestra el ícono de luna y el aria-label para pasar a oscuro", () => {
    useThemeMock.mockReturnValue("light");
    render(<ThemeToggle />);

    const button = screen.getByRole("button", {
      name: "Cambiar a modo oscuro",
    });
    expect(button).toBeInTheDocument();
    expect(button.querySelector(".lucide-moon")).toBeTruthy();
  });

  it("al hacer clic invoca toggleTheme", () => {
    useThemeMock.mockReturnValue("dark");
    render(<ThemeToggle />);

    fireEvent.click(screen.getByRole("button"));
    expect(toggleThemeMock).toHaveBeenCalledTimes(1);
  });
});
