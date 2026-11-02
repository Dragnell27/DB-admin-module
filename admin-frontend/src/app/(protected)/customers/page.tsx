"use client";

/**
 * Vista de listado de Customers (tarea 16.1, Req. 6).
 *
 * - Muestra la tabla paginada/ordenable/buscable (`CustomersTable`).
 * - Presenta un botón "Crear" que enlaza a `/customers/new`, visible SOLO para
 *   el rol `admin` (Req. 5.1); los `viewer` ven el listado en modo lectura
 *   (Req. 5.2). La seguridad real la aplica el BFF; esto es visibilidad de UI.
 * - Tras crear/editar con éxito, las páginas de formulario redirigen aquí con
 *   `?created=1`/`?updated=1`; mostramos un banner de éxito inline (Req. 7.11,
 *   8.5) sin un proveedor de toasts compartido.
 */
import * as React from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { Plus } from "lucide-react";

import { Button } from "@/components/ui/button";
import { ReloadButton } from "@/components/reload-button";
import { getCountry, subscribeCountry } from "@/lib/country";
import { can } from "@/lib/permissions";
import { usePermissions } from "@/lib/use-permissions";
import { CustomersTable } from "@/features/customers/components/customers-table";
import { FeedbackBanner } from "@/components/feedback-banner";

/** Lee el país seleccionado de forma reactiva y SSR-safe (igual que el selector). */
function useSelectedCountry(): string | null {
  return React.useSyncExternalStore(
    subscribeCountry,
    () => getCountry(),
    () => null,
  );
}

function CustomersSuccessBanner() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const created = searchParams.get("created") === "1";
  const updated = searchParams.get("updated") === "1";

  if (!created && !updated) {
    return null;
  }

  const message = created
    ? "Cliente creado correctamente."
    : "Cliente actualizado correctamente.";

  return (
    <FeedbackBanner
      tone="success"
      message={message}
      onDismiss={() => router.replace("/customers")}
    />
  );
}

export default function CustomersPage() {
  const perms = usePermissions();
  const country = useSelectedCountry() ?? "";
  // Visibilidad de controles de escritura según permiso en el país seleccionado
  // (Req. 9.1, 9.6). La seguridad real la aplica el BFF; esto es solo UI.
  const canCrear = can(perms, "customers", "crear", country);

  return (
    <div className="mx-auto w-full max-w-5xl space-y-6">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
        <div className="space-y-1">
          <h1 className="text-2xl font-semibold text-foreground">Customers</h1>
          <p className="text-sm text-muted-foreground">
            Lista, busca y gestiona los clientes de la plataforma.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <ReloadButton resource="customers" />
          {canCrear ? (
            <Button asChild>
              <Link href="/customers/new">
                <Plus aria-hidden="true" />
                Crear
              </Link>
            </Button>
          ) : null}
        </div>
      </div>

      <React.Suspense fallback={null}>
        <CustomersSuccessBanner />
      </React.Suspense>

      <CustomersTable />
    </div>
  );
}
