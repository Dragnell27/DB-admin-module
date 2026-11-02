"use client";

/**
 * Tabla de Subscriptions con filtros, paginación y orden (tarea 16.3, Req. 13).
 *
 * Es un componente controlado-contenedor: mantiene el estado de listado (página,
 * tamaño, orden, búsqueda y filtros por `customer_id`/`plan_id`), lo traduce a
 * los parámetros/filtros de `useSubscriptions` y alimenta la `ResourceTable`
 * reutilizable (tarea 14.2). El 404 de listado se refleja como estado vacío sin
 * error gracias a la normalización del BFF (Req. 13.5).
 *
 * Navegación en lugar de paneles (tarea 9.1, Req. 1.1, 1.2, 2.1): las acciones de
 * escritura (Crear/Cambiar/Cancelar) ya no abren paneles embebidos vía callbacks;
 * ahora navegan a rutas dedicadas con `useRouter()`. Las props `onCreate`/
 * `onChange`/`onCancel` se conservan como opcionales por compatibilidad con el
 * contenedor/tests, pero la navegación la resuelve internamente la tabla.
 *
 * Celdas legibles (Req. 6.1, 6.2, 6.6): se añaden columnas "Plan" y "Cliente"
 * que resuelven nombre/correo mediante `PlanCell`/`CustomerCell`, manteniendo
 * visibles las columnas con los ids crudos (Req. 6.6).
 *
 * Ver detalle (Req. 5.1, 5.2, 8.3): junto al plan_id y customer_id de cada fila
 * hay un botón "Ver detalle" disponible para cualquier rol que notifica al
 * contenedor (`onViewDetail`) para abrir el `DetailDialog`.
 */
import * as React from "react";
import Link from "next/link";
import { Eye, Plus, Search, X } from "lucide-react";
import { useRouter } from "next/navigation";

import { Button } from "@/components/ui/button";
import { ReloadButton } from "@/components/reload-button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent } from "@/components/ui/card";
import { ScrollToTopButton } from "@/components/scroll-to-top-button";
import { useScrollToTop } from "@/lib/use-scroll-to-top";
import {
  MAX_RECORDS_PER_PAGE,
  ResourceTable,
  type PaginationMeta,
  type ResourceColumn,
  type SortState,
} from "@/components/resource-table";
import { getCountry, subscribeCountry } from "@/lib/country";
import { can } from "@/lib/permissions";
import { usePermissions } from "@/lib/use-permissions";
import {
  useSubscriptions,
  type Subscription,
  type SubscriptionFilters,
} from "@/features/subscriptions/hooks";
import type { ListParams } from "@/types/api";
import {
  CustomerCell,
  PlanCell,
} from "@/features/subscriptions/components/readable-cells";

/** Criterio por el que se busca una suscripción (patrón del selector de Cards). */
type SubscriptionSearchKind = "customer_id" | "plan_id";

/** Metadatos de cada criterio para el selector y el campo de valor. */
const SUBSCRIPTION_SEARCH_KINDS: ReadonlyArray<{
  readonly value: SubscriptionSearchKind;
  readonly label: string;
  readonly placeholder: string;
}> = [
  { value: "customer_id", label: "Customer ID", placeholder: "UUID del cliente" },
  { value: "plan_id", label: "Plan ID", placeholder: "UUID del plan" },
];

/**
 * Campo de ordenamiento por defecto para Subscriptions. El backend sólo admite
 * `order_by ∈ {created_at, updated_at, customer_id, plan_id, status}` para este
 * recurso (validation/ordering.go). `subscription_id` NO es un campo de orden
 * permitido, por lo que se usa `created_at` (el default del contrato OTT).
 */
const DEFAULT_ORDER_BY = "created_at";
const DEFAULT_PAGE = 1;
const DEFAULT_RECORDS_PER_PAGE = 10;

/** Lee el país seleccionado de forma reactiva y SSR-safe (igual que el selector). */
function useSelectedCountry(): string | null {
  return React.useSyncExternalStore(
    subscribeCountry,
    () => getCountry(),
    () => null,
  );
}

/**
 * Formatea la fecha de activación (`subscription_start_date`, RFC3339) a una
 * fecha local legible (DD/MM/AAAA). Devuelve "—" si falta o no es parseable.
 */
function formatStartDate(value: unknown): string {
  if (typeof value !== "string" || value.trim() === "") {
    return "—";
  }
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) {
    return value;
  }
  return d.toLocaleDateString("es-GT", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  });
}

/** Tipo de recurso que puede inspeccionarse con el botón "Ver detalle". */
export type DetailKind = "plan" | "customer";

/**
 * Firma del callback para abrir el diálogo de detalle. El contenedor
 * (`page.tsx`, tarea 9.2) monta el `DetailDialog` y usa `triggerRef` para
 * devolver el foco al botón que lo abrió al cerrarse.
 */
export type OnViewDetail = (
  kind: DetailKind,
  id: string,
  triggerRef: React.RefObject<HTMLElement | null>,
) => void;

export interface SubscriptionsTableProps {
  /** Si el usuario puede ejecutar escrituras (admin). Controla acciones. */
  readonly canWrite: boolean;
  /**
   * @deprecated La creación ahora navega a `/subscriptions/new`. Se conserva
   * como opcional por compatibilidad con el contenedor/tests; ya no se usa.
   */
  readonly onCreate?: () => void;
  /**
   * @deprecated El cambio ahora navega a `/subscriptions/edit`. Se conserva
   * como opcional por compatibilidad; ya no se usa.
   */
  readonly onChange?: (row: Subscription) => void;
  /**
   * @deprecated La cancelación ahora navega a `/subscriptions/cancel`. Se
   * conserva como opcional por compatibilidad; ya no se usa.
   */
  readonly onCancel?: (row: Subscription) => void;
  /**
   * Notifica al contenedor para abrir el `DetailDialog` del plan o cliente de
   * una fila (Req. 5.1, 5.2). Disponible para cualquier rol (Req. 8.3).
   */
  readonly onViewDetail?: OnViewDetail;
}

/**
 * Botón "Ver detalle" por celda (Req. 5.1, 5.2, 8.3).
 *
 * Es un subcomponente propio para poder usar `useRef` por botón respetando las
 * reglas de hooks (la tabla itera filas, así que no puede crear refs en el
 * bucle). Al hacer click entrega su `ref` al contenedor para que el diálogo
 * devuelva el foco al cerrarse.
 */
function ViewDetailButton({
  kind,
  id,
  onViewDetail,
}: {
  readonly kind: DetailKind;
  readonly id: string;
  readonly onViewDetail?: OnViewDetail;
}) {
  const ref = React.useRef<HTMLButtonElement | null>(null);
  const label =
    kind === "plan" ? "Ver detalle del plan" : "Ver detalle del cliente";
  return (
    <Button
      ref={ref}
      type="button"
      variant="ghost"
      size="sm"
      className="h-6 px-1.5"
      aria-label={label}
      onClick={() => onViewDetail?.(kind, id, ref)}
    >
      <Eye aria-hidden="true" />
      <span className="sr-only">Ver detalle</span>
    </Button>
  );
}

export function SubscriptionsTable({
  onViewDetail,
}: SubscriptionsTableProps) {
  const router = useRouter();

  const perms = usePermissions();
  const country = useSelectedCountry() ?? "";
  // Visibilidad del botón "Crear" según permiso en el país seleccionado. Las
  // acciones Cambiar/Cancelar se movieron a la página de detalle.
  const canCrear = can(perms, "subscriptions", "crear", country);

  const [page, setPage] = React.useState(DEFAULT_PAGE);
  const [recordsPerPage, setRecordsPerPage] = React.useState(
    DEFAULT_RECORDS_PER_PAGE,
  );
  const [sort, setSort] = React.useState<SortState>({
    field: DEFAULT_ORDER_BY,
    order: "asc",
  });
  const { topRef, scrollToTop } = useScrollToTop();

  // Filtro con selector "Buscar por" + un único campo (patrón de Cards).
  const [searchKind, setSearchKind] =
    React.useState<SubscriptionSearchKind>("customer_id");
  const [searchDraft, setSearchDraft] = React.useState("");
  const [filters, setFilters] = React.useState<SubscriptionFilters>({});

  const params: ListParams = React.useMemo(
    () => ({
      page,
      recordsPerPage,
      orderBy: sort.field,
      order: sort.order,
    }),
    [page, recordsPerPage, sort],
  );

  const { data, isLoading, isError, error } = useSubscriptions(params, filters);

  const pagination: PaginationMeta = data?.pagination ?? {
    totalRecords: 0,
    page,
    recordsPerPage,
    totalPages: 0,
  };

  const rows = React.useMemo<ReadonlyArray<Subscription>>(
    () => data?.items ?? [],
    [data],
  );

  // Navegación a la ruta de creación (Req. 1.1).
  const goToCreate = React.useCallback(() => {
    router.push("/subscriptions/new");
  }, [router]);

  const columns = React.useMemo<ReadonlyArray<ResourceColumn<Subscription>>>(
    () => [
      {
        // `subscription_id` NO está en el conjunto de campos ordenables del
        // backend para subscriptions, por eso la columna no es sortable.
        key: "subscription_id",
        header: "Subscription ID",
        render: (row) => (
          <span className="font-mono text-xs">
            {row.subscription_id ?? "—"}
          </span>
        ),
      },
      {
        // Columna legible del cliente (nombre) con botón "Ver detalle" (Req. 5.2,
        // 6.2). Ya no se muestra el customer_id crudo en la tabla.
        key: "customer_readable",
        header: "Cliente",
        render: (row) => (
          <div className="flex items-center justify-between gap-2">
            <CustomerCell customerId={row.customer_id} />
            {row.customer_id ? (
              <ViewDetailButton
                kind="customer"
                id={row.customer_id}
                onViewDetail={onViewDetail}
              />
            ) : null}
          </div>
        ),
      },
      {
        // Columna legible del plan (nombre) con botón "Ver detalle" (Req. 5.1,
        // 6.1). Ya no se muestra el plan_id crudo en la tabla.
        key: "plan_readable",
        header: "Plan",
        render: (row) => (
          <div className="flex items-center justify-between gap-2">
            <PlanCell planId={row.plan_id} />
            {row.plan_id ? (
              <ViewDetailButton
                kind="plan"
                id={row.plan_id}
                onViewDetail={onViewDetail}
              />
            ) : null}
          </div>
        ),
      },
      {
        // Fecha de activación de la suscripción (subscription_start_date).
        // Ordenable: el BFF ordena en memoria sobre el caché por este campo.
        key: "subscription_start_date",
        header: "Fecha de activación",
        sortable: true,
        render: (row) => <span>{formatStartDate(row.subscription_start_date)}</span>,
      },
      {
        key: "status",
        header: "Estado",
        sortable: true,
        render: (row) => row.status ?? "—",
      },
      {
        // "Ver" abre el detalle de la suscripción, desde donde se cambia/cancela
        // (según permiso). Es una acción de lectura, disponible para cualquiera
        // que pueda ver el listado.
        key: "actions",
        header: "Acciones",
        className: "text-right",
        render: (row: Subscription) => {
          const id = row.subscription_id ?? "";
          const href = id
            ? `/subscriptions/detail?subscription_id=${encodeURIComponent(id)}`
            : "/subscriptions/detail";
          return (
            <Button asChild variant="outline" size="sm">
              <Link href={href} aria-label="Ver suscripción">
                <Eye aria-hidden="true" />
                Ver
              </Link>
            </Button>
          );
        },
      } satisfies ResourceColumn<Subscription>,
    ],
    [onViewDetail],
  );

  const handleSort = React.useCallback((next: SortState) => {
    setSort(next);
    setPage(DEFAULT_PAGE);
  }, []);

  const handleRecordsPerPage = React.useCallback((next: number) => {
    if (next > 0 && next <= MAX_RECORDS_PER_PAGE) {
      setRecordsPerPage(next);
      setPage(DEFAULT_PAGE);
    }
  }, []);

  const applyFilters = React.useCallback(() => {
    const value = searchDraft.trim();
    setFilters(
      value === "" ? {} : ({ [searchKind]: value } as SubscriptionFilters),
    );
    setPage(DEFAULT_PAGE);
  }, [searchKind, searchDraft]);

  const clearFilters = React.useCallback(() => {
    setSearchDraft("");
    setFilters({});
    setPage(DEFAULT_PAGE);
  }, []);

  const hasActiveFilters =
    filters.customer_id !== undefined || filters.plan_id !== undefined;

  const currentKind =
    SUBSCRIPTION_SEARCH_KINDS.find((k) => k.value === searchKind) ??
    SUBSCRIPTION_SEARCH_KINDS[0];

  return (
    <div className="space-y-4">
      {/* Ancla invisible al tope de la vista para el botón flotante "Ir al inicio". */}
      <div ref={topRef} aria-hidden="true" />

      {/* Botón flotante (esquina inferior derecha) para volver al tope. */}
      <ScrollToTopButton onClick={scrollToTop} />

      {/* Barra de acciones (recargar + crear). */}
      <div className="flex items-center justify-end gap-2">
        <ReloadButton resource="subscriptions" />
        {canCrear ? (
          <Button type="button" onClick={goToCreate}>
            <Plus aria-hidden="true" />
            Crear
          </Button>
        ) : null}
      </div>

      {/* Filtro con selector "Buscar por" + un único campo (patrón de Cards). */}
      <Card>
        <CardContent className="pt-6">
          <form
            className="flex flex-col gap-3 sm:flex-row sm:items-end"
            onSubmit={(event) => {
              event.preventDefault();
              applyFilters();
            }}
          >
            <div className="space-y-1.5">
              <Label htmlFor="subscription_search_kind">Buscar por</Label>
              <select
                id="subscription_search_kind"
                value={searchKind}
                onChange={(event) =>
                  setSearchKind(event.target.value as SubscriptionSearchKind)
                }
                className="h-10 w-full rounded-md border border-input bg-input px-3 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring sm:w-48"
              >
                {SUBSCRIPTION_SEARCH_KINDS.map((k) => (
                  <option key={k.value} value={k.value}>
                    {k.label}
                  </option>
                ))}
              </select>
            </div>

            <div className="flex-1 space-y-1.5">
              <Label htmlFor="subscription_search_value">
                {currentKind.label}
              </Label>
              <Input
                id="subscription_search_value"
                value={searchDraft}
                onChange={(event) => setSearchDraft(event.target.value)}
                placeholder={currentKind.placeholder}
                autoComplete="off"
              />
            </div>

            <div className="flex items-center gap-2">
              <Button type="submit" variant="secondary">
                <Search aria-hidden="true" />
                Aplicar filtros
              </Button>
              {hasActiveFilters || searchDraft !== "" ? (
                <Button type="button" variant="ghost" onClick={clearFilters}>
                  <X aria-hidden="true" />
                  Limpiar
                </Button>
              ) : null}
            </div>
          </form>
        </CardContent>
      </Card>

      {isError ? (
        <div
          role="alert"
          className="rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive"
        >
          {error?.message ?? "No se pudo cargar el listado de suscripciones."}
        </div>
      ) : null}

      <ResourceTable<Subscription>
        columns={columns}
        rows={rows}
        rowKey={(row) =>
          row.subscription_id ??
          `${row.customer_id ?? ""}:${row.plan_id ?? ""}`
        }
        pagination={pagination}
        isLoading={isLoading}
        sort={sort}
        onSortChange={handleSort}
        onPageChange={setPage}
        onRecordsPerPageChange={handleRecordsPerPage}
        emptyMessage="Sin resultados"
      />
    </div>
  );
}

export default SubscriptionsTable;
