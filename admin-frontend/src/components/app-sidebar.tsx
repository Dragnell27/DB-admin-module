"use client";

/**
 * Menú lateral del panel protegido (Req. 4.1, 4.2, 4.3, 4.4).
 *
 * Muestra las entradas agrupadas en dos secciones con encabezado: "Operación"
 * (recursos OTT: Customers, Plans, Subscriptions, Cards) y "Administración"
 * (módulos del panel: Usuarios, Roles). La separación deja claro qué temas son
 * administrativos y cuáles operativos. Cada sección solo se muestra si tiene al
 * menos una entrada visible. Usa enlaces del App Router con resaltado de la ruta
 * activa (Req. 4.1, 4.2) y la paleta oscura vía tokens `--sidebar-*` (Req. 4.3).
 *
 * Visibilidad por permisos (Req. 9.2, 9.3): cada entrada se condiciona a que los
 * Permisos_Efectivos del usuario incluyan la acción `ver` sobre el Recurso del
 * módulo en el país seleccionado. Los módulos Usuarios y Roles sólo aparecen si
 * el usuario puede ver los recursos `users` / `roles` respectivamente. El país
 * seleccionado se lee de forma reactiva de `lib/country` (el mismo que alimenta
 * la cabecera `X-Country`). La seguridad real la aplica siempre el BFF
 * (design.md, Security); esto es sólo visibilidad de la UI.
 *
 * Para el rol `viewer` se expone un distintivo "Solo lectura" que comunica que
 * las acciones de escritura no estarán disponibles; el ocultado fino de botones
 * Crear/Editar/Eliminar/Cancelar lo aplican las vistas de cada recurso.
 */
import * as React from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { Boxes, ChevronRight, Folder, Plug } from "lucide-react";

import { cn } from "@/lib/utils";
import { useRole } from "@/lib/use-role";
import { can, canModule } from "@/lib/permissions";
import { usePermissions, useModulePermissions } from "@/lib/use-permissions";
import { getCountry, subscribeCountry } from "@/lib/country";
import {
  type ExpandedFolders,
  folderPath,
  getExpandedFolders,
  isExpanded,
  subscribeFolders,
  toggleFolder,
} from "@/lib/sidebar-folders";
import { getCollapsed, subscribeCollapsed } from "@/lib/sidebar-collapse";
import { useVisibleModules } from "@/features/modules/hooks";
import { buildMenuTree, type MenuTreeNode } from "@/features/modules/menu-tree";
import { NAV_GROUPS, NAV_ITEMS } from "@/features/navigation/nav-items";

/** Snapshot estable (todo contraído) para el servidor y el primer render. */
const EMPTY_FOLDERS: ExpandedFolders = {};

/**
 * Lee el país seleccionado de forma reactiva y SSR-safe (igual que el selector
 * de país). En el servidor y en el primer render del cliente devuelve `""` para
 * evitar desajustes de hidratación; tras montar refleja la selección real.
 */
function useSelectedCountry(): string {
  return React.useSyncExternalStore(
    subscribeCountry,
    () => getCountry() ?? "",
    () => "",
  );
}

/**
 * Lee el mapa de carpetas expandidas del menú de forma reactiva y SSR-safe. En
 * el servidor y en el primer render del cliente devuelve `{}` (todo contraído),
 * evitando desajustes de hidratación; tras montar refleja el estado persistido.
 */
function useExpandedFolders(): ExpandedFolders {
  return React.useSyncExternalStore(
    subscribeFolders,
    getExpandedFolders,
    () => EMPTY_FOLDERS,
  );
}

/**
 * Lee el estado contraído del menú lateral global de forma reactiva y SSR-safe.
 * En el servidor y en el primer render del cliente devuelve `false` (expandido)
 * para evitar desajustes de hidratación; tras montar refleja el estado
 * persistido en `localStorage`.
 */
function useCollapsed(): boolean {
  return React.useSyncExternalStore(
    subscribeCollapsed,
    getCollapsed,
    () => false,
  );
}

/**
 * Calcula las rutas de carpeta ANCESTRAS de la hoja activa (`/modules/<slug>`)
 * para auto-expandir su cadena y dejar visible la página actual. Devuelve un
 * conjunto vacío si no hay hoja activa. Helper puro sobre el árbol + pathname.
 */
function activeAncestorPaths(
  nodes: ReadonlyArray<MenuTreeNode>,
  pathname: string,
): Set<string> {
  const found = new Set<string>();
  const walk = (
    level: ReadonlyArray<MenuTreeNode>,
    parentPath: string,
  ): boolean => {
    for (const node of level) {
      if (node.slug === undefined) {
        const path = folderPath(parentPath, node.label);
        if (walk(node.children, path)) {
          // El subárbol contiene la hoja activa: esta carpeta es un ancestro.
          found.add(path);
          return true;
        }
      } else if (isActivePath(pathname, `/modules/${node.slug}`)) {
        return true;
      }
    }
    return false;
  };
  walk(nodes, "");
  return found;
}

/**
 * Indica si `href` corresponde a la ruta activa. Se considera activa tanto la
 * ruta exacta como cualquier sub-ruta (p. ej. `/plans/new` activa `Plans`).
 */
function isActivePath(pathname: string, href: string): boolean {
  return pathname === href || pathname.startsWith(`${href}/`);
}

/**
 * Renderiza recursivamente los nodos del árbol de menú: las carpetas (sin slug)
 * como encabezados visuales con sus hijos sangrados; las hojas (con slug) como
 * enlaces al módulo. El árbol ya viene filtrado por RBAC (solo módulos
 * visibles), así que no quedan carpetas sin hojas (D-7).
 */
function MenuTreeNodes({
  nodes,
  pathname,
  depth,
  parentPath,
  expanded,
  activeAncestors,
}: {
  nodes: ReadonlyArray<MenuTreeNode>;
  pathname: string;
  depth: number;
  /** Ruta completa de la carpeta padre ("" en la raíz) para la clave estable. */
  parentPath: string;
  /** Mapa de carpetas expandidas (ruta → true/false). */
  expanded: Readonly<ExpandedFolders>;
  /** Rutas de carpeta ancestras de la hoja activa (auto-expandido). */
  activeAncestors: ReadonlySet<string>;
}) {
  return (
    <>
      {nodes.map((node) => {
        if (node.slug === undefined) {
          // Nodo carpeta: encabezado contraíble (botón) con chevron. Arranca
          // contraída por defecto; su estado se identifica por la ruta completa.
          const path = folderPath(parentPath, node.label);
          const open = isExpanded(expanded, path, activeAncestors);
          return (
            <div key={`folder:${path}`} className="space-y-1">
              <button
                type="button"
                onClick={() => toggleFolder(path)}
                aria-expanded={open}
                style={{ paddingLeft: `${0.75 + depth * 0.75}rem` }}
                className={cn(
                  "flex w-full items-center gap-2 rounded-md py-1 pr-3 text-xs font-medium text-muted-foreground transition-colors",
                  "hover:text-sidebar-accent-foreground",
                  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-sidebar-ring",
                )}
              >
                <ChevronRight
                  className={cn(
                    "size-3.5 shrink-0 transition-transform",
                    open ? "rotate-90" : "",
                  )}
                  aria-hidden="true"
                />
                <Folder className="size-3.5 shrink-0" aria-hidden="true" />
                <span>{node.label}</span>
              </button>
              {open ? (
                <MenuTreeNodes
                  nodes={node.children}
                  pathname={pathname}
                  depth={depth + 1}
                  parentPath={path}
                  expanded={expanded}
                  activeAncestors={activeAncestors}
                />
              ) : null}
            </div>
          );
        }
        // Nodo hoja (módulo).
        const href = `/modules/${node.slug}`;
        const active = isActivePath(pathname, href);
        return (
          <Link
            key={node.slug}
            href={href}
            aria-current={active ? "page" : undefined}
            style={{ paddingLeft: `${0.75 + depth * 0.75}rem` }}
            className={cn(
              "flex items-center gap-3 rounded-md py-2 pr-3 text-sm font-medium transition-colors",
              "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-sidebar-ring",
              active
                ? "bg-sidebar-accent text-sidebar-accent-foreground"
                : "text-sidebar-foreground hover:bg-sidebar-accent/60 hover:text-sidebar-accent-foreground",
            )}
          >
            <Boxes className="size-4 shrink-0" aria-hidden="true" />
            <span>{node.label}</span>
          </Link>
        );
      })}
    </>
  );
}

/**
 * Menú lateral del panel. Renderiza la marca, el distintivo de rol y la lista
 * de recursos con resaltado de la ruta activa.
 */
export function AppSidebar() {
  const pathname = usePathname();
  const role = useRole();
  const perms = usePermissions();
  const modperms = useModulePermissions();
  const country = useSelectedCountry();

  // Sólo se muestran las entradas cuyo recurso el usuario puede `ver` en el país
  // seleccionado (Req. 9.2, 9.3). Antes de la hidratación/selección de país la
  // lista queda vacía, consistente con que los permisos aún no están disponibles.
  const visibleItems = NAV_ITEMS.filter((item) =>
    can(perms, item.resource, "ver", country),
  );

  // Módulos dinámicos del constructor (module-builder): se descubren vía
  // GET /modules (ya filtrado por permiso en el backend) y se condicionan
  // además por canModule en el país seleccionado, para coherencia inmediata al
  // cambiar de país. La seguridad real la aplica el backend.
  const { data: dynamicModules } = useVisibleModules();
  const visibleModules = (dynamicModules ?? []).filter((m) =>
    canModule(modperms, m.slug, "ver", country),
  );
  // Árbol de menú por ruta libre (menu_path). Las carpetas sin hojas visibles no
  // se emiten porque el árbol se arma solo con los módulos visibles (D-7).
  const menuTree = buildMenuTree(visibleModules);

  // Estado contraíble de las carpetas del árbol de módulos (persistido en
  // localStorage, contraído por defecto) y cadena de ancestros de la ruta
  // activa para auto-expandirla.
  const expandedFolders = useExpandedFolders();
  const activeAncestors = React.useMemo(
    () => activeAncestorPaths(menuTree, pathname),
    [menuTree, pathname],
  );

  // Estado contraído del menú global (riel estrecho solo con iconos). Reclama
  // ancho para el contenido en todo el panel; se persiste en localStorage.
  const collapsed = useCollapsed();

  return (
    <aside
      className={cn(
        "flex h-full shrink-0 flex-col border-r border-sidebar-border bg-sidebar text-sidebar-foreground",
        collapsed ? "w-16" : "w-64",
      )}
      aria-label="Navegación principal"
    >
      <div
        className={cn(
          "flex h-16 items-center gap-2 border-b border-sidebar-border",
          collapsed ? "justify-center px-2" : "px-6",
        )}
      >
        {collapsed ? (
          <Boxes className="size-5 text-foreground" aria-hidden="true" />
        ) : (
          <span className="text-base font-semibold text-foreground">
            Panel Admin
          </span>
        )}
      </div>

      {role === "viewer" && !collapsed ? (
        <div className="px-4 pt-4">
          <span className="inline-flex items-center rounded-md bg-sidebar-accent px-2 py-1 text-xs font-medium text-sidebar-accent-foreground">
            Solo lectura
          </span>
        </div>
      ) : null}

      <nav className="flex-1 space-y-6 px-3 py-4" aria-label="Recursos">
        {NAV_GROUPS.map((group) => {
          // Entradas visibles de este grupo; si no hay ninguna, se omite el
          // grupo completo (incluido su encabezado).
          const groupItems = visibleItems.filter(
            (item) => item.section === group.section,
          );
          if (groupItems.length === 0) {
            return null;
          }
          return (
            <div key={group.section} className="space-y-1">
              {collapsed ? null : (
                <h2 className="px-3 pb-1 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                  {group.heading}
                </h2>
              )}
              {groupItems.map((item) => {
                const active = isActivePath(pathname, item.href);
                const Icon = item.icon;
                return (
                  <Link
                    key={item.href}
                    href={item.href}
                    aria-current={active ? "page" : undefined}
                    title={collapsed ? item.label : undefined}
                    className={cn(
                      "flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors",
                      "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-sidebar-ring",
                      collapsed ? "justify-center px-0" : "",
                      active
                        ? "bg-sidebar-accent text-sidebar-accent-foreground"
                        : "text-sidebar-foreground hover:bg-sidebar-accent/60 hover:text-sidebar-accent-foreground",
                    )}
                  >
                    <Icon className="size-4 shrink-0" aria-hidden="true" />
                    {collapsed ? null : <span>{item.label}</span>}
                  </Link>
                );
              })}
            </div>
          );
        })}

        {/* Acceso al constructor de módulos (CRUD de definiciones), visible si
            el usuario puede ver el recurso module-builder (slug reservado
            __defs__ en modperms). */}
        {canModule(modperms, "__defs__", "ver", country) ? (
          <div className="space-y-1">
            {collapsed ? null : (
              <h2 className="px-3 pb-1 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                Constructor
              </h2>
            )}
            {(() => {
              const href = "/module-defs";
              const active = isActivePath(pathname, href);
              return (
                <Link
                  href={href}
                  aria-current={active ? "page" : undefined}
                  title={collapsed ? "Módulos" : undefined}
                  className={cn(
                    "flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors",
                    "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-sidebar-ring",
                    collapsed ? "justify-center px-0" : "",
                    active
                      ? "bg-sidebar-accent text-sidebar-accent-foreground"
                      : "text-sidebar-foreground hover:bg-sidebar-accent/60 hover:text-sidebar-accent-foreground",
                  )}
                >
                  <Boxes className="size-4 shrink-0" aria-hidden="true" />
                  {collapsed ? null : <span>Módulos</span>}
                </Link>
              );
            })()}
          </div>
        ) : null}

        {/* Acceso a las plantillas de conexión (CRUD), visible si el usuario
            puede ver el recurso connections (slug reservado __connections__ en
            modperms). Mismo guardado que la entrada "Módulos" (__defs__). */}
        {canModule(modperms, "__connections__", "ver", country) ? (
          <div className="space-y-1">
            {collapsed ? null : (
              <h2 className="px-3 pb-1 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                Conexiones
              </h2>
            )}
            {(() => {
              const href = "/connection-templates";
              const active = isActivePath(pathname, href);
              return (
                <Link
                  href={href}
                  aria-current={active ? "page" : undefined}
                  title={collapsed ? "Conexiones" : undefined}
                  className={cn(
                    "flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors",
                    "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-sidebar-ring",
                    collapsed ? "justify-center px-0" : "",
                    active
                      ? "bg-sidebar-accent text-sidebar-accent-foreground"
                      : "text-sidebar-foreground hover:bg-sidebar-accent/60 hover:text-sidebar-accent-foreground",
                  )}
                >
                  <Plug className="size-4 shrink-0" aria-hidden="true" />
                  {collapsed ? null : <span>Conexiones</span>}
                </Link>
              );
            })()}
          </div>
        ) : null}

        {/* Módulos dinámicos (module-builder) agrupados por su menu_path. Sección
            aparte al final, solo si el usuario tiene al menos un módulo visible.
            Con el menú contraído el árbol de carpetas se oculta por completo
            (comportamiento coherente más simple: el riel solo muestra iconos de
            nivel superior). */}
        {!collapsed && menuTree.length > 0 ? (
          <div className="space-y-1">
            <h2 className="px-3 pb-1 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
              Módulos
            </h2>
            <MenuTreeNodes
              nodes={menuTree}
              pathname={pathname}
              depth={0}
              parentPath=""
              expanded={expandedFolders}
              activeAncestors={activeAncestors}
            />
          </div>
        ) : null}
      </nav>
    </aside>
  );
}

export default AppSidebar;
