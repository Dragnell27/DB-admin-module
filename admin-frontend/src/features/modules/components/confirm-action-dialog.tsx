"use client";

/**
 * Diálogo de confirmación genérico para acciones destructivas de un módulo
 * dinámico (module-builder, tarea 14.3, Req. 10.5). Mismo patrón de modal
 * controlado que `confirm-delete-dialog` del proyecto (cierre con Escape/telón).
 */
import * as React from "react";
import { AlertTriangle, Loader2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

export interface ConfirmActionDialogProps {
  readonly open: boolean;
  /** Etiqueta de la acción (p. ej. "Eliminar", "Activar"). */
  readonly label: string;
  readonly isPending?: boolean;
  readonly errorMessage?: string | null;
  /** Contenido extra (p. ej. el conjunto ordenado de borrado multi-tabla). */
  readonly details?: React.ReactNode;
  readonly onConfirm: () => void;
  readonly onCancel: () => void;
}

export function ConfirmActionDialog({
  open,
  label,
  isPending = false,
  errorMessage = null,
  details = null,
  onConfirm,
  onCancel,
}: ConfirmActionDialogProps) {
  React.useEffect(() => {
    if (!open) {
      return;
    }
    function handleKeyDown(event: KeyboardEvent): void {
      if (event.key === "Escape" && !isPending) {
        onCancel();
      }
    }
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [open, isPending, onCancel]);

  if (!open) {
    return null;
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4"
      role="dialog"
      aria-modal="true"
      aria-labelledby="confirm-action-title"
      onClick={() => {
        if (!isPending) {
          onCancel();
        }
      }}
    >
      <Card className="w-full max-w-md shadow-lg" onClick={(e) => e.stopPropagation()}>
        <CardHeader className="space-y-1">
          <CardTitle id="confirm-action-title" className="flex items-center gap-2 text-xl">
            <AlertTriangle className="size-5 text-destructive-text" aria-hidden="true" />
            {label}
          </CardTitle>
          <CardDescription>
            ¿Seguro que deseas ejecutar esta acción? Esta operación puede no ser
            reversible.
          </CardDescription>
        </CardHeader>

        {details ? <CardContent>{details}</CardContent> : null}

        {errorMessage ? (
          <CardContent>
            <div
              role="alert"
              className="flex items-start gap-2 rounded-md border border-destructive/50 bg-destructive/10 px-3 py-2 text-sm text-destructive-text"
            >
              <AlertTriangle className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
              <span>{errorMessage}</span>
            </div>
          </CardContent>
        ) : null}

        <CardFooter className="justify-end gap-2">
          <Button type="button" variant="outline" onClick={onCancel} disabled={isPending}>
            Cancelar
          </Button>
          <Button type="button" variant="destructive" onClick={onConfirm} disabled={isPending}>
            {isPending ? (
              <>
                <Loader2 className="size-4 animate-spin" aria-hidden="true" />
                Ejecutando...
              </>
            ) : (
              "Confirmar"
            )}
          </Button>
        </CardFooter>
      </Card>
    </div>
  );
}

export default ConfirmActionDialog;
