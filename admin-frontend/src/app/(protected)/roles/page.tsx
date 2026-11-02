"use client";

/**
 * Vista de listado de Roles (tarea 20.1, Req. 11.2, 11.4, 16.3, 16.4, 16.5).
 *
 * - Lista los roles con `useRoles`, mostrando el nombre y el número de permisos.
 * - Enlaza a crear (`/roles/new`) y editar (`/roles/edit?id=...`).
 * - El botón "Crear" se muestra sólo si el usuario puede crear roles en el país
 *   seleccionado (`can(perms, "roles", "crear", country)`); el botón "Eliminar"
 *   sólo si puede eliminarlos (`"eliminar"`). La seguridad REAL la aplica el BFF;
 *   esto es sólo visibilidad de UI (Req. 9.x).
 * - El borrado usa `useDeleteRole`; si el BFF responde 409 (`ROLE_ASSIGNED`)
 *   porque el rol está asignado a usuarios, se muestra un `FeedbackBanner` con
 *   el mensaje del servidor en lugar de borrarlo (Req. 6.5).
 * - Cubre los estados de carga/vacío/error con etiquetas accesibles.
 */
import * as React from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { Pencil, Plus, Trash2, Loader2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { FeedbackBanner } from "@/components/feedback-banner";
import { toast } from "@/lib/use-toast";
import { getCountry, subscribeCountry } from "@/lib/country";
import { usePermissions } from "@/lib/use-permissions";
import { can } from "@/lib/permissions";
import { useRoles, useDeleteRole, type Role } from "@/features/roles/hooks";

/** Lee de forma reactiva (y SSR-safe) el país seleccionado en el selector. */
function useSelectedCountry(): string {
  return React.useSyncExternalStore(
    subscribeCountry,
    () => getCountry() ?? "",
    () => "",
  );
}

/**
 * Éxito tras crear/editar: las páginas de formulario redirigen aquí con
 * `?created=1`/`?updated=1`. Dispara un TOAST de éxito (no un banner inline, que
 * quedaba fuera de pantalla si la lista estaba scrolleada) y limpia el parámetro
 * con `router.replace`. Usa `useSearchParams`, por lo que se envuelve en
 * <Suspense> en el consumidor. No renderiza nada.
 */
function RolesSuccessToast() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const created = searchParams.get("created") === "1";
  const updated = searchParams.get("updated") === "1";

  React.useEffect(() => {
    if (!created && !updated) {
      return;
    }
    toast.success(
      created ? "Rol creado correctamente." : "Rol actualizado correctamente.",
    );
    router.replace("/roles");
  }, [created, updated, router]);

  return null;
}

export default function RolesPage() {
  const perms = usePermissions();
  const country = useSelectedCountry();
  const canCreate = can(perms, "roles", "crear", country);
  const canDelete = can(perms, "roles", "eliminar", country);

  const { data, isLoading, isError, error } = useRoles();
  const deleteRole = useDeleteRole();

  const [deletingId, setDeletingId] = React.useState<string | null>(null);

  const roles = data?.items ?? [];

  const handleDelete = React.useCallback(
    (role: Role) => {
      setDeletingId(role.id);
      deleteRole.mutate(role.id, {
        // El error de eliminar se muestra como TOAST flotante (siempre visible
        // sin importar el scroll de la lista), no como banner inline.
        onError: (apiError) => {
          if (apiError.status === 409 || apiError.errorCode === "ROLE_ASSIGNED") {
            toast.error(
              apiError.message ??
                "No se puede eliminar el rol porque está asignado a uno o más usuarios.",
            );
          } else {
            toast.error(
              apiError.message ?? "No se pudo eliminar el rol. Intenta de nuevo.",
            );
          }
        },
        onSettled: () => {
          setDeletingId(null);
        },
      });
    },
    [deleteRole],
  );

  return (
    <div className="mx-auto w-full max-w-5xl space-y-6">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
        <div className="space-y-1">
          <h1 className="text-2xl font-semibold text-foreground">Roles</h1>
          <p className="text-sm text-muted-foreground">
            Define roles y los permisos recurso × acción por país.
          </p>
        </div>
        {canCreate ? (
          <Button asChild>
            <Link href="/roles/new">
              <Plus aria-hidden="true" />
              Crear
            </Link>
          </Button>
        ) : null}
      </div>

      <React.Suspense fallback={null}>
        <RolesSuccessToast />
      </React.Suspense>

      {isLoading ? (
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <Loader2 className="size-4 animate-spin" aria-hidden="true" />
          Cargando roles...
        </div>
      ) : isError ? (
        <FeedbackBanner
          tone="error"
          message={
            error?.message ?? "No se pudieron cargar los roles. Intenta de nuevo."
          }
        />
      ) : roles.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          No hay roles creados todavía.
        </p>
      ) : (
        <div className="rounded-md border border-border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead scope="col">Nombre</TableHead>
                <TableHead scope="col">Permisos</TableHead>
                <TableHead scope="col" className="text-right">
                  Acciones
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {roles.map((role) => (
                <TableRow key={role.id}>
                  <TableCell className="font-medium">{role.name}</TableCell>
                  <TableCell className="text-muted-foreground">
                    {role.permissions.length}
                  </TableCell>
                  <TableCell>
                    <div className="flex items-center justify-end gap-2">
                      <Button asChild variant="outline" size="sm">
                        <Link
                          href={`/roles/edit?id=${encodeURIComponent(role.id)}`}
                          aria-label={`Editar ${role.name}`}
                        >
                          <Pencil aria-hidden="true" />
                          Editar
                        </Link>
                      </Button>
                      {canDelete ? (
                        <Button
                          type="button"
                          variant="destructive"
                          size="sm"
                          disabled={deleteRole.isPending && deletingId === role.id}
                          onClick={() => handleDelete(role)}
                          aria-label={`Eliminar ${role.name}`}
                        >
                          {deleteRole.isPending && deletingId === role.id ? (
                            <Loader2
                              className="size-4 animate-spin"
                              aria-hidden="true"
                            />
                          ) : (
                            <Trash2 aria-hidden="true" />
                          )}
                          Eliminar
                        </Button>
                      ) : null}
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
    </div>
  );
}
