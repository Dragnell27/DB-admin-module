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
import { ConfirmActionDialog } from "@/features/modules/components/confirm-action-dialog";
import { getCountry, subscribeCountry } from "@/lib/country";
import { usePermissions } from "@/lib/use-permissions";
import { can } from "@/lib/permissions";
import { useRoles, useDeleteRole, type Role } from "@/features/roles/hooks";
import { usePageTitle } from "@/lib/page-chrome";
import { RolesSuccessToast } from "./roles-success-toast";

/** Lee de forma reactiva (y SSR-safe) el país seleccionado en el selector. */
function useSelectedCountry(): string {
  return React.useSyncExternalStore(
    subscribeCountry,
    () => getCountry() ?? "",
    () => "",
  );
}

export default function RolesPage() {
  usePageTitle("Roles");
  const perms = usePermissions();
  const country = useSelectedCountry();
  const canCreate = can(perms, "roles", "crear", country);
  const canDelete = can(perms, "roles", "eliminar", country);

  const { data, isLoading, isError, error } = useRoles();
  const deleteRole = useDeleteRole();

  // Flujo de borrado en dos pasos (espejo de Conexiones/Módulos): el botón
  // "Eliminar" guarda el rol pendiente y abre el `ConfirmActionDialog`; el DELETE
  // solo se dispara al Confirmar. Se guarda el `Role` completo para mostrar su
  // nombre en el diálogo y conservar el `id` del DELETE.
  const [pendingDelete, setPendingDelete] = React.useState<Role | null>(null);

  const roles = data?.items ?? [];

  return (
    <div className="mx-auto w-full max-w-5xl space-y-6">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
        <p className="text-sm text-muted-foreground">
          Define roles y los permisos recurso × acción por país.
        </p>
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
                          onClick={() => setPendingDelete(role)}
                          aria-label={`Eliminar ${role.name}`}
                        >
                          <Trash2 aria-hidden="true" />
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

      {pendingDelete ? (
        <ConfirmActionDialog
          open
          label={`Eliminar rol "${pendingDelete.name}"`}
          isPending={deleteRole.isPending}
          onConfirm={() =>
            deleteRole.mutate(pendingDelete.id, {
              // Tras DELETE 200 se cierra el modal (el hook ya invalidó los
              // listados en su `onSuccess` interno).
              onSuccess: () => setPendingDelete(null),
              // El error de eliminar se muestra como TOAST flotante (siempre
              // visible sin importar el scroll de la lista), no por el canal de
              // error del diálogo: se cierra el modal para que el toast se vea.
              onError: (apiError) => {
                if (
                  apiError.status === 409 ||
                  apiError.errorCode === "ROLE_ASSIGNED"
                ) {
                  toast.error(
                    apiError.message ??
                      "No se puede eliminar el rol porque está asignado a uno o más usuarios.",
                  );
                } else {
                  toast.error(
                    apiError.message ??
                      "No se pudo eliminar el rol. Intenta de nuevo.",
                  );
                }
                setPendingDelete(null);
              },
            })
          }
          onCancel={() => setPendingDelete(null)}
        />
      ) : null}
    </div>
  );
}
