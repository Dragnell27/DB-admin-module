"use client";

/**
 * Vista de DETALLE de un Customer. Punto de entrada del flujo "Ver": la tabla
 * enlaza aquí con `email`/`external_id`/`customer_id` en la query. Muestra el
 * ID del cliente destacado y sus campos de forma legible. Desde aquí se accede
 * a "Editar" (si el usuario tiene permiso `customers:editar` en el país
 * seleccionado). Customers NO soporta eliminación (el contrato OTT no la
 * permite), por lo que no se ofrece "Eliminar".
 *
 * Envuelta en <Suspense> por el uso de `useSearchParams`.
 */
import * as React from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { ArrowLeft, Loader2, Pencil } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { FeedbackBanner } from "@/components/feedback-banner";
import { CopyableId } from "@/components/copyable-id";
import { DetailField } from "@/components/detail-field";
import { useCustomer, type CustomerLocator } from "@/features/customers/hooks";
import { usePermissions } from "@/lib/use-permissions";
import { can } from "@/lib/permissions";
import { getCountry, subscribeCountry } from "@/lib/country";

/** Lee el país seleccionado de forma reactiva y SSR-safe (igual que el selector). */
function useSelectedCountry(): string {
  return React.useSyncExternalStore(
    subscribeCountry,
    () => getCountry() ?? "",
    () => "",
  );
}

/** Normaliza un valor desconocido del detalle a string para mostrarlo. */
function asText(value: unknown): string | undefined {
  if (typeof value === "string") {
    return value;
  }
  if (typeof value === "number" || typeof value === "boolean") {
    return String(value);
  }
  return undefined;
}

/** Construye la URL de edición preservando el localizador y el origen. */
function buildEditHref(locator: CustomerLocator): string {
  const params = new URLSearchParams();
  if (locator.email) {
    params.set("email", locator.email);
  }
  if (locator.external_id) {
    params.set("external_id", locator.external_id);
  }
  params.set("from", "/customers");
  return `/customers/edit?${params.toString()}`;
}

function CustomerDetailContent() {
  const searchParams = useSearchParams();
  const perms = usePermissions();
  const country = useSelectedCountry();

  const email = searchParams.get("email") ?? undefined;
  const externalId = searchParams.get("external_id") ?? undefined;
  const customerId = searchParams.get("customer_id") ?? undefined;

  const locator: CustomerLocator = React.useMemo(
    () => ({ email, external_id: externalId, customer_id: customerId }),
    [email, externalId, customerId],
  );
  const hasLocator = Boolean(email || externalId || customerId);

  const {
    data: customer,
    isLoading,
    isError,
    error,
  } = useCustomer(locator, { enabled: hasLocator });

  const canEditar = can(perms, "customers", "editar", country);

  if (!hasLocator) {
    return (
      <FeedbackBanner
        tone="error"
        message="No se indicó el cliente a mostrar. Vuelve al listado y selecciona uno."
      />
    );
  }

  if (isLoading) {
    return (
      <div
        className="flex items-center gap-2 text-sm text-muted-foreground"
        role="status"
      >
        <Loader2 className="size-4 animate-spin" aria-hidden="true" />
        Cargando cliente...
      </div>
    );
  }

  if (isError || !customer) {
    return (
      <FeedbackBanner
        tone="error"
        message={error?.message ?? "No se pudo cargar el cliente. Intenta de nuevo."}
      />
    );
  }

  const resolvedId =
    asText(customer.customer_id) ?? customerId ?? "(sin customer_id)";
  const fullName =
    [asText(customer.first_name), asText(customer.last_name)]
      .filter(Boolean)
      .join(" ") || undefined;

  return (
    <div className="space-y-5">
      <CopyableId label="Customer ID" value={resolvedId} />

      <Card>
        <CardHeader className="flex flex-row items-center justify-between gap-4 space-y-0">
          <CardTitle className="text-lg">Datos del cliente</CardTitle>
          {canEditar ? (
            <Button asChild size="sm">
              <Link href={buildEditHref(locator)}>
                <Pencil aria-hidden="true" />
                Editar
              </Link>
            </Button>
          ) : null}
        </CardHeader>
        <CardContent>
          <dl className="grid grid-cols-1 gap-x-8 gap-y-5 sm:grid-cols-2">
            <DetailField label="Nombre completo" value={fullName} />
            <DetailField label="Correo" value={asText(customer.email)} mono />
            <DetailField
              label="External ID"
              value={asText(customer.external_id)}
              mono
            />
            <DetailField label="Teléfono" value={asText(customer.phone)} />
            <DetailField
              label="Fecha de nacimiento"
              value={asText(customer.birth_date)}
            />
            <DetailField label="Género" value={asText(customer.gender)} />
            <DetailField
              label="Dirección"
              value={asText(customer.address)}
              className="sm:col-span-2"
            />
          </dl>
        </CardContent>
      </Card>

    </div>
  );
}

export default function CustomerDetailPage() {
  return (
    <div className="mx-auto w-full max-w-3xl space-y-4">
      <Button asChild variant="ghost" size="sm" className="-ml-2">
        <Link href="/customers">
          <ArrowLeft aria-hidden="true" />
          Volver a Customers
        </Link>
      </Button>

      <React.Suspense
        fallback={
          <div
            className="flex items-center gap-2 text-sm text-muted-foreground"
            role="status"
          >
            <Loader2 className="size-4 animate-spin" aria-hidden="true" />
            Cargando...
          </div>
        }
      >
        <CustomerDetailContent />
      </React.Suspense>
    </div>
  );
}
