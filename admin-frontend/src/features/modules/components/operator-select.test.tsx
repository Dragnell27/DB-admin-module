// @vitest-environment jsdom
// Pruebas del selector de operador y de su integración en el editor de
// condiciones de listado (ColumnsSection) y en las constantes del ON.
import "@testing-library/jest-dom/vitest";
import * as React from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { emptyWizardState, type WizardState } from "@/features/modules/schema";
import { OperatorSelect } from "@/features/modules/components/operator-select";

vi.mock("@/features/modules/hooks", () => ({
  useModuleDef: () => ({ data: undefined, isFetching: false, isError: false, error: null }),
  useVisibleModules: () => ({ data: [] }),
}));
import { ColumnsSection, TablesSection } from "@/features/modules/components/wizard-sections";

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("OperatorSelect", () => {
  it("muestra las 9 etiquetas en español y el default cuando no hay valor", () => {
    render(<OperatorSelect value={undefined} defaultOperator="in" onChange={() => {}} />);
    const select = screen.getByLabelText("Operador") as HTMLSelectElement;
    expect(Array.from(select.options).map((o) => o.text)).toEqual([
      "igual a",
      "distinto de",
      "está en",
      "no está en",
      "contiene",
      "empieza con",
      "termina con",
      "es vacío",
      "no es vacío",
    ]);
    expect(select.value).toBe("in");
  });

  it("emite el operador elegido", () => {
    const onChange = vi.fn();
    render(<OperatorSelect value="eq" defaultOperator="eq" onChange={onChange} />);
    fireEvent.change(screen.getByLabelText("Operador"), { target: { value: "contains" } });
    expect(onChange).toHaveBeenCalledWith("contains");
  });
});

function Harness({ initial, children }: { initial: WizardState; children: (s: WizardState, p: (x: Partial<WizardState>) => void) => React.ReactNode }) {
  const [state, setState] = React.useState(initial);
  const patch = (p: Partial<WizardState>) => setState((s) => ({ ...s, ...p }));
  return <>{children(state, patch)}</>;
}

describe("condiciones de listado", () => {
  it("oculta el input de valores con is_null/is_not_null", () => {
    const initial: WizardState = {
      ...emptyWizardState(),
      pgTable: "pagos",
      pgWhitelist: { pagos: ["estado"] },
      conditions: [{ field: "estado", valuesText: "A" }],
    };
    render(
      <Harness initial={initial}>
        {(state, patch) => <ColumnsSection state={state} patch={patch} />}
      </Harness>,
    );
    expect(screen.getByPlaceholderText("active, pending")).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Operador"), { target: { value: "is_null" } });
    expect(screen.queryByPlaceholderText("active, pending")).not.toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Operador"), { target: { value: "neq" } });
    expect(screen.getByPlaceholderText("active, pending")).toHaveValue("A");
    expect(screen.getByText(/quedan fuera/)).toBeInTheDocument();
  });
});

describe("constantes del ON", () => {
  it("cambiar el operador conserva el texto de value", () => {
    const initial: WizardState = {
      ...emptyWizardState(),
      pgTable: "pagos",
      foreignTables: ["listas"],
      pgWhitelist: { pagos: ["id", "lista_id"], listas: ["id", "num"] },
      relationships: [
        {
          childTable: "pagos",
          parentTable: "listas",
          on: [{ child: "lista_id", parent: "id" }],
          onConstants: [{ table: "listas", column: "num", value: "1, 2" }],
          joinType: "left",
        },
      ],
    };
    render(
      <Harness initial={initial}>
        {(state, patch) => (
          <>
            <TablesSection state={state} patch={patch} />
            <output data-testid="rels">{JSON.stringify(state.relationships)}</output>
          </>
        )}
      </Harness>,
    );
    fireEvent.click(screen.getByRole("button", { name: /pagos → listas/ }));
    fireEvent.change(screen.getByLabelText("Operador"), { target: { value: "in" } });
    const rels = JSON.parse(screen.getByTestId("rels").textContent ?? "[]");
    expect(rels[0].onConstants[0]).toMatchObject({ operator: "in", value: "1, 2" });
    expect(screen.getByPlaceholderText("10001, 10002")).toHaveValue("1, 2");
    fireEvent.change(screen.getByLabelText("Operador"), { target: { value: "is_not_null" } });
    expect(screen.queryByPlaceholderText("10001, 10002")).not.toBeInTheDocument();
  });
});
