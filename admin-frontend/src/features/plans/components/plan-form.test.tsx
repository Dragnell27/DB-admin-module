// @vitest-environment jsdom

// Pruebas de componente del `PlanForm` (FEAT-003, BLOQUE 2b). Validan el error
// inline anclado al campo que renderiza el helper `Field`: al enviar el
// formulario vacío en modo creación, el campo obligatorio "Nombre" muestra un
// <p role="alert"> con id `${htmlFor}-error`, el icono AlertCircle (aria-hidden)
// y la clase de texto accesible `text-destructive-text`.

import "@testing-library/jest-dom/vitest";
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { PlanForm } from "@/features/plans/components/plan-form";

afterEach(() => {
  cleanup();
});

describe("PlanForm — error inline del helper Field", () => {
  it("muestra el <p role=alert> con id, icono y text-destructive-text al validar un campo vacío", async () => {
    const user = userEvent.setup({ delay: null });
    const onSubmit = vi.fn();

    render(
      <PlanForm mode="create" onSubmit={onSubmit} onCancel={() => {}} />,
    );

    await user.click(screen.getByRole("button", { name: "Crear plan" }));

    // La validación falla y no se envía.
    expect(onSubmit).not.toHaveBeenCalled();

    // El error del campo obligatorio "Nombre" se ancla al campo con el id derivado
    // del htmlFor ("plan-name" → "plan-name-error"). Se busca por id porque varios
    // campos obligatorios fallan a la vez (varios role="alert").
    await waitFor(() => {
      expect(document.getElementById("plan-name-error")).not.toBeNull();
    });
    const nameError = document.getElementById("plan-name-error");
    expect(nameError).toHaveAttribute("role", "alert");
    expect(nameError).toHaveClass("text-destructive-text");
    expect(nameError?.textContent?.trim().length ?? 0).toBeGreaterThan(0);

    // El icono es decorativo (aria-hidden) y vive dentro del <p> de error.
    const icon = nameError?.querySelector("svg");
    expect(icon).not.toBeNull();
    expect(icon).toHaveAttribute("aria-hidden", "true");
  });

  it("ancla el mensaje de error al campo Nombre (mismo contenedor que su input)", async () => {
    const user = userEvent.setup({ delay: null });

    render(
      <PlanForm mode="create" onSubmit={vi.fn()} onCancel={() => {}} />,
    );

    await user.click(screen.getByRole("button", { name: "Crear plan" }));

    // El error se ancla al campo "Nombre": vive en el mismo contenedor que su input.
    await waitFor(() => {
      expect(document.getElementById("plan-name-error")).not.toBeNull();
    });
    const group = document.getElementById("plan-name-error")?.parentElement;
    expect(group).not.toBeNull();
    expect(within(group as HTMLElement).getByLabelText(/Nombre/)).toBeInTheDocument();
  });
});
