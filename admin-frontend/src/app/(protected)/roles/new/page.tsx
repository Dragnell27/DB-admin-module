"use client";

/**
 * Vista de creación de Rol (tarea 20.1, Req. 5.1, 11.4, 16.3, 16.4, 16.5).
 *
 * Presenta el `RoleForm` en modo creación dentro de una tarjeta. Al enviar un
 * formulario válido llama a `useCreateRole` (`POST /roles`); ante éxito vuelve
 * al listado con un flag de feedback, y ante error muestra el mensaje del BFF
 * en un `FeedbackBanner`.
 *
 * Como salvaguarda de UI, si el usuario no puede crear roles en el país
 * seleccionado se le redirige al listado. La seguridad REAL la aplica el BFF.
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
import { getCountry, subscribeCountry } from "@/lib/country";
import { usePermissions } from "@/lib/use-permissions";
import { can } from "@/lib/permissions";
import { useCreateRole } from "@/features/roles/hooks";
import { RoleForm, type RoleFormValues } from "../role-form";

/** Lee de forma reactiva (y SSR-safe) el país seleccionado en el selector. */
function useSelectedCountry(): string {
  return React.useSyncExternalStore(
    subscribeCountry,
    () => getCountry() ?? "",
    () => "",
  );
}

export default function RoleCreatePage() {
  const router = useRouter();
  const perms = usePermissions();
  const country = useSelectedCountry();
  const [errorMessage, setErrorMessage] = React.useState<string | null>(null);

  const createRole = useCreateRole({
    onSuccess: () => {
      router.replace("/roles?created=1");
    },
    onError: (apiError) => {
      setErrorMessage(
        apiError.message ?? "No se pudo crear el rol. Intenta de nuevo.",
      );
    },
  });

  // Salvaguarda de UI: sin permiso de creación, de vuelta al listado. Sólo se
  // actúa cuando ya hay permisos hidratados (perms no vacío) para no redirigir
  // durante el primer render del cliente.
  React.useEffect(() => {
    if (perms.length > 0 && !can(perms, "roles", "crear", country)) {
      router.replace("/roles");
    }
  }, [perms, country, router]);

  const handleSubmit = React.useCallback(
    (values: RoleFormValues) => {
      setErrorMessage(null);
      createRole.mutate({ name: values.name, permissions: values.permissions });
    },
    [createRole],
  );

  return (
    <div className="mx-auto w-full max-w-5xl space-y-4">
      <Button asChild variant="ghost" size="sm" className="-ml-2">
        <Link href="/roles">
          <ArrowLeft aria-hidden="true" />
          Volver a Roles
        </Link>
      </Button>

      <Card>
        <CardHeader>
          <CardTitle className="text-xl">Crear rol</CardTitle>
          <CardDescription>
            Asigna un nombre y marca los permisos recurso × acción por país.
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

          <RoleForm
            submitLabel="Crear rol"
            isSubmitting={createRole.isPending}
            onCancel={() => router.push("/roles")}
            onSubmit={handleSubmit}
          />
        </CardContent>
      </Card>
    </div>
  );
}
