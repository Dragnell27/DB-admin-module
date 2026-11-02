"use client";

/**
 * `DetailDialog`: diálogo de detalle accesible y compartido (tarea 2.1, Req 5.7, 9.5).
 *
 * Resuelve recursos relacionados (plan / cliente) en un modal SIN cambiar la ruta:
 * es estado local controlado por el padre (`open`, `kind`, `id`, `onClose`) (Req 5.7).
 *
 * Se construye con primitivas existentes (`Card`, `Button`) siguiendo el patrón de
 * `features/plans/components/confirm-delete-dialog.tsx` (telón `fixed inset-0 z-50`,
 * `role="dialog" aria-modal aria-labelledby`, clic en el telón cierra,
 * `stopPropagation` en la `Card` interna).
 *
 * Esta tarea (2.1) deja preparado SÓLO el esqueleto del diálogo. Los cuerpos reales
 * y la gestión de foco se implementan en tareas posteriores sobre este mismo archivo:
 *   - Tarea 2.2: resolver datos reales en `PlanDetailBody`/`CustomerDetailBody`.
 *   - Tarea 2.3: gestión de foco (focus-in, focus trap, Escape, retorno de foco).
 */
import * as React from "react";
import { usePathname, useRouter } from "next/navigation";
import { Loader2, Pencil } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { planId, usePlan } from "@/features/plans/hooks";
import { useCustomer } from "@/features/customers/hooks";
import { readableCustomer } from "@/features/customers/readable-customer";
import { usePermissions } from "@/lib/use-permissions";
import { can } from "@/lib/permissions";
import { getCountry, subscribeCountry } from "@/lib/country";

/**
 * País OTT seleccionado leído de forma reactiva y SSR-safe (snapshot del
 * servidor `null`), con el mismo patrón que el resto de la UI. Se usa para el
 * permiso de "Editar cliente", ya que el detalle de cliente no trae país.
 */
function useSelectedCountry(): string | null {
  return React.useSyncExternalStore(
    subscribeCountry,
    () => getCountry(),
    () => null,
  );
}

export type DetailKind = "plan" | "customer";

export interface DetailDialogProps {
  /** Controla la visibilidad del diálogo. */
  readonly open: boolean;
  /** Qué recurso relacionado mostrar. */
  readonly kind: DetailKind;
  /** Identificador del recurso: plan_id, o customer_id/email del cliente. */
  readonly id: string;
  /** Se invoca al cerrar (Escape, telón, botón). */
  readonly onClose: () => void;
  /**
   * Referencia al botón "Ver detalle" que abrió el diálogo, para devolverle el
   * foco al cerrar (Req 9.4). Opcional por robustez.
   */
  readonly triggerRef?: React.RefObject<HTMLElement | null>;
}

/** Marcador de ausencia usado cuando un campo no viene en la respuesta. */
const EMPTY_FIELD = "—";

/**
 * Normaliza un valor para mostrarlo: devuelve `EMPTY_FIELD` cuando el campo
 * falta o es una cadena vacía/compuesta solo por espacios (Req 5.3, 5.4).
 */
function displayField(value?: unknown): string {
  if (typeof value !== "string") {
    return EMPTY_FIELD;
  }
  const trimmed = value.trim();
  return trimmed === "" ? EMPTY_FIELD : trimmed;
}

/**
 * Formatea un importe numérico de forma legible. Devuelve `EMPTY_FIELD` cuando
 * no hay un número utilizable (Req 5.3).
 */
function displayAmount(amount?: number): string {
  if (typeof amount !== "number" || Number.isNaN(amount)) {
    return EMPTY_FIELD;
  }
  return amount.toLocaleString("es-MX");
}

/** Fila etiqueta/valor de la lista de definición del detalle. */
function DetailRow({
  label,
  children,
}: {
  readonly label: string;
  readonly children: React.ReactNode;
}) {
  return (
    <div className="flex flex-col gap-0.5 sm:flex-row sm:gap-2">
      <dt className="min-w-28 text-sm font-medium text-muted-foreground">
        {label}
      </dt>
      <dd className="text-sm">{children}</dd>
    </div>
  );
}

/** Indicador de carga reutilizable con texto accesible (Req 5.5). */
function DetailLoader({ label }: { readonly label: string }) {
  return (
    <div className="flex items-center gap-2 text-muted-foreground">
      <Loader2 className="size-4 animate-spin" aria-hidden />
      {label}
    </div>
  );
}

/**
 * Mensaje de error que conserva visible el identificador del recurso para que el
 * usuario pueda reintentar o reportarlo (Req 5.6).
 */
function DetailError({
  message,
  idLabel,
  id,
}: {
  readonly message: string;
  readonly idLabel: string;
  readonly id: string;
}) {
  return (
    <div className="space-y-1 text-sm">
      <p className="text-destructive">{message}</p>
      <p className="text-muted-foreground">
        {idLabel} <span className="font-mono">{id}</span>
      </p>
    </div>
  );
}

/**
 * Cuerpo del detalle de un plan: resuelve los datos con `usePlan(id)` y muestra
 * estado de carga (Req 5.5), el detalle del plan (Req 5.3) o un error que
 * conserva visible el `id` (Req 5.6).
 */
function PlanDetailBody({
  id,
  onClose,
}: {
  readonly id: string;
  readonly onClose: () => void;
}) {
  const { data, isLoading, isError } = usePlan(id, { enabled: Boolean(id) });
  const perms = usePermissions();
  const router = useRouter();
  const pathname = usePathname();

  if (isLoading) {
    return <DetailLoader label="Cargando plan…" />;
  }

  if (isError || !data) {
    return (
      <DetailError
        message="No se pudo cargar el plan."
        idLabel="Plan:"
        id={id}
      />
    );
  }

  // Identificador del plan, resuelto con independencia de `id`/`plan_id`; si no
  // viene en los datos se cae a la prop `id`.
  const resolvedPlanId = planId(data) || id;
  // Visibilidad del botón "Editar plan": solo si el permiso lo permite para el
  // país del propio plan (la seguridad real la aplica el BFF).
  const canEdit = can(perms, "plans", "editar", (data.country ?? "").toLowerCase());

  function handleEdit(): void {
    // Cierra el diálogo y navega a la edición conservando el origen (`from`)
    // para que "Volver" regrese justo a donde se abrió el detalle.
    onClose();
    const params = new URLSearchParams({
      plan_id: resolvedPlanId,
      from: pathname ?? "/subscriptions",
    });
    router.push("/plans/edit?" + params.toString());
  }

  return (
    <div className="space-y-4">
      <dl className="space-y-2">
        <DetailRow label="Nombre">{displayField(data.name)}</DetailRow>
        <DetailRow label="External ID">
          {displayField(data.external_id)}
        </DetailRow>
        <DetailRow label="País">{displayField(data.country)}</DetailRow>
        <DetailRow label="Moneda">{displayField(data.currency)}</DetailRow>
        <DetailRow label="Monto">{displayAmount(data.amount)}</DetailRow>
        <DetailRow label="Plan ID">
          <span className="font-mono text-xs">{resolvedPlanId}</span>
        </DetailRow>
      </dl>

      {canEdit && resolvedPlanId !== "" ? (
        <Button
          type="button"
          variant="secondary"
          onClick={handleEdit}
          aria-label="Editar plan"
        >
          <Pencil aria-hidden="true" />
          Editar plan
        </Button>
      ) : null}
    </div>
  );
}

/**
 * Cuerpo del detalle de un cliente: resuelve los datos con
 * `useCustomer({ external_id })` y muestra estado de carga (Req 5.5), el detalle
 * del cliente (Req 5.4) o un error que conserva visible el `id` (Req 5.6).
 */
function CustomerDetailBody({
  id,
  onClose,
}: {
  readonly id: string;
  readonly onClose: () => void;
}) {
  // Se resuelve por `customer_id` interno de OTT: el BFF recorre el padrón y
  // puede devolver "no disponible" (data vacío) sin ser un error de red.
  const { data, isLoading, isError } = useCustomer(
    { customer_id: id },
    { enabled: Boolean(id) },
  );
  const perms = usePermissions();
  const router = useRouter();
  const pathname = usePathname();
  // El detalle de cliente no trae país; se usa el país activo del selector,
  // igual que el resto de acciones de UI sobre clientes.
  const country = useSelectedCountry() ?? "";

  if (isLoading) {
    return <DetailLoader label="Cargando cliente…" />;
  }

  // Sin datos (no disponible en este entorno) o error: se informa y se conserva
  // el id visible (Req 5.6) sin tumbar la vista.
  if (isError || !data) {
    return (
      <DetailError
        message="La información del cliente no está disponible en este entorno."
        idLabel="Customer ID:"
        id={id}
      />
    );
  }

  // Visibilidad del botón "Editar cliente": solo si el permiso lo permite para
  // el país activo (la seguridad real la aplica el BFF).
  const canEdit = can(perms, "customers", "editar", country);
  // La edición localiza al cliente por `email`/`external_id`; sin ninguno de
  // los dos no hay forma de abrir la vista de edición.
  const hasLocator = Boolean(data.email || data.external_id);

  // Se capturan antes del handler para que TS conserve el estrechamiento de
  // tipos (el guard de `data` no cruza la frontera del closure).
  const customerEmail = data.email;
  const customerExternalId = data.external_id;

  function handleEdit(): void {
    // Cierra el diálogo y navega a la edición conservando el origen (`from`)
    // para que "Volver" regrese justo a donde se abrió el detalle.
    onClose();
    const params = new URLSearchParams();
    if (customerEmail) {
      params.set("email", customerEmail);
    }
    if (customerExternalId) {
      params.set("external_id", customerExternalId);
    }
    params.set("from", pathname ?? "/subscriptions");
    router.push("/customers/edit?" + params.toString());
  }

  return (
    <div className="space-y-4">
      <dl className="space-y-2">
        <DetailRow label="Nombre">
          {readableCustomer(data) ?? EMPTY_FIELD}
        </DetailRow>
        <DetailRow label="Correo">{displayField(data.email)}</DetailRow>
        <DetailRow label="Teléfono">{displayField(data.phone)}</DetailRow>
        <DetailRow label="External ID">
          {displayField(data.external_id)}
        </DetailRow>
        <DetailRow label="Dirección">{displayField(data.address)}</DetailRow>
        <DetailRow label="Customer ID">
          <span className="font-mono text-xs">{id}</span>
        </DetailRow>
      </dl>

      {canEdit && hasLocator ? (
        <Button
          type="button"
          variant="secondary"
          onClick={handleEdit}
          aria-label="Editar cliente"
        >
          <Pencil aria-hidden="true" />
          Editar cliente
        </Button>
      ) : null}
    </div>
  );
}

/**
 * Selector de elementos enfocables por teclado dentro del diálogo (Req 9.2).
 * Se excluyen los elementos deshabilitados y los marcados con `tabindex="-1"`.
 */
const FOCUSABLE_SELECTOR =
  'a[href], button:not([disabled]), input, select, textarea, [tabindex]:not([tabindex="-1"])';

export function DetailDialog({
  open,
  kind,
  id,
  onClose,
  triggerRef,
}: DetailDialogProps) {
  /** Nodo contenedor del diálogo; raíz del focus trap (Req 9.2). */
  const dialogRef = React.useRef<HTMLDivElement | null>(null);
  /**
   * Elemento que tenía el foco justo antes de abrir el diálogo. Sirve de
   * respaldo para devolver el foco al cerrar cuando no hay `triggerRef`
   * (Req 9.4).
   */
  const previouslyFocused = React.useRef<HTMLElement | null>(null);

  // `onClose` se usa dentro del efecto de teclado; guardamos la última
  // referencia en un ref para no re-ejecutar el efecto (ni re-enganchar el
  // listener) cada vez que el padre pase una función nueva.
  const onCloseRef = React.useRef(onClose);
  React.useEffect(() => {
    onCloseRef.current = onClose;
  }, [onClose]);

  /**
   * Gestión de foco y teclado mientras el diálogo está abierto (Req 9.1-9.4):
   * - Al abrir: recuerda el elemento activo previo y mueve el foco al interior
   *   del diálogo (primer enfocable o el propio contenedor).
   * - Mientras está abierto: atrapa Tab/Shift+Tab para ciclar dentro del
   *   diálogo y cierra con Escape.
   * - Al cerrar (cleanup): devuelve el foco a `triggerRef` o, en su defecto, al
   *   elemento que lo tenía antes de abrir.
   */
  React.useEffect(() => {
    if (!open) {
      return;
    }

    // (Req 9.4) Recordar el foco previo como respaldo para restaurarlo.
    previouslyFocused.current =
      (document.activeElement as HTMLElement | null) ?? null;

    // Capturamos el disparador al abrir: es el botón "Ver detalle" que lanzó el
    // diálogo y permanece montado durante toda la vida del modal, por lo que es
    // seguro leerlo aquí y usarlo en el cleanup (Req 9.4).
    const triggerNode = triggerRef?.current ?? null;

    const dialogNode = dialogRef.current;

    /** Lista actual de elementos enfocables dentro del diálogo. */
    function getFocusable(): HTMLElement[] {
      if (!dialogNode) {
        return [];
      }
      return Array.from(
        dialogNode.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR),
      );
    }

    // (Req 9.1) Llevar el foco al interior del diálogo al abrir: primer
    // elemento enfocable o, si no hubiera ninguno, el contenedor (tabIndex=-1).
    const focusables = getFocusable();
    if (focusables.length > 0) {
      focusables[0].focus();
    } else {
      dialogNode?.focus();
    }

    function handleKeyDown(event: KeyboardEvent): void {
      // (Req 9.3) Escape cierra el diálogo.
      if (event.key === "Escape") {
        event.preventDefault();
        onCloseRef.current();
        return;
      }

      // (Req 9.2) Focus trap sobre Tab / Shift+Tab.
      if (event.key !== "Tab") {
        return;
      }

      const items = getFocusable();

      // 0 enfocables: no hay a dónde tabular; retener el foco en el contenedor.
      if (items.length === 0) {
        event.preventDefault();
        dialogNode?.focus();
        return;
      }

      const first = items[0];
      const last = items[items.length - 1];
      const active = document.activeElement as HTMLElement | null;

      // 1 enfocable (o foco fuera del diálogo): mantenerlo siempre en él.
      if (items.length === 1) {
        event.preventDefault();
        first.focus();
        return;
      }

      if (event.shiftKey) {
        // Shift+Tab desde el primero → ciclar al último.
        if (active === first || !dialogNode?.contains(active)) {
          event.preventDefault();
          last.focus();
        }
      } else {
        // Tab desde el último → ciclar al primero.
        if (active === last || !dialogNode?.contains(active)) {
          event.preventDefault();
          first.focus();
        }
      }
    }

    window.addEventListener("keydown", handleKeyDown);

    return () => {
      window.removeEventListener("keydown", handleKeyDown);
      // (Req 9.4) Devolver el foco a quien abrió el diálogo: preferimos el
      // `triggerRef`; si no existe, el elemento recordado al abrir.
      const target = triggerNode ?? previouslyFocused.current;
      target?.focus();
    };
  }, [open, triggerRef]);

  if (!open) {
    return null;
  }

  const title = kind === "plan" ? "Detalle del plan" : "Detalle del cliente";
  const closeLabel =
    kind === "plan" ? "Cerrar el detalle del plan" : "Cerrar el detalle del cliente";

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4"
      role="dialog"
      aria-modal="true"
      aria-labelledby="detail-dialog-title"
      onClick={onClose}
    >
      <Card
        ref={dialogRef}
        tabIndex={-1}
        className="w-full max-w-md shadow-lg outline-none"
        onClick={(event) => event.stopPropagation()}
      >
        <CardHeader className="space-y-1">
          <CardTitle id="detail-dialog-title" className="text-xl">
            {title}
          </CardTitle>
        </CardHeader>

        <CardContent className="space-y-4">
          {kind === "plan" ? (
            <PlanDetailBody id={id} onClose={onClose} />
          ) : (
            <CustomerDetailBody id={id} onClose={onClose} />
          )}

          <div className="flex justify-end">
            <Button
              type="button"
              variant="outline"
              onClick={onClose}
              aria-label={closeLabel}
            >
              Cerrar
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

export default DetailDialog;
