"use client";

/**
 * Vista de la bitácora de auditoría (audit-log, tarea 10).
 *
 * Muestra el historial de operaciones de escritura del panel (quién hizo qué,
 * sobre qué objeto, cuándo, en qué país y con qué resultado), con filtros por id
 * de objeto (principal), recurso, usuario y rango de fechas, más el detalle del
 * payload registrado.
 *
 * Visibilidad de UI: la vista solo se muestra a usuarios con el permiso
 * `audit:ver` en el país seleccionado; en caso contrario se presenta un aviso de
 * acceso restringido. La autorización REAL la aplica el BFF
 * (`RequirePermission(audit, ver)` sobre `GET /audit-logs`); esto es solo UI.
 */
import * as React from "react";

import { Card, CardContent } from "@/components/ui/card";
import { getCountry, subscribeCountry } from "@/lib/country";
import { can } from "@/lib/permissions";
import { usePermissions } from "@/lib/use-permissions";
import { AuditTable } from "@/features/audit/components/audit-table";

/** Lee el país seleccionado de forma reactiva y SSR-safe (igual que el selector). */
function useSelectedCountry(): string | null {
  return React.useSyncExternalStore(
    subscribeCountry,
    () => getCountry(),
    () => null,
  );
}

export default function AuditPage() {
  const perms = usePermissions();
  const country = useSelectedCountry() ?? "";
  const canVer = can(perms, "audit", "ver", country);

  return (
    <div className="mx-auto w-full max-w-6xl space-y-6">
      <div className="space-y-1">
        <h1 className="text-2xl font-semibold text-foreground">Auditoría</h1>
        <p className="text-sm text-muted-foreground">
          Consulta el historial de creaciones, ediciones y eliminaciones del
          panel. Filtra por el id de cada objeto para seguir su rastro.
        </p>
      </div>

      {canVer ? (
        <AuditTable />
      ) : (
        <Card>
          <CardContent className="pt-6">
            <p className="text-sm text-muted-foreground">
              No tienes permiso para consultar la bitácora de auditoría en el
              país seleccionado.
            </p>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
