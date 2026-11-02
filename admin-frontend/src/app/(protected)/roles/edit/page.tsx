"use client";

/**
 * Vista de edición de Rol (tarea 20.1, Req. 6.1, 11.2, 11.4, 16.3, 16.4, 16.5).
 *
 * Localiza el rol por `id` en la query string (`/roles/edit?id=...`), carga su
 * detalle con `useRoleDetail` y presenta el `RoleForm` pre-poblado con el nombre
 * y la matriz de permisos existentes. Al enviar llama a `useUpdateRole`
 * (`PUT /roles`); ante éxito vuelve al listado con feedback, y ante error
 * muestra el mensaje del BFF en un `FeedbackBanner`.
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
import { FeedbackBanner } from "@/components/feedback-banner";
import { getCountry, subscribeCountry } from "@/lib/country";
import { usePermissions } from "@/lib/use-permissions";
import { can } from "@/lib/permissions";
import { useRoleDetail, useUpdateRole } from "@/features/roles/hooks";
import { RoleForm, type RoleFormValues } from "../role-form";

/** Lee de forma reactiva (y SSR-safe) el país seleccionado en el selector. */
function useSelectedCountry(): string {
  return React.useSyncExternalStore(
    subscribeCountry,
    () => getCountry() ?? "",
    () => "",
  );
}

function RoleEditContent() {
  const router = useRouter();
  const perms = usePermissions();
  const country = useSelectedCountry();
  const searchParams = useSearchParams();
  const [errorMessage, setErrorMessage] = React.useState<string | null>(null);

  const id = searchParams.get("id") ?? undefined;
  const hasId = Boolean(id);

  const {
    data: role,
    isLoading,
    isError,
    error,
  } = useRoleDetail(id, { enabled: hasId });

  const updateRole = useUpdateRole({
    onSuccess: () => {
      router.replace("/roles?updated=1");
    },
    onError: (apiError) => {
      setErrorMessage(
        apiError.message ?? "No se pudo editar el rol. Intenta de nuevo.",
      );
    },
  });

  // Salvaguarda de UI: sin permiso de edición, de vuelta al listado.
  React.useEffect(() => {
    if (perms.length > 0 && !can(perms, "roles", "editar", country)) {
      router.replace("/roles");
    }
  }, [perms, country, router]);

  const handleSubmit = React.useCallback(
    (values: RoleFormValues) => {
      if (!id) {
        return;
      }
      setErrorMessage(null);
      updateRole.mutate({
        id,
        name: values.name,
        permissions: values.permissions,
      });
    },
    [updateRole, id],
  );

  if (!hasId) {
    return (
      <FeedbackBanner
        tone="error"
        message="No se indicó el rol a editar. Vuelve al listado y selecciona uno."
      />
    );
  }

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Loader2 className="size-4 animate-spin" aria-hidden="true" />
        Cargando rol...
      </div>
    );
  }

  if (isError || !role) {
    return (
      <FeedbackBanner
        tone="error"
        message={
          error?.message ?? "No se pudo cargar el rol. Intenta de nuevo."
        }
      />
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

      <RoleForm
        defaultName={role.name}
        defaultPermissions={role.permissions}
        submitLabel="Guardar cambios"
        isSubmitting={updateRole.isPending}
        onCancel={() => router.push("/roles")}
        onSubmit={handleSubmit}
      />
    </>
  );
}

export default function RoleEditPage() {
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
          <CardTitle className="text-xl">Editar rol</CardTitle>
          <CardDescription>
            Modifica el nombre y los permisos recurso × acción por país.
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
            <RoleEditContent />
          </React.Suspense>
        </CardContent>
      </Card>
    </div>
  );
}
