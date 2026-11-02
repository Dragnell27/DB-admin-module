"use client";

/**
 * Botón "Recargar" para las vistas de recursos cacheables (customers, plans,
 * subscriptions).
 *
 * Fuerza el refresco del caché del BFF (POST /{resource}/refresh) y, al
 * terminar, invalida el listado de la vista actual para mostrar datos frescos
 * de OTT. Solo se muestra si el usuario tiene el permiso `{resource}:recargar`
 * en el país seleccionado (la seguridad real la aplica el BFF; esto es
 * visibilidad de UI).
 */
import * as React from "react";
import { RefreshCcw } from "lucide-react";

import { Button, type ButtonProps } from "@/components/ui/button";
import { can, type Resource } from "@/lib/permissions";
import { usePermissions } from "@/lib/use-permissions";
import { getCountry, subscribeCountry } from "@/lib/country";
import {
  useRefreshResource,
  type RefreshableResource,
} from "@/features/refresh/use-refresh-resource";

/** Lee el país seleccionado de forma reactiva y SSR-safe (igual que el selector). */
function useSelectedCountry(): string {
  return React.useSyncExternalStore(
    subscribeCountry,
    () => getCountry() ?? "",
    () => "",
  );
}

export interface ReloadButtonProps
  extends Omit<ButtonProps, "onClick" | "children"> {
  /** Recurso cacheable cuyo caché se refresca. */
  readonly resource: RefreshableResource;
  /** Callback opcional tras un refresco exitoso (p. ej. mostrar un aviso). */
  readonly onRefreshed?: (count: number) => void;
}

/**
 * Botón que refresca el caché del recurso. No renderiza nada si el usuario no
 * tiene permiso `recargar` sobre el recurso en el país seleccionado.
 */
export function ReloadButton({
  resource,
  onRefreshed,
  variant = "outline",
  size = "sm",
  ...props
}: ReloadButtonProps) {
  const perms = usePermissions();
  const country = useSelectedCountry();
  const refresh = useRefreshResource(resource);

  // Visibilidad por permiso: `{resource}:recargar` en el país seleccionado.
  if (!can(perms, resource as Resource, "recargar", country)) {
    return null;
  }

  return (
    <Button
      type="button"
      variant={variant}
      size={size}
      disabled={refresh.isPending}
      aria-label="Recargar datos"
      onClick={() => {
        refresh.mutate(undefined, {
          onSuccess: (result) => onRefreshed?.(result.count),
        });
      }}
      {...props}
    >
      <RefreshCcw
        aria-hidden="true"
        className={refresh.isPending ? "animate-spin" : undefined}
      />
      {refresh.isPending ? "Recargando..." : "Recargar"}
    </Button>
  );
}

export default ReloadButton;
