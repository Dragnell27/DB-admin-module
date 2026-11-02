"use client";

/**
 * Vista de edición de Usuario del panel RBAC (tarea 19.1, Req. 4.5, 16.3-16.6).
 *
 * Localiza al usuario por `id` tomado de la query string (`?id=...`), carga su
 * detalle con `useUser` y presenta el formulario de edición con:
 *   - multi-select de roles (checkboxes) poblado desde `useRoles`.
 *   - un toggle `isActive`.
 * El envío usa `useUpdateUser` (`PUT /users`).
 *
 * BRECHA CONOCIDA: el listado/detalle de usuarios del backend devuelve de
 * momento `roles: []` (aún no resuelve usuario→roles). Lo tratamos con
 * elegancia: si llegan roles, se preseleccionan; si no, se parte de una
 * selección vacía y el administrador fija los roles. Nunca rompemos la vista.
 *
 * Incluye además un botón "Resetear contraseña" (`useResetUserPassword`) que
 * muestra la nueva contraseña UNA sola vez (Req. 4.x, 13.5).
 *
 * Envuelta en <Suspense> por el uso de `useSearchParams` (requisito de Next.js
 * App Router para rutas que leen parámetros de búsqueda en el cliente). La ruta
 * se guarda por permiso `users:editar` sobre el país seleccionado (salvaguarda
 * de UI; la seguridad real la aplica el BFF) (Req. 16.6).
 */
import * as React from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { Controller, useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { ArrowLeft, Check, Copy, KeyRound, Loader2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { FeedbackBanner } from "@/components/feedback-banner";
import { useRoles } from "@/features/roles/hooks";
import { useCountriesMeta } from "@/features/meta/hooks";
import {
  useResetUserPassword,
  useUpdateUser,
  useUser,
} from "@/features/users/hooks";
import { userEditSchema, type UserEditInput } from "@/features/users/schema";
import { usePermissions } from "@/lib/use-permissions";
import { can } from "@/lib/permissions";
import { countryLabel, getCountry, subscribeCountry } from "@/lib/country";

/** Lee el país seleccionado de forma reactiva y SSR-safe (igual que el selector). */
function useSelectedCountry(): string | null {
  return React.useSyncExternalStore(
    subscribeCountry,
    () => getCountry(),
    () => null,
  );
}

/** Bloque destacado que muestra una contraseña generada con botón de copiar. */
function GeneratedPasswordNotice({ password }: { readonly password: string }) {
  const [copied, setCopied] = React.useState(false);

  const handleCopy = React.useCallback(async () => {
    try {
      await navigator.clipboard.writeText(password);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2000);
    } catch {
      // Si el navegador bloquea el portapapeles, el usuario puede copiar a mano.
    }
  }, [password]);

  return (
    <div
      role="alert"
      className="space-y-2 rounded-md border border-amber-500/50 bg-amber-500/10 p-3 text-sm"
    >
      <div className="flex items-center gap-2 font-medium text-amber-300">
        <KeyRound className="size-4" aria-hidden="true" />
        Nueva contraseña (se muestra una sola vez)
      </div>
      <p className="text-muted-foreground">
        Copia esta contraseña y entrégala al usuario de forma segura. No podrás
        volver a verla.
      </p>
      <div className="flex items-center gap-2">
        <code className="flex-1 break-all rounded-md border border-border bg-input px-3 py-2 font-mono text-sm text-foreground">
          {password}
        </code>
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={handleCopy}
          aria-label="Copiar nueva contraseña"
        >
          {copied ? <Check aria-hidden="true" /> : <Copy aria-hidden="true" />}
          {copied ? "Copiada" : "Copiar"}
        </Button>
      </div>
    </div>
  );
}

function UserEditContent() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const perms = usePermissions();
  const country = useSelectedCountry() ?? "";
  const allowed = can(perms, "users", "editar", country);

  const id = searchParams.get("id") ?? undefined;
  const hasId = Boolean(id);

  const [errorMessage, setErrorMessage] = React.useState<string | null>(null);
  const [resetPassword, setResetPassword] = React.useState<string | null>(null);

  const {
    data: user,
    isLoading,
    isError,
    error,
  } = useUser(id, { enabled: hasId });

  const {
    data: rolesResult,
    isLoading: rolesLoading,
    isError: rolesError,
    error: rolesErrorObj,
  } = useRoles();
  const roles = rolesResult?.items ?? [];

  const {
    data: countriesMeta,
    isLoading: countriesLoading,
    isError: countriesError,
    error: countriesErrorObj,
  } = useCountriesMeta();
  const availableCountries = countriesMeta?.countries ?? [];

  const {
    control,
    register,
    handleSubmit,
    reset,
    formState: { errors },
  } = useForm<UserEditInput>({
    resolver: zodResolver(userEditSchema),
    defaultValues: { id: id ?? "", roleIds: [], countries: [], isActive: true },
  });

  // Precargar valores cuando llega el detalle del usuario. La BRECHA CONOCIDA
  // (roles vacíos del backend) se maneja de forma natural: si `roles` viene
  // vacío, `roleIds` queda vacío y el admin elige los roles manualmente.
  React.useEffect(() => {
    if (user) {
      reset({
        id: user.id,
        roleIds: user.roles.map((role) => role.id),
        countries: [...user.countries],
        isActive: user.isActive,
      });
    }
  }, [user, reset]);

  const updateUser = useUpdateUser({
    onSuccess: () => {
      router.replace("/users?updated=1");
    },
    onError: (apiError) => {
      setErrorMessage(
        apiError.message ?? "No se pudo editar el usuario. Intenta de nuevo.",
      );
    },
  });

  const resetUserPassword = useResetUserPassword({
    onSuccess: (result) => {
      setResetPassword(result.generatedPassword);
      setErrorMessage(null);
    },
    onError: (apiError) => {
      setErrorMessage(
        apiError.message ??
          "No se pudo resetear la contraseña. Intenta de nuevo.",
      );
    },
  });

  // Salvaguarda de UI: sin permiso de edición, volver al listado.
  React.useEffect(() => {
    if (perms.length > 0 && !allowed) {
      router.replace("/users");
    }
  }, [perms.length, allowed, router]);

  const onSubmit = React.useCallback(
    (values: UserEditInput) => {
      setErrorMessage(null);
      updateUser.mutate(values);
    },
    [updateUser],
  );

  if (!hasId) {
    return (
      <FeedbackBanner
        tone="error"
        message="No se indicó el usuario a editar. Vuelve al listado y selecciona uno."
      />
    );
  }

  if (isLoading) {
    return (
      <div
        className="flex items-center gap-2 text-sm text-muted-foreground"
        role="status"
      >
        <Loader2 className="size-4 animate-spin" aria-hidden="true" />
        Cargando usuario...
      </div>
    );
  }

  if (isError) {
    return (
      <FeedbackBanner
        tone="error"
        message={
          error?.message ?? "No se pudo cargar el usuario. Intenta de nuevo."
        }
      />
    );
  }

  return (
    <div className="space-y-5">
      {errorMessage ? (
        <FeedbackBanner
          tone="error"
          message={errorMessage}
          onDismiss={() => setErrorMessage(null)}
        />
      ) : null}

      {resetPassword ? (
        <GeneratedPasswordNotice password={resetPassword} />
      ) : null}

      <div className="space-y-1">
        <p className="text-sm text-muted-foreground">Usuario</p>
        <p className="text-base font-medium text-foreground">
          {user?.username ?? "—"}
        </p>
      </div>

      <form className="space-y-5" onSubmit={handleSubmit(onSubmit)} noValidate>
        {/* `id` viaja oculto para el payload de edición. */}
        <input type="hidden" {...register("id")} />

        <fieldset className="space-y-2">
          <legend className="text-sm font-medium leading-none text-foreground">
            Roles
          </legend>
          <p className="text-sm text-muted-foreground">
            Selecciona al menos un rol para el usuario.
          </p>

          {rolesLoading ? (
            <div
              className="flex items-center gap-2 text-sm text-muted-foreground"
              role="status"
            >
              <Loader2 className="size-4 animate-spin" aria-hidden="true" />
              Cargando roles...
            </div>
          ) : rolesError ? (
            <FeedbackBanner
              tone="error"
              message={
                rolesErrorObj?.message ??
                "No se pudieron cargar los roles. Intenta de nuevo."
              }
            />
          ) : roles.length === 0 ? (
            <p className="rounded-md border border-border bg-input px-3 py-2 text-sm text-muted-foreground">
              No hay roles disponibles. Crea un rol antes de asignar usuarios.
            </p>
          ) : (
            <Controller
              control={control}
              name="roleIds"
              render={({ field }) => (
                <div className="space-y-2 rounded-md border border-border bg-input p-3">
                  {roles.map((role) => {
                    const checked = field.value.includes(role.id);
                    return (
                      <label
                        key={role.id}
                        className="flex items-center gap-2 text-sm text-foreground"
                      >
                        <input
                          type="checkbox"
                          className="size-4 rounded border-input accent-primary"
                          checked={checked}
                          onChange={(event) => {
                            const next = event.target.checked
                              ? [...field.value, role.id]
                              : field.value.filter((rid) => rid !== role.id);
                            field.onChange(next);
                          }}
                        />
                        {role.name}
                      </label>
                    );
                  })}
                </div>
              )}
            />
          )}

          {errors.roleIds ? (
            <p className="text-sm text-destructive">
              {errors.roleIds.message}
            </p>
          ) : null}
        </fieldset>

        <fieldset className="space-y-2">
          <legend className="text-sm font-medium leading-none text-foreground">
            Países
          </legend>
          <p className="text-sm text-muted-foreground">
            Países en los que el usuario puede operar.
          </p>

          {countriesLoading ? (
            <div
              className="flex items-center gap-2 text-sm text-muted-foreground"
              role="status"
            >
              <Loader2 className="size-4 animate-spin" aria-hidden="true" />
              Cargando países...
            </div>
          ) : countriesError ? (
            <FeedbackBanner
              tone="error"
              message={
                countriesErrorObj?.message ??
                "No se pudieron cargar los países. Intenta de nuevo."
              }
            />
          ) : availableCountries.length === 0 ? (
            <p className="rounded-md border border-border bg-input px-3 py-2 text-sm text-muted-foreground">
              No hay países configurados.
            </p>
          ) : (
            <Controller
              control={control}
              name="countries"
              render={({ field }) => (
                <div className="space-y-2 rounded-md border border-border bg-input p-3">
                  {availableCountries.map((code) => {
                    const checked = field.value.includes(code);
                    return (
                      <label
                        key={code}
                        className="flex items-center gap-2 text-sm text-foreground"
                      >
                        <input
                          type="checkbox"
                          className="size-4 rounded border-input accent-primary"
                          checked={checked}
                          onChange={(event) => {
                            const next = event.target.checked
                              ? [...field.value, code]
                              : field.value.filter((c) => c !== code);
                            field.onChange(next);
                          }}
                        />
                        {countryLabel(code)}
                      </label>
                    );
                  })}
                </div>
              )}
            />
          )}

          {errors.countries ? (
            <p className="text-sm text-destructive">
              {errors.countries.message}
            </p>
          ) : null}
        </fieldset>

        <div className="space-y-2">
          <Label htmlFor="isActive" className="flex items-center gap-2">
            <input
              id="isActive"
              type="checkbox"
              className="size-4 rounded border-input accent-primary"
              {...register("isActive")}
            />
            Usuario activo
          </Label>
          <p className="text-sm text-muted-foreground">
            Un usuario inactivo no puede iniciar sesión en el panel.
          </p>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <Button type="submit" disabled={updateUser.isPending}>
            {updateUser.isPending ? (
              <Loader2 className="size-4 animate-spin" aria-hidden="true" />
            ) : null}
            Guardar cambios
          </Button>
          <Button
            type="button"
            variant="ghost"
            onClick={() => router.push("/users")}
          >
            Cancelar
          </Button>
          <Button
            type="button"
            variant="outline"
            className="ml-auto"
            disabled={resetUserPassword.isPending || !id}
            onClick={() => {
              if (id) {
                setResetPassword(null);
                resetUserPassword.mutate({ id });
              }
            }}
          >
            {resetUserPassword.isPending ? (
              <Loader2 className="size-4 animate-spin" aria-hidden="true" />
            ) : (
              <KeyRound aria-hidden="true" />
            )}
            Resetear contraseña
          </Button>
        </div>
      </form>
    </div>
  );
}

export default function UserEditPage() {
  return (
    <div className="mx-auto w-full max-w-3xl space-y-4">
      <Button asChild variant="ghost" size="sm" className="-ml-2">
        <Link href="/users">
          <ArrowLeft aria-hidden="true" />
          Volver a Usuarios
        </Link>
      </Button>

      <Card>
        <CardHeader>
          <CardTitle className="text-xl">Editar usuario</CardTitle>
          <CardDescription>
            Ajusta los roles asignados y el estado del usuario. También puedes
            resetear su contraseña, que se mostrará una sola vez.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <React.Suspense
            fallback={
              <div
                className="flex items-center gap-2 text-sm text-muted-foreground"
                role="status"
              >
                <Loader2 className="size-4 animate-spin" aria-hidden="true" />
                Cargando...
              </div>
            }
          >
            <UserEditContent />
          </React.Suspense>
        </CardContent>
      </Card>
    </div>
  );
}
