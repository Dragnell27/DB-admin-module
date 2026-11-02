"use client";

/**
 * Vista de creación de Subscription (tarea 4.1, Req. 1).
 *
 * Presenta el formulario de creación (`SubscriptionForm` en modo `create`)
 * dentro de una tarjeta. Al enviar un formulario válido llama a
 * `useCreateSubscription` (`POST /subscriptions`, Req. 1.3, 1.6); ante éxito
 * redirige al listado con un flag (Req. 1.4) y ante error muestra el mensaje
 * devuelto por el BFF/OTT en un banner inline (Req. 7.3).
 *
 * La ruta es accesible solo con rol `admin`: los `viewer` no ven el botón
 * "Crear" y, si llegan por URL, el BFF rechaza el `POST`. Como salvaguarda de
 * UI, redirigimos a los no-admin de vuelta al listado (Req. 8.1).
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
import { FeedbackBanner } from "@/components/feedback-banner";
import { useRole } from "@/lib/use-role";
import { useCreateSubscription } from "@/features/subscriptions/hooks";
import type { SubscriptionCreateInput } from "@/features/subscriptions/schema";
import {
  SubscriptionForm,
  type SubscriptionFormValues,
} from "@/features/subscriptions/components/subscription-form";

export default function SubscriptionCreatePage() {
  const router = useRouter();
  const role = useRole();
  const [errorMessage, setErrorMessage] = React.useState<string | null>(null);

  const createMutation = useCreateSubscription({
    onSuccess: () => {
      // Éxito: volver al listado (que ya se invalida en el hook) con un flag.
      router.replace("/subscriptions?created=1");
    },
    onError: (apiError) => {
      setErrorMessage(
        apiError.message ??
          "No se pudo crear la suscripción. Intenta de nuevo.",
      );
    },
  });

  // Salvaguarda de UI: los no-admin no deberían ver este formulario.
  React.useEffect(() => {
    if (role !== null && role !== "admin") {
      router.replace("/subscriptions");
    }
  }, [role, router]);

  const handleSubmit = React.useCallback(
    (values: SubscriptionFormValues) => {
      setErrorMessage(null);
      createMutation.mutate(values as SubscriptionCreateInput);
    },
    [createMutation],
  );

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
          <CardTitle className="text-xl">Crear suscripción</CardTitle>
          <CardDescription>
            Completa los datos de la nueva suscripción. Los campos marcados como
            opcionales pueden dejarse vacíos.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {errorMessage ? (
            <FeedbackBanner
              tone="error"
              message={errorMessage}
              onDismiss={() => setErrorMessage(null)}
            />
          ) : null}

          <SubscriptionForm
            mode="create"
            isSubmitting={createMutation.isPending}
            onCancel={() => router.push("/subscriptions")}
            onSubmit={handleSubmit}
          />
        </CardContent>
      </Card>
    </div>
  );
}
