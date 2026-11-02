import type { Metadata } from "next";
import "./globals.css";

import { QueryProvider } from "@/lib/query-client";

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
  // El panel es siempre tema oscuro (Req. 1.2, 4.3): fijamos la clase `dark`
  // y el esquema de color en el <html> para toda la aplicación.
  //
  // `suppressHydrationWarning` en el <html> evita el error de hidratación que
  // provocan las extensiones del navegador (p. ej. QuillBot inyecta
  // `data-qb-installed`, Grammarly, gestores de tema, etc.) al modificar la
  // etiqueta raíz antes de que React hidrate. Solo silencia diferencias en este
  // nodo; no oculta mismatches reales dentro de la app.
  return (
    <html
      lang="es"
      className="dark"
      style={{ colorScheme: "dark" }}
      suppressHydrationWarning
    >
      <body className="min-h-screen bg-background text-foreground antialiased">
        <QueryProvider>{children}</QueryProvider>
      </body>
    </html>
  );
}
