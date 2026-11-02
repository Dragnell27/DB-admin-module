/**
 * Catálogo de navegación compartido entre el menú lateral (`AppSidebar`) y la
 * pantalla de inicio (`/`).
 *
 * El objetivo es que el HOME y el sidebar NUNCA discrepen: ambos derivan qué
 * puede ver el usuario de la MISMA fuente y con la MISMA lógica de permisos
 * (Req. 9.2, 9.3). Por eso aquí vive:
 *   - la lista de recursos FIJOS del catálogo (customers, plans, ...), cada uno
 *     condicionado a `can(perms, resource, "ver", country)`,
 *   - las entradas RESERVADAS del constructor de módulos (`__defs__`) y de las
 *     plantillas de conexión (`__connections__`), condicionadas a
 *     `canModule(modperms, slug, "ver", country)`,
 *   - y una función PURA `selectVisibleNavEntries(...)` que combina todo (más
 *     los módulos dinámicos descubiertos vía `GET /modules`) en la lista de
 *     accesos visibles, ordenada como la muestra el sidebar.
 *
 * Es sólo visibilidad de UI: la autorización real la aplica siempre el backend.
 */
import {
  Boxes,
  CreditCard,
  KeyRound,
  LayoutGrid,
  Plug,
  RefreshCcw,
  ScrollText,
  UserCog,
  Users,
  type LucideIcon,
} from "lucide-react";

import { can, canModule, type Resource } from "@/lib/permissions";
import type { EffectivePermissions, ModulePermissions } from "@/lib/permissions";
import type { ModuleSummary } from "@/features/modules/types";

/**
 * Secciones del menú lateral. Separan los módulos OPERATIVOS (recursos OTT) de
 * los ADMINISTRATIVOS (gestión de usuarios y roles del propio panel).
 */
export type NavSection = "operacion" | "administracion";

/** Entrada de navegación de un recurso FIJO del catálogo cerrado. */
export interface NavItem {
  /** Ruta destino dentro del área protegida. */
  readonly href: string;
  /** Etiqueta visible del recurso. */
  readonly label: string;
  /** Icono representativo del recurso. */
  readonly icon: LucideIcon;
  /** Recurso cuyo permiso `ver` condiciona la visibilidad de la entrada. */
  readonly resource: Resource;
  /** Sección a la que pertenece la entrada. */
  readonly section: NavSection;
  /** Descripción corta para la tarjeta de acceso del HOME. */
  readonly description: string;
}

/** Grupo de navegación: encabezado de sección y su orden de aparición. */
export interface NavGroup {
  /** Identificador estable de la sección. */
  readonly section: NavSection;
  /** Encabezado legible mostrado sobre las entradas del grupo. */
  readonly heading: string;
}

/** Encabezados de cada sección, en orden de aparición. */
export const NAV_GROUPS: readonly NavGroup[] = [
  { section: "operacion", heading: "Operación" },
  { section: "administracion", heading: "Administración" },
] as const;

/**
 * Entradas del menú para los recursos FIJOS, agrupadas por sección. Cada entrada
 * se oculta si el usuario no puede `ver` su recurso en el país seleccionado
 * (Req. 9.2, 9.3). Es la fuente única que consumen el sidebar y el HOME.
 */
export const NAV_ITEMS: readonly NavItem[] = [
  {
    href: "/customers",
    label: "Customers",
    icon: Users,
    resource: "customers",
    section: "operacion",
    description: "Gestiona los clientes de la plataforma.",
  },
  {
    href: "/plans",
    label: "Plans",
    icon: LayoutGrid,
    resource: "plans",
    section: "operacion",
    description: "Administra los planes de suscripción disponibles.",
  },
  {
    href: "/subscriptions",
    label: "Subscriptions",
    icon: RefreshCcw,
    resource: "subscriptions",
    section: "operacion",
    description: "Revisa y gestiona las suscripciones de los clientes.",
  },
  {
    href: "/cards",
    label: "Cards",
    icon: CreditCard,
    resource: "cards",
    section: "operacion",
    description: "Consulta los medios de pago de cada cliente.",
  },
  {
    href: "/users",
    label: "Usuarios",
    icon: UserCog,
    resource: "users",
    section: "administracion",
    description: "Administra los usuarios del panel.",
  },
  {
    href: "/roles",
    label: "Roles",
    icon: KeyRound,
    resource: "roles",
    section: "administracion",
    description: "Define roles y sus permisos.",
  },
  {
    href: "/audit",
    label: "Auditoría",
    icon: ScrollText,
    resource: "audit",
    section: "administracion",
    description: "Revisa la bitácora de auditoría.",
  },
] as const;

/**
 * Entrada RESERVADA del menú (constructor de módulos y plantillas de conexión):
 * no viaja en el bitmask `perms` sino en `modperms` bajo un slug reservado, por
 * lo que su visibilidad se evalúa con `canModule`, igual que hace el sidebar.
 */
export interface ReservedNavEntry {
  /** Slug reservado en `modperms` (p. ej. `__defs__`, `__connections__`). */
  readonly slug: string;
  /** Ruta destino dentro del área protegida. */
  readonly href: string;
  /** Etiqueta visible. */
  readonly label: string;
  /** Icono representativo. */
  readonly icon: LucideIcon;
  /** Encabezado de su sección en el sidebar (reutilizado como grupo del HOME). */
  readonly heading: string;
  /** Descripción corta para la tarjeta de acceso del HOME. */
  readonly description: string;
}

/**
 * Entradas reservadas (Constructor de módulos y Conexiones), en el mismo orden
 * y con las mismas etiquetas/encabezados que usa el sidebar. El slug reservado
 * se evalúa contra `modperms` con `canModule(..., "ver", country)`.
 */
export const RESERVED_NAV_ENTRIES: readonly ReservedNavEntry[] = [
  {
    slug: "__defs__",
    href: "/module-defs",
    label: "Módulos",
    icon: Boxes,
    heading: "Constructor",
    description: "Crea y administra módulos dinámicos del panel.",
  },
  {
    slug: "__connections__",
    href: "/connection-templates",
    label: "Conexiones",
    icon: Plug,
    heading: "Conexiones",
    description: "Define las plantillas de conexión reutilizables.",
  },
] as const;

/** Icono de un módulo dinámico (hoja del árbol de menú). */
export const DYNAMIC_MODULE_ICON: LucideIcon = Boxes;

/**
 * Acceso visible resuelto para el HOME: una tarjeta por recurso/módulo que el
 * usuario puede ver, con su ruta, etiqueta, icono y descripción.
 */
export interface NavEntry {
  /** Clave estable para listas de React. */
  readonly key: string;
  readonly href: string;
  readonly label: string;
  readonly description: string;
  readonly icon: LucideIcon;
}

/**
 * Filtra los recursos FIJOS visibles para `(perms, country)`, preservando el
 * orden de `NAV_ITEMS` (operación antes que administración). Es la MISMA
 * condición que aplica el sidebar: `can(perms, resource, "ver", country)`.
 */
export function selectVisibleFixedItems(
  perms: EffectivePermissions,
  country: string,
): readonly NavItem[] {
  return NAV_ITEMS.filter((item) => can(perms, item.resource, "ver", country));
}

/**
 * Filtra las entradas RESERVADAS visibles para `(modperms, country)`, con la
 * MISMA condición que el sidebar: `canModule(modperms, slug, "ver", country)`.
 */
export function selectVisibleReservedEntries(
  modperms: ModulePermissions,
  country: string,
): readonly ReservedNavEntry[] {
  return RESERVED_NAV_ENTRIES.filter((entry) =>
    canModule(modperms, entry.slug, "ver", country),
  );
}

/**
 * Filtra los módulos DINÁMICOS visibles para `(modperms, country)`, con la MISMA
 * condición que el sidebar: `canModule(modperms, slug, "ver", country)`. Los
 * módulos ya vienen pre-filtrados por el backend; esto añade la coherencia
 * inmediata al cambiar de país.
 */
export function selectVisibleDynamicModules(
  modules: ReadonlyArray<ModuleSummary>,
  modperms: ModulePermissions,
  country: string,
): readonly ModuleSummary[] {
  return modules.filter((m) => canModule(modperms, m.slug, "ver", country));
}

/**
 * Combina recursos fijos, entradas reservadas y módulos dinámicos en la lista de
 * accesos visibles del HOME, en el mismo orden en que el sidebar los presenta:
 * primero los recursos fijos (operación → administración), luego Constructor y
 * Conexiones, y por último los módulos dinámicos (ordenados por nombre, como el
 * árbol del sidebar).
 *
 * Función PURA: no usa hooks; recibe ya resueltos los permisos, el país y los
 * módulos visibles. Así el HOME deriva su contenido de la misma lógica que el
 * sidebar y ambos no pueden discrepar.
 */
export function selectVisibleNavEntries(input: {
  readonly perms: EffectivePermissions;
  readonly modperms: ModulePermissions;
  readonly country: string;
  readonly modules: ReadonlyArray<ModuleSummary>;
}): readonly NavEntry[] {
  const { perms, modperms, country, modules } = input;

  const fixed: NavEntry[] = selectVisibleFixedItems(perms, country).map((item) => ({
    key: `fixed:${item.resource}`,
    href: item.href,
    label: item.label,
    description: item.description,
    icon: item.icon,
  }));

  const reserved: NavEntry[] = selectVisibleReservedEntries(modperms, country).map(
    (entry) => ({
      key: `reserved:${entry.slug}`,
      href: entry.href,
      label: entry.label,
      description: entry.description,
      icon: entry.icon,
    }),
  );

  const dynamic: NavEntry[] = selectVisibleDynamicModules(modules, modperms, country)
    .slice()
    .sort((a, b) => a.name.localeCompare(b.name))
    .map((m) => ({
      key: `module:${m.slug}`,
      href: `/modules/${m.slug}`,
      label: m.name,
      description: m.menuPath ? m.menuPath : "Módulo del panel.",
      icon: DYNAMIC_MODULE_ICON,
    }));

  return [...fixed, ...reserved, ...dynamic];
}
