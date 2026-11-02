// @vitest-environment jsdom

// Pruebas de componente de `ActionFormDialog` para el campo de tipo "select"
// (module-field-select, tarea 11, capa C). Validan que:
// - El <select> renderiza las ETIQUETAS de las opciones y, al enviar, onSubmit
//   recibe el VALUE elegido (no la etiqueta).
// - Con `defaultValue` el <select> arranca preseleccionado en esa opción.
// - El placeholder "— Selecciona —" está `disabled` para un select obligatorio
//   y NO `disabled` para uno opcional sin default.
// - Con `destructive` el botón de envío es destructivo, existe la advertencia
//   de irreversibilidad y el envío (no otro acto) dispara onSubmit.

import "@testing-library/jest-dom/vitest";
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { ActionFormDialog } from "@/features/modules/components/action-form-dialog";
import type { FieldSpec, ModuleAction } from "@/features/modules/types";

afterEach(() => {
  cleanup();
});

/** Construye una acción mínima con los campos de entrada dados. */
function makeAction(
  inputFields: FieldSpec[],
  overrides: Partial<ModuleAction> = {},
): ModuleAction {
  return {
    actionKey: "do_it",
    kind: "custom",
    label: "Hacer algo",
    placement: "header",
    destructive: false,
    permAction: "custom:do_it",
    isWrite: true,
    inputFields,
    ...overrides,
  };
}

const COUNTRY_FIELD: FieldSpec = {
  name: "country",
  type: "string",
  required: true,
  source: "select",
  options: [
    { label: "Guatemala", value: "gt" },
    { label: "El Salvador", value: "sv" },
  ],
};

describe("ActionFormDialog — campo select", () => {
  it("renderiza las etiquetas de las opciones y envía el value elegido", async () => {
    const user = userEvent.setup({ delay: null });
    const onSubmit = vi.fn();

    render(
      <ActionFormDialog
        open
        action={makeAction([COUNTRY_FIELD])}
        onSubmit={onSubmit}
        onCancel={vi.fn()}
      />,
    );

    const selectEl = screen.getByLabelText(/country/) as HTMLSelectElement;
    // Las opciones muestran la ETIQUETA (no el value).
    expect(screen.getByRole("option", { name: "Guatemala" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "El Salvador" })).toBeInTheDocument();

    await user.selectOptions(selectEl, "sv");
    await user.click(screen.getByRole("button", { name: "Confirmar" }));

    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalledTimes(1);
    });
    // onSubmit recibe el VALUE de la opción elegida, no la etiqueta.
    expect(onSubmit.mock.calls[0][0]).toEqual({ country: "sv" });
  });

  it("arranca preseleccionado en la opción de defaultValue", () => {
    render(
      <ActionFormDialog
        open
        action={makeAction([{ ...COUNTRY_FIELD, defaultValue: "sv" }])}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
      />,
    );

    const selectEl = screen.getByLabelText(/country/) as HTMLSelectElement;
    expect(selectEl.value).toBe("sv");
  });

  it("deshabilita el placeholder para un select obligatorio", () => {
    render(
      <ActionFormDialog
        open
        action={makeAction([{ ...COUNTRY_FIELD, required: true }])}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
      />,
    );

    const placeholder = screen.getByRole("option", {
      name: "— Selecciona —",
    }) as HTMLOptionElement;
    expect(placeholder.disabled).toBe(true);
  });

  it("deja habilitado el placeholder para un select opcional sin default", () => {
    render(
      <ActionFormDialog
        open
        action={makeAction([
          { ...COUNTRY_FIELD, required: false, defaultValue: undefined },
        ])}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
      />,
    );

    const placeholder = screen.getByRole("option", {
      name: "— Selecciona —",
    }) as HTMLOptionElement;
    expect(placeholder.disabled).toBe(false);
  });

  it("modo columnas: readOnly deshabilitado y onSubmit solo con editables", async () => {
    const user = userEvent.setup({ delay: null });
    const onSubmit = vi.fn();

    render(
      <ActionFormDialog
        open
        action={makeAction([], { kind: "edit", label: "Editar" })}
        columnFields={[
          { field: "id", label: "ID", type: "string", readOnly: true, required: true },
          { field: "monto", label: "Monto", type: "number", readOnly: false, required: false },
        ]}
        initialValues={{ id: 42, monto: 100 }}
        onSubmit={onSubmit}
        onCancel={vi.fn()}
      />,
    );

    // La columna PK (readOnly) se muestra precargada y deshabilitada.
    const idInput = screen.getByLabelText(/ID/) as HTMLInputElement;
    expect(idInput.value).toBe("42");
    expect(idInput.disabled).toBe(true);

    // La columna editable es modificable.
    const montoInput = screen.getByLabelText(/Monto/) as HTMLInputElement;
    expect(montoInput.disabled).toBe(false);
    await user.clear(montoInput);
    await user.type(montoInput, "250");

    await user.click(screen.getByRole("button", { name: "Confirmar" }));
    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalledTimes(1);
    });
    // Solo viaja la columna editable; la PK readOnly (id) la reañade el padre.
    expect(onSubmit.mock.calls[0][0]).toEqual({ monto: 250 });
  });

  it("modo columnas: una columna objetivo con options se renderiza como desplegable", async () => {
    const user = userEvent.setup({ delay: null });
    const onSubmit = vi.fn();

    render(
      <ActionFormDialog
        open
        action={makeAction([], { kind: "edit", label: "Cambiar estado" })}
        columnFields={[
          { field: "id", label: "ID", type: "string", readOnly: true, required: true },
          {
            field: "estado",
            label: "Estado",
            type: "string",
            readOnly: false,
            required: true,
            options: [
              { label: "Activo", value: "1" },
              { label: "Inactivo", value: "2" },
            ],
            defaultValue: "1",
          },
        ]}
        initialValues={{ id: 7 }}
        onSubmit={onSubmit}
        onCancel={vi.fn()}
      />,
    );

    const estadoSelect = screen.getByLabelText(/Estado/) as HTMLSelectElement;
    expect(estadoSelect.tagName).toBe("SELECT");
    // Arranca en la preselección.
    expect(estadoSelect.value).toBe("1");
    expect(screen.getByRole("option", { name: "Inactivo" })).toBeInTheDocument();

    await user.selectOptions(estadoSelect, "2");
    await user.click(screen.getByRole("button", { name: "Confirmar" }));
    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalledTimes(1);
    });
    // El value elegido viaja como string (coherente con el backend).
    expect(onSubmit.mock.calls[0][0]).toEqual({ estado: "2" });
  });

  it("con destructive muestra la advertencia de irreversibilidad y envía al confirmar", async () => {
    const user = userEvent.setup({ delay: null });
    const onSubmit = vi.fn();

    render(
      <ActionFormDialog
        open
        destructive
        action={makeAction([{ ...COUNTRY_FIELD, defaultValue: "gt" }], {
          destructive: true,
        })}
        onSubmit={onSubmit}
        onCancel={vi.fn()}
      />,
    );

    // Advertencia textual de irreversibilidad (paridad con ConfirmActionDialog).
    expect(
      screen.getByText(
        "¿Seguro que deseas ejecutar esta acción? Esta operación puede no ser reversible.",
      ),
    ).toBeInTheDocument();

    // El botón de envío es destructivo (variant="destructive").
    const submitBtn = screen.getByRole("button", { name: "Confirmar" });
    expect(submitBtn.className).toMatch(/destructive/);

    // Solo el envío (no otro acto) dispara onSubmit.
    expect(onSubmit).not.toHaveBeenCalled();
    await user.click(submitBtn);

    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalledTimes(1);
    });
    expect(onSubmit.mock.calls[0][0]).toEqual({ country: "gt" });
  });
});

describe("ActionFormDialog — campo dynamic (lista cargada)", () => {
  /** Campo dinámico con sus opciones ya INYECTADAS por el padre (module-view). */
  const PLAN_FIELD: FieldSpec = {
    name: "plan_id",
    type: "number",
    required: true,
    source: "dynamic",
    dynamicSource: { table: "planes", idColumn: "id", labelColumn: "nombre" },
    options: [
      { label: "Básico", value: "1" },
      { label: "Premium", value: "2" },
    ],
  };

  it("renderiza las etiquetas cargadas y envía el id (value) VERBATIM, no la etiqueta", async () => {
    const user = userEvent.setup({ delay: null });
    const onSubmit = vi.fn();

    render(
      <ActionFormDialog
        open
        action={makeAction([PLAN_FIELD])}
        onSubmit={onSubmit}
        onCancel={vi.fn()}
      />,
    );

    const selectEl = screen.getByLabelText(/plan_id/) as HTMLSelectElement;
    expect(selectEl.tagName).toBe("SELECT");
    expect(screen.getByRole("option", { name: "Básico" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "Premium" })).toBeInTheDocument();

    await user.selectOptions(selectEl, "2");
    await user.click(screen.getByRole("button", { name: "Confirmar" }));

    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalledTimes(1);
    });
    // El id viaja como string "2" (no coaccionado a número por type:"number"),
    // y nunca la etiqueta "Premium".
    expect(onSubmit.mock.calls[0][0]).toEqual({ plan_id: "2" });
  });

  it("id COLGANTE: abre preseleccionado en la opción de respaldo con el id crudo", () => {
    render(
      <ActionFormDialog
        open
        action={makeAction([
          {
            ...PLAN_FIELD,
            // El padre PREPENDE la opción de respaldo {value,label}=String(rawId)
            // cuando el id de la fila no está en el catálogo cargado.
            options: [
              { label: "99", value: "99" },
              { label: "Básico", value: "1" },
              { label: "Premium", value: "2" },
            ],
          },
        ])}
        initialValues={{ plan_id: 99 }}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
      />,
    );

    const selectEl = screen.getByLabelText(/plan_id/) as HTMLSelectElement;
    // Preseleccionado en el id crudo (comparado como string), sin romper.
    expect(selectEl.value).toBe("99");
    expect(screen.getByRole("option", { name: "99" })).toBeInTheDocument();
  });

  it("optionsLoading deshabilita el desplegable y muestra 'Cargando…'", () => {
    render(
      <ActionFormDialog
        open
        action={makeAction([{ ...PLAN_FIELD, options: [], optionsLoading: true }])}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
      />,
    );

    const selectEl = screen.getByLabelText(/plan_id/) as HTMLSelectElement;
    expect(selectEl.disabled).toBe(true);
    expect(screen.getByRole("option", { name: "Cargando…" })).toBeInTheDocument();
  });

  it("optionsError muestra el aviso de carga fallida", () => {
    render(
      <ActionFormDialog
        open
        action={makeAction([{ ...PLAN_FIELD, options: [], optionsError: true }])}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
      />,
    );

    expect(
      screen.getByText("No se pudieron cargar las opciones"),
    ).toBeInTheDocument();
  });
});
