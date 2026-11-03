// @vitest-environment jsdom

// Bug #2 (regresión de no-reset): cambiar la plantilla de conexión NO debe
// vaciar `uniqueness.columns` de una acción create. Reproduce el flujo real del
// informe: fijar unicidad → ir a "Origen y conexión" → cambiar la plantilla →
// volver a "Acciones", y asertar tanto el valor VISIBLE del input como la fuente
// de verdad (`toDefinitionInput(state)` vía el payload de guardar).

import "@testing-library/jest-dom/vitest";
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";

import {
  defaultColumn,
  emptyUniquenessFormState,
  emptyWizardState,
  type WizardState,
} from "@/features/modules/schema";
import { setPageTitle } from "@/lib/page-chrome";

const updateMutate = vi.fn();

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn() }),
}));

vi.mock("@/features/modules/hooks", () => ({
  useCreateModuleDef: () => ({ mutate: vi.fn(), isPending: false, isError: false }),
  useUpdateModuleDef: () => ({ mutate: updateMutate, isPending: false, isError: false }),
  useModuleDef: () => ({ data: undefined, isFetching: false, isError: false, error: null }),
  useVisibleModules: () => ({ data: [] }),
}));

// Dos plantillas para poder CAMBIAR de una a otra en el <select>.
vi.mock("@/features/connections/hooks", () => ({
  useConnectionTemplates: () => ({
    data: [
      { id: "tmpl-1", name: "Postgres local", fields: [] },
      { id: "tmpl-2", name: "Postgres super-app", fields: [] },
    ],
  }),
}));

import { ModuleWizard } from "@/features/modules/components/module-wizard";

afterEach(() => {
  cleanup();
  setPageTitle(null);
  vi.clearAllMocks();
});

const UNIQ_COLUMNS = ["padr_numero", "valor_lista", "valor_objeto"];

/**
 * Estado de edición multi-tabla con una acción create cuyo `uniqueness.columns`
 * ya trae las 3 columnas del caso real.
 */
function multiTableCreateState(): WizardState {
  return {
    ...emptyWizardState(),
    slug: "qa-medios-pago-x-comercio",
    name: "Medios de pago por comercio",
    connectionTemplateId: "tmpl-1",
    pgTable: "gt_valores",
    pgWhitelist: {
      gt_valores: ["valo_numero"],
      gt_valores_hijo: ["padr_numero", "valor_lista", "valor_objeto"],
    },
    foreignTables: ["gt_valores_hijo"],
    columns: [{ ...defaultColumn("valo_numero", 0, "gt_valores"), isPrimaryKey: true, editable: false }],
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
        uniqueness: emptyUniquenessFormState(),
      },
      {
        actionKey: "crear",
        kind: "create",
        label: "Crear",
        placement: "header",
        destructive: false,
        permAction: "crear",
        isWrite: true,
        sqlOp: "INSERT",
        inputFields: [],
        targetTable: "gt_valores_hijo",
        uniqueness: { columns: [...UNIQ_COLUMNS], message: "Ya existe este medio de pago para el comercio" },
      },
    ],
  };
}

describe("Bug #2 — cambiar la plantilla no resetea uniqueness.columns", () => {
  it("conserva las columnas de unicidad (visible + estado) tras cambiar la plantilla", () => {
    render(<ModuleWizard initial={multiTableCreateState()} editSlug="qa-medios-pago-x-comercio" />);

    const nav = screen.getByRole("navigation", { name: "Secciones del asistente" });

    // 1) Ir a "Acciones" y confirmar que el input muestra las 3 columnas.
    fireEvent.click(within(nav).getByRole("button", { name: /Acciones/ }));
    const inputBefore = screen.getByPlaceholderText("p. ej. padr_numero, depe_numero") as HTMLInputElement;
    expect(inputBefore.value).toBe(UNIQ_COLUMNS.join(", "));

    // 2) Ir a "Origen y conexión" y cambiar la plantilla.
    fireEvent.click(within(nav).getByRole("button", { name: /Origen y conexión/ }));
    const templateSelect = screen.getByLabelText("Plantilla de conexión") as HTMLSelectElement;
    fireEvent.change(templateSelect, { target: { value: "tmpl-2" } });
    expect(templateSelect.value).toBe("tmpl-2");

    // 3) Volver a "Acciones": el input sigue mostrando las 3 columnas (resync).
    fireEvent.click(within(nav).getByRole("button", { name: /Acciones/ }));
    const inputAfter = screen.getByPlaceholderText("p. ej. padr_numero, depe_numero") as HTMLInputElement;
    expect(inputAfter.value).toBe(UNIQ_COLUMNS.join(", "));

    // 4) Aserción de ESTADO (fuente de verdad del reducer): el payload de guardar
    // conserva exactamente las 3 columnas y ahora apunta a la nueva plantilla.
    fireEvent.click(screen.getByRole("button", { name: "Guardar cambios" }));
    expect(updateMutate).toHaveBeenCalledTimes(1);
    const payload = updateMutate.mock.calls[0][0];
    expect(payload.dataSource.connectionTemplateId).toBe("tmpl-2");
    const createAction = payload.actions.find((a: { kind: string }) => a.kind === "create");
    expect(createAction.uniqueness.columns).toEqual(UNIQ_COLUMNS);
  });
});
