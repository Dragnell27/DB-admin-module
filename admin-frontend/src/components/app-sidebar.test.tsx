// @vitest-environment jsdom

// Pruebas de componente de `AppSidebar` (tarea 22, checkpoint admin-rbac).
// El menú lateral pasó de ser por rol a ser por permisos (Req. 9.2, 9.3): cada
// entrada sólo aparece si los Permisos_Efectivos incluyen la acción `ver` sobre
// el recurso del módulo en el país seleccionado. Estas pruebas validan la
// visibilidad por permisos, el distintivo "Solo lectura" para `viewer` y el
// resaltado de la ruta activa (Req. 4.1, 4.2).

import "@testing-library/jest-dom/vitest";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";

import type { EffectivePermissions, Resource } from "@/lib/permissions";
import type { Role } from "@/lib/session";

// --- Mocks de dependencias del componente -----------------------------------

const usePathnameMock = vi.fn<() => string>();

vi.mock("next/navigation", () => ({
  usePathname: () => usePathnameMock(),
}));

const useRoleMock = vi.fn<() => Role | null>();

vi.mock("@/lib/use-role", () => ({
  useRole: () => useRoleMock(),
}));

const usePermissionsMock = vi.fn<() => EffectivePermissions>();
const useModulePermissionsMock = vi.fn<() => ReadonlyArray<{ slug: string; action: string; country: string }>>();

vi.mock("@/lib/use-permissions", () => ({
  usePermissions: () => usePermissionsMock(),
  // El sidebar también lee permisos de módulos dinámicos; por defecto vacío.
  useModulePermissions: () => useModulePermissionsMock(),
}));

// Descubrimiento de módulos dinámicos (module-builder): configurable por prueba.
const useVisibleModulesMock = vi.fn<() => { data: ReadonlyArray<{ slug: string; name: string; menuPath?: string }> }>();

vi.mock("@/features/modules/hooks", () => ({
  useVisibleModules: () => useVisibleModulesMock(),
}));

import { __resetFoldersCacheForTests } from "@/lib/sidebar-folders";
import {
  __resetCollapseCacheForTests,
  SIDEBAR_COLLAPSE_KEY,
} from "@/lib/sidebar-collapse";

vi.mock("@/lib/country", () => ({
  // País fijo en las pruebas: el componente lo lee vía useSyncExternalStore.
  getCountry: () => "gt",
  // Suscripción no-op que devuelve una función de desuscripción.
  subscribeCountry: () => () => {},
}));

// Se importa DESPUÉS de declarar los mocks para que el componente los use.
import { AppSidebar } from "@/components/app-sidebar";

// --- Fixtures ----------------------------------------------------------------

const ALL_RESOURCES: readonly Resource[] = [
  "customers",
  "plans",
  "subscriptions",
  "cards",
  "users",
  "roles",
  "audit",
];

/** Construye permisos que conceden `ver` sobre los recursos indicados en "gt". */
function verPermsFor(resources: readonly Resource[]): EffectivePermissions {
  return resources.map((resource) => ({
    resource,
    action: "ver" as const,
    country: "gt",
  }));
}

/** Localiza la navegación de recursos (`aria-label="Recursos"`). */
function recursosNav(): HTMLElement {
  return screen.getByRole("navigation", { name: "Recursos" });
}

beforeEach(() => {
  usePathnameMock.mockReturnValue("/customers");
  useRoleMock.mockReturnValue("admin");
  usePermissionsMock.mockReturnValue([]);
  useModulePermissionsMock.mockReturnValue([]);
  useVisibleModulesMock.mockReturnValue({ data: [] });
  // Estado de menú contraído aislado por prueba: expandido por defecto.
  window.localStorage.removeItem(SIDEBAR_COLLAPSE_KEY);
  __resetCollapseCacheForTests();
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  window.localStorage.removeItem(SIDEBAR_COLLAPSE_KEY);
  __resetCollapseCacheForTests();
});

describe("AppSidebar — visibilidad por permisos", () => {
  it("muestra todos los enlaces cuando el usuario puede ver todos los recursos en gt", () => {
    usePermissionsMock.mockReturnValue(verPermsFor(ALL_RESOURCES));

    render(<AppSidebar />);

    const nav = recursosNav();
    const links = within(nav).getAllByRole("link");
    expect(links).toHaveLength(7);

    for (const label of [
      "Customers",
      "Plans",
      "Subscriptions",
      "Cards",
      "Usuarios",
      "Roles",
      "Auditoría",
    ]) {
      expect(within(nav).getByRole("link", { name: label })).toBeInTheDocument();
    }
  });

  it("no muestra ningún enlace cuando no hay permisos", () => {
    usePermissionsMock.mockReturnValue([]);

    render(<AppSidebar />);

    const nav = recursosNav();
    expect(within(nav).queryAllByRole("link")).toHaveLength(0);
  });

  it("muestra sólo los recursos con permiso `ver` en gt", () => {
    usePermissionsMock.mockReturnValue(verPermsFor(["customers", "plans"]));

    render(<AppSidebar />);

    const nav = recursosNav();
    expect(within(nav).getAllByRole("link")).toHaveLength(2);
    expect(within(nav).getByRole("link", { name: "Customers" })).toBeInTheDocument();
    expect(within(nav).getByRole("link", { name: "Plans" })).toBeInTheDocument();

    for (const label of ["Subscriptions", "Cards", "Usuarios", "Roles", "Auditoría"]) {
      expect(within(nav).queryByRole("link", { name: label })).not.toBeInTheDocument();
    }
  });
});

describe("AppSidebar — distintivo de rol", () => {
  it("muestra \"Solo lectura\" cuando el rol es viewer", () => {
    useRoleMock.mockReturnValue("viewer");
    usePermissionsMock.mockReturnValue(verPermsFor(ALL_RESOURCES));

    render(<AppSidebar />);

    expect(screen.getByText("Solo lectura")).toBeInTheDocument();
  });

  it("no muestra \"Solo lectura\" cuando el rol es admin", () => {
    useRoleMock.mockReturnValue("admin");
    usePermissionsMock.mockReturnValue(verPermsFor(ALL_RESOURCES));

    render(<AppSidebar />);

    expect(screen.queryByText("Solo lectura")).not.toBeInTheDocument();
  });

  it("no muestra \"Solo lectura\" cuando no hay rol (null)", () => {
    useRoleMock.mockReturnValue(null);
    usePermissionsMock.mockReturnValue(verPermsFor(ALL_RESOURCES));

    render(<AppSidebar />);

    expect(screen.queryByText("Solo lectura")).not.toBeInTheDocument();
  });
});

describe("AppSidebar — entrada Conexiones (connection-templates)", () => {
  it("muestra 'Conexiones' solo con canModule(__connections__, ver)", () => {
    useModulePermissionsMock.mockReturnValue([
      { slug: "__connections__", action: "ver", country: "gt" },
    ]);
    render(<AppSidebar />);
    const nav = recursosNav();
    expect(within(nav).getByRole("link", { name: "Conexiones" })).toHaveAttribute(
      "href",
      "/connection-templates",
    );
  });

  it("no muestra 'Conexiones' sin el permiso", () => {
    useModulePermissionsMock.mockReturnValue([]);
    render(<AppSidebar />);
    const nav = recursosNav();
    expect(within(nav).queryByRole("link", { name: "Conexiones" })).not.toBeInTheDocument();
  });
});

describe("AppSidebar — módulos agrupados por menu_path (carpetas contraíbles)", () => {
  beforeEach(() => {
    // Estado de carpetas aislado por prueba: todo contraído de inicio.
    window.localStorage.clear();
    __resetFoldersCacheForTests();
    useModulePermissionsMock.mockReturnValue([
      { slug: "recaudo-gt", action: "ver", country: "gt" },
      { slug: "recaudo-sv", action: "ver", country: "gt" },
    ]);
    useVisibleModulesMock.mockReturnValue({
      data: [
        { slug: "recaudo-gt", name: "Recaudo GT", menuPath: "CENAM/Recaudo/GT" },
        { slug: "recaudo-sv", name: "Recaudo SV", menuPath: "CENAM/Recaudo/SV" },
      ],
    });
  });

  afterEach(() => {
    __resetFoldersCacheForTests();
  });

  it("arranca con las carpetas contraídas: solo la raíz y sin hojas visibles", () => {
    render(<AppSidebar />);
    const nav = recursosNav();
    // La carpeta raíz se muestra como botón contraído.
    const cenam = within(nav).getByRole("button", { name: "CENAM" });
    expect(cenam).toHaveAttribute("aria-expanded", "false");
    // Las subcarpetas y las hojas no se renderizan mientras está contraída.
    expect(within(nav).queryByText("Recaudo")).not.toBeInTheDocument();
    expect(
      within(nav).queryByRole("link", { name: "Recaudo GT" }),
    ).not.toBeInTheDocument();
  });

  it("al expandir las carpetas revela las subcarpetas y las hojas enlazadas", () => {
    render(<AppSidebar />);
    const nav = recursosNav();
    // Expandir CENAM → aparece la subcarpeta Recaudo (aún contraída).
    fireEvent.click(within(nav).getByRole("button", { name: "CENAM" }));
    expect(within(nav).getByRole("button", { name: "CENAM" })).toHaveAttribute(
      "aria-expanded",
      "true",
    );
    const recaudo = within(nav).getByRole("button", { name: "Recaudo" });
    expect(recaudo).toHaveAttribute("aria-expanded", "false");
    // Expandir Recaudo → aparecen las subcarpetas de país (GT/SV), aún contraídas.
    fireEvent.click(recaudo);
    const gt = within(nav).getByRole("button", { name: "GT" });
    expect(gt).toHaveAttribute("aria-expanded", "false");
    // Expandir GT → aparece la hoja enlazada del módulo.
    fireEvent.click(gt);
    expect(within(nav).getByRole("link", { name: "Recaudo GT" })).toHaveAttribute(
      "href",
      "/modules/recaudo-gt",
    );
    // Expandir SV → aparece su hoja.
    fireEvent.click(within(nav).getByRole("button", { name: "SV" }));
    expect(within(nav).getByRole("link", { name: "Recaudo SV" })).toHaveAttribute(
      "href",
      "/modules/recaudo-sv",
    );
  });

  it("auto-expande solo la cadena de ancestros de la ruta activa", () => {
    usePathnameMock.mockReturnValue("/modules/recaudo-gt");
    render(<AppSidebar />);
    const nav = recursosNav();
    // Sin interacción, la hoja activa es visible porque sus ancestros
    // (CENAM → Recaudo → GT) se auto-expanden.
    expect(within(nav).getByRole("link", { name: "Recaudo GT" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    // La carpeta hermana «SV» NO es ancestro de la ruta activa, así que queda
    // contraída y su hoja no se renderiza.
    expect(within(nav).getByRole("button", { name: "SV" })).toHaveAttribute(
      "aria-expanded",
      "false",
    );
    expect(
      within(nav).queryByRole("link", { name: "Recaudo SV" }),
    ).not.toBeInTheDocument();
  });
});

describe("AppSidebar — menú contraído (riel solo con iconos)", () => {
  beforeEach(() => {
    // Sembrar el estado contraído directamente en localStorage y releerlo.
    window.localStorage.setItem(SIDEBAR_COLLAPSE_KEY, JSON.stringify(true));
    __resetCollapseCacheForTests();
    useModulePermissionsMock.mockReturnValue([
      { slug: "recaudo-gt", action: "ver", country: "gt" },
    ]);
    useVisibleModulesMock.mockReturnValue({
      data: [{ slug: "recaudo-gt", name: "Recaudo GT", menuPath: "CENAM/Recaudo/GT" }],
    });
  });

  it("mantiene los enlaces/iconos de navegación pero oculta etiquetas y encabezados", () => {
    usePermissionsMock.mockReturnValue(verPermsFor(["customers", "plans"]));

    render(<AppSidebar />);

    const nav = recursosNav();
    // Los enlaces de recursos siguen presentes (RBAC intacto), con 2 visibles.
    expect(within(nav).getAllByRole("link").length).toBeGreaterThanOrEqual(2);
    // Pero las etiquetas de texto y los encabezados de grupo quedan ocultos.
    expect(within(nav).queryByText("Customers")).not.toBeInTheDocument();
    expect(within(nav).queryByText("Plans")).not.toBeInTheDocument();
    expect(within(nav).queryByText("Operación")).not.toBeInTheDocument();
    expect(within(nav).queryByText("Administración")).not.toBeInTheDocument();
  });

  it("preserva el resaltado de la ruta activa (aria-current) en el riel", () => {
    usePathnameMock.mockReturnValue("/customers");
    usePermissionsMock.mockReturnValue(verPermsFor(["customers", "plans"]));

    render(<AppSidebar />);

    const nav = recursosNav();
    const active = within(nav)
      .getAllByRole("link")
      .find((el) => el.getAttribute("aria-current") === "page");
    expect(active).toBeTruthy();
    expect(active).toHaveAttribute("href", "/customers");
  });

  it("no renderiza el distintivo 'Solo lectura' ni el árbol de módulos al estar contraído", () => {
    useRoleMock.mockReturnValue("viewer");
    usePathnameMock.mockReturnValue("/modules/recaudo-gt");
    usePermissionsMock.mockReturnValue(verPermsFor(["customers"]));

    render(<AppSidebar />);

    // El distintivo de rol se oculta en el riel contraído.
    expect(screen.queryByText("Solo lectura")).not.toBeInTheDocument();
    const nav = recursosNav();
    // El árbol de carpetas de módulos no se renderiza (ni carpetas ni hojas).
    expect(within(nav).queryByRole("button", { name: "CENAM" })).not.toBeInTheDocument();
    expect(
      within(nav).queryByRole("link", { name: "Recaudo GT" }),
    ).not.toBeInTheDocument();
  });
});

describe("AppSidebar — ruta activa", () => {
  it("marca aria-current=page sólo en el enlace de la ruta activa", () => {
    usePathnameMock.mockReturnValue("/customers");
    usePermissionsMock.mockReturnValue(verPermsFor(ALL_RESOURCES));

    render(<AppSidebar />);

    const nav = recursosNav();
    expect(within(nav).getByRole("link", { name: "Customers" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(within(nav).getByRole("link", { name: "Plans" })).not.toHaveAttribute(
      "aria-current",
    );
  });
});
