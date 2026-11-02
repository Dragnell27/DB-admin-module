// @vitest-environment jsdom

// Pruebas de componente de `CustomerForm` (tarea 16.5, Req. 7.12, 8.2).
// Validan que:
// - En modo creación, un envío inválido muestra errores de validación y NO
//   invoca `onSubmit` (Req. 7.12), mientras que un envío válido invoca
//   `onSubmit` con los valores capturados.
// - En modo edición, `external_id` y `email` se renderizan deshabilitados por
//   ser inmutables (Req. 8.2).

import "@testing-library/jest-dom/vitest";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { CustomerForm } from "@/features/customers/components/customer-form";

afterEach(() => {
  cleanup();
});

/** Rellena los campos requeridos del formulario de creación con datos válidos. */
async function fillValidCreateForm(user: ReturnType<typeof userEvent.setup>) {
  await user.type(screen.getByLabelText("External ID"), "ext-123");
  await user.type(screen.getByLabelText("Email"), "ana@example.com");
  await user.type(screen.getByLabelText("Nombre"), "Ana");
  await user.type(screen.getByLabelText("Apellido", { exact: true }), "Garcia");
  await user.type(screen.getByLabelText("Teléfono"), "+50255551234");
  // Los campos de fecha son `<input type="date">` nativos (DateField): se
  // selecciona el DÍA con fireEvent.change (userEvent.type no funciona en
  // inputs de tipo date). En modo datetime el componente compone el RFC3339.
  fireEvent.change(screen.getByLabelText("Fecha de nacimiento"), {
    target: { value: "1990-01-31" },
  });
  fireEvent.change(screen.getByLabelText("Fecha de creación externa"), {
    target: { value: "2024-01-31" },
  });
  // El `<select>` de género arranca en "" (que no es un valor de enum válido);
  // se elige una opción concreta para que el formulario sea válido.
  await user.selectOptions(screen.getByLabelText("Género (opcional)"), "M");
}

describe("CustomerForm — creación: validación bloquea el envío", () => {
  it("no invoca onSubmit y muestra errores cuando el formulario está vacío", async () => {
    const user = userEvent.setup({ delay: null });
    const onSubmit = vi.fn();

    render(<CustomerForm mode="create" onSubmit={onSubmit} />);

    await user.click(screen.getByRole("button", { name: "Crear cliente" }));

    await waitFor(() => {
      expect(screen.getAllByRole("alert").length).toBeGreaterThan(0);
    });
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("muestra error de email con formato inválido y no envía", async () => {
    const user = userEvent.setup({ delay: null });
    const onSubmit = vi.fn();

    render(<CustomerForm mode="create" onSubmit={onSubmit} />);

    await user.type(screen.getByLabelText("Email"), "no-es-un-email");
    await user.click(screen.getByRole("button", { name: "Crear cliente" }));

    await waitFor(() => {
      expect(
        screen.getByText("El email no tiene un formato válido"),
      ).toBeInTheDocument();
    });
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("invoca onSubmit con los valores cuando el formulario es válido", async () => {
    const user = userEvent.setup({ delay: null });
    const onSubmit = vi.fn();

    render(<CustomerForm mode="create" onSubmit={onSubmit} />);

    await fillValidCreateForm(user);
    await user.click(screen.getByRole("button", { name: "Crear cliente" }));

    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalledTimes(1);
    });
    const values = onSubmit.mock.calls[0][0] as Record<string, unknown>;
    expect(values).toMatchObject({
      external_id: "ext-123",
      email: "ana@example.com",
      first_name: "Ana",
      last_name: "Garcia",
      phone: "+50255551234",
      birth_date: "1990-01-31",
    });
    // En modo datetime, DateField fija el DÍA seleccionado y compone la HORA
    // con el momento de la selección (UTC). Se valida el día y el formato
    // RFC3339 (`YYYY-MM-DDTHH:MM:SSZ`), no una hora fija.
    expect(values.external_created).toMatch(
      /^2024-01-31T\d{2}:\d{2}:\d{2}Z$/,
    );
  });
});

describe("CustomerForm — edición: inmutabilidad (Req. 8.2)", () => {
  it("renderiza external_id y email deshabilitados", () => {
    render(
      <CustomerForm
        mode="edit"
        immutable={{ external_id: "ext-999", email: "fijo@example.com" }}
        onSubmit={vi.fn()}
      />,
    );

    const externalId = screen.getByLabelText(
      "External ID (no editable)",
    ) as HTMLInputElement;
    const email = screen.getByLabelText("Email (no editable)") as HTMLInputElement;

    expect(externalId).toBeDisabled();
    expect(externalId).toHaveValue("ext-999");
    expect(email).toBeDisabled();
    expect(email).toHaveValue("fijo@example.com");
  });

  it("permite editar campos mutables e invoca onSubmit", async () => {
    const user = userEvent.setup({ delay: null });
    const onSubmit = vi.fn();

    render(
      <CustomerForm
        mode="edit"
        immutable={{ external_id: "ext-999", email: "fijo@example.com" }}
        onSubmit={onSubmit}
      />,
    );

    // Los campos editables de tipo texto requerido no admiten cadena vacía:
    // se rellenan nombre y apellido con valores válidos antes de enviar.
    // Los campos editables validan su formato cuando no están vacíos; dado que
    // los `defaultValues` los inicializan en "" (no `undefined`), se rellenan
    // con valores válidos (nombre, apellido, teléfono y fecha) y se elige género
    // para que el formulario sea válido y pueda enviarse.
    await user.type(screen.getByLabelText("Nombre"), "Nuevo");
    await user.type(screen.getByLabelText("Apellido", { exact: true }), "Apellido");
    await user.type(screen.getByLabelText("Teléfono (opcional)"), "+50255551234");
    // birth_date es un `<input type="date">` (DateField en modo date).
    fireEvent.change(
      screen.getByLabelText("Fecha de nacimiento (opcional)"),
      { target: { value: "1990-01-31" } },
    );
    await user.selectOptions(screen.getByLabelText("Género (opcional)"), "F");
    await user.click(screen.getByRole("button", { name: "Guardar cambios" }));

    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalledTimes(1);
    });
    expect(onSubmit.mock.calls[0][0]).toMatchObject({
      first_name: "Nuevo",
      last_name: "Apellido",
      phone: "+50255551234",
      birth_date: "1990-01-31",
    });
  });
});
