/**
 * Construcción del árbol de menú por ruta libre (connection-templates, Req. 6).
 *
 * Cada módulo tiene un `menuPath` tipo `CENAM/Cuentas de recaudo/GT` (segmentos
 * separados por «/», profundidad ilimitada). Los segmentos intermedios se
 * vuelven NODOS CARPETA (solo visuales) y cada módulo cuelga como HOJA bajo su
 * ruta. Módulos con `menuPath` vacío quedan en la raíz. Rutas con prefijo común
 * comparten nodos.
 *
 * D-7: el árbol se construye SOLO con módulos visibles (ya filtrados por RBAC),
 * de modo que una carpeta cuyo subárbol quede sin hojas no se emite (se omite).
 * Es un helper puro, testeable de forma aislada.
 */
import type { ModuleSummary } from "@/features/modules/types";

/** Nodo del árbol de menú: carpeta (con hijos) u hoja (módulo con slug). */
export interface MenuTreeNode {
  /** Etiqueta visible del nodo (segmento de ruta o nombre del módulo). */
  label: string;
  /** Slug del módulo si el nodo es una HOJA; undefined si es carpeta. */
  slug?: string;
  /** Hijos del nodo (carpetas y/o hojas), ordenados; vacío para hojas. */
  children: MenuTreeNode[];
}

/** Divide un menuPath en segmentos no vacíos (tolera «/» de sobra). */
function splitPath(menuPath: string | undefined): string[] {
  if (!menuPath) {
    return [];
  }
  return menuPath
    .split("/")
    .map((s) => s.trim())
    .filter((s) => s !== "");
}

/**
 * Construye el árbol de menú a partir de los módulos VISIBLES. Las carpetas se
 * crean on-demand y se comparten por prefijo; las hojas son los módulos. El
 * resultado está ordenado alfabéticamente por etiqueta en cada nivel, con las
 * carpetas y hojas intercaladas por nombre (orden estable para la UI/tests).
 */
export function buildMenuTree(modules: ReadonlyArray<ModuleSummary>): MenuTreeNode[] {
  const roots: MenuTreeNode[] = [];

  /** Busca o crea una carpeta hija por etiqueta dentro de `level`. */
  const folderIn = (level: MenuTreeNode[], label: string): MenuTreeNode => {
    let node = level.find((n) => n.slug === undefined && n.label === label);
    if (!node) {
      node = { label, children: [] };
      level.push(node);
    }
    return node;
  };

  for (const mod of modules) {
    const segments = splitPath(mod.menuPath);
    let level = roots;
    for (const seg of segments) {
      level = folderIn(level, seg).children;
    }
    // Hoja del módulo bajo su ruta (nombre como etiqueta).
    level.push({ label: mod.name, slug: mod.slug, children: [] });
  }

  sortTree(roots);
  return roots;
}

/** Ordena recursivamente cada nivel por etiqueta (orden estable). */
function sortTree(level: MenuTreeNode[]): void {
  level.sort((a, b) => a.label.localeCompare(b.label));
  for (const node of level) {
    if (node.children.length > 0) {
      sortTree(node.children);
    }
  }
}
