"use client";

/**
 * Vista de creación de Customer (tarea 16.1, Req. 7).
 *
 * Presenta el formulario de creación (`CustomerForm` en modo `create`) dentro
 * de una tarjeta. Al enviar un formulario válido llama a `useCreateCustomer`
 * (`POST /customers`); ante éxito muestra feedback y redirige al listado
 * (Req. 7.11), y ante error muestra el mensaje devuelto por el BFF/OTT
 * (Req. 7.12).
 *
 * La ruta es accesible solo con rol `admin`: los `viewer` no ven el botón
 * "Crear" y, si llegan por URL, el BFF rechaza el `POST` (Req. 5.3). Como
 * salvaguarda de UI, redirigimos a los no-admin de vuelta al listado.
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
import { useRole } from "@/lib/use-role";
import { useCreateCustomer } from "@/features/customers/hooks";
import type { CustomerCreateInput } from "@/features/customers/schema";
import { CustomerForm } from "@/features/customers/components/customer-form";
import { FeedbackBanner } from "@/components/feedback-banner";

export default function CustomerCreatePage() {
  const router = useRouter();
  const role = useRole();
  const [errorMessage, setErrorMessage] = React.useState<string | null>(null);

  const createCustomer = useCreateCustomer({
    onSuccess: () => {
      // Éxito: volver al listado (que ya se invalida en el hook) con un flag.
      router.replace("/customers?created=1");
    },
    onError: (apiError) => {
      setErrorMessage(
        apiError.message ?? "No se pudo crear el cliente. Intenta de nuevo.",
      );
    },
  });

  // Salvaguarda de UI: los no-admin no deberían ver este formulario.
  React.useEffect(() => {
    if (role !== null && role !== "admin") {
      router.replace("/customers");
    }
  }, [role, router]);

  const handleSubmit = React.useCallback(
    (values: CustomerCreateInput) => {
      setErrorMessage(null);
      createCustomer.mutate(values);
    },
    [createCustomer],
  );

  return (
    <div className="mx-auto w-full max-w-3xl space-y-4">
      <Button asChild variant="ghost" size="sm" className="-ml-2">
        <Link href="/customers">
          <ArrowLeft aria-hidden="true" />
          Volver a Customers
        </Link>
      </Button>

      <Card>
        <CardHeader>
          <CardTitle className="text-xl">Crear cliente</CardTitle>
          <CardDescription>
            Completa los datos del nuevo cliente. Los campos marcados como
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

          <CustomerForm
            mode="create"
            isSubmitting={createCustomer.isPending}
            submitLabel="Crear cliente"
            onCancel={() => router.push("/customers")}
            onSubmit={handleSubmit}
          />
        </CardContent>
      </Card>
    </div>
  );
}
