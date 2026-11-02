"use client";

/**
 * Botón de alternancia de tema CLARO / OSCURO de la barra superior (Entrega A).
 *
 * Lee el tema actual con `useTheme()` y alterna con `toggleTheme()` (store
 * SSR-safe de `lib/theme.ts`). Reutiliza `Button` (variant ghost/icon), imitando
 * el botón de contraer menú ya presente en la topbar.
 *
 * Accesibilidad: `aria-label`/`title` dinámicos que describen la ACCIÓN
 * ("Cambiar a modo claro" cuando está en oscuro / "Cambiar a modo oscuro" cuando
 * está en claro), ícono `Sun`/`Moon` con `aria-hidden` (el significado lo da el
 * `aria-label`), y foco visible heredado de `buttonVariants`.
 *
 * Hidratación (regla dura de `use-theme.ts`): como SSR y el primer render de
 * cliente devuelven `"dark"`, el nodo cuyo MARCADO depende del tema (el ícono) se
 * envuelve en `<span suppressHydrationWarning>` para no reportar mismatch si el
 * usuario tenía `"light"` guardado.
 */
import { Moon, Sun } from "lucide-react";

import { Button } from "@/components/ui/button";
import { useTheme } from "@/lib/use-theme";
import { toggleTheme } from "@/lib/theme";

export function ThemeToggle() {
  const theme = useTheme();
  const isDark = theme === "dark";
  const label = isDark ? "Cambiar a modo claro" : "Cambiar a modo oscuro";

  return (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      onClick={() => toggleTheme()}
      aria-label={label}
      title={label}
    >
      <span suppressHydrationWarning>
        {isDark ? (
          <Sun className="size-4" aria-hidden="true" />
        ) : (
          <Moon className="size-4" aria-hidden="true" />
        )}
      </span>
    </Button>
  );
}

export default ThemeToggle;
