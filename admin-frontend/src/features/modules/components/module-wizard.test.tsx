// @vitest-environment jsdom

// Prueba de componente del rediseño del asistente de módulos (`ModuleWizard`).
// Verifica el modelo de ACORDEÓN de secciones apiladas (navegación por colapso)
// SIN debilitar el comportamiento preservado:
//   - Todas las secciones están presentes a la vez; cada encabezado es un
//     <button> con `aria-expanded` que expande/contrae su panel.
//   - La primera sección («Origen y conexión») arranca abierta; el resto cerrado.
//   - Los switches de columna conservan la invariante PK ⇒ Editable off/disabled.
//   - Guardar produce el MISMO `toDefinitionInput(state)` y llama a la mutación.
//   - Al guardar con una sección inválida, se AUTO-EXPANDE la primera inválida y
//     se muestra el aviso de cuál corregir.

import "@testing-library/jest-dom/vitest";
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";

import { defaultColumn, emptyWizardState, toDefinitionInput, type WizardState } from "@/features/modules/schema";
import { getPageTitle, setPageTitle } from "@/lib/page-chrome";

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
  // El título es un store en memoria: se limpia para no filtrar entre pruebas.
  setPageTitle(null);
  vi.clearAllMocks();
});

/** Encabezado (botón de toggle) de una sección por su título. */
function sectionHeader(title: RegExp) {
  return screen.getByRole("button", { name: title });
}

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

describe("ModuleWizard — acordeón de secciones (navegación por colapso)", () => {
  it("apila todas las secciones; la primera abierta y el resto cerradas", () => {
    render(<ModuleWizard initial={validEditState()} editSlug="facturas" />);

    // Los cuatro encabezados existen a la vez (acordeón, no pestañas).
    expect(sectionHeader(/Origen y conexión/)).toHaveAttribute("aria-expanded", "true");
    expect(sectionHeader(/Tablas y relaciones/)).toHaveAttribute("aria-expanded", "false");
    expect(sectionHeader(/Columnas/)).toHaveAttribute("aria-expanded", "false");
    expect(sectionHeader(/Acciones/)).toHaveAttribute("aria-expanded", "false");
  });

  it("expande y contrae una sección al pulsar su encabezado", () => {
    render(<ModuleWizard initial={validEditState()} editSlug="facturas" />);

    const columnas = sectionHeader(/Columnas/);
    // Cerrada de inicio; al pulsar se abre (no exclusivo: la primera sigue abierta).
    fireEvent.click(columnas);
    expect(sectionHeader(/Columnas/)).toHaveAttribute("aria-expanded", "true");
    expect(sectionHeader(/Origen y conexión/)).toHaveAttribute("aria-expanded", "true");

    // Pulsar de nuevo la contrae.
    fireEvent.click(sectionHeader(/Columnas/));
    expect(sectionHeader(/Columnas/)).toHaveAttribute("aria-expanded", "false");
  });

  it("marca llave primaria y fuerza Editable off + deshabilitado (invariante PK)", () => {
    render(<ModuleWizard initial={validEditState()} editSlug="facturas" />);

    // Abrir la sección Columnas para acceder a sus switches.
    fireEvent.click(sectionHeader(/Columnas/));

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

  it("declara el título en el store de la topbar y mantiene las acciones arriba", () => {
    render(<ModuleWizard initial={validEditState()} editSlug="facturas" title="Editar módulo · Facturas" />);

    // TÍTULO: ya no se pinta un <h1> en el cuerpo del asistente; se publica en
    // el store que la topbar del panel renderiza.
    expect(getPageTitle()).toBe("Editar módulo · Facturas");

    // La barra de acciones (arriba del acordeón) es visible desde el montaje.
    expect(screen.getByRole("button", { name: "Guardar cambios" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Cancelar" })).toBeInTheDocument();
  });

  it("al guardar con una sección inválida la auto-expande e indica cuál corregir", () => {
    const invalid = { ...validEditState(), name: "" }; // "Origen y conexión" inválida.
    render(<ModuleWizard initial={invalid} editSlug="facturas" />);

    // Contraer «Origen y conexión» para comprobar que el guardado la reabre.
    fireEvent.click(sectionHeader(/Origen y conexión/));
    expect(sectionHeader(/Origen y conexión/)).toHaveAttribute("aria-expanded", "false");

    fireEvent.click(screen.getByRole("button", { name: "Guardar cambios" }));

    // No se guarda y se avisa cuál sección revisar.
    expect(updateMutate).not.toHaveBeenCalled();
    expect(screen.getByText(/Revisa la sección «Origen y conexión»/)).toBeInTheDocument();
    // La sección inválida queda AUTO-EXPANDIDA.
    expect(sectionHeader(/Origen y conexión/)).toHaveAttribute("aria-expanded", "true");
  });
});
