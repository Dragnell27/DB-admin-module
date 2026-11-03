"use client";

/**
 * Banner de feedback inline compartido para las vistas del panel
 * (customers, subscriptions, cards, etc.).
 *
 * Componente autocontenido que muestra el resultado de una operación
 * (éxito/error) usando solo elementos nativos y el tema del panel. El mensaje
 * de error proviene del BFF/OTT (Req. 7.3, 10.2).
 */
import { AlertCircle, AlertTriangle, CheckCircle2, X, type LucideIcon } from "lucide-react";

import { cn } from "@/lib/utils";

export type FeedbackTone = "success" | "error" | "warning";

export interface FeedbackBannerProps {
  readonly tone: FeedbackTone;
  readonly message: string;
  /** Permite cerrar el banner manualmente. */
  readonly onDismiss?: () => void;
  readonly className?: string;
}

/** Banner de estado accesible (`role="alert"`) con icono y mensaje. */
export function FeedbackBanner({
  tone,
  message,
  onDismiss,
  className,
}: FeedbackBannerProps) {
  // Estilo e icono resueltos por `tone`: success (emerald/CheckCircle2),
  // error (destructive/AlertCircle) y warning (amber/AlertTriangle).
  // Fondo OPACO (bg-card) + borde de color por tono. Antes el fondo era un tinte
  // translúcido (bg-*/10) que dejaba transparentar lo de atrás; se reemplaza por
  // superficie sólida para legibilidad, dejando el color en el borde/texto/icono.
  const toneStyles: Readonly<Record<FeedbackTone, string>> = {
    success: "border-emerald-500/50 bg-card text-ok-text",
    error: "border-destructive/50 bg-card text-destructive-text border-l-4 border-l-destructive",
    warning: "border-amber-500/50 bg-card text-warn-text",
  };
  const toneIcons: Readonly<Record<FeedbackTone, LucideIcon>> = {
    success: CheckCircle2,
    error: AlertCircle,
    warning: AlertTriangle,
  };
  const Icon = toneIcons[tone];
  return (
    <div
      role="alert"
      className={cn(
        "flex items-start gap-2 rounded-md border px-3 py-2 text-sm",
        toneStyles[tone],
        className,
      )}
    >
      <Icon className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
      <span className="flex-1">{message}</span>
      {onDismiss ? (
        <button
          type="button"
          onClick={onDismiss}
          className="shrink-0 opacity-70 transition-opacity hover:opacity-100"
          aria-label="Cerrar notificación"
        >
          <X className="size-4" aria-hidden="true" />
        </button>
      ) : null}
    </div>
  );
}

export default FeedbackBanner;
