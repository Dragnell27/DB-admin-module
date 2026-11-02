"use client";

/**
 * Vista de creación de Card (tarea 8.1; Req. 3).
 *
 * Presenta el formulario de creación (`CardForm`) en una página con ruta
 * propia (`/cards/new`), reemplazando el panel embebido anterior (Req. 3.1,
 * 3.2). El `customer_id` del cliente seleccionado en la lista llega por la
 * query string y se usa como valor inicial del formulario (Req. 3.2). Al
 * crear con éxito se vuelve al listado con un flag (Req. 3.3); al cancelar se
 * vuelve al listado sin crear nada (Req. 3.4).
 *
 * El contenido se envuelve en <Suspense> porque lee `useSearchParams`
 * (requisito de Next.js App Router para rutas que leen parámetros de búsqueda
 * en el cliente).
 *
 * La ruta es accesible solo con rol `admin`: los `viewer` no ven el botón
 * "Crear" y, si llegan por URL, el BFF rechaza el `POST` (Req. 8.1, 8.4). Como
 * salvaguarda de UI, redirigimos a los no-admin de vuelta al listado.
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
import { CardForm } from "@/app/(protected)/cards/components/card-form";
import { FeedbackBanner } from "@/components/feedback-banner";

function CardCreateContent() {
  const router = useRouter();
  const role = useRole();
  const searchParams = useSearchParams();

  const customerId = searchParams.get("customer_id") ?? "";
  const email = searchParams.get("email") ?? undefined;
  const hasCustomerId = customerId !== "";

  // Salvaguarda de UI: los no-admin no deberían ver este formulario.
  React.useEffect(() => {
    if (role !== null && role !== "admin") {
      router.replace("/cards");
    }
  }, [role, router]);

  return (
    <>
      {!hasCustomerId ? (
        <FeedbackBanner
          tone="error"
          message={
            email
              ? `No se recibió el ID del cliente (${email}). Puedes capturarlo manualmente o volver al listado para seleccionar un cliente.`
              : "No se seleccionó un cliente. Puedes capturar su ID manualmente o volver al listado para seleccionar uno."
          }
        />
      ) : null}

      {!hasCustomerId ? (
        <Button asChild variant="outline" size="sm" className="w-fit">
          <Link href="/cards">Elegir cliente</Link>
        </Button>
      ) : null}

      <CardForm
        defaultCustomerId={customerId}
        onSuccess={() => router.replace("/cards?created=1")}
        onCancel={() => router.push("/cards")}
      />
    </>
  );
}

export default function CardCreatePage() {
  return (
    <div className="mx-auto w-full max-w-3xl space-y-4">
      <Button asChild variant="ghost" size="sm" className="-ml-2">
        <Link href="/cards">
          <ArrowLeft aria-hidden="true" />
          Volver a Cards
        </Link>
      </Button>

      <Card>
        <CardHeader>
          <CardTitle className="text-xl">Crear tarjeta</CardTitle>
          <CardDescription>
            Registra un nuevo medio de pago para el cliente seleccionado.
            Completa los datos de la tarjeta y guarda los cambios.
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <React.Suspense
            fallback={
              <div className="flex items-center gap-2 text-muted-foreground">
                <Loader2 className="size-4 animate-spin" aria-hidden="true" />
                Cargando...
              </div>
            }
          >
            <CardCreateContent />
          </React.Suspense>
        </CardContent>
      </Card>
    </div>
  );
}
