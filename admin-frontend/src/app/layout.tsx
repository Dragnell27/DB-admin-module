import type { Metadata } from "next";
import "./globals.css";

import { QueryProvider } from "@/lib/query-client";
import { THEME_INIT } from "@/lib/theme-script";

export const metadata: Metadata = {
  title: "Panel Admin",
  description:
    "Panel de administración para gestionar los recursos y módulos del sistema.",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  // Sistema de temas CLARO / OSCURO (Entrega A). La clase `dark` ya NO se
  // hardcodea: la fija el script anti-FOUC (`THEME_INIT`), inyectado como primer
  // hijo del <head> para ejecutarse de forma síncrona antes del primer paint y
  // evitar el parpadeo. El script lee `localStorage['panel-admin-ott.theme']`
  // con defecto OSCURO y aplica `class`/`color-scheme` en <html>.
  //
  // `suppressHydrationWarning` en el <html> se MANTIENE: es imprescindible
  // porque el script muta `class`/`style` del <html> antes de hidratar (y las
  // extensiones del navegador también lo tocan). Solo silencia diferencias en
  // este nodo; no oculta mismatches reales dentro de la app.
  return (
    <html lang="es" suppressHydrationWarning>
      <head>
        {/* Primer hijo del <head>: fija el tema antes de aplicar estilos al body. */}
        <script dangerouslySetInnerHTML={{ __html: THEME_INIT }} />
      </head>
      <body className="min-h-screen bg-background text-foreground antialiased">
        <QueryProvider>{children}</QueryProvider>
      </body>
    </html>
  );
}
