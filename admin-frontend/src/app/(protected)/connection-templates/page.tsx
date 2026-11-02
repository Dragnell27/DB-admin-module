"use client";

/**
 * Administración de plantillas de conexión (connection-templates). Lista las
 * plantillas (nombre + campos con marca FIJO/VARIABLE, sin valores), y permite
 * crear, editar y eliminar. Protegida por el recurso estático `connections` en
 * el backend; aquí solo UI.
 */
import * as React from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { Pencil, Plus, Trash2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { FeedbackBanner } from "@/components/feedback-banner";
import { toast } from "@/lib/use-toast";
import { ConfirmActionDialog } from "@/features/modules/components/confirm-action-dialog";
import {
  useConnectionTemplates,
  useDeleteConnectionTemplate,
} from "@/features/connections/hooks";
import { usePageTitle } from "@/lib/page-chrome";

/**
 * Al volver del editor con `?saved=1`, dispara un TOAST de éxito (no un banner
 * inline, que quedaba fuera de pantalla si la lista estaba scrolleada) y limpia
 * el parámetro con `router.replace` para que no se repita al refrescar. No
 * renderiza nada; sigue dentro de <Suspense> por usar `useSearchParams`.
 */
function SavedToast() {
  const router = useRouter();
  const params = useSearchParams();
  const saved = params.get("saved") === "1";
  // Guard de idempotencia: en React Strict Mode (dev) el efecto se ejecuta dos
  // veces, y `toast.success` se dispara antes de que `router.replace` quite el
  // `?saved=1`, lo que mostraba el toast DUPLICADO. El ref asegura una sola
  // emisión aunque el efecto corra más de una vez.
  const emitted = React.useRef(false);
  React.useEffect(() => {
    if (saved && !emitted.current) {
      emitted.current = true;
      toast.success("Plantilla de conexión guardada correctamente.");
      router.replace("/connection-templates");
    }
  }, [saved, router]);
  return null;
}

export default function ConnectionTemplatesPage() {
  usePageTitle("Conexiones");
  const { data: templates, isLoading, isError, error } = useConnectionTemplates();
  const deleteMut = useDeleteConnectionTemplate();
  const [pendingDelete, setPendingDelete] = React.useState<string | null>(null);

  return (
    <div className="mx-auto w-full max-w-5xl space-y-6">
      <div className="flex items-center justify-between gap-4">
        <p className="text-sm text-muted-foreground">
          Define plantillas de conexión reutilizables con campos fijos y variables.
        </p>
        <Button asChild>
          <Link href="/connection-templates/new">
            <Plus aria-hidden="true" />
            Nueva plantilla
          </Link>
        </Button>
      </div>

      <React.Suspense fallback={null}>
        <SavedToast />
      </React.Suspense>

      {isError ? (
        <FeedbackBanner
          tone="error"
          message={error?.message ?? "No se pudieron cargar las plantillas."}
        />
      ) : null}

      <div className="rounded-lg border border-border bg-card">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Nombre</TableHead>
              <TableHead>Campos</TableHead>
              <TableHead>En uso</TableHead>
              <TableHead className="w-32 text-right">Acciones</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {isLoading ? (
              <TableRow>
                <TableCell colSpan={4} className="h-24 text-center text-muted-foreground">
                  Cargando...
                </TableCell>
              </TableRow>
            ) : (templates ?? []).length === 0 ? (
              <TableRow>
                <TableCell colSpan={4} className="h-24 text-center text-muted-foreground">
                  Aún no hay plantillas de conexión.
                </TableCell>
              </TableRow>
            ) : (
              (templates ?? []).map((t) => (
                <TableRow key={t.id}>
                  <TableCell>{t.name}</TableCell>
                  <TableCell className="text-xs text-muted-foreground">
                    {t.fields
                      .map((f) => `${f.key} (${f.kind === "fixed" ? "fijo" : "variable"})`)
                      .join(", ")}
                  </TableCell>
                  <TableCell>{t.usedByModules}</TableCell>
                  <TableCell className="text-right">
                    <div className="flex justify-end gap-2">
                      <Button asChild variant="outline" size="sm">
                        <Link href={`/connection-templates/${t.id}/edit`} aria-label="Editar plantilla">
                          <Pencil aria-hidden="true" />
                        </Link>
                      </Button>
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        onClick={() => setPendingDelete(t.id)}
                        aria-label="Eliminar plantilla"
                      >
                        <Trash2 aria-hidden="true" />
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>

      {pendingDelete ? (
        <ConfirmActionDialog
          open
          label="Eliminar plantilla de conexión"
          isPending={deleteMut.isPending}
          errorMessage={
            deleteMut.isError ? deleteMut.error?.message ?? "No se pudo eliminar" : null
          }
          onConfirm={() =>
            deleteMut.mutate(pendingDelete, { onSuccess: () => setPendingDelete(null) })
          }
          onCancel={() => setPendingDelete(null)}
        />
      ) : null}
    </div>
  );
}
