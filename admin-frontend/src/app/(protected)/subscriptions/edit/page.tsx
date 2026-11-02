"use client";

/**
 * Vista de cambio de Subscription (tarea 5.1, Req. 1, 4).
 *
 * Toma `subscription_id` (requerido) y, opcionalmente, `plan_id` /
 * `customer_id` de la query string. El `subscription_id` se muestra de forma
 * visible, con rótulo textual, únicamente para que el admin confirme sobre qué
 * suscripción está operando (Req. 4.1, 4.3); el cambio real se envía con el
 * payload del formulario vía `useChangeSubscription` (el hook recibe el payload
 * directo, no un localizador).
 *
 * Si no hay `subscription_id` en la query, no se habilita ninguna acción y se
 * muestra un aviso para volver al listado (Req. 4.4). Ante éxito vuelve al
 * listado con `?updated=1` (Req. 1.5); ante error muestra el mensaje del
 * BFF/OTT (Req. 7.3).
 *
 * Envuelto en <Suspense> por el uso de `useSearchParams` (requisito de Next.js
 * App Router para rutas que leen parámetros de búsqueda en el cliente).
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
import { useRole } from "@/lib/use-role";
import {
  useChangeSubscription,
  useSubscription,
} from "@/features/subscriptions/hooks";
import type { SubscriptionChangeInput } from "@/features/subscriptions/schema";
import {
  SubscriptionForm,
  type SubscriptionFormValues,
} from "@/features/subscriptions/components/subscription-form";
import { FeedbackBanner } from "@/components/feedback-banner";

function SubscriptionEditContent() {
  const router = useRouter();
  const role = useRole();
  const searchParams = useSearchParams();
  const [errorMessage, setErrorMessage] = React.useState<string | null>(null);

  const subscriptionId = searchParams.get("subscription_id") ?? undefined;
  const hasSubscription = Boolean(subscriptionId);

  // Precarga: trae el detalle de la suscripción para rellenar el formulario.
  const detailQuery = useSubscription(subscriptionId, {
    enabled: hasSubscription,
  });

  const changeMutation = useChangeSubscription({
    onSuccess: () => {
      router.replace("/subscriptions?updated=1");
    },
    onError: (apiError) => {
      setErrorMessage(
        apiError.message ??
          "No se pudo cambiar la suscripción. Intenta de nuevo.",
      );
    },
  });

  // Salvaguarda de UI: los no-admin no deberían cambiar suscripciones.
  React.useEffect(() => {
    if (role !== null && role !== "admin") {
      router.replace("/subscriptions");
    }
  }, [role, router]);

  const handleSubmit = React.useCallback(
    (values: SubscriptionFormValues) => {
      setErrorMessage(null);
      changeMutation.mutate(values as SubscriptionChangeInput);
    },
    [changeMutation],
  );

  if (!hasSubscription) {
    return (
      <div className="space-y-3">
        <FeedbackBanner
          tone="error"
          message="No se indicó la suscripción a cambiar. Vuelve al listado y selecciona una."
        />
        <Button asChild variant="outline" size="sm">
          <Link href="/subscriptions">Volver a Subscriptions</Link>
        </Button>
      </div>
    );
  }

  const detail = detailQuery.data;
  const defaultValues =
    detail != null
      ? {
          plan_id: typeof detail.plan_id === "string" ? detail.plan_id : "",
          customer_id:
            typeof detail.customer_id === "string" ? detail.customer_id : "",
          payment_method:
            typeof detail.payment_method === "string"
              ? detail.payment_method
              : "",
          payment_method_token:
            typeof detail.payment_method_token === "string"
              ? detail.payment_method_token
              : "",
          amount:
            detail.amount != null && detail.amount !== ""
              ? String(detail.amount)
              : "",
          start_date:
            typeof detail.subscription_start_date === "string"
              ? detail.subscription_start_date
              : "",
        }
      : undefined;
  const lockedCustomerId =
    typeof detail?.customer_id === "string" ? detail.customer_id : undefined;

  return (
    <>
      <div className="rounded-md border bg-muted/30 px-3 py-2 text-sm">
        <span className="text-muted-foreground">Suscripción:</span>{" "}
        <span className="font-mono">{subscriptionId}</span>
      </div>

      {errorMessage ? (
        <FeedbackBanner
          tone="error"
          message={errorMessage}
          onDismiss={() => setErrorMessage(null)}
        />
      ) : null}

      {detailQuery.isLoading ? (
        <div className="flex items-center gap-2 text-muted-foreground">
          <Loader2 className="size-4 animate-spin" aria-hidden="true" />
          Cargando datos de la suscripción...
        </div>
      ) : detailQuery.isError ? (
        <FeedbackBanner
          tone="error"
          message={
            detailQuery.error?.message ??
            "No se pudo cargar la suscripción. Intenta de nuevo."
          }
        />
      ) : (
        <SubscriptionForm
          mode="change"
          defaultValues={defaultValues}
          lockedCustomerId={lockedCustomerId}
          subscriptionId={subscriptionId}
          isSubmitting={changeMutation.isPending}
          onCancel={() => router.push("/subscriptions")}
          onSubmit={handleSubmit}
        />
      )}
    </>
  );
}

export default function SubscriptionEditPage() {
  return (
    <div className="mx-auto w-full max-w-3xl space-y-4">
      <Button asChild variant="ghost" size="sm" className="-ml-2">
        <Link href="/subscriptions">
          <ArrowLeft aria-hidden="true" />
          Volver a Subscriptions
        </Link>
      </Button>

      <Card>
        <CardHeader>
          <CardTitle className="text-xl">Cambiar suscripción</CardTitle>
          <CardDescription>
            Modifica el plan o el método de pago de la suscripción. Solo se
            envían los campos que completes.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <React.Suspense
            fallback={
              <div className="flex items-center gap-2 text-muted-foreground">
                <Loader2 className="size-4 animate-spin" aria-hidden="true" />
                Cargando...
              </div>
            }
          >
            <SubscriptionEditContent />
          </React.Suspense>
        </CardContent>
      </Card>
    </div>
  );
}
