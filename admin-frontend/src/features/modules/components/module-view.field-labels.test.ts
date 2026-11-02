// Pruebas puras del helper fieldLabelsForAction de module-view: arma el mapa
// nombre-de-campo → etiqueta legible emparejando cada FieldSpec con su
// DisplayColumn por (field === name) y, en multi-tabla, por sourceTable (con el
// principal como default). No renderiza el componente (evita el hang
// jsdom/userEvent en Windows).
import { describe, expect, it } from "vitest";

import { fieldLabelsForAction } from "@/features/modules/components/module-view";
import type { DisplayColumn, FieldSpec } from "@/features/modules/types";

/** Columna mínima. En runtime columnsFromMapping ya aplica label ?? field. */
function col(field: string, label: string, sourceTable?: string): DisplayColumn {
  return {
    field,
    label,
    order: 0,
    visible: true,
    isPrimaryKey: false,
    editable: true,
    sourceTable,
  };
}

/** FieldSpec mínimo (source "input"). */
function field(name: string, sourceTable?: string): FieldSpec {
  return { name, type: "string", required: false, source: "input", sourceTable };
}

describe("fieldLabelsForAction", () => {
  it("(a) etiqueta no vacía: devuelve la label de la columna que empareja", () => {
    const fields = [field("forma_pago")];
    const columns = [col("forma_pago", "Forma de pago")];
    expect(fieldLabelsForAction(fields, columns, "pagos")).toEqual({
      forma_pago: "Forma de pago",
    });
  });

  it("(b) etiqueta vacía (fallback): respeta la label ya normalizada a field", () => {
    // columnsFromMapping ya hace label = label ?? field; aquí la columna llega
    // con label === field, y el resolver lo respeta tal cual.
    const fields = [field("nombre")];
    const columns = [col("nombre", "nombre")];
    expect(fieldLabelsForAction(fields, columns, "pagos")).toEqual({
      nombre: "nombre",
    });
  });

  it("(c) sin columna que empareje: el name no entra en el mapa", () => {
    const fields = [field("extra")];
    const columns = [col("forma_pago", "Forma de pago")];
    const result = fieldLabelsForAction(fields, columns, "pagos");
    expect(result).not.toHaveProperty("extra");
    expect(result).toEqual({});
  });

  it("(d) multi-tabla: empareja por (name, sourceTable) sin confundir homónimas", () => {
    const columns = [
      col("estado", "Estado pago", "pagos"),
      col("estado", "Estado cliente", "clientes"),
    ];
    // Campo con sourceTable explícito ⇒ empareja la columna de esa tabla.
    expect(
      fieldLabelsForAction([field("estado", "clientes")], columns, "pagos"),
    ).toEqual({ estado: "Estado cliente" });
    // Campo sin sourceTable ⇒ cae al principal (pagos).
    expect(fieldLabelsForAction([field("estado")], columns, "pagos")).toEqual({
      estado: "Estado pago",
    });
  });
});
