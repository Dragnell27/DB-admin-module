"use client";

/**
 * Tabla de listado de Plans (tarea 16.2, Req. 9).
 *
 * Conecta el `ResourceTable` controlado (tarea 14.2) con el hook `usePlans`
 * (tarea 15.2): mantiene localmente `page`, `recordsPerPage`, `orderBy`,
 * `order` y `search`, y los traduce a `ListParams`. El término de búsqueda que
 * tenga forma de `plan_id` (UUID) se envía como filtro de detalle (Req. 9.2);
 * en otro caso se usa como búsqueda libre.
 *
 * Las acciones de fila (Editar/Eliminar) y el botón "Crear" solo se muestran al
 * rol `admin` (Req. 5.1); la seguridad real la impone el BFF. Los `order_by`
 * ofrecidos coinciden con los campos permitidos del recurso Plans en el backend
 * (`created_at, updated_at, name, external_id, amount, country`).
 */
import * as React from "react";
import Link from "next/link";
import { Eye, Plus, Search, X } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent } from "@/components/ui/card";
import { ReloadButton } from "@/components/reload-button";
import { ScrollToTopButton } from "@/components/scroll-to-top-button";
import { useScrollToTop } from "@/lib/use-scroll-to-top";
import {
  ResourceTable,
  type PaginationMeta,
  type ResourceColumn,
  type SortState,
} from "@/components/resource-table";
import { getCountry, subscribeCountry } from "@/lib/country";
import { can } from "@/lib/permissions";
import { usePermissions } from "@/lib/use-permissions";
import {
  planId,
  usePlans,
  type Plan,
  type PlanFilters,
} from "@/features/plans/hooks";
import type { ListParams } from "@/types/api";

/** Criterio por el que se busca un plan (patrón del selector de Cards). */
type PlanSearchKind = "plan_id" | "name" | "external_id";

/** Metadatos de cada criterio para el selector y el campo de valor. */
const PLAN_SEARCH_KINDS: ReadonlyArray<{
  readonly value: PlanSearchKind;
  readonly label: string;
  readonly placeholder: string;
}> = [
  { value: "plan_id", label: "Plan ID", placeholder: "UUID del plan" },
  { value: "name", label: "Nombre", placeholder: "Nombre del plan" },
  { value: "external_id", label: "External ID", placeholder: "External ID" },
];

/** Lee el país seleccionado de forma reactiva y SSR-safe (igual que el selector). */
function useSelectedCountry(): string | null {
  return React.useSyncExternalStore(
    subscribeCountry,
    () => getCountry(),
    () => null,
  );
}

export interface PlansTableProps {
  /** Abre el formulario de creación (solo con permiso `crear`). */
  readonly onCreate: () => void;
  /**
   * @deprecated La edición vive ahora en la página de detalle (`/plans/detail`).
   * Se conserva como opcional por compatibilidad con el contenedor.
   */
  readonly onEdit?: (plan: Plan) => void;
  /**
   * @deprecated La eliminación vive ahora en la página de detalle. Se conserva
   * como opcional por compatibilidad con el contenedor.
   */
  readonly onDelete?: (plan: Plan) => void;
}

const DEFAULT_PAGE = 1;
const DEFAULT_RECORDS_PER_PAGE = 10;

export function PlansTable({ onCreate }: PlansTableProps) {
  const perms = usePermissions();
  const country = useSelectedCountry() ?? "";
  // Visibilidad del botón "Crear" según permiso en el país seleccionado. Las
  // acciones de edición/eliminación se movieron a la página de detalle.
  const canCrear = can(perms, "plans", "crear", country);

  const [page, setPage] = React.useState(DEFAULT_PAGE);
  const [recordsPerPage, setRecordsPerPage] = React.useState(
    DEFAULT_RECORDS_PER_PAGE,
  );
  const [sort, setSort] = React.useState<SortState | null>(null);
  const { topRef, scrollToTop } = useScrollToTop();

  // Filtro con selector "Buscar por" + un único campo (patrón de Cards).
  const [searchKind, setSearchKind] = React.useState<PlanSearchKind>("name");
  const [searchDraft, setSearchDraft] = React.useState("");
  const [filters, setFilters] = React.useState<PlanFilters>({});

  const params: ListParams = React.useMemo(
    () => ({
      page,
      recordsPerPage,
      orderBy: sort?.field,
      order: sort?.order,
    }),
    [page, recordsPerPage, sort],
  );

  const query = usePlans(params, filters);

  const applyFilters = React.useCallback(() => {
    const value = searchDraft.trim();
    setFilters(value === "" ? {} : ({ [searchKind]: value } as PlanFilters));
    setPage(DEFAULT_PAGE);
  }, [searchKind, searchDraft]);

  const clearFilters = React.useCallback(() => {
    setSearchDraft("");
    setFilters({});
    setPage(DEFAULT_PAGE);
  }, []);

  const hasActiveFilters =
    filters.plan_id !== undefined ||
    filters.name !== undefined ||
    filters.external_id !== undefined;

  const currentKind =
    PLAN_SEARCH_KINDS.find((k) => k.value === searchKind) ??
    PLAN_SEARCH_KINDS[0];

  const rows: ReadonlyArray<Plan> = query.data?.items ?? [];
  const pagination: PaginationMeta = query.data?.pagination ?? {
    totalRecords: 0,
    page,
    recordsPerPage,
    totalPages: 0,
  };

  const columns = React.useMemo<ReadonlyArray<ResourceColumn<Plan>>>(() => {
    const base: ResourceColumn<Plan>[] = [
      {
        key: "plan_id",
        header: "Plan ID",
        render: (plan) => {
          const id = planId(plan);
          return <span className="font-mono text-xs">{id || "—"}</span>;
        },
      },
      {
        key: "name",
        header: "Nombre",
        sortable: true,
        render: (plan) => plan.name ?? "—",
      },
      {
        key: "external_id",
        header: "External ID",
        sortable: true,
        render: (plan) => plan.external_id ?? "—",
      },
      {
        key: "country",
        header: "País",
        sortable: true,
        render: (plan) =>
          plan.country ? String(plan.country).toUpperCase() : "—",
      },
      {
        key: "amount",
        header: "Monto",
        sortable: true,
        render: (plan) =>
          plan.amount === undefined || plan.amount === null
            ? "—"
            : `${plan.amount} ${plan.currency ? String(plan.currency).toUpperCase() : ""}`.trim(),
      },
      {
        // Acción de fila: "Ver" abre el detalle, desde donde se editan/eliminan
        // los planes (según permiso). Es una acción de lectura, disponible para
        // cualquiera que pueda ver el listado.
        key: "actions",
        header: "Acciones",
        className: "w-px whitespace-nowrap text-right",
        render: (plan) => {
          const id = planId(plan);
          const href = id
            ? `/plans/detail?plan_id=${encodeURIComponent(id)}`
            : "/plans/detail";
          return (
            <Button asChild variant="outline" size="sm">
              <Link href={href} aria-label={`Ver plan ${plan.name ?? id}`}>
                <Eye aria-hidden="true" />
                Ver
              </Link>
            </Button>
          );
        },
      },
    ];

    return base;
  }, []);

  const rowKey = React.useCallback(
    (plan: Plan): React.Key =>
      planId(plan) || plan.external_id || plan.name || "",
    [],
  );

  return (
    <div className="space-y-4">
      {/* Ancla invisible al tope de la vista para el botón flotante "Ir al inicio". */}
      <div ref={topRef} aria-hidden="true" />

      {/* Botón flotante (esquina inferior derecha) para volver al tope. */}
      <ScrollToTopButton onClick={scrollToTop} />

      <div className="flex items-center justify-between gap-4">
        <p className="text-sm text-muted-foreground">
          Gestiona los planes de suscripción disponibles.
        </p>
        <div className="flex items-center gap-2">
          <ReloadButton resource="plans" />
          {canCrear ? (
            <Button type="button" onClick={onCreate}>
              <Plus aria-hidden="true" />
              Crear
            </Button>
          ) : null}
        </div>
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
              <Label htmlFor="plan_search_kind">Buscar por</Label>
              <select
                id="plan_search_kind"
                value={searchKind}
                onChange={(event) =>
                  setSearchKind(event.target.value as PlanSearchKind)
                }
                className="h-10 w-full rounded-md border border-input-border bg-input px-3 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring sm:w-48"
              >
                {PLAN_SEARCH_KINDS.map((k) => (
                  <option key={k.value} value={k.value}>
                    {k.label}
                  </option>
                ))}
              </select>
            </div>

            <div className="flex-1 space-y-1.5">
              <Label htmlFor="plan_search_value">{currentKind.label}</Label>
              <Input
                id="plan_search_value"
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

      <ResourceTable<Plan>
        columns={columns}
        rows={rows}
        rowKey={rowKey}
        pagination={pagination}
        isLoading={query.isLoading || query.isFetching}
        sort={sort}
        onSortChange={(next) => {
          setSort(next);
          setPage(DEFAULT_PAGE);
        }}
        onPageChange={setPage}
        onRecordsPerPageChange={(next) => {
          setRecordsPerPage(next);
          setPage(DEFAULT_PAGE);
        }}
        emptyMessage="Sin resultados"
      />
    </div>
  );
}

export default PlansTable;
