"use client";

/**
 * Vista de DETALLE de un Plan. Punto de entrada del flujo "Ver": la tabla enlaza
 * aquí con `plan_id` en la query. Muestra el Plan ID destacado y los campos del
 * plan. Desde aquí se accede a "Editar" (permiso `plans:editar`) y "Eliminar"
 * (permiso `plans:eliminar`, con confirmación), según el país seleccionado.
 *
 * Envuelta en <Suspense> por el uso de `useSearchParams`.
 */
import * as React from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { ArrowLeft, Loader2, Pencil, Trash2 } from "lucide-react";

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
import { ConfirmDeleteDialog } from "@/features/plans/components/confirm-delete-dialog";
import { usePlan, useDeletePlan, planId } from "@/features/plans/hooks";
import { usePermissions } from "@/lib/use-permissions";
import { can } from "@/lib/permissions";
import { getCountry, subscribeCountry } from "@/lib/country";
import { isApiError } from "@/lib/api-client";

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

function errorMessage(error: unknown, fallback: string): string {
  if (isApiError(error)) {
    return error.message || fallback;
  }
  if (error instanceof Error && error.message) {
    return error.message;
  }
  return fallback;
}

function PlanDetailContent() {
  const searchParams = useSearchParams();
  const router = useRouter();
  const perms = usePermissions();
  const country = useSelectedCountry();

  const id = searchParams.get("plan_id") ?? undefined;
  const hasId = Boolean(id);

  const { data: plan, isLoading, isError, error } = usePlan(id, {
    enabled: hasId,
  });

  const canEditar = can(perms, "plans", "editar", country);
  const canEliminar = can(perms, "plans", "eliminar", country);

  const [confirmOpen, setConfirmOpen] = React.useState(false);
  const [deleteError, setDeleteError] = React.useState<string | null>(null);
  const deleteMutation = useDeletePlan();

  const handleConfirmDelete = React.useCallback(() => {
    if (!id) {
      return;
    }
    setDeleteError(null);
    deleteMutation.mutate(id, {
      onSuccess: () => {
        setConfirmOpen(false);
        router.replace("/plans?deleted=1");
      },
      onError: (err) =>
        setDeleteError(errorMessage(err, "No se pudo eliminar el plan.")),
    });
  }, [id, deleteMutation, router]);

  if (!hasId) {
    return (
      <FeedbackBanner
        tone="error"
        message="No se indicó el plan a mostrar. Vuelve al listado y selecciona uno."
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
        Cargando plan...
      </div>
    );
  }

  if (isError || !plan) {
    return (
      <FeedbackBanner
        tone="error"
        message={error?.message ?? "No se pudo cargar el plan. Intenta de nuevo."}
      />
    );
  }

  const resolvedId = planId(plan) || id || "(sin plan_id)";
  const amount =
    plan.amount === undefined || plan.amount === null
      ? undefined
      : `${plan.amount} ${plan.currency ? String(plan.currency).toUpperCase() : ""}`.trim();

  return (
    <div className="space-y-5">
      <CopyableId label="Plan ID" value={resolvedId} />

      <Card>
        <CardHeader className="flex flex-row items-center justify-between gap-4 space-y-0">
          <CardTitle className="text-lg">Datos del plan</CardTitle>
          <div className="flex items-center gap-2">
            {canEditar ? (
              <Button asChild size="sm">
                <Link
                  href={`/plans/edit?plan_id=${encodeURIComponent(resolvedId)}`}
                >
                  <Pencil aria-hidden="true" />
                  Editar
                </Link>
              </Button>
            ) : null}
            {canEliminar ? (
              <Button
                type="button"
                variant="destructive"
                size="sm"
                onClick={() => {
                  setDeleteError(null);
                  setConfirmOpen(true);
                }}
              >
                <Trash2 aria-hidden="true" />
                Eliminar
              </Button>
            ) : null}
          </div>
        </CardHeader>
        <CardContent>
          <dl className="grid grid-cols-1 gap-x-8 gap-y-5 sm:grid-cols-2">
            <DetailField label="Nombre" value={asText(plan.name)} />
            <DetailField
              label="External ID"
              value={asText(plan.external_id)}
              mono
            />
            <DetailField
              label="País"
              value={plan.country ? String(plan.country).toUpperCase() : undefined}
            />
            <DetailField label="Monto" value={amount} />
          </dl>
        </CardContent>
      </Card>

      <ConfirmDeleteDialog
        open={confirmOpen}
        planName={asText(plan.name)}
        isPending={deleteMutation.isPending}
        errorMessage={deleteError}
        onConfirm={handleConfirmDelete}
        onCancel={() => {
          if (!deleteMutation.isPending) {
            setConfirmOpen(false);
            setDeleteError(null);
          }
        }}
      />
    </div>
  );
}

export default function PlanDetailPage() {
  return (
    <div className="mx-auto w-full max-w-3xl space-y-4">
      <Button asChild variant="ghost" size="sm" className="-ml-2">
        <Link href="/plans">
          <ArrowLeft aria-hidden="true" />
          Volver a Plans
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
        <PlanDetailContent />
      </React.Suspense>
    </div>
  );
}
