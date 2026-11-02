"use client";

/**
 * Administración de definiciones de módulo (module-builder). Lista los módulos
 * definidos y permite crear uno nuevo, editar o eliminar. Protegida por el
 * recurso estático `module-builder` en el backend; aquí solo UI.
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
  useDeleteModuleDef,
  useModuleDefs,
} from "@/features/modules/hooks";

/**
 * Al volver del asistente con `?saved=1`, dispara un TOAST de éxito (no un
 * banner inline, que quedaba fuera de pantalla si la lista estaba scrolleada) y
 * limpia el parámetro con `router.replace` para que no se repita al refrescar.
 * No renderiza nada; sigue dentro de <Suspense> por usar `useSearchParams`.
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
      toast.success("Módulo guardado correctamente.");
      router.replace("/module-defs");
    }
  }, [saved, router]);
  return null;
}

export default function ModuleDefsPage() {
  const { data: defs, isLoading, isError, error } = useModuleDefs();
  const deleteMut = useDeleteModuleDef();
  const [pendingDelete, setPendingDelete] = React.useState<string | null>(null);

  return (
    <div className="mx-auto w-full max-w-5xl space-y-6">
      <div className="flex items-center justify-between">
        <div className="space-y-1">
          <h1 className="text-2xl font-semibold text-foreground">Módulos</h1>
          <p className="text-sm text-muted-foreground">
            Define módulos administrables conectados a una API o a PostgreSQL.
          </p>
        </div>
        <Button asChild>
          <Link href="/module-defs/new">
            <Plus aria-hidden="true" />
            Nuevo módulo
          </Link>
        </Button>
      </div>

      <React.Suspense fallback={null}>
        <SavedToast />
      </React.Suspense>

      {isError ? (
        <FeedbackBanner tone="error" message={error?.message ?? "No se pudieron cargar los módulos."} />
      ) : null}

      <div className="rounded-lg border border-border bg-card">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Nombre</TableHead>
              <TableHead>Identificador</TableHead>
              <TableHead>Origen</TableHead>
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
            ) : (defs ?? []).length === 0 ? (
              <TableRow>
                <TableCell colSpan={4} className="h-24 text-center text-muted-foreground">
                  Aún no hay módulos definidos.
                </TableCell>
              </TableRow>
            ) : (
              (defs ?? []).map((d) => (
                <TableRow key={d.slug}>
                  <TableCell>{d.name}</TableCell>
                  <TableCell className="font-mono text-xs">{d.slug}</TableCell>
                  <TableCell>PostgreSQL</TableCell>
                  <TableCell className="text-right">
                    <div className="flex justify-end gap-2">
                      <Button asChild variant="outline" size="sm">
                        <Link href={`/module-defs/${d.slug}/edit`} aria-label="Editar módulo">
                          <Pencil aria-hidden="true" />
                        </Link>
                      </Button>
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        onClick={() => setPendingDelete(d.slug)}
                        aria-label="Eliminar módulo"
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
          label={`Eliminar módulo "${pendingDelete}"`}
          isPending={deleteMut.isPending}
          errorMessage={deleteMut.isError ? deleteMut.error?.message ?? "No se pudo eliminar" : null}
          onConfirm={() =>
            deleteMut.mutate(pendingDelete, { onSuccess: () => setPendingDelete(null) })
          }
          onCancel={() => setPendingDelete(null)}
        />
      ) : null}
    </div>
  );
}
