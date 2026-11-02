"use client";

/**
 * Vista de creación de Plan en página propia (consistente con Customers y
 * Subscriptions). Reemplaza el panel modal anterior.
 *
 * Usa `useCreatePlan`; ante éxito vuelve al listado con `?created=1`, ante error
 * muestra el mensaje del BFF/OTT. Salvaguarda de rol: redirige a los no-admin.
 */
import * as React from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { ArrowLeft } from "lucide-react";

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
import { useCreatePlan } from "@/features/plans/hooks";
import type { PlanCreateInput, PlanEditInput } from "@/features/plans/schema";
import { PlanForm } from "@/features/plans/components/plan-form";

export default function PlanCreatePage() {
  const router = useRouter();
  const role = useRole();
  const [serverError, setServerError] = React.useState<string | null>(null);

  const createPlan = useCreatePlan({
    onSuccess: () => {
      router.replace("/plans?created=1");
    },
    onError: (error) => {
      setServerError(
        isApiError(error)
          ? error.message
          : "No se pudo crear el plan. Intenta de nuevo.",
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
      setServerError(null);
      createPlan.mutate(values as PlanCreateInput);
    },
    [createPlan],
  );

  return (
    <div className="mx-auto w-full max-w-3xl space-y-4">
      <Button asChild variant="ghost" size="sm" className="-ml-2">
        <Link href="/plans">
          <ArrowLeft aria-hidden="true" />
          Volver a Plans
        </Link>
      </Button>

      <Card>
        <CardHeader>
          <CardTitle className="text-xl">Crear plan</CardTitle>
          <CardDescription>
            Completa los datos del nuevo plan de suscripción.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <PlanForm
            mode="create"
            isSubmitting={createPlan.isPending}
            serverError={serverError}
            onSubmit={handleSubmit}
            onCancel={() => router.push("/plans")}
          />
        </CardContent>
      </Card>
    </div>
  );
}
