"use client";

/**
 * Vista de creación de Usuario del panel RBAC (tarea 19.1, Req. 3.4, 16.3-16.6).
 *
 * Formulario con React Hook Form + zod (`userCreateSchema`):
 *   - `username` (texto, 3-64).
 *   - multi-select de roles (checkboxes) poblado desde `useRoles` (Req. 16.4).
 *
 * La contraseña NO se captura: el BFF la GENERA y la devuelve UNA sola vez en
 * la respuesta de creación (`useCreateUser` → `{ user, generatedPassword }`).
 * Tras un alta correcta mostramos esa contraseña de forma destacada para que el
 * administrador pueda copiarla, ya que no vuelve a mostrarse (Req. 3.2, 13.5).
 * Como ayuda adicional ofrecemos `generatePassword()` para previsualizar una
 * contraseña que cumple la política antes de crear, pero la fuente de verdad es
 * la que devuelve el backend.
 *
 * La ruta está guardada por permiso: si el usuario no tiene `users:crear` sobre
 * el país seleccionado se redirige al listado (salvaguarda de UI; el BFF aplica
 * la seguridad real) (Req. 16.6).
 */
import * as React from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
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
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { FeedbackBanner } from "@/components/feedback-banner";
import { useRoles } from "@/features/roles/hooks";
import { useCountriesMeta } from "@/features/meta/hooks";
import { useCreateUser } from "@/features/users/hooks";
import {
  userCreateSchema,
  type UserCreateInput,
} from "@/features/users/schema";
import { usePermissions } from "@/lib/use-permissions";
import { can, generatePassword } from "@/lib/permissions";
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
        Contraseña generada (se muestra una sola vez)
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
          aria-label="Copiar contraseña generada"
        >
          {copied ? (
            <Check aria-hidden="true" />
          ) : (
            <Copy aria-hidden="true" />
          )}
          {copied ? "Copiada" : "Copiar"}
        </Button>
      </div>
    </div>
  );
}

function UserCreateContent() {
  const router = useRouter();
  const perms = usePermissions();
  const country = useSelectedCountry() ?? "";
  const allowed = can(perms, "users", "crear", country);

  const [errorMessage, setErrorMessage] = React.useState<string | null>(null);
  const [generatedPassword, setGeneratedPassword] = React.useState<
    string | null
  >(null);
  const [preview, setPreview] = React.useState<string>("");

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
  } = useForm<UserCreateInput>({
    resolver: zodResolver(userCreateSchema),
    defaultValues: { username: "", roleIds: [], countries: [] },
  });

  const createUser = useCreateUser({
    onSuccess: (result) => {
      // Mostrar la contraseña devuelta por el BFF una sola vez y limpiar el form.
      setGeneratedPassword(result.generatedPassword);
      setErrorMessage(null);
      setPreview("");
      reset({ username: "", roleIds: [], countries: [] });
    },
    onError: (apiError) => {
      setErrorMessage(
        apiError.message ?? "No se pudo crear el usuario. Intenta de nuevo.",
      );
    },
  });

  // Salvaguarda de UI: sin permiso de creación, volver al listado.
  React.useEffect(() => {
    if (perms.length > 0 && !allowed) {
      router.replace("/users");
    }
  }, [perms.length, allowed, router]);

  const onSubmit = React.useCallback(
    (values: UserCreateInput) => {
      setErrorMessage(null);
      setGeneratedPassword(null);
      createUser.mutate(values);
    },
    [createUser],
  );

  return (
    <CardContent className="space-y-4">
      {errorMessage ? (
        <FeedbackBanner
          tone="error"
          message={errorMessage}
          onDismiss={() => setErrorMessage(null)}
        />
      ) : null}

      {generatedPassword ? (
        <>
          <GeneratedPasswordNotice password={generatedPassword} />
          <div className="flex flex-wrap gap-2">
            <Button asChild>
              <Link href="/users?created=1">Ir al listado</Link>
            </Button>
            <Button
              type="button"
              variant="outline"
              onClick={() => setGeneratedPassword(null)}
            >
              Crear otro usuario
            </Button>
          </div>
        </>
      ) : (
        <form
          className="space-y-5"
          onSubmit={handleSubmit(onSubmit)}
          noValidate
        >
          <div className="space-y-1.5">
            <Label htmlFor="username">Nombre de usuario</Label>
            <Input
              id="username"
              autoComplete="off"
              aria-invalid={errors.username ? true : undefined}
              aria-describedby={
                errors.username ? "username-error" : undefined
              }
              {...register("username")}
            />
            {errors.username ? (
              <p id="username-error" className="text-sm text-destructive">
                {errors.username.message}
              </p>
            ) : null}
          </div>

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
                                : field.value.filter((id) => id !== role.id);
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
              Selecciona los países en los que el usuario podrá operar.
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

          <div className="space-y-1.5">
            <Label htmlFor="password-preview">
              Vista previa de contraseña (opcional)
            </Label>
            <p className="text-sm text-muted-foreground">
              El servidor genera la contraseña definitiva al crear el usuario.
              Puedes previsualizar una que cumple la política.
            </p>
            <div className="flex items-center gap-2">
              <Input
                id="password-preview"
                readOnly
                value={preview}
                placeholder="Pulsa Generar para previsualizar"
                className="font-mono"
              />
              <Button
                type="button"
                variant="outline"
                onClick={() => setPreview(generatePassword())}
              >
                <KeyRound aria-hidden="true" />
                Generar contraseña
              </Button>
            </div>
          </div>

          <div className="flex items-center gap-2">
            <Button type="submit" disabled={createUser.isPending}>
              {createUser.isPending ? (
                <Loader2 className="size-4 animate-spin" aria-hidden="true" />
              ) : null}
              Crear usuario
            </Button>
            <Button
              type="button"
              variant="ghost"
              onClick={() => router.push("/users")}
            >
              Cancelar
            </Button>
          </div>
        </form>
      )}
    </CardContent>
  );
}

export default function UserCreatePage() {
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
          <CardTitle className="text-xl">Crear usuario</CardTitle>
          <CardDescription>
            Indica el nombre de usuario y asigna al menos un rol. La contraseña
            se genera automáticamente y se muestra una sola vez al crear.
          </CardDescription>
        </CardHeader>
        <UserCreateContent />
      </Card>
    </div>
  );
}
