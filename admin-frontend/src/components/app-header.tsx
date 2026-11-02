"use client";

/**
 * Barra superior del panel protegido (Req. 2.4, 4.3, 5.1).
 * (RBAC: muestra los nombres de rol asignados y el selector de país.)
 *
 * Presenta la identidad del usuario autenticado (username y rol, derivados de
 * los claims del JWT de sesión vía `getSessionUser()`) y el control de cierre
 * de sesión (`LogoutButton`, Req. 2.4). Comparte la paleta oscura del panel
 * (Req. 4.3).
 *
 * El usuario se resuelve desde `localStorage` en el cliente, por lo que se lee
 * tras montar para evitar desajustes de hidratación (el render del servidor y
 * el primer render del cliente devuelven el estado vacío). Reacciona al evento
 * `storage` para reflejar login/logout en otras pestañas.
 */
import { useEffect, useState, useSyncExternalStore } from "react";
import { usePathname } from "next/navigation";
import { PanelLeft } from "lucide-react";

import { Button } from "@/components/ui/button";
import { LogoutButton } from "@/components/logout-button";
import { CountrySelector } from "@/components/country-selector";
import { ThemeToggle } from "@/components/theme-toggle";
import { showsCountrySelector } from "@/lib/ott-routes";
import { usePageTitleValue } from "@/lib/page-chrome";
import {
  getCollapsed,
  subscribeCollapsed,
  toggleCollapsed,
} from "@/lib/sidebar-collapse";
import {
  SESSION_TOKEN_KEY,
  getSessionUser,
  type SessionUser,
} from "@/lib/session";

/**
 * Etiqueta legible de los roles para la cabecera. Prioriza los nombres de los
 * roles RBAC asignados (claim `roles`); si no hay, cae al rol legacy
 * admin/viewer por compatibilidad; si tampoco, no muestra nada.
 */
function rolesLabel(user: SessionUser): string | null {
  if (user.roles.length > 0) {
    return user.roles.join(", ");
  }
  if (user.role === "admin") {
    return "Administrador";
  }
  if (user.role === "viewer") {
    return "Solo lectura";
  }
  return null;
}

/**
 * Barra superior con el usuario actual y el botón de cierre de sesión.
 */
export function AppHeader() {
  // Primer render (incl. SSR): sin usuario, para coincidir con la hidratación.
  const [user, setUser] = useState<SessionUser | null>(null);

  useEffect(() => {
    setUser(getSessionUser());

    function handleStorage(event: StorageEvent): void {
      if (event.key === null || event.key === SESSION_TOKEN_KEY) {
        setUser(getSessionUser());
      }
    }

    window.addEventListener("storage", handleStorage);
    return () => window.removeEventListener("storage", handleStorage);
  }, []);

  const label = user ? rolesLabel(user) : null;

  // Estado contraído del menú lateral global (SSR-safe: expandido en el primer
  // render). El botón alterna y persiste el estado para todo el panel.
  const collapsed = useSyncExternalStore(
    subscribeCollapsed,
    getCollapsed,
    () => false,
  );

  // Título contextual de la vista activa: lo declara cada página con
  // `usePageTitle(...)` y aquí se pinta como el ÚNICO <h1> de la vista.
  const pageTitle = usePageTitleValue();

  // El selector de país SOLO aplica a los recursos OTT por país. La ruta decide.
  const pathname = usePathname();
  const showCountry = showsCountrySelector(pathname);

  return (
    <header className="flex h-16 shrink-0 items-center justify-between gap-4 border-b border-border bg-card px-6">
      {/* Lado izquierdo: botón de contraer/expandir el menú y el TÍTULO de la
          vista activa (ocupa el espacio que antes gastaba el cuerpo). */}
      <div className="flex min-w-0 items-center gap-3">
        <Button
          type="button"
          variant="ghost"
          size="icon"
          onClick={() => toggleCollapsed()}
          aria-expanded={!collapsed}
          aria-label={collapsed ? "Expandir menú" : "Contraer menú"}
        >
          <PanelLeft className="size-4" aria-hidden="true" />
        </Button>
        {pageTitle ? (
          <h1 className="truncate text-lg font-semibold text-foreground">
            {pageTitle}
          </h1>
        ) : null}
      </div>

      <div className="flex items-center gap-4">
        {/* Selector de país: solo en recursos OTT por país. */}
        {showCountry ? <CountrySelector /> : null}

        <ThemeToggle />

        {user?.username ? (
          <div className="flex flex-col items-end leading-tight">
            <span className="truncate text-sm font-medium text-foreground">
              {user.username}
            </span>
            {label ? (
              <span className="text-xs text-muted-foreground">{label}</span>
            ) : null}
          </div>
        ) : null}

        <LogoutButton variant="outline" size="sm" />
      </div>
    </header>
  );
}

export default AppHeader;
