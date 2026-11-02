"use client";

/**
 * Vista de Subscriptions (tarea 9.2, Req. 5.x, 7.x, 8.3).
 *
 * La página se reduce a un contenedor delgado: encabezado, resolución del rol
 * (`useRole`) para habilitar escrituras y el montaje de `SubscriptionsTable`.
 * La tabla navega por su cuenta a las rutas dedicadas (`/subscriptions/new`,
 * `/subscriptions/edit`, `/subscriptions/cancel`, tarea 9.1), por lo que ya no
 * se le pasan callbacks de creación/cambio/cancelación ni se renderizan
 * formularios embebidos.
 *
 * Responsabilidades que conserva/añade:
 *   - "Ver detalle" (Req. 5.1, 5.2, 5.5, 5.6, 5.7, 8.3): la tabla notifica vía
 *     `onViewDetail(kind, id, triggerRef)`; aquí se mantiene el estado del
 *     `DetailDialog` y se devuelve el foco al botón que lo abrió al cerrarse.
 *   - Banner de éxito por flags de query (Req. 7.4): al volver de las rutas de
 *     escritura con `?created=1 | ?updated=1 | ?canceled=1` se muestra un
 *     `FeedbackBanner tone="success"`. Como lee `useSearchParams`, vive en un
 *     subcomponente envuelto en `<Suspense>` (requisito de Next.js App Router),
 *     igual que `cards/page.tsx`.
 *   - Estados de carga/vacío/error del listado los resuelve `ResourceTable`
 *     dentro de `SubscriptionsTable` (Req. 7.1, 7.2).
 */
import * as React from "react";
import { useRouter, useSearchParams } from "next/navigation";

import { FeedbackBanner } from "@/components/feedback-banner";
import {
  DetailDialog,
  type DetailKind,
} from "@/components/detail-dialog";
import { useRole } from "@/lib/use-role";
import { SubscriptionsTable } from "@/features/subscriptions/components/subscriptions-table";

const SUCCESS_CREATE = "Suscripción creada correctamente.";
const SUCCESS_UPDATE = "Suscripción actualizada correctamente.";
const SUCCESS_CANCEL = "Suscripción cancelada correctamente.";

/** Estado del diálogo de detalle controlado por la página. */
interface DetailState {
  readonly open: boolean;
  readonly kind: DetailKind;
  readonly id: string;
  /** Botón que abrió el diálogo, para devolverle el foco al cerrar (Req. 9.4). */
  readonly triggerRef: React.RefObject<HTMLElement | null> | null;
}

const INITIAL_DETAIL: DetailState = {
  open: false,
  kind: "plan",
  id: "",
  triggerRef: null,
};

/**
 * Banner de éxito mostrado al volver de las rutas de escritura con
 * `?created=1 | ?updated=1 | ?canceled=1` (Req. 7.4). Lee `useSearchParams`,
 * por lo que se renderiza dentro de un <Suspense> (requisito del App Router).
 */
function SubscriptionsSuccessBanner() {
  const router = useRouter();
  const searchParams = useSearchParams();

  let message: string | null = null;
  if (searchParams.get("created") === "1") {
    message = SUCCESS_CREATE;
  } else if (searchParams.get("updated") === "1") {
    message = SUCCESS_UPDATE;
  } else if (searchParams.get("canceled") === "1") {
    message = SUCCESS_CANCEL;
  }

  if (message === null) {
    return null;
  }

  return (
    <FeedbackBanner
      tone="success"
      message={message}
      onDismiss={() => router.replace("/subscriptions")}
    />
  );
}

export default function SubscriptionsPage() {
  const role = useRole();
  const canWrite = role === "admin";

  const [detail, setDetail] = React.useState<DetailState>(INITIAL_DETAIL);

  const handleViewDetail = React.useCallback(
    (
      kind: DetailKind,
      id: string,
      triggerRef: React.RefObject<HTMLElement | null>,
    ) => {
      setDetail({ open: true, kind, id, triggerRef });
    },
    [],
  );

  return (
    <div className="mx-auto w-full max-w-6xl space-y-6">
      <div className="space-y-2">
        <h1 className="text-2xl font-semibold text-foreground">Subscriptions</h1>
        <p className="text-sm text-muted-foreground">
          Lista, filtra y gestiona las suscripciones de los clientes.
        </p>
      </div>

      {/* Banner de éxito por flags de query (Req. 7.4) */}
      <React.Suspense fallback={null}>
        <SubscriptionsSuccessBanner />
      </React.Suspense>

      <SubscriptionsTable canWrite={canWrite} onViewDetail={handleViewDetail} />

      <DetailDialog
        open={detail.open}
        kind={detail.kind}
        id={detail.id}
        triggerRef={detail.triggerRef ?? undefined}
        onClose={() => setDetail((prev) => ({ ...prev, open: false }))}
      />
    </div>
  );
}
