"use client";

/**
 * Vista de Plans (listar + eliminar). Crear y editar viven ahora en páginas
 * propias (`/plans/new`, `/plans/edit`), consistentes con Customers y
 * Subscriptions; ya NO se usa un panel modal para el formulario.
 *
 * - `PlansTable`: listado con búsqueda/paginación/orden (Req. 9). Para admin, el
 *   botón "Crear" y las acciones de fila Editar/Eliminar navegan o abren la
 *   confirmación de borrado.
 * - `ConfirmDeleteDialog`: confirmación previa a `useDeletePlan` (Req. 12).
 * - `FeedbackBanner`: mensaje de éxito al volver de crear/editar (?created/?updated)
 *   y de eliminar.
 */
import * as React from "react";
import { useRouter, useSearchParams } from "next/navigation";

import { isApiError, type ApiError } from "@/lib/api-client";
import { planId, useDeletePlan, type Plan } from "@/features/plans/hooks";
import { PlansTable } from "@/features/plans/components/plans-table";
import { ConfirmDeleteDialog } from "@/features/plans/components/confirm-delete-dialog";
import { usePageTitle } from "@/lib/page-chrome";
import {
  FeedbackBanner,
  type FeedbackTone,
} from "@/features/plans/components/feedback-banner";

interface Feedback {
  readonly tone: FeedbackTone;
  readonly message: string;
}

function errorMessage(error: unknown, fallback: string): string {
  if (isApiError(error)) {
    return (error as ApiError).message || fallback;
  }
  if (error instanceof Error && error.message) {
    return error.message;
  }
  return fallback;
}

/**
 * Banner de éxito al volver de las rutas de crear/editar con `?created=1` o
 * `?updated=1`. Lee `useSearchParams`, por eso vive en un subcomponente envuelto
 * en <Suspense> (requisito del App Router).
 */
function PlansSuccessBanner() {
  const router = useRouter();
  const searchParams = useSearchParams();

  let message: string | null = null;
  if (searchParams.get("created") === "1") {
    message = "Plan creado correctamente.";
  } else if (searchParams.get("updated") === "1") {
    message = "Plan actualizado correctamente.";
  } else if (searchParams.get("deleted") === "1") {
    message = "Plan eliminado correctamente.";
  }

  if (message === null) {
    return null;
  }

  return (
    <FeedbackBanner
      tone="success"
      message={message}
      onDismiss={() => router.replace("/plans")}
    />
  );
}

export default function PlansPage() {
  usePageTitle("Plans");
  const router = useRouter();
  const [planToDelete, setPlanToDelete] = React.useState<Plan | null>(null);
  const [feedback, setFeedback] = React.useState<Feedback | null>(null);
  const [deleteError, setDeleteError] = React.useState<string | null>(null);

  const deleteMutation = useDeletePlan();

  const handleConfirmDelete = React.useCallback(() => {
    if (!planToDelete) {
      return;
    }
    const id = planId(planToDelete);
    if (id === "") {
      setDeleteError("El plan no tiene un identificador válido para eliminar.");
      return;
    }
    setDeleteError(null);
    deleteMutation.mutate(id, {
      onSuccess: () => {
        setFeedback({ tone: "success", message: "Plan eliminado correctamente." });
        setPlanToDelete(null);
      },
      onError: (error) =>
        setDeleteError(errorMessage(error, "No se pudo eliminar el plan.")),
    });
  }, [planToDelete, deleteMutation]);

  const handleCancelDelete = React.useCallback(() => {
    setPlanToDelete(null);
    setDeleteError(null);
  }, []);

  return (
    <div className="mx-auto w-full max-w-5xl space-y-6">
      <React.Suspense fallback={null}>
        <PlansSuccessBanner />
      </React.Suspense>

      {feedback ? (
        <FeedbackBanner
          tone={feedback.tone}
          message={feedback.message}
          onDismiss={() => setFeedback(null)}
        />
      ) : null}

      <PlansTable
        onCreate={() => router.push("/plans/new")}
        onEdit={(plan) => {
          const id = planId(plan);
          router.push(`/plans/edit?plan_id=${encodeURIComponent(id)}`);
        }}
        onDelete={(plan) => {
          setDeleteError(null);
          setPlanToDelete(plan);
        }}
      />

      <ConfirmDeleteDialog
        open={planToDelete !== null}
        planName={planToDelete?.name}
        isPending={deleteMutation.isPending}
        errorMessage={deleteError}
        onConfirm={handleConfirmDelete}
        onCancel={handleCancelDelete}
      />
    </div>
  );
}
