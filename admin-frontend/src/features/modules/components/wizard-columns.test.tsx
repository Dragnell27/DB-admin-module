// @vitest-environment jsdom

// Prueba de la sección COLUMNAS rediseñada (accordion por tabla + buscador +
// expandir/colapsar). Verifica:
//  - Multi-tabla: una cabecera de grupo por tabla (nombre mono, pill Principal/
//    Foránea, pills «N columnas» y «N PK»); principal abierta y foránea
//    colapsada por defecto.
//  - El buscador por nombre filtra las tarjetas visibles.
//  - Expandir/Colapsar todo alterna los grupos.
//  - N=1 (una sola tabla): render PLANO sin cabecera de grupo.

import "@testing-library/jest-dom/vitest";
import * as React from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";

import {
  defaultColumn,
  emptyWizardState,
  type WizardState,
} from "@/features/modules/schema";
import { ColumnsSection } from "@/features/modules/components/wizard-sections";

// Mock de los hooks de red que NO usa ColumnsSection pero sí el módulo.
vi.mock("@/features/modules/hooks", () => ({
  useModuleDef: () => ({ data: undefined, isFetching: false, isError: false, error: null }),
  useVisibleModules: () => ({ data: [] }),
}));

afterEach(() => cleanup());

/**
 * Harness controlado: mantiene el estado y aplica `patch` (merge) como lo hace
 * el asistente real, para que el efecto de sincronización de columnas no entre
 * en bucle y los cambios se reflejen.
 */
function Harness({ initial }: { initial: WizardState }) {
  const [state, setState] = React.useState(initial);
  return (
    <ColumnsSection
      state={state}
      patch={(p: Partial<WizardState>) => setState((prev) => ({ ...prev, ...p }))}
    />
  );
}

/** Estado multi-tabla: principal `facturas` (2 cols, 1 PK) + foránea `clientes`. */
function multiTableState(): WizardState {
  return {
    ...emptyWizardState(),
    pgTable: "facturas",
    foreignTables: ["clientes"],
    pgWhitelist: { facturas: ["id", "monto"], clientes: ["cliente_id", "nombre"] },
    columns: [
      { ...defaultColumn("id", 0, "facturas"), isPrimaryKey: true, editable: false },
      { ...defaultColumn("monto", 1, "facturas") },
      { ...defaultColumn("cliente_id", 2, "clientes") },
      { ...defaultColumn("nombre", 3, "clientes") },
    ],
  };
}

describe("ColumnsSection — accordion por tabla (multi-tabla)", () => {
  it("muestra una cabecera por tabla con pills de resumen", () => {
    render(<Harness initial={multiTableState()} />);

    const principal = screen.getByRole("button", { name: /facturas/ });
    expect(within(principal).getByText("Principal")).toBeInTheDocument();
    expect(within(principal).getByText("2 columnas")).toBeInTheDocument();
    expect(within(principal).getByText("1 PK")).toBeInTheDocument();

    const foranea = screen.getByRole("button", { name: /clientes/ });
    expect(within(foranea).getByText("Foránea")).toBeInTheDocument();
    expect(within(foranea).getByText("2 columnas")).toBeInTheDocument();
  });

  it("abre la principal y colapsa la foránea por defecto", () => {
    render(<Harness initial={multiTableState()} />);

    expect(screen.getByRole("button", { name: /facturas/ })).toHaveAttribute(
      "aria-expanded",
      "true",
    );
    expect(screen.getByRole("button", { name: /clientes/ })).toHaveAttribute(
      "aria-expanded",
      "false",
    );
  });

  it("expandir todo / colapsar todo alterna los grupos", () => {
    render(<Harness initial={multiTableState()} />);

    fireEvent.click(screen.getByRole("button", { name: "Expandir todo" }));
    expect(screen.getByRole("button", { name: /clientes/ })).toHaveAttribute(
      "aria-expanded",
      "true",
    );

    fireEvent.click(screen.getByRole("button", { name: "Colapsar todo" }));
    expect(screen.getByRole("button", { name: /facturas/ })).toHaveAttribute(
      "aria-expanded",
      "false",
    );
  });

  it("el buscador filtra por nombre y oculta grupos sin coincidencias", () => {
    render(<Harness initial={multiTableState()} />);

    const search = screen.getByLabelText("Buscar columna por nombre");
    fireEvent.change(search, { target: { value: "monto" } });

    // El grupo principal (con `monto`) queda; el de clientes se oculta.
    expect(screen.getByRole("button", { name: /facturas/ })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /clientes/ })).not.toBeInTheDocument();
    // Solo se ve la tarjeta de `monto` (índice global 1): su campo Etiqueta.
    expect(document.getElementById("w_col_label_1")).toHaveValue("monto");
    // La tarjeta de `id` (índice 0) queda filtrada fuera.
    expect(document.getElementById("w_col_label_0")).toBeNull();
  });
});

describe("ColumnsSection — render plano (una sola tabla)", () => {
  it("no dibuja cabecera de grupo cuando N=1", () => {
    const state: WizardState = {
      ...emptyWizardState(),
      pgTable: "facturas",
      pgWhitelist: { facturas: ["id", "monto"] },
      columns: [
        { ...defaultColumn("id", 0, "facturas"), isPrimaryKey: true, editable: false },
        { ...defaultColumn("monto", 1, "facturas") },
      ],
    };
    render(<Harness initial={state} />);

    // Sin grupos: no hay botón de cabecera de tabla ni expandir/colapsar.
    expect(screen.queryByRole("button", { name: "Expandir todo" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /facturas/ })).not.toBeInTheDocument();
    // Las dos columnas se ven planas.
    expect(screen.getByLabelText("Buscar columna por nombre")).toBeInTheDocument();
  });
});
