// @vitest-environment jsdom

// Prueba de componente del editor de relaciones (RelationshipsEditor, dentro de
// TablesSection). Verifica que se pueden añadir condiciones constantes del ON
// (tabla.columna = valor) y que el estado del asistente las refleja en
// state.relationships[].onConstants, listo para toDefinitionInput.

import "@testing-library/jest-dom/vitest";
import * as React from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";

import {
  emptyWizardState,
  type RelationshipFormState,
  type WizardState,
} from "@/features/modules/schema";

// TablesSection usa estos hooks (vía otros editores); se neutralizan.
vi.mock("@/features/modules/hooks", () => ({
  useModuleDef: () => ({ data: undefined, isFetching: false, isError: false, error: null }),
  useVisibleModules: () => ({ data: [] }),
}));

import { TablesSection } from "@/features/modules/components/wizard-sections";

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

/** Estado multi-tabla con una relación pagos ← clientes y ON sin constantes. */
function multiState(): WizardState {
  return {
    ...emptyWizardState(),
    pgTable: "pagos",
    foreignTables: ["clientes"],
    pgWhitelist: { pagos: ["id", "cliente_id"], clientes: ["id", "estado"] },
    relationships: [
      {
        childTable: "pagos",
        parentTable: "clientes",
        on: [{ child: "cliente_id", parent: "id" }],
        onConstants: [],
        joinType: "left",
      },
    ],
  };
}

/**
 * Harness que mantiene el estado y aplica los `patch` del editor, para observar
 * el efecto real sobre state.relationships.
 */
function Harness({ initial }: { initial: WizardState }) {
  const [state, setState] = React.useState(initial);
  const patch = (p: Partial<WizardState>) => setState((s) => ({ ...s, ...p }));
  return (
    <>
      <TablesSection state={state} patch={patch} />
      <output data-testid="rels">{JSON.stringify(state.relationships)}</output>
    </>
  );
}

/**
 * Expande la fila de relación (en el rediseño por colapso arranca COLAPSADA al
 * tener tablas) pulsando su encabezado, identificado por el resumen
 * «pagos → clientes · LEFT».
 */
function expandRelationshipRow() {
  fireEvent.click(screen.getByRole("button", { name: /pagos → clientes/ }));
}

describe("RelationshipsEditor — condiciones constantes del ON", () => {
  it("añade una condición constante y la refleja en onConstants", () => {
    render(<Harness initial={multiState()} />);
    expandRelationshipRow();

    // Añade una fila de condición constante.
    fireEvent.click(screen.getByRole("button", { name: "Agregar condición constante" }));

    let rels = JSON.parse(
      screen.getByTestId("rels").textContent ?? "[]",
    ) as RelationshipFormState[];
    expect(rels[0].onConstants).toHaveLength(1);
    // Por defecto la tabla es la padre de la relación.
    expect(rels[0].onConstants[0].table).toBe("clientes");

    // Completa columna y valor.
    fireEvent.change(screen.getByPlaceholderText("list_numero"), {
      target: { value: "estado" },
    });
    fireEvent.change(screen.getByPlaceholderText("10001"), {
      target: { value: "1" },
    });

    rels = JSON.parse(
      screen.getByTestId("rels").textContent ?? "[]",
    ) as RelationshipFormState[];
    expect(rels[0].onConstants[0]).toEqual({ table: "clientes", column: "estado", value: "1" });
  });

  it("elimina una condición constante con «Quitar»", () => {
    const initial = multiState();
    initial.relationships[0].onConstants = [{ table: "clientes", column: "estado", value: "1" }];
    render(<Harness initial={initial} />);
    expandRelationshipRow();

    // El valor preexistente se muestra; se elimina con el botón Quitar de su fila.
    const valueInput = screen.getByPlaceholderText("10001") as HTMLInputElement;
    expect(valueInput.value).toBe("1");

    // Hay varios botones "Quitar" (relación + condición); el de la condición es
    // el último renderizado dentro del bloque de constantes.
    const quitarButtons = screen.getAllByRole("button", { name: "Quitar" });
    fireEvent.click(quitarButtons[quitarButtons.length - 1]);

    const rels = JSON.parse(
      screen.getByTestId("rels").textContent ?? "[]",
    ) as RelationshipFormState[];
    expect(rels[0].onConstants).toHaveLength(0);
  });
});
