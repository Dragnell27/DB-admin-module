"use client";

/**
 * Vista de edición de Plan en página propia (consistente con Customers y
 * Subscriptions). Reemplaza el panel modal anterior.
 *
 * Lee `plan_id` de la query (useSearchParams → Suspense), carga el detalle con
 * `usePlan` y precarga el `PlanForm`. Al guardar usa `useUpdatePlan`; ante éxito
 * vuelve al listado con `?updated=1`. Si falta el `plan_id`, muestra un aviso.
 */
import * as React from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { ArrowLeft, Loader2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { isApiError } from "@/lib/api-client";
import { useRole } from "@/lib/use-role";
import { usePlan, useUpdatePlan } from "@/features/plans/hooks";
import type { PlanCreateInput, PlanEditInput } from "@/features/plans/schema";
import { PlanForm } from "@/features/plans/components/plan-form";
import { FeedbackBanner } from "@/features/plans/components/feedback-banner";

/**
 * Valida el `from` de la query antes de usarlo para navegar: solo acepta rutas
 * internas (empiezan por "/" pero no por "//") para evitar open-redirect; si no
 * es válido usa el `fallback` del recurso.
 */
function safeFrom(from: string | null, fallback: string): string {
  return from && from.startsWith("/") && !from.startsWith("//")
    ? from
    : fallback;
}

/**
 * Enlace "Volver" del encabezado. Lee `from` de la query para regresar al
 * origen (p. ej. `/subscriptions`); por usar `useSearchParams` se renderiza
 * dentro de su propio <Suspense>.
 */
function PlanEditBackButton() {
  const searchParams = useSearchParams();
  const backHref = safeFrom(searchParams.get("from"), "/plans");
  return (
    <Button asChild variant="ghost" size="sm" className="-ml-2">
      <Link href={backHref}>
        <ArrowLeft aria-hidden="true" />
        Volver a Plans
      </Link>
    </Button>
  );
}

function PlanEditContent() {
  const router = useRouter();
  const role = useRole();
  const searchParams = useSearchParams();
  const [serverError, setServerError] = React.useState<string | null>(null);

  const planId = searchParams.get("plan_id") ?? undefined;
  const hasPlan = Boolean(planId);
  // Destino de "Volver"/cancelar: el origen (`from`) si es válido, o el listado.
  const backHref = safeFrom(searchParams.get("from"), "/plans");

  const planQuery = usePlan(planId, { enabled: hasPlan });

  const updatePlan = useUpdatePlan({
    onSuccess: () => {
      router.replace("/plans?updated=1");
    },
    onError: (error) => {
      setServerError(
        isApiError(error)
          ? error.message
          : "No se pudo actualizar el plan. Intenta de nuevo.",
      );
    },
  });

  React.useEffect(() => {
    if (role !== null && role !== "admin") {
      router.replace("/plans");
    }
  }, [role, router]);

  const handleSubmit = React.useCallback(
    (values: PlanCreateInput | PlanEditInput) => {
      if (!planId) {
        return;
      }
      setServerError(null);
      updatePlan.mutate({ planId, data: values as PlanEditInput });
    },
    [planId, updatePlan],
  );

  if (!hasPlan) {
    return (
      <div className="space-y-3">
        <FeedbackBanner
          tone="error"
          message="No se indicó el plan a editar. Vuelve al listado y selecciona uno."
        />
        <Button asChild variant="outline" size="sm">
          <Link href="/plans">Volver a Plans</Link>
        </Button>
      </div>
    );
  }

  if (planQuery.isLoading) {
    return (
      <div className="flex items-center gap-2 text-muted-foreground">
        <Loader2 className="size-4 animate-spin" aria-hidden="true" />
        Cargando plan...
      </div>
    );
  }

  if (planQuery.isError) {
    return (
      <FeedbackBanner
        tone="error"
        message={
          planQuery.error?.message ??
          "No se pudo cargar el plan. Intenta de nuevo."
        }
      />
    );
  }

  return (
    <PlanForm
      mode="edit"
      initialPlan={planQuery.data}
      isSubmitting={updatePlan.isPending}
      serverError={serverError}
      onSubmit={handleSubmit}
      onCancel={() => router.push(backHref)}
    />
  );
}

export default function PlanEditPage() {
  return (
    <div className="mx-auto w-full max-w-3xl space-y-4">
      <React.Suspense fallback={null}>
        <PlanEditBackButton />
      </React.Suspense>

      <Card>
        <CardHeader>
          <CardTitle className="text-xl">Editar plan</CardTitle>
          <CardDescription>
            Modifica los campos que deseas actualizar; todos son opcionales.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <React.Suspense
            fallback={
              <div className="flex items-center gap-2 text-muted-foreground">
                <Loader2 className="size-4 animate-spin" aria-hidden="true" />
                Cargando...
              </div>
            }
          >
            <PlanEditContent />
          </React.Suspense>
        </CardContent>
      </Card>
    </div>
  );
}
