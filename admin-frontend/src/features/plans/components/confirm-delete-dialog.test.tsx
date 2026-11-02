// @vitest-environment jsdom

// Pruebas de componente de `ConfirmDeleteDialog` (tarea 16.5, Req. 12.1, 12.3).
// Validan que el diálogo:
// - No se renderiza cuando `open` es false.
// - Se renderiza cuando `open` es true.
// - Invoca `onConfirm` al pulsar "Eliminar".
// - Invoca `onCancel` (sin ejecutar la eliminación) al pulsar "Cancelar",
//   presionar Escape o hacer clic en el telón (backdrop).

import "@testing-library/jest-dom/vitest";
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { ConfirmDeleteDialog } from "@/features/plans/components/confirm-delete-dialog";

afterEach(() => {
  cleanup();
});

describe("ConfirmDeleteDialog", () => {
  it("no renderiza nada cuando open es false", () => {
    render(
      <ConfirmDeleteDialog
        open={false}
        onConfirm={vi.fn()}
        onCancel={vi.fn()}
      />,
    );

    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("renderiza el diálogo cuando open es true", () => {
    render(
      <ConfirmDeleteDialog
        open
        planName="Plan Oro"
        onConfirm={vi.fn()}
        onCancel={vi.fn()}
      />,
    );

    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByText(/Plan Oro/)).toBeInTheDocument();
  });

  it("invoca onConfirm al pulsar \"Eliminar\"", async () => {
    const user = userEvent.setup({ delay: null });
    const onConfirm = vi.fn();
    const onCancel = vi.fn();

    render(
      <ConfirmDeleteDialog open onConfirm={onConfirm} onCancel={onCancel} />,
    );

    await user.click(screen.getByRole("button", { name: "Eliminar" }));

    expect(onConfirm).toHaveBeenCalledTimes(1);
    expect(onCancel).not.toHaveBeenCalled();
  });

  it("invoca onCancel al pulsar \"Cancelar\" sin confirmar", async () => {
    const user = userEvent.setup({ delay: null });
    const onConfirm = vi.fn();
    const onCancel = vi.fn();

    render(
      <ConfirmDeleteDialog open onConfirm={onConfirm} onCancel={onCancel} />,
    );

    await user.click(screen.getByRole("button", { name: "Cancelar" }));

    expect(onCancel).toHaveBeenCalledTimes(1);
    expect(onConfirm).not.toHaveBeenCalled();
  });

  it("invoca onCancel al presionar Escape", async () => {
    const user = userEvent.setup({ delay: null });
    const onConfirm = vi.fn();
    const onCancel = vi.fn();

    render(
      <ConfirmDeleteDialog open onConfirm={onConfirm} onCancel={onCancel} />,
    );

    await user.keyboard("{Escape}");

    expect(onCancel).toHaveBeenCalledTimes(1);
    expect(onConfirm).not.toHaveBeenCalled();
  });

  it("invoca onCancel al hacer clic en el telón (backdrop)", async () => {
    const user = userEvent.setup({ delay: null });
    const onConfirm = vi.fn();
    const onCancel = vi.fn();

    render(
      <ConfirmDeleteDialog open onConfirm={onConfirm} onCancel={onCancel} />,
    );

    // El telón es el contenedor con role="dialog"; un clic directo sobre él
    // (fuera de la tarjeta) dispara onCancel.
    await user.click(screen.getByRole("dialog"));

    expect(onCancel).toHaveBeenCalledTimes(1);
    expect(onConfirm).not.toHaveBeenCalled();
  });

  it("no invoca onCancel por Escape cuando está en curso (isPending)", async () => {
    const user = userEvent.setup({ delay: null });
    const onCancel = vi.fn();

    render(
      <ConfirmDeleteDialog open isPending onConfirm={vi.fn()} onCancel={onCancel} />,
    );

    await user.keyboard("{Escape}");

    expect(onCancel).not.toHaveBeenCalled();
  });
});
