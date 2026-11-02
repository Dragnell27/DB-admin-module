/**
 * Layout del área protegida del panel (Req. 2.2, 2.6, 4.1, 4.3).
 *
 * Agrupa todas las vistas que requieren sesión bajo el grupo de rutas
 * `(protected)` (no altera las URLs: `/`, `/customers`, `/plans`, etc.). Envuelve
 * el contenido con `<AuthGuard>` para que solo usuarios con una sesión válida
 * accedan y, en su ausencia, se redirija a `/login` (Req. 2.2, 2.6).
 *
 * Compone el "shell" del panel: menú lateral por recurso (`AppSidebar`,
 * Req. 4.1–4.4) y barra superior con el usuario y el cierre de sesión
 * (`AppHeader`, Req. 2.4). El área principal hace scroll de forma independiente
 * y aloja la vista de cada recurso (tareas 16.x). Toda la UI usa la paleta
 * oscura del panel (Req. 4.3).
 */
import { AuthGuard } from "@/components/auth-guard";
import { AppHeader } from "@/components/app-header";
import { AppSidebar } from "@/components/app-sidebar";
import { ToastViewport } from "@/components/toast-viewport";

export default function ProtectedLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <AuthGuard>
      <div className="flex h-screen overflow-hidden bg-background">
        <AppSidebar />
        <div className="flex min-w-0 flex-1 flex-col">
          <AppHeader />
          <main className="flex-1 overflow-y-auto p-6">{children}</main>
        </div>
        {/* Pila de notificaciones flotantes: montada UNA vez, fuera del <main>
            con scroll, para que los resultados (éxito/error) sean siempre
            visibles sin importar la posición de scroll. */}
        <ToastViewport />
      </div>
    </AuthGuard>
  );
}
