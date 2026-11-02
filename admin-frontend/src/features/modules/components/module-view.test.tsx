// @vitest-environment jsdom

// Prueba de componente de `ModuleView`: cuando la definición del módulo NO se
// puede cargar (defError/!def, p. ej. 403 por permisos obsoletos), el estado de
// error debe seguir mostrando el control "Recargar permisos" junto al banner,
// porque es precisamente el botón que permite recuperar permisos obsoletos y
// debe quedar alcanzable desde esa pantalla.

import "@testing-library/jest-dom/vitest";
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";

// --- Mocks de dependencias del componente -----------------------------------

const useModuleDefMock = vi.fn();
const useModuleListMock = vi.fn();
const useModuleActionListMock = vi.fn();
const useModuleActionMock = vi.fn();
const useModuleFieldOptionsMock = vi.fn();
const useRefreshPermissionsMock = vi.fn();

vi.mock("@/features/modules/hooks", () => ({
  useModuleDef: (...args: unknown[]) => useModuleDefMock(...args),
  useModuleList: (...args: unknown[]) => useModuleListMock(...args),
  useModuleActionList: (...args: unknown[]) => useModuleActionListMock(...args),
  useModuleAction: (...args: unknown[]) => useModuleActionMock(...args),
  useModuleFieldOptions: (...args: unknown[]) => useModuleFieldOptionsMock(...args),
  useRefreshPermissions: (...args: unknown[]) => useRefreshPermissionsMock(...args),
}));

vi.mock("@/lib/use-permissions", () => ({
  useModulePermissions: () => [],
}));

vi.mock("@/lib/country", () => ({
  getCountry: () => "gt",
  subscribeCountry: () => () => {},
}));

// Se importa DESPUÉS de declarar los mocks para que el componente los use.
import { ModuleView } from "@/features/modules/components/module-view";

// --- Helpers -----------------------------------------------------------------

/** Configura los mocks de los hooks con un estado base y overrides. */
function setupHooks(overrides?: { defError?: boolean; def?: unknown }) {
  useModuleDefMock.mockReturnValue({
    data: overrides?.def,
    isLoading: false,
    isError: overrides?.defError ?? false,
  });
  useModuleListMock.mockReturnValue({
    data: undefined,
    isLoading: false,
    isError: false,
    error: null,
  });
  useModuleActionListMock.mockReturnValue({
    data: undefined,
    isLoading: false,
    isError: false,
    error: null,
  });
  useModuleFieldOptionsMock.mockReturnValue({
    data: [],
    isLoading: false,
    isError: false,
  });
  useModuleActionMock.mockReturnValue({
    isPending: false,
    isError: false,
    error: null,
    mutate: vi.fn(),
    reset: vi.fn(),
  });
  useRefreshPermissionsMock.mockReturnValue({
    isPending: false,
    mutate: vi.fn(),
  });
}

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("ModuleView — estado de error de la definición", () => {
  it("muestra 'Recargar permisos' junto al banner cuando la definición falla (defError)", () => {
    setupHooks({ defError: true });
    render(<ModuleView slug="afiliaciones" />);

    expect(
      screen.getByText("No se pudo cargar la definición del módulo."),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Recargar permisos" }),
    ).toBeInTheDocument();
  });

  it("muestra 'Recargar permisos' cuando la definición viene vacía (!def)", () => {
    setupHooks({ defError: false, def: undefined });
    render(<ModuleView slug="afiliaciones" />);

    expect(
      screen.getByText("No se pudo cargar la definición del módulo."),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Recargar permisos" }),
    ).toBeInTheDocument();
  });
});

/** Definición mínima con una columna visible (sin acciones). */
function simpleDef() {
  return {
    slug: "afiliaciones",
    name: "Afiliaciones",
    sourceType: "postgres",
    countryScope: "none",
    fetchMapping: { table: "afiliaciones" },
    displayMapping: {
      columns: [{ field: "nombre", label: "Nombre", order: 0 }],
    },
    listParams: {},
    dataSource: { sourceType: "postgres", connectionTemplateId: "t", hasSecrets: false },
    actions: [],
  };
}

/**
 * Definición con una columna NO dinámica (nombre) y una columna DINÁMICA
 * (forma_pago) respaldada por un FieldSpec dinámico de la acción "crear".
 */
function dynamicDef() {
  return {
    slug: "planes",
    name: "Planes",
    sourceType: "postgres",
    countryScope: "none",
    fetchMapping: { table: "planes" },
    displayMapping: {
      columns: [
        { field: "nombre", label: "Nombre", order: 0 },
        { field: "forma_pago", label: "Forma de pago", order: 1 },
      ],
    },
    listParams: {},
    dataSource: { sourceType: "postgres", connectionTemplateId: "t", hasSecrets: false },
    actions: [
      {
        actionKey: "crear",
        kind: "create",
        label: "Crear",
        placement: "header",
        destructive: false,
        permAction: "crear",
        isWrite: true,
        inputFields: [
          {
            name: "forma_pago",
            type: "string",
            required: true,
            source: "dynamic",
            dynamicSource: { table: "formas_pago", idColumn: "id", labelColumn: "nombre" },
          },
        ],
      },
    ],
  };
}

describe("ModuleView — resolución id→etiqueta en columnas dinámicas del grid", () => {
  it("renderiza la ETIQUETA para un id conocido de una columna dinámica", () => {
    setupHooks({ def: dynamicDef() });
    useModuleListMock.mockReturnValue({
      data: {
        items: [{ nombre: "Plan Oro", forma_pago: 3 }],
        pagination: { totalRecords: 1, page: 1, recordsPerPage: 25, totalPages: 1 },
      },
      isLoading: false,
      isError: false,
      error: null,
    });
    // El fetcher de la columna dinámica recibe estas opciones (misma caché que
    // crear/editar). id 3 → "Tarjeta".
    useModuleFieldOptionsMock.mockReturnValue({
      data: [{ id: 3, label: "Tarjeta" }],
      isLoading: false,
      isError: false,
    });

    render(<ModuleView slug="planes" />);

    // La celda dinámica muestra la etiqueta, no el id crudo "3".
    expect(screen.getByText("Tarjeta")).toBeInTheDocument();
    expect(screen.queryByText("3")).not.toBeInTheDocument();
    // La columna NO dinámica se mantiene igual.
    expect(screen.getByText("Plan Oro")).toBeInTheDocument();
  });

  it("FALLBACK: muestra el id crudo cuando el id no tiene opción (colgante)", () => {
    setupHooks({ def: dynamicDef() });
    useModuleListMock.mockReturnValue({
      data: {
        items: [{ nombre: "Plan Oro", forma_pago: 99 }],
        pagination: { totalRecords: 1, page: 1, recordsPerPage: 25, totalPages: 1 },
      },
      isLoading: false,
      isError: false,
      error: null,
    });
    // id 99 NO está en las opciones ⇒ se muestra el id crudo.
    useModuleFieldOptionsMock.mockReturnValue({
      data: [{ id: 3, label: "Tarjeta" }],
      isLoading: false,
      isError: false,
    });

    render(<ModuleView slug="planes" />);

    expect(screen.getByText("99")).toBeInTheDocument();
    expect(screen.queryByText("Tarjeta")).not.toBeInTheDocument();
  });

  it("una columna NO dinámica se renderiza sin cambios", () => {
    setupHooks({ def: dynamicDef() });
    useModuleListMock.mockReturnValue({
      data: {
        items: [{ nombre: "Plan Plata", forma_pago: 3 }],
        pagination: { totalRecords: 1, page: 1, recordsPerPage: 25, totalPages: 1 },
      },
      isLoading: false,
      isError: false,
      error: null,
    });
    useModuleFieldOptionsMock.mockReturnValue({
      data: [{ id: 3, label: "Tarjeta" }],
      isLoading: false,
      isError: false,
    });

    render(<ModuleView slug="planes" />);

    expect(screen.getByText("Plan Plata")).toBeInTheDocument();
  });

  it("sin columnas dinámicas NO se invoca useModuleFieldOptions (cero fetches)", () => {
    setupHooks({ def: simpleDef() });
    useModuleListMock.mockReturnValue({
      data: {
        items: [{ nombre: "Ana" }],
        pagination: { totalRecords: 1, page: 1, recordsPerPage: 25, totalPages: 1 },
      },
      isLoading: false,
      isError: false,
      error: null,
    });

    render(<ModuleView slug="afiliaciones" />);

    expect(screen.getByText("Ana")).toBeInTheDocument();
    // Ningún ColumnLabelFetcher montado ⇒ el hook de opciones no se invoca.
    expect(useModuleFieldOptionsMock).not.toHaveBeenCalled();
  });
});

describe("ModuleView — listado base vs filtro de acción SELECT", () => {
  it("sin filtro activo, el grid se alimenta del listado base y la ruta de acción está deshabilitada", () => {
    setupHooks({ def: simpleDef() });
    useModuleListMock.mockReturnValue({
      data: { items: [{ nombre: "Ana" }], pagination: { totalRecords: 1, page: 1, recordsPerPage: 25, totalPages: 1 } },
      isLoading: false,
      isError: false,
      error: null,
    });

    render(<ModuleView slug="afiliaciones" />);

    // La fila del listado base se renderiza.
    expect(screen.getByText("Ana")).toBeInTheDocument();
    // Sin filtro activo no aparece el botón "Limpiar filtro".
    expect(
      screen.queryByRole("button", { name: "Limpiar filtro" }),
    ).not.toBeInTheDocument();
    // La ruta dedicada de acción se invocó deshabilitada (no hay filtro activo).
    const actionListCall = useModuleActionListMock.mock.calls.at(-1);
    expect(actionListCall?.[4]).toMatchObject({ enabled: false });
  });
});
