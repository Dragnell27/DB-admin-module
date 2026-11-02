"use client";

/**
 * Guardia de autenticación del área protegida (Req. 2.2, 2.6).
 *
 * Envuelve las vistas protegidas del panel y garantiza que solo un usuario con
 * una sesión válida (JWT de sesión del admin presente y NO expirado, según
 * `isAuthenticated()`) vea el contenido. Si no hay sesión utilizable, redirige
 * a `/login` (Req. 2.6). Mientras exista un JWT válido, permite el acceso sin
 * pedir un nuevo login (Req. 2.2).
 *
 * Diseño como componente de cliente (ver design.md, "Auth guard"):
 * - La comprobación de sesión depende de `localStorage`, que no existe en SSR.
 *   Para evitar desajustes de hidratación, el servidor y el primer render del
 *   cliente devuelven el mismo marcador neutro (un loader), y solo tras montar
 *   se evalúa `isAuthenticated()` y se decide renderizar o redirigir.
 * - La redirección usa `next/navigation` (`useRouter().replace`) dentro de un
 *   efecto, nunca durante el render.
 * - Reacciona al evento `storage` del navegador para que un logout en otra
 *   pestaña (o el borrado del token por el interceptor 401) cierre también esta
 *   vista protegida.
 *
 * Es intencionalmente reutilizable y componible: la tarea 14.1 lo consumirá
 * desde el layout protegido (sidebar) envolviendo el contenido con `<AuthGuard>`.
 */
import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { Loader2 } from "lucide-react";

import { SESSION_TOKEN_KEY, isAuthenticated } from "@/lib/session";

/** Ruta pública de login a la que se redirige cuando no hay sesión válida. */
const LOGIN_PATH = "/login";

/** Estado de la comprobación de sesión en el cliente. */
type AuthStatus = "checking" | "authenticated" | "unauthenticated";

export interface AuthGuardProps {
  /** Contenido protegido que se renderiza solo con una sesión válida. */
  readonly children: React.ReactNode;
  /**
   * Nodo opcional a mostrar mientras se resuelve la comprobación de sesión en
   * el cliente (y durante la hidratación). Por defecto, un loader centrado.
   */
  readonly fallback?: React.ReactNode;
}

/** Loader por defecto mostrado mientras se verifica la sesión. */
function DefaultFallback() {
  return (
    <div
      className="flex min-h-screen items-center justify-center bg-background"
      role="status"
      aria-live="polite"
      aria-busy="true"
    >
      <Loader2 className="size-6 animate-spin text-muted-foreground" />
      <span className="sr-only">Verificando sesión...</span>
    </div>
  );
}

/**
 * Protege su contenido exigiendo una sesión válida. Redirige a `/login` cuando
 * no la hay. Renderiza el `fallback` mientras resuelve (incl. hidratación).
 */
export function AuthGuard({ children, fallback }: AuthGuardProps) {
  const router = useRouter();
  // Primer render (incl. SSR): "checking" para que servidor y cliente coincidan.
  const [status, setStatus] = useState<AuthStatus>("checking");

  useEffect(() => {
    // Evalúa la sesión actual y actúa: renderizar o redirigir.
    function evaluate(): void {
      if (isAuthenticated()) {
        setStatus("authenticated");
        return;
      }
      setStatus("unauthenticated");
      router.replace(LOGIN_PATH);
    }

    evaluate();

    // Reaccionar a login/logout en otras pestañas o al borrado del token
    // (p. ej. por el interceptor 401 del cliente HTTP).
    function handleStorage(event: StorageEvent): void {
      if (event.key === null || event.key === SESSION_TOKEN_KEY) {
        evaluate();
      }
    }

    window.addEventListener("storage", handleStorage);
    return () => window.removeEventListener("storage", handleStorage);
  }, [router]);

  // Solo mostramos el contenido protegido cuando hay sesión confirmada.
  // En "checking" (incl. hidratación) y "unauthenticated" (mientras ocurre la
  // redirección) mostramos el fallback para evitar destellos de contenido.
  if (status !== "authenticated") {
    return <>{fallback ?? <DefaultFallback />}</>;
  }

  return <>{children}</>;
}

export default AuthGuard;
