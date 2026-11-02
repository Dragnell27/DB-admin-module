"use client";

/**
 * Vista de DETALLE de una Subscription. Punto de entrada del flujo "Ver": la
 * tabla enlaza aquí con `subscription_id` en la query. Muestra el ID destacado
 * y los campos de la suscripción. Desde aquí se accede a "Cambiar plan"
 * (permiso `subscriptions:cambiar`) y "Cancelar" (permiso
 * `subscriptions:cancelar`), según el país seleccionado.
 *
 * Envuelta en <Suspense> por el uso de `useSearchParams`.
 */
import * as React from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { ArrowLeft, Loader2, Pencil, XCircle } from "lucide-react";

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
import { useSubscription } from "@/features/subscriptions/hooks";
import { usePermissions } from "@/lib/use-permissions";
import { can } from "@/lib/permissions";
import { getCountry, subscribeCountry } from "@/lib/country";

/** Lee el país seleccionado de forma reactiva y SSR-safe. */
function useSelectedCountry(): string {
  return React.useSyncExternalStore(
    subscribeCountry,
    () => getCountry() ?? "",
    () => "",
  );
}

function asText(value: unknown): string | undefined {
  if (typeof value === "string") {
    return value;
  }
  if (typeof value === "number" || typeof value === "boolean") {
    return String(value);
  }
  return undefined;
}

/** Formatea una fecha RFC3339 a DD/MM/AAAA local; "—" si no es parseable. */
function formatDate(value: unknown): string | undefined {
  const raw = asText(value);
  if (!raw) {
    return undefined;
  }
  const d = new Date(raw);
  if (Number.isNaN(d.getTime())) {
    return raw;
  }
  return d.toLocaleDateString("es-GT", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  });
}

function SubscriptionDetailContent() {
  const searchParams = useSearchParams();
  const perms = usePermissions();
  const country = useSelectedCountry();

  const id = searchParams.get("subscription_id") ?? undefined;
  const hasId = Boolean(id);

  const { data: sub, isLoading, isError, error } = useSubscription(id, {
    enabled: hasId,
  });

  const canCambiar = can(perms, "subscriptions", "cambiar", country);
  const canCancelar = can(perms, "subscriptions", "cancelar", country);

  if (!hasId) {
    return (
      <FeedbackBanner
        tone="error"
        message="No se indicó la suscripción a mostrar. Vuelve al listado y selecciona una."
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
        Cargando suscripción...
      </div>
    );
  }

  if (isError || !sub) {
    return (
      <FeedbackBanner
        tone="error"
        message={
          error?.message ?? "No se pudo cargar la suscripción. Intenta de nuevo."
        }
      />
    );
  }

  const resolvedId = asText(sub.subscription_id) ?? id ?? "(sin subscription_id)";
  const customerId = asText(sub.customer_id);
  const planId = asText(sub.plan_id);

  // URLs de las acciones, preservando los ids que necesitan.
  const changeHref = (() => {
    const q = new URLSearchParams({ subscription_id: resolvedId });
    if (planId) q.set("plan_id", planId);
    if (customerId) q.set("customer_id", customerId);
    return `/subscriptions/edit?${q.toString()}`;
  })();
  const cancelHref = (() => {
    const q = new URLSearchParams({ subscription_id: resolvedId });
    if (customerId) q.set("customer_id", customerId);
    return `/subscriptions/cancel?${q.toString()}`;
  })();

  return (
    <div className="space-y-5">
      <CopyableId label="Subscription ID" value={resolvedId} />

      <Card>
        <CardHeader className="flex flex-row items-center justify-between gap-4 space-y-0">
          <CardTitle className="text-lg">Datos de la suscripción</CardTitle>
          <div className="flex items-center gap-2">
            {canCambiar ? (
              <Button asChild size="sm">
                <Link href={changeHref}>
                  <Pencil aria-hidden="true" />
                  Cambiar plan
                </Link>
              </Button>
            ) : null}
            {canCancelar ? (
              <Button asChild variant="destructive" size="sm">
                <Link href={cancelHref}>
                  <XCircle aria-hidden="true" />
                  Cancelar
                </Link>
              </Button>
            ) : null}
          </div>
        </CardHeader>
        <CardContent>
          <dl className="grid grid-cols-1 gap-x-8 gap-y-5 sm:grid-cols-2">
            <DetailField label="Customer ID" value={customerId} mono />
            <DetailField label="Plan ID" value={planId} mono />
            <DetailField label="Estado" value={asText(sub.status)} />
            <DetailField
              label="Fecha de activación"
              value={formatDate(sub.subscription_start_date)}
            />
          </dl>
        </CardContent>
      </Card>

    </div>
  );
}

export default function SubscriptionDetailPage() {
  return (
    <div className="mx-auto w-full max-w-3xl space-y-4">
      <Button asChild variant="ghost" size="sm" className="-ml-2">
        <Link href="/subscriptions">
          <ArrowLeft aria-hidden="true" />
          Volver a Subscriptions
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
        <SubscriptionDetailContent />
      </React.Suspense>
    </div>
  );
}
