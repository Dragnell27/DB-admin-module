"use client";

/**
 * Botón GLOBAL de recarga de la barra superior.
 *
 * Reemplaza al antiguo botón "Recargar permisos" que vivía dentro de cada vista
 * de módulo. SOLO ÍCONO (`RefreshCw` de lucide), con `aria-label`/`title`
 * descriptivos, reutilizando el MISMO patrón visual que `ThemeToggle`
 * (`variant="ghost" size="icon"`, colores del tema vía `buttonVariants`, nada de
 * estilos ad-hoc), por lo que es legible en claro y oscuro.
 *
 * Al pulsarlo ejecuta `useGlobalRefresh().refresh()`, que (a) reemite y persiste
 * el JWT con los permisos actuales —propagando permisos/rol a la UI vía el bus
 * de sesión— y (b) invalida la query del menú lateral (`GET /modules`), de modo
 * que un módulo recién creado aparezca en el sidebar SIN recargar el navegador y
 * MANTENIENDO la ruta actual (refetch suave). Durante la operación el ícono gira
 * (`animate-spin`) y el botón queda deshabilitado; el resultado se comunica con
 * un toast de éxito o de error, sin romper la vista.
 */
import { RefreshCw } from "lucide-react";

import { Button } from "@/components/ui/button";
import { useGlobalRefresh } from "@/features/modules/hooks";
import { toast } from "@/lib/use-toast";

const LABEL = "Recargar permisos y módulos";

export function RefreshButton() {
  const { refresh, isPending } = useGlobalRefresh();

  async function onClick(): Promise<void> {
    try {
      await refresh();
      toast.success("Permisos y módulos actualizados");
    } catch (err) {
      const message =
        err && typeof err === "object" && "message" in err
          ? String((err as { message?: unknown }).message ?? "")
          : "";
      toast.error(message || "No se pudieron recargar permisos y módulos.");
    }
  }

  return (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      onClick={onClick}
      disabled={isPending}
      aria-label={LABEL}
      title={LABEL}
    >
      <RefreshCw
        className={isPending ? "size-4 animate-spin" : "size-4"}
        aria-hidden="true"
      />
    </Button>
  );
}

export default RefreshButton;
