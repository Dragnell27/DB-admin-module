"use client";

/**
 * Diálogo de confirmación de eliminación de Plan (tarea 16.2, Req. 12.1-12.3).
 *
 * Se construye con primitivas existentes (`Card`, `Button`) + estado local, sin
 * introducir una primitiva de diálogo compartida (para evitar colisiones con las
 * tareas 16.1/16.3/16.4). Es un modal controlado: el padre decide si está
 * abierto (`open`) y recibe `onConfirm`/`onCancel`.
 *
 * - Al confirmar, invoca `onConfirm` (el padre dispara `useDeletePlan`).
 * - Al cancelar (botón, tecla Escape o clic en el telón), invoca `onCancel` y no
 *   ejecuta ninguna eliminación (Req. 12.3).
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

export interface ConfirmDeleteDialogProps {
  /** Controla la visibilidad del diálogo. */
  readonly open: boolean;
  /** Nombre del plan a eliminar, para el texto de confirmación. */
  readonly planName?: string;
  /** Indica una eliminación en curso (deshabilita acciones). */
  readonly isPending?: boolean;
  /** Mensaje de error de la eliminación, si lo hubo. */
  readonly errorMessage?: string | null;
  /** Se invoca cuando el usuario confirma la eliminación. */
  readonly onConfirm: () => void;
  /** Se invoca cuando el usuario cancela o cierra el diálogo (Req. 12.3). */
  readonly onCancel: () => void;
}

export function ConfirmDeleteDialog({
  open,
  planName,
  isPending = false,
  errorMessage = null,
  onConfirm,
  onCancel,
}: ConfirmDeleteDialogProps) {
  // Cerrar con la tecla Escape mientras el diálogo esté abierto.
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
      aria-labelledby="confirm-delete-title"
      onClick={() => {
        if (!isPending) {
          onCancel();
        }
      }}
    >
      <Card
        className="w-full max-w-md shadow-lg"
        onClick={(event) => event.stopPropagation()}
      >
        <CardHeader className="space-y-1">
          <CardTitle
            id="confirm-delete-title"
            className="flex items-center gap-2 text-xl"
          >
            <AlertTriangle className="size-5 text-destructive" aria-hidden="true" />
            Eliminar plan
          </CardTitle>
          <CardDescription>
            ¿Seguro que deseas eliminar
            {planName ? ` el plan "${planName}"` : " este plan"}? Esta acción no
            se puede deshacer.
          </CardDescription>
        </CardHeader>

        {errorMessage ? (
          <CardContent>
            <div
              role="alert"
              className="flex items-start gap-2 rounded-md border border-destructive/50 bg-destructive/10 px-3 py-2 text-sm text-destructive"
            >
              <AlertTriangle className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
              <span>{errorMessage}</span>
            </div>
          </CardContent>
        ) : null}

        <CardFooter className="justify-end gap-2">
          <Button
            type="button"
            variant="outline"
            onClick={onCancel}
            disabled={isPending}
          >
            Cancelar
          </Button>
          <Button
            type="button"
            variant="destructive"
            onClick={onConfirm}
            disabled={isPending}
          >
            {isPending ? (
              <>
                <Loader2 className="size-4 animate-spin" aria-hidden="true" />
                Eliminando...
              </>
            ) : (
              "Eliminar"
            )}
          </Button>
        </CardFooter>
      </Card>
    </div>
  );
}

export default ConfirmDeleteDialog;
