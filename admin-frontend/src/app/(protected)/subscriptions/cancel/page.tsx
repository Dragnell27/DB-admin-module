"use client";

/**
 * Vista de cancelación de Subscription (tarea 6.1, Req. 2, 4).
 *
 * Toma el `subscription_id` a cancelar de la query string (requerido, Req. 4.2)
 * y lo muestra con etiqueta "Suscripción:" (Req. 4.2, 4.3). El `customer_id` de
 * la query es sólo contexto: el formulario `CancelSubscriptionForm` lo solicita
 * en su propio campo, por lo que no se inyecta aquí (su contrato no acepta un
 * valor inicial y no debe modificarse — Req. 2.2, 10.2).
 *
 * Al confirmar llama a `useCancelSubscription`; ante éxito vuelve al listado con
 * marca `?canceled=1` (Req. 2.3) y ante error muestra el mensaje del BFF/OTT en
 * un banner (Req. 7.3). Si falta el `subscription_id` se informa y se ofrece
 * volver al listado (Req. 4.4). Una salvaguarda de rol redirige a los no-admin
 * fuera de la vista (Req. 8.1).
 *
 * Envuelta en <Suspense> por el uso de `useSearchParams` (requisito de Next.js
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
import { useCancelSubscription } from "@/features/subscriptions/hooks";
import type { SubscriptionCancelInput } from "@/features/subscriptions/schema";
import { CancelSubscriptionForm } from "@/features/subscriptions/components/cancel-subscription-form";
import { FeedbackBanner } from "@/components/feedback-banner";

function SubscriptionCancelContent() {
  const router = useRouter();
  const role = useRole();
  const searchParams = useSearchParams();
  const [errorMessage, setErrorMessage] = React.useState<string | null>(null);

  // `subscription_id` es requerido (para mostrar y cancelar). `customer_id` es
  // sólo contexto de la query; el formulario lo pide en su propio campo.
  const subscriptionId = searchParams.get("subscription_id") ?? undefined;

  const cancelMutation = useCancelSubscription({
    onSuccess: () => {
      router.replace("/subscriptions?canceled=1");
    },
    onError: (apiError) => {
      setErrorMessage(
        apiError.message ??
          "No se pudo cancelar la suscripción. Intenta de nuevo.",
      );
    },
  });

  // Salvaguarda de UI: los no-admin no deberían cancelar (Req. 8.1).
  React.useEffect(() => {
    if (role !== null && role !== "admin") {
      router.replace("/subscriptions");
    }
  }, [role, router]);

  const handleConfirm = React.useCallback(
    (values: SubscriptionCancelInput) => {
      setErrorMessage(null);
      cancelMutation.mutate(values);
    },
    [cancelMutation],
  );

  if (!subscriptionId) {
    return (
      <div className="space-y-4">
        <FeedbackBanner
          tone="error"
          message="No se indicó la suscripción a cancelar. Vuelve al listado y selecciona una."
        />
        <Button asChild variant="outline" size="sm">
          <Link href="/subscriptions">Volver a Subscriptions</Link>
        </Button>
      </div>
    );
  }

  return (
    <>
      {errorMessage ? (
        <FeedbackBanner
          tone="error"
          message={errorMessage}
          onDismiss={() => setErrorMessage(null)}
        />
      ) : null}

      <p className="text-sm text-muted-foreground">
        Suscripción:{" "}
        <span className="font-mono text-foreground">{subscriptionId}</span>
      </p>

      <CancelSubscriptionForm
        subscriptionId={subscriptionId}
        onConfirm={handleConfirm}
        onCancel={() => router.push("/subscriptions")}
        isSubmitting={cancelMutation.isPending}
      />
    </>
  );
}

export default function SubscriptionCancelPage() {
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
          <CardTitle className="text-xl">Cancelar suscripción</CardTitle>
          <CardDescription>
            Confirma la cancelación de la suscripción. Esta acción da de baja la
            suscripción del cliente.
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
            <SubscriptionCancelContent />
          </React.Suspense>
        </CardContent>
      </Card>
    </div>
  );
}
