"use client";

/**
 * Viewport de NOTIFICACIONES FLOTANTES (toasts). Se monta UNA vez en el layout
 * protegido (`app/(protected)/layout.tsx`) y dibuja una pila FIJA anclada arriba
 * a la derecha, por ENCIMA del contenido y FUERA del `<main>` con scroll, de
 * modo que la posición de scroll nunca oculta un toast (el problema que esta
 * migración resuelve, crítico para los errores).
 *
 * Reutiliza el LENGUAJE VISUAL de `FeedbackBanner` (mismos tokens de color e
 * iconos de lucide, sin hex crudo). Cada toast tiene SIEMPRE un botón de cerrar
 * (×); el auto-descarte lo gestiona cada item con su propio `setTimeout`
 * (éxito/aviso se van solos; los errores persisten hasta que el usuario los
 * cierra). El tope de pila (`TOAST_CAP`) lo impone el store.
 *
 * Accesibilidad: DOS regiones vivas apiladas en la misma columna —una
 * `role="status" aria-live="polite"` para éxito/aviso y otra `role="alert"
 * aria-live="assertive"` para errores— de forma que los errores interrumpen al
 * lector de pantalla mientras los éxitos se anuncian con cortesía.
 */
import * as React from "react";
import {
  AlertCircle,
  AlertTriangle,
  CheckCircle2,
  X,
  type LucideIcon,
} from "lucide-react";

import { cn } from "@/lib/utils";
import { AUTO_DISMISS_MS, type Toast, type ToastTone } from "@/lib/toast-store";
import { useToast } from "@/lib/use-toast";

/**
 * Mapas de tono → clases e icono, ESPEJO de `FeedbackBanner` para que los
 * toasts se vean nativos (mismos tokens del tema oscuro, sin hex crudo).
 */
const toneStyles: Readonly<Record<ToastTone, string>> = {
  success: "border-emerald-500/50 bg-emerald-500/10 text-ok-text",
  error: "border-destructive/50 bg-destructive/10 text-destructive",
  warning: "border-amber-500/50 bg-amber-500/10 text-warn-text",
};
const toneIcons: Readonly<Record<ToastTone, LucideIcon>> = {
  success: CheckCircle2,
  error: AlertCircle,
  warning: AlertTriangle,
};

/**
 * Un item de la pila. Posee su propio temporizador de auto-descarte: cuando
 * `AUTO_DISMISS_MS[tone]` es un número, programa un `setTimeout` que llama a
 * `onDismiss(id)`; cuando es `null` (error) no programa nada y persiste. El
 * `setTimeout` se limpia al desmontar o al cerrarse manualmente.
 */
function ToastItem({
  toast,
  onDismiss,
}: {
  readonly toast: Toast;
  readonly onDismiss: (id: string) => void;
}) {
  const Icon = toneIcons[toast.tone];

  React.useEffect(() => {
    const ms = AUTO_DISMISS_MS[toast.tone];
    if (ms == null) {
      return;
    }
    const timer = window.setTimeout(() => onDismiss(toast.id), ms);
    return () => window.clearTimeout(timer);
  }, [toast.id, toast.tone, onDismiss]);

  return (
    <div
      className={cn(
        "pointer-events-auto flex items-start gap-2 rounded-md border bg-card px-3 py-2 text-sm shadow-lg",
        toneStyles[toast.tone],
      )}
    >
      <Icon className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
      <span className="flex-1">{toast.message}</span>
      <button
        type="button"
        onClick={() => onDismiss(toast.id)}
        className="shrink-0 opacity-70 transition-opacity hover:opacity-100"
        aria-label="Cerrar notificación"
      >
        <X className="size-4" aria-hidden="true" />
      </button>
    </div>
  );
}

export function ToastViewport() {
  const { toasts, dismiss } = useToast();

  // Separa por rol de región viva: errores → assertive; éxito/aviso → polite.
  // Dentro de cada región se respeta el orden de llegada (`createdAt`); el más
  // nuevo se dibuja ARRIBA invirtiendo la lista (el store agrega al final).
  const errors = toasts.filter((t) => t.tone === "error");
  const polite = toasts.filter((t) => t.tone !== "error");

  return (
    <div className="pointer-events-none fixed top-4 right-4 z-50 flex w-full max-w-sm flex-col gap-2">
      <div role="alert" aria-live="assertive" className="flex flex-col gap-2">
        {[...errors].reverse().map((t) => (
          <ToastItem key={t.id} toast={t} onDismiss={dismiss} />
        ))}
      </div>
      <div role="status" aria-live="polite" className="flex flex-col gap-2">
        {[...polite].reverse().map((t) => (
          <ToastItem key={t.id} toast={t} onDismiss={dismiss} />
        ))}
      </div>
    </div>
  );
}

export default ToastViewport;
