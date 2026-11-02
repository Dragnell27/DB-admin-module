"use client";

/**
 * Vista de listado de Usuarios del panel RBAC (tarea 19.1, Req. 10.2, 16.3).
 *
 * - Lista los usuarios con `useUsers` mostrando username, estado activo y los
 *   roles asignados. Cubre los estados de carga, vacío y error con mensajes y
 *   marcado accesible (Req. 16.3, 16.4, 16.5).
 * - Presenta un botón "Crear" (enlaza a `/users/new`), visible SOLO si el
 *   usuario tiene el permiso `users:crear` sobre el país seleccionado
 *   (`can(perms, "users", "crear", country)`). La seguridad real la aplica el
 *   BFF; esto es visibilidad de UI (Req. 3.4, 16.6).
 * - La edición enlaza a `/users/edit?id=...`.
 * - Tras crear/editar/resetear con éxito, las páginas de formulario redirigen
 *   aquí con `?created=1` / `?updated=1` / `?reset=1`; mostramos un banner de
 *   éxito inline (sin un proveedor de toasts compartido).
 */
import * as React from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { Loader2, Plus } from "lucide-react";

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
import { useUsers, type AdminUser } from "@/features/users/hooks";
import { usePermissions } from "@/lib/use-permissions";
import { can } from "@/lib/permissions";
import { getCountry, subscribeCountry } from "@/lib/country";

/** Lee el país seleccionado de forma reactiva y SSR-safe (igual que el selector). */
function useSelectedCountry(): string | null {
  return React.useSyncExternalStore(
    subscribeCountry,
    () => getCountry(),
    () => null,
  );
}

/** Banner de éxito tras crear/editar/resetear, leído de la query string. */
function UsersSuccessBanner() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const created = searchParams.get("created") === "1";
  const updated = searchParams.get("updated") === "1";

  if (!created && !updated) {
    return null;
  }

  const message = created
    ? "Usuario creado correctamente."
    : "Usuario actualizado correctamente.";

  return (
    <FeedbackBanner
      tone="success"
      message={message}
      onDismiss={() => router.replace("/users")}
    />
  );
}

/** Representa los roles de un usuario como texto legible (o un guion si no hay). */
function rolesLabel(user: AdminUser): string {
  if (user.roles.length === 0) {
    return "—";
  }
  return user.roles.map((role) => role.name).join(", ");
}

export default function UsersPage() {
  const perms = usePermissions();
  const country = useSelectedCountry() ?? "";
  const canCreate = can(perms, "users", "crear", country);

  const { data: users, isLoading, isError, error } = useUsers();

  return (
    <div className="mx-auto w-full max-w-5xl space-y-6">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
        <div className="space-y-1">
          <h1 className="text-2xl font-semibold text-foreground">Usuarios</h1>
          <p className="text-sm text-muted-foreground">
            Gestiona los usuarios del panel y los roles que tienen asignados.
          </p>
        </div>
        {canCreate ? (
          <Button asChild>
            <Link href="/users/new">
              <Plus aria-hidden="true" />
              Crear
            </Link>
          </Button>
        ) : null}
      </div>

      <React.Suspense fallback={null}>
        <UsersSuccessBanner />
      </React.Suspense>

      {isLoading ? (
        <div
          className="flex items-center gap-2 text-sm text-muted-foreground"
          role="status"
        >
          <Loader2 className="size-4 animate-spin" aria-hidden="true" />
          Cargando usuarios...
        </div>
      ) : isError ? (
        <FeedbackBanner
          tone="error"
          message={
            error?.message ??
            "No se pudieron cargar los usuarios. Intenta de nuevo."
          }
        />
      ) : !users || users.length === 0 ? (
        <p className="rounded-md border border-border bg-card px-4 py-6 text-center text-sm text-muted-foreground">
          No hay usuarios para mostrar.
        </p>
      ) : (
        <div className="rounded-md border border-border bg-card">
          <Table>
            <caption className="sr-only">Lista de usuarios del panel</caption>
            <TableHeader>
              <TableRow>
                <TableHead scope="col">Usuario</TableHead>
                <TableHead scope="col">Estado</TableHead>
                <TableHead scope="col">Roles</TableHead>
                <TableHead scope="col" className="text-right">
                  Acciones
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {users.map((user) => (
                <TableRow key={user.id}>
                  <TableCell className="font-medium">{user.username}</TableCell>
                  <TableCell>
                    <span
                      className={
                        user.isActive
                          ? "inline-flex items-center rounded-full border border-emerald-500/50 bg-emerald-500/10 px-2 py-0.5 text-xs text-emerald-400"
                          : "inline-flex items-center rounded-full border border-border bg-muted/40 px-2 py-0.5 text-xs text-muted-foreground"
                      }
                    >
                      {user.isActive ? "Activo" : "Inactivo"}
                    </span>
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {rolesLabel(user)}
                  </TableCell>
                  <TableCell className="text-right">
                    <Button asChild variant="outline" size="sm">
                      <Link
                        href={`/users/edit?id=${encodeURIComponent(user.id)}`}
                        aria-label={`Editar usuario ${user.username}`}
                      >
                        Editar
                      </Link>
                    </Button>
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
