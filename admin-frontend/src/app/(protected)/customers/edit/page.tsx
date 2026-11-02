"use client";

/**
 * Vista de edición de Customer (tarea 16.1, Req. 8).
 *
 * Localiza al cliente por `email` o `external_id` tomados de la query string
 * (Req. 8.4), carga su detalle con `useCustomer` y presenta el formulario de
 * edición. `external_id` y `email` se muestran DESHABILITADOS porque son
 * inmutables (Req. 8.2). Al enviar llama a `useUpdateCustomer` con el
 * localizador; ante éxito muestra feedback y vuelve al listado (Req. 8.5), y
 * ante error muestra el mensaje del BFF/OTT (Req. 8.6).
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
  useCustomer,
  useUpdateCustomer,
  type CustomerLocator,
} from "@/features/customers/hooks";
import type { CustomerEditInput } from "@/features/customers/schema";
import type { CustomerFormEditValues } from "@/features/customers/components/customer-form";
import { CustomerForm } from "@/features/customers/components/customer-form";
import { FeedbackBanner } from "@/components/feedback-banner";

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
function CustomerEditBackButton() {
  const searchParams = useSearchParams();
  const backHref = safeFrom(searchParams.get("from"), "/customers");
  return (
    <Button asChild variant="ghost" size="sm" className="-ml-2">
      <Link href={backHref}>
        <ArrowLeft aria-hidden="true" />
        Volver a Customers
      </Link>
    </Button>
  );
}

/** Normaliza un valor desconocido del detalle a `string` para el formulario. */
function asFormString(value: unknown): string | undefined {
  return typeof value === "string" && value !== "" ? value : undefined;
}

/**
 * Normaliza la fecha de nacimiento que devuelve el OTT (que puede venir como
 * `DD/MM/YYYY`) al formato que espera el formulario y el schema (`YYYY-MM-DD`).
 * Si ya viene en `YYYY-MM-DD` o no es reconocible, se devuelve tal cual.
 */
function toFormBirthDate(value: unknown): string | undefined {
  const raw = asFormString(value);
  if (!raw) {
    return undefined;
  }
  // Ya está en YYYY-MM-DD.
  if (/^\d{4}-\d{2}-\d{2}$/.test(raw)) {
    return raw;
  }
  // DD/MM/YYYY -> YYYY-MM-DD.
  const m = /^(\d{2})\/(\d{2})\/(\d{4})$/.exec(raw);
  if (m) {
    const [, dd, mm, yyyy] = m;
    return `${yyyy}-${mm}-${dd}`;
  }
  return raw;
}

function CustomerEditContent() {
  const router = useRouter();
  const role = useRole();
  const searchParams = useSearchParams();
  const [errorMessage, setErrorMessage] = React.useState<string | null>(null);

  const email = searchParams.get("email") ?? undefined;
  const externalId = searchParams.get("external_id") ?? undefined;
  // Destino de "Volver"/cancelar: el origen (`from`) si es válido, o el listado.
  const backHref = safeFrom(searchParams.get("from"), "/customers");
  const locator: CustomerLocator = React.useMemo(
    () => ({ email, external_id: externalId }),
    [email, externalId],
  );
  const hasLocator = Boolean(email || externalId);

  const {
    data: customer,
    isLoading,
    isError,
    error,
  } = useCustomer(locator, { enabled: hasLocator });

  const updateCustomer = useUpdateCustomer({
    onSuccess: () => {
      router.replace("/customers?updated=1");
    },
    onError: (apiError) => {
      setErrorMessage(
        apiError.message ?? "No se pudo editar el cliente. Intenta de nuevo.",
      );
    },
  });

  // Salvaguarda de UI: los no-admin no deberían editar.
  React.useEffect(() => {
    if (role !== null && role !== "admin") {
      router.replace("/customers");
    }
  }, [role, router]);

  const handleSubmit = React.useCallback(
    (values: CustomerEditInput) => {
      setErrorMessage(null);
      updateCustomer.mutate({ locator, data: values });
    },
    [updateCustomer, locator],
  );

  if (!hasLocator) {
    return (
      <FeedbackBanner
        tone="error"
        message="No se indicó el cliente a editar. Vuelve al listado y selecciona uno."
      />
    );
  }

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 text-muted-foreground">
        <Loader2 className="size-4 animate-spin" aria-hidden="true" />
        Cargando cliente...
      </div>
    );
  }

  if (isError) {
    return (
      <FeedbackBanner
        tone="error"
        message={
          error?.message ??
          "No se pudo cargar el cliente. Intenta de nuevo."
        }
      />
    );
  }

  const immutable = {
    external_id: asFormString(customer?.external_id) ?? externalId,
    email: asFormString(customer?.email) ?? email,
  };

  const defaultValues: Partial<CustomerFormEditValues> = {
    first_name: asFormString(customer?.first_name),
    last_name: asFormString(customer?.last_name),
    phone: asFormString(customer?.phone),
    birth_date: toFormBirthDate(customer?.birth_date),
    gender: (["M", "F", "O"] as const).includes(
      customer?.gender as "M" | "F" | "O",
    )
      ? (customer?.gender as "M" | "F" | "O")
      : undefined,
    address: asFormString(customer?.address),
    device_fingerprint: asFormString(customer?.device_fingerprint),
  };

  return (
    <>
      {errorMessage ? (
        <FeedbackBanner
          tone="error"
          message={errorMessage}
          onDismiss={() => setErrorMessage(null)}
        />
      ) : null}

      <CustomerForm
        mode="edit"
        immutable={immutable}
        defaultValues={defaultValues}
        isSubmitting={updateCustomer.isPending}
        submitLabel="Guardar cambios"
        onCancel={() => router.push(backHref)}
        onSubmit={handleSubmit}
      />
    </>
  );
}

export default function CustomerEditPage() {
  return (
    <div className="mx-auto w-full max-w-3xl space-y-4">
      <React.Suspense fallback={null}>
        <CustomerEditBackButton />
      </React.Suspense>

      <Card>
        <CardHeader>
          <CardTitle className="text-xl">Editar cliente</CardTitle>
          <CardDescription>
            El External ID y el Email no son editables. Modifica los demás datos
            y guarda los cambios.
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
            <CustomerEditContent />
          </React.Suspense>
        </CardContent>
      </Card>
    </div>
  );
}
