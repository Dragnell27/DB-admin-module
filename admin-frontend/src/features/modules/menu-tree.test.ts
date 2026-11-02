// Pruebas puras de buildMenuTree (connection-templates, Req. 6, D-7): rutas con
// prefijo común comparten nodos, profundidad variable, path vacío -> raíz, y
// (por construcción con módulos ya filtrados) no hay carpetas sin hojas.

import { describe, expect, it } from "vitest";

import { buildMenuTree, type MenuTreeNode } from "@/features/modules/menu-tree";
import type { ModuleSummary } from "@/features/modules/types";

function mod(slug: string, name: string, menuPath?: string): ModuleSummary {
  return { slug, name, menuPath };
}

/** Busca una carpeta por etiqueta en un nivel. */
function folder(nodes: MenuTreeNode[], label: string): MenuTreeNode | undefined {
  return nodes.find((n) => n.slug === undefined && n.label === label);
}

describe("buildMenuTree", () => {
  it("coloca en la raíz los módulos con menuPath vacío", () => {
    const tree = buildMenuTree([mod("a", "Alfa", ""), mod("b", "Beta")]);
    expect(tree).toHaveLength(2);
    expect(tree.every((n) => n.slug !== undefined)).toBe(true);
    expect(tree.map((n) => n.slug)).toEqual(["a", "b"]);
  });

  it("comparte nodos de prefijo común", () => {
    // Dos módulos cuyo menuPath comparte el prefijo CENAM/Recaudo. Cada módulo
    // cuelga como hoja bajo el ÚLTIMO segmento de su ruta (GT / SV).
    const tree = buildMenuTree([
      mod("gt", "GT", "CENAM/Recaudo/GT"),
      mod("sv", "SV", "CENAM/Recaudo/SV"),
    ]);
    // Un solo nodo raíz CENAM.
    expect(tree).toHaveLength(1);
    const cenam = folder(tree, "CENAM")!;
    const recaudo = folder(cenam.children, "Recaudo")!;
    // Bajo CENAM/Recaudo hay dos carpetas GT y SV (prefijo compartido).
    const gtFolder = folder(recaudo.children, "GT")!;
    const svFolder = folder(recaudo.children, "SV")!;
    expect(gtFolder.children.some((n) => n.slug === "gt")).toBe(true);
    expect(svFolder.children.some((n) => n.slug === "sv")).toBe(true);
  });

  it("coloca la hoja directamente bajo la carpeta cuando la ruta termina en folder-compartido", () => {
    // Dos módulos bajo la MISMA ruta exacta: comparten todas las carpetas y
    // ambas hojas cuelgan del mismo nodo (prefijo totalmente compartido).
    const tree = buildMenuTree([
      mod("a", "Alfa", "CENAM/Recaudo"),
      mod("b", "Beta", "CENAM/Recaudo"),
    ]);
    const cenam = folder(tree, "CENAM")!;
    const recaudo = folder(cenam.children, "Recaudo")!;
    const leaves = recaudo.children.filter((n) => n.slug !== undefined);
    expect(leaves.map((n) => n.slug).sort()).toEqual(["a", "b"]);
  });

  it("soporta profundidad variable", () => {
    const tree = buildMenuTree([
      mod("x", "X", "A"),
      mod("y", "Y", "A/B/C/D"),
    ]);
    const a = folder(tree, "A")!;
    // Hoja X directamente bajo A.
    expect(a.children.some((n) => n.slug === "x")).toBe(true);
    // Y cuelga de A/B/C/D.
    const b = folder(a.children, "B")!;
    const c = folder(b.children, "C")!;
    const d = folder(c.children, "D")!;
    expect(d.children.some((n) => n.slug === "y")).toBe(true);
  });

  it("no emite carpetas sin hojas (solo se arma con módulos visibles, D-7)", () => {
    // Al pasar solo módulos visibles, cualquier carpeta del árbol tiene al menos
    // una hoja en su subárbol.
    const tree = buildMenuTree([mod("gt", "GT", "CENAM/Recaudo/GT")]);
    const hasLeaf = (nodes: MenuTreeNode[]): boolean =>
      nodes.some((n) => n.slug !== undefined || hasLeaf(n.children));
    const allFoldersHaveLeaf = (nodes: MenuTreeNode[]): boolean =>
      nodes.every(
        (n) => n.slug !== undefined || (hasLeaf(n.children) && allFoldersHaveLeaf(n.children)),
      );
    expect(allFoldersHaveLeaf(tree)).toBe(true);
  });

  it("devuelve un árbol vacío si no hay módulos", () => {
    expect(buildMenuTree([])).toEqual([]);
  });
});
