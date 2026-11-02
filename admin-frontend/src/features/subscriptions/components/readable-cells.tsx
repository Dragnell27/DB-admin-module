"use client";

/**
 * Celdas de datos legibles para la tabla de suscripciones (Req 6).
 *
 * Patrón "una celda = un hook": cada celda es un subcomponente independiente
 * que llama a su propio hook de detalle (`usePlan`/`useCustomer`) en el nivel
 * superior de su render. Así se respetan las reglas de hooks de React —no se
 * llaman hooks dentro de bucles del componente padre que itera las filas—. La
 * caché de TanStack Query deduplica por `queryKey`, de modo que ids repetidos
 * entre filas comparten una sola petición.
 *
 * Respaldo al id: si la resolución falla, aún no está disponible, o no hay un
 * dato legible, cada celda muestra el identificador crudo (`planId`/
 * `customerId`) en lugar de romperse (Req 6.4, 6.5).
 */
import { Loader2 } from "lucide-react";

import { usePlan } from "@/features/plans/hooks";
import { useCustomer } from "@/features/customers/hooks";
import { readableCustomer } from "@/features/customers/readable-customer";

/** Guion largo mostrado cuando una celda no tiene identificador que resolver. */
function EmptyDash() {
  return <span className="text-muted-foreground">—</span>;
}

/** Indicador de carga inline, del tamaño de una celda (Req 6.3). */
function LoaderInline() {
  return (
    <>
      <Loader2 className="size-3 animate-spin" aria-hidden />
      <span className="sr-only">Cargando</span>
    </>
  );
}

/** Respaldo: muestra el identificador crudo en estilo monoespaciado. */
function IdFallback({ id }: { id: string }) {
  return <span className="font-mono text-xs">{id}</span>;
}

/**
 * Resuelve y muestra el nombre del plan asociado a una fila de suscripción
 * (Req 6.1, 6.3, 6.4).
 *
 * Una celda = un hook: llama a `usePlan` en el nivel superior de su render.
 *
 * - Sin `planId`: muestra un guion largo.
 * - Mientras carga: muestra un loader inline.
 * - Con datos: muestra `data.name` si es un texto; en caso de error, dato
 *   ausente o nombre faltante, cae al `planId` como respaldo.
 *
 * @param planId - Identificador del plan de la suscripción (opcional).
 */
export function PlanCell({ planId }: { planId?: string }) {
  const { data, isLoading } = usePlan(planId, { enabled: Boolean(planId) });

  if (!planId) {
    return <EmptyDash />;
  }
  if (isLoading) {
    return <LoaderInline />;
  }

  const name = typeof data?.name === "string" ? data.name : undefined;
  return name !== undefined ? <span>{name}</span> : <IdFallback id={planId} />;
}

/**
 * Resuelve y muestra el nombre o correo del cliente asociado a una fila de
 * suscripción (Req 6.2, 6.3, 6.5).
 *
 * Una celda = un hook: llama a `useCustomer` en el nivel superior de su render.
 *
 * - Sin `customerId`: muestra un guion largo.
 * - Mientras carga: muestra un loader inline.
 * - Con datos: usa `readableCustomer(data)` (nombre o correo); en caso de error
 *   o si no hay dato legible, cae al `customerId` como respaldo.
 *
 * @param customerId - `external_id` del cliente de la suscripción (opcional).
 */
export function CustomerCell({ customerId }: { customerId?: string }) {
  // Se resuelve por `customer_id` interno de OTT (no por external_id): el BFF
  // lo busca en el padrón y puede responder "no disponible" (data vacío), en
  // cuyo caso se cae al respaldo del id.
  const { data, isLoading } = useCustomer(
    { customer_id: customerId },
    { enabled: Boolean(customerId) },
  );

  if (!customerId) {
    return <EmptyDash />;
  }
  if (isLoading) {
    return <LoaderInline />;
  }

  const label = readableCustomer(data);
  return label !== undefined ? (
    <span>{label}</span>
  ) : (
    <IdFallback id={customerId} />
  );
}
