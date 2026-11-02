"use client";

/**
 * Banner de retroalimentación inline para las vistas de Plans (tarea 16.2).
 *
 * Reemplaza a un proveedor de toasts compartido (que correspondería a otra
 * tarea) para no colisionar con las tareas concurrentes 16.1/16.3/16.4. Muestra
 * mensajes de éxito o error con el texto devuelto por el BFF/OTT
 * (Req. 10.14, 10.15, 11.4, 11.5, 12.4, 12.5).
 */
import { AlertCircle, CheckCircle2, X } from "lucide-react";

import { cn } from "@/lib/utils";

export type FeedbackTone = "success" | "error";

export interface FeedbackBannerProps {
  readonly tone: FeedbackTone;
  readonly message: string;
  /** Si se provee, muestra un botón para descartar el banner. */
  readonly onDismiss?: () => void;
  readonly className?: string;
}

export function FeedbackBanner({
  tone,
  message,
  onDismiss,
  className,
}: FeedbackBannerProps) {
  const isSuccess = tone === "success";
  return (
    <div
      role={isSuccess ? "status" : "alert"}
      className={cn(
        "flex items-start gap-2 rounded-md border px-3 py-2 text-sm",
        isSuccess
          ? "border-emerald-500/50 bg-emerald-500/10 text-ok-text"
          : "border-destructive/50 bg-destructive/10 text-destructive-text",
        className,
      )}
    >
      {isSuccess ? (
        <CheckCircle2 className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
      ) : (
        <AlertCircle className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
      )}
      <span className="flex-1">{message}</span>
      {onDismiss ? (
        <button
          type="button"
          onClick={onDismiss}
          className="shrink-0 rounded-sm opacity-70 transition-opacity hover:opacity-100"
          aria-label="Descartar mensaje"
        >
          <X className="size-4" aria-hidden="true" />
        </button>
      ) : null}
    </div>
  );
}

export default FeedbackBanner;
