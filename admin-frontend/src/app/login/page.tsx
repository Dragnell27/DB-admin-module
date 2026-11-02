"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { AlertCircle, Loader2 } from "lucide-react";

import { apiClient, isApiError, normalizeApiError } from "@/lib/api-client";
import { setSessionToken } from "@/lib/session";
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

/**
 * Pantalla de login del panel (Req. 1).
 *
 * - Presenta el título "Iniciar Sesión", el subtítulo "Ingresa tus credenciales
 *   para acceder", los campos "Usuario" y "Contraseña" y un botón primario de
 *   ancho completo, en un formulario centrado de ancho máx. 560px sobre el tema
 *   oscuro (Req. 1.1, 1.2).
 * - Valida con React Hook Form + Zod que usuario y contraseña sean requeridos,
 *   impidiendo el envío mientras falten (Req. 1.3, 1.4).
 * - Envía las credenciales al admin-backend (BFF) vía `apiClient`
 *   (`POST /auth/login`), persiste el JWT de sesión devuelto con
 *   `setSessionToken` y redirige a la vista principal (Req. 1.5, 1.8, 2.1).
 * - Ante credenciales inválidas (401), muestra el mensaje del BFF
 *   ("Credenciales incorrectas") sin salir de la pantalla (Req. 1.7).
 *
 * La guardia de rutas protegidas y el cierre de sesión se implementan en la
 * tarea 13.3; aquí solo se cubre el login.
 */

/** Ruta principal del panel a la que se redirige tras un login exitoso. */
const HOME_PATH = "/";

/**
 * Esquema de validación del formulario de login: usuario y contraseña son
 * requeridos (Req. 1.3, 1.4). Los mensajes están en español, acordes al panel.
 */
const loginSchema = z.object({
  username: z.string().trim().min(1, { message: "El usuario es requerido" }),
  password: z.string().min(1, { message: "La contraseña es requerida" }),
});

type LoginFormValues = z.infer<typeof loginSchema>;

/** Forma esperada de la carga útil (`data`) de un login exitoso del BFF. */
interface LoginData {
  token?: unknown;
}

/** Mensaje por defecto cuando el BFF no devuelve uno legible. */
const FALLBACK_ERROR_MESSAGE =
  "No se pudo iniciar sesión. Intenta de nuevo.";

export default function LoginPage() {
  const router = useRouter();
  const [formError, setFormError] = useState<string | null>(null);

  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<LoginFormValues>({
    resolver: zodResolver(loginSchema),
    defaultValues: { username: "", password: "" },
  });

  const onSubmit = async (values: LoginFormValues) => {
    setFormError(null);
    try {
      // El BFF responde el envoltorio normalizado { ok, data: { token, user }, message }.
      const response = await apiClient.post<{ data?: LoginData }>(
        "/auth/login",
        values,
      );

      const token = response.data?.data?.token;
      if (typeof token !== "string" || token.trim() === "") {
        setFormError(FALLBACK_ERROR_MESSAGE);
        return;
      }

      // Persistir el JWT de sesión entre recargas (Req. 2.1) y entrar al panel (Req. 1.8).
      setSessionToken(token);
      router.replace(HOME_PATH);
    } catch (error) {
      // `apiClient` ya normaliza errores; se protege ante cualquier otro origen.
      const apiError = isApiError(error) ? error : normalizeApiError(error);
      setFormError(apiError.message || FALLBACK_ERROR_MESSAGE);
    }
  };

  return (
    <main className="flex min-h-screen items-center justify-center bg-background px-4 py-8">
      <Card className="w-full max-w-[560px] shadow-lg">
        <CardHeader className="space-y-1 text-center">
          <CardTitle>Iniciar Sesión</CardTitle>
          <CardDescription>
            Ingresa tus credenciales para acceder
          </CardDescription>
        </CardHeader>

        <CardContent>
          <form
            noValidate
            onSubmit={handleSubmit(onSubmit)}
            className="space-y-4"
          >
            {formError ? (
              <div
                role="alert"
                className="flex items-start gap-2 rounded-md border border-destructive/50 bg-destructive/10 px-3 py-2 text-sm text-destructive"
              >
                <AlertCircle className="mt-0.5 size-4 shrink-0" />
                <span>{formError}</span>
              </div>
            ) : null}

            <div className="space-y-2">
              <Label htmlFor="username">Usuario</Label>
              <Input
                id="username"
                type="text"
                autoComplete="username"
                autoFocus
                aria-invalid={errors.username ? "true" : undefined}
                disabled={isSubmitting}
                {...register("username")}
              />
              {errors.username ? (
                <p className="text-sm text-destructive" role="alert">
                  {errors.username.message}
                </p>
              ) : null}
            </div>

            <div className="space-y-2">
              <Label htmlFor="password">Contraseña</Label>
              <Input
                id="password"
                type="password"
                autoComplete="current-password"
                aria-invalid={errors.password ? "true" : undefined}
                disabled={isSubmitting}
                {...register("password")}
              />
              {errors.password ? (
                <p className="text-sm text-destructive" role="alert">
                  {errors.password.message}
                </p>
              ) : null}
            </div>

            <Button type="submit" className="w-full" disabled={isSubmitting}>
              {isSubmitting ? (
                <>
                  <Loader2 className="size-4 animate-spin" />
                  Iniciando sesión...
                </>
              ) : (
                "Iniciar Sesión"
              )}
            </Button>
          </form>
        </CardContent>
      </Card>
    </main>
  );
}
