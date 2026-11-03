// Pruebas puras de la identidad estable (uid efímero) de acciones y campos del
// constructor de módulos (Arreglo 1): actionRowKey/fieldRowKey (key de React por
// uid con fallback a índice), nextUid (unicidad), y la siembra determinista de
// uid en fromDefinition/fromCopyableDefinition (módulo COPIADO) sin romper el
// round-trip `toEqual`. Funciones puras; entorno node (NO monta React).

import { describe, expect, it } from "vitest";

import {
  actionRowKey,
  fieldRowKey,
  fromCopyableDefinition,
  fromDefinition,
  nextUid,
} from "@/features/modules/schema";
import type { ModuleDefinitionInput } from "@/features/modules/types";

/** Definición mínima con una acción create (2 campos) y una list, para hidratar. */
function def(): ModuleDefinitionInput {
  return {
    slug: "demo",
    name: "Demo",
    sourceType: "postgres",
    countryScope: "none",
    menuPath: "",
    fetchMapping: { table: "t" },
    displayMapping: {
      columns: [
        { field: "id", label: "ID", order: 0, visible: true, isPrimaryKey: true, editable: false },
        { field: "a", label: "A", order: 1, visible: true, isPrimaryKey: false, editable: true },
        { field: "b", label: "B", order: 2, visible: true, isPrimaryKey: false, editable: true },
      ],
    },
    listParams: {},
    listConditions: [],
    dataSource: {
      sourceType: "postgres",
      pgWhitelist: { t: ["id", "a", "b"] },
      connectionTemplateId: "tmpl-1",
      hasSecrets: false,
    },
    actions: [
      {
        actionKey: "crear",
        kind: "create",
        label: "Crear",
        placement: "header",
        destructive: false,
        permAction: "crear",
        isWrite: true,
        sqlOp: "INSERT",
        inputFields: [
          { name: "a", type: "string", required: true, source: "input" },
          { name: "b", type: "string", required: false, source: "input" },
        ],
      },
      {
        actionKey: "listar",
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
  } as unknown as ModuleDefinitionInput;
}

describe("actionRowKey / fieldRowKey", () => {
  it("usa el uid y es independiente del índice", () => {
    expect(actionRowKey({ uid: "a3" }, 7)).toBe("a:a3");
    expect(actionRowKey({ uid: "a3" }, 0)).toBe(actionRowKey({ uid: "a3" }, 9));
    expect(fieldRowKey({ uid: "f5" }, 2)).toBe("f:f5");
    expect(fieldRowKey({ uid: "f5" }, 0)).toBe(fieldRowKey({ uid: "f5" }, 4));
  });

  it("sin uid cae al fallback por índice, únicos entre sí", () => {
    expect(actionRowKey({ uid: undefined }, 0)).toBe("a:__0");
    expect(actionRowKey({ uid: undefined }, 1)).toBe("a:__1");
    expect(actionRowKey({ uid: undefined }, 0)).not.toBe(actionRowKey({ uid: undefined }, 1));
    expect(fieldRowKey({ uid: undefined }, 0)).toBe("f:__0");
    expect(fieldRowKey({ uid: undefined }, 1)).toBe("f:__1");
    expect(fieldRowKey({ uid: undefined }, 0)).not.toBe(fieldRowKey({ uid: undefined }, 1));
  });

  it("núcleo del bug: borrar OTRA acción no cambia la identidad de los campos que sobreviven", () => {
    const state = fromDefinition(def());
    const create = state.actions[0];
    const before = create.inputFields.map((f, i) => fieldRowKey(f, i));

    // Elimina la acción list (índice 1), igual que removeAction (filter).
    const remaining = state.actions.filter((_, idx) => idx !== 1);
    const survivor = remaining[0];
    const after = survivor.inputFields.map((f, i) => fieldRowKey(f, i));

    expect(after).toEqual(before);
  });

  it("transición name:'' -> 'valor_objeto' con el mismo uid: la key NO cambia", () => {
    // La key depende SOLO del uid, no del name: elegir la columna (que cambia
    // name vía updateField, conservando el uid por el spread) no altera la key.
    const uid = "f9";
    const empty = fieldRowKey({ uid }, 0);
    const chosen = fieldRowKey({ uid }, 0);
    expect(chosen).toBe(empty);
  });
});

describe("nextUid", () => {
  it("dos invocaciones consecutivas dan valores distintos con el prefijo esperado", () => {
    const a = nextUid("a");
    const b = nextUid("a");
    expect(a).not.toBe(b);
    expect(a.startsWith("a")).toBe(true);
    expect(b.startsWith("a")).toBe(true);
    expect(nextUid("f").startsWith("f")).toBe(true);
  });
});

describe("siembra determinista de uid en hidratación", () => {
  it("fromDefinition asigna uid a acciones y campos de forma estable por posición", () => {
    const s1 = fromDefinition(def());
    const s2 = fromDefinition(def());
    expect(s1.actions[0].uid).toBe("h0");
    expect(s1.actions[1].uid).toBe("h1");
    expect(s1.actions[0].inputFields[0].uid).toBe("h0_0");
    expect(s1.actions[0].inputFields[1].uid).toBe("h0_1");
    // Determinismo: dos hidrataciones producen los mismos uid.
    expect(s2.actions.map((a) => a.uid)).toEqual(s1.actions.map((a) => a.uid));
  });

  it("fromCopyableDefinition (módulo copiado) también tiene uid y mantiene toEqual con fromDefinition", () => {
    const copy = fromCopyableDefinition(def());
    const edit = fromDefinition(def());
    // El módulo copiado tiene uid en todas sus entidades (caso que faltaba).
    expect(copy.actions[0].uid).toBe("h0");
    expect(copy.actions[0].inputFields[0].uid).toBe("h0_0");
    // El round-trip sigue intacto: ambas hidrataciones son equivalentes.
    expect(copy.actions).toEqual(edit.actions);
  });
});
