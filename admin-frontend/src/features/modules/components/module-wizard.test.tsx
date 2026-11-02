// @vitest-environment jsdom

// Prueba de componente del rediseño del asistente de módulos (`ModuleWizard`).
// Verifica el NUEVO layout de pestañas de secciones en barra SUPERIOR + área de
// trabajo a todo el ancho SIN debilitar el comportamiento preservado:
//   - Navegación LIBRE entre secciones (las pestañas cambian el área).
//   - Los switches de columna conservan la invariante PK ⇒ Editable off/disabled.
//   - Guardar produce el MISMO `toDefinitionInput(state)` y llama a la mutación
//     existente (equivalencia de payload con la versión previa del stepper).

import "@testing-library/jest-dom/vitest";
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";

import { defaultColumn, emptyWizardState, toDefinitionInput, type WizardState } from "@/features/modules/schema";

// --- Mocks de dependencias del componente -----------------------------------

const createMutate = vi.fn();
const updateMutate = vi.fn();
const pushMock = vi.fn();

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: pushMock }),
}));

vi.mock("@/features/modules/hooks", () => ({
  useCreateModuleDef: () => ({ mutate: createMutate, isPending: false, isError: false }),
  useUpdateModuleDef: () => ({ mutate: updateMutate, isPending: false, isError: false }),
  // Usados por CopyConfigPicker (solo en modo creación).
  useModuleDef: () => ({ data: undefined, isFetching: false, isError: false, error: null }),
  useVisibleModules: () => ({ data: [] }),
}));

vi.mock("@/features/connections/hooks", () => ({
  useConnectionTemplates: () => ({
    data: [{ id: "tmpl-1", name: "Postgres local", fields: [] }],
  }),
}));

import { ModuleWizard } from "@/features/modules/components/module-wizard";

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

/**
 * Estado de edición VÁLIDO en todas las secciones: slug/nombre/plantilla,
 * tabla, una columna y una acción con su operación SQL. Permite ejercitar el
 * guardado sin recorrer manualmente cada campo.
 */
function validEditState(): WizardState {
  return {
    ...emptyWizardState(),
    slug: "facturas",
    name: "Facturas",
    connectionTemplateId: "tmpl-1",
    pgTable: "facturas",
    pgWhitelist: { facturas: ["id", "monto"] },
    columns: [
      { ...defaultColumn("id", 0, ""), label: "ID", isPrimaryKey: true, editable: false },
      { ...defaultColumn("monto", 1, ""), label: "Monto" },
    ],
    actions: [
      {
        actionKey: "list",
        kind: "list",
        label: "Listar",
        placement: "header",
        destructive: false,
        permAction: "ver",
        isWrite: false,
        sqlOp: "SELECT",
        inputFields: [],
      },
    ],
  };
}

describe("ModuleWizard — rediseño de secciones (fase 1)", () => {
  it("navega libremente entre secciones desde la barra lateral", () => {
    render(<ModuleWizard initial={validEditState()} editSlug="facturas" />);

    const nav = screen.getByRole("navigation", { name: "Secciones del asistente" });
    // El nombre de sección ya NO se duplica en un H2 del área de trabajo: la
    // sección activa se identifica por la PESTAÑA activa (aria-current="step").
    // La sección inicial es "Origen y conexión".
    expect(within(nav).getByRole("button", { name: /Origen y conexión/ })).toHaveAttribute(
      "aria-current",
      "step",
    );

    // Saltar directamente a "Columnas" sin pasar por un "Siguiente" forzado.
    fireEvent.click(within(nav).getByRole("button", { name: /Columnas/ }));
    expect(within(nav).getByRole("button", { name: /Columnas/ })).toHaveAttribute(
      "aria-current",
      "step",
    );

    // Y volver a "Tablas y relaciones".
    fireEvent.click(within(nav).getByRole("button", { name: /Tablas y relaciones/ }));
    expect(within(nav).getByRole("button", { name: /Tablas y relaciones/ })).toHaveAttribute(
      "aria-current",
      "step",
    );
  });

  it("marca llave primaria y fuerza Editable off + deshabilitado (invariante PK)", () => {
    render(<ModuleWizard initial={validEditState()} editSlug="facturas" />);

    const nav = screen.getByRole("navigation", { name: "Secciones del asistente" });
    fireEvent.click(within(nav).getByRole("button", { name: /Columnas/ }));

    // La columna "monto" (índice 1) empieza editable y no-PK.
    const pkMonto = screen.getByLabelText("Llave primaria", { selector: `#w_col_pk_1` });
    const editableMonto = screen.getByLabelText("Editable", { selector: `#w_col_editable_1` });
    expect(editableMonto).toHaveAttribute("aria-checked", "true");
    expect(editableMonto).not.toBeDisabled();

    // Al marcar PK, Editable pasa a off y queda deshabilitado.
    fireEvent.click(pkMonto);
    const editableAfter = screen.getByLabelText("Editable", { selector: `#w_col_editable_1` });
    expect(editableAfter).toHaveAttribute("aria-checked", "false");
    expect(editableAfter).toBeDisabled();
  });

  it("guarda enviando exactamente toDefinitionInput(state) a la mutación de edición", () => {
    const initial = validEditState();
    render(<ModuleWizard initial={initial} editSlug="facturas" />);

    fireEvent.click(screen.getByRole("button", { name: "Guardar cambios" }));

    expect(updateMutate).toHaveBeenCalledTimes(1);
    expect(updateMutate.mock.calls[0][0]).toEqual(toDefinitionInput(initial));
  });

  it("renderiza las tres zonas (cabecera + cuerpo + pie) en el primer montaje", () => {
    render(<ModuleWizard initial={validEditState()} editSlug="facturas" title="Editar módulo · Facturas" />);

    // CABECERA: título dinámico + barra de secciones.
    expect(
      screen.getByRole("heading", { name: "Editar módulo · Facturas" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("navigation", { name: "Secciones del asistente" }),
    ).toBeInTheDocument();

    // PIE: la acción primaria de guardar SIEMPRE visible desde el primer montaje.
    expect(screen.getByRole("button", { name: "Guardar cambios" })).toBeInTheDocument();
  });

  it("al guardar con una sección inválida indica cuál corregir y no llama a la mutación", () => {
    const invalid = { ...validEditState(), name: "" }; // "Origen y conexión" inválida.
    render(<ModuleWizard initial={invalid} editSlug="facturas" />);

    // Ir a otra sección para comprobar que el guardado reconduce a la inválida.
    const nav = screen.getByRole("navigation", { name: "Secciones del asistente" });
    fireEvent.click(within(nav).getByRole("button", { name: /Columnas/ }));

    fireEvent.click(screen.getByRole("button", { name: "Guardar cambios" }));

    expect(updateMutate).not.toHaveBeenCalled();
    expect(
      screen.getByText(/Revisa la sección «Origen y conexión»/),
    ).toBeInTheDocument();
    // El área de trabajo vuelve a la sección inválida: su pestaña queda activa.
    expect(within(nav).getByRole("button", { name: /Origen y conexión/ })).toHaveAttribute(
      "aria-current",
      "step",
    );
  });
});
