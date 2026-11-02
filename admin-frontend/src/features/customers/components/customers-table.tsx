"use client";

/**
 * Tabla de Customers (tarea 16.1, Req. 6).
 *
 * Conecta el hook `useCustomers` (TanStack Query) con el componente controlado
 * `ResourceTable`, manteniendo en estado local la página, el tamaño de página,
 * el ordenamiento y el término de búsqueda. La búsqueda localiza por email o
 * external_id (Req. 6.2); el BFF la recibe como `search`.
 *
 * El estado de lista vacía ("sin resultados") lo maneja `ResourceTable` sin
 * mostrar error (Req. 6.5); un error real de red/servidor se muestra como un
 * banner inline (sin proveedor de toasts compartido).
 */
import * as React from "react";
import Link from "next/link";
import { Eye, Search, X } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent } from "@/components/ui/card";
import { ScrollToTopButton } from "@/components/scroll-to-top-button";
import { useScrollToTop } from "@/lib/use-scroll-to-top";
import {
  ResourceTable,
  type PaginationMeta,
  type ResourceColumn,
  type SortState,
} from "@/components/resource-table";
import type { ListParams } from "@/types/api";
import {
  useCustomers,
  type Customer,
  type CustomerFilters,
} from "@/features/customers/hooks";
import { FeedbackBanner } from "@/components/feedback-banner";

/** Campos de ordenamiento permitidos para Customers (design.md). */
const CUSTOMER_SORT_FIELDS = [
  "updated_at",
  "created_at",
  "state",
  "first_name",
  "second_last_name",
  "email",
  "gender",
] as const;

const DEFAULT_RECORDS_PER_PAGE = 10;

/** Construye la URL de detalle preservando el localizador del cliente. */
function buildDetailHref(row: Customer): string {
  const params = new URLSearchParams();
  if (row.email) {
    params.set("email", String(row.email));
  }
  if (row.external_id) {
    params.set("external_id", String(row.external_id));
  }
  if (row.customer_id) {
    params.set("customer_id", String(row.customer_id));
  }
  const query = params.toString();
  return query ? `/customers/detail?${query}` : "/customers/detail";
}

/** Criterio por el que se busca un cliente (patrón del selector de Cards). */
type SearchKind = "customer_id" | "email" | "external_id";

/** Metadatos de cada criterio para poblar el selector y el campo de valor. */
const SEARCH_KINDS: ReadonlyArray<{
  readonly value: SearchKind;
  readonly label: string;
  readonly placeholder: string;
}> = [
  { value: "customer_id", label: "Customer ID", placeholder: "UUID del cliente" },
  {
    value: "email",
    label: "Correo electrónico",
    placeholder: "cliente@correo.com",
  },
  { value: "external_id", label: "External ID", placeholder: "External ID" },
];

export function CustomersTable() {
  const [page, setPage] = React.useState(1);
  const [recordsPerPage, setRecordsPerPage] = React.useState(
    DEFAULT_RECORDS_PER_PAGE,
  );
  const [sort, setSort] = React.useState<SortState | null>(null);
  const { topRef, scrollToTop } = useScrollToTop();

  // Filtro al estilo del selector de Cards: un criterio ("Buscar por") + un
  // único campo de valor. `searchDraft` es lo que el usuario escribe; `filters`
  // es lo aplicado (se envía al hook) al pulsar "Aplicar".
  const [searchKind, setSearchKind] = React.useState<SearchKind>("customer_id");
  const [searchDraft, setSearchDraft] = React.useState("");
  const [filters, setFilters] = React.useState<CustomerFilters>({});

  const params: ListParams = React.useMemo(
    () => ({
      page,
      recordsPerPage,
      orderBy: sort?.field,
      order: sort?.order,
    }),
    [page, recordsPerPage, sort],
  );

  const { data, isLoading, isError, error } = useCustomers(params, filters, {
    placeholderData: (previous) => previous,
  });

  const applyFilters = React.useCallback(() => {
    const value = searchDraft.trim();
    // Solo el criterio seleccionado viaja como filtro; los demás se limpian.
    setFilters(
      value === "" ? {} : ({ [searchKind]: value } as CustomerFilters),
    );
    setPage(1);
  }, [searchKind, searchDraft]);

  const clearFilters = React.useCallback(() => {
    setSearchDraft("");
    setFilters({});
    setPage(1);
  }, []);

  const hasActiveFilters =
    filters.email !== undefined ||
    filters.external_id !== undefined ||
    filters.customer_id !== undefined;

  const currentKind =
    SEARCH_KINDS.find((k) => k.value === searchKind) ?? SEARCH_KINDS[0];

  const columns = React.useMemo<ReadonlyArray<ResourceColumn<Customer>>>(() => {
    const base: ResourceColumn<Customer>[] = [
      {
        key: "customer_id",
        header: "Customer ID",
        render: (row) => (
          <span className="font-mono text-xs">
            {row.customer_id ? String(row.customer_id) : "—"}
          </span>
        ),
      },
      { key: "external_id", header: "External ID" },
      {
        key: "first_name",
        header: "Nombre",
        sortable: true,
        render: (row) =>
          [row.first_name, row.last_name].filter(Boolean).join(" ") || "—",
      },
      { key: "email", header: "Email", sortable: true },
      { key: "gender", header: "Género", sortable: true },
      {
        key: "__actions",
        header: "Acciones",
        className: "w-24 text-right",
        render: (row) => (
          <Button asChild variant="outline" size="sm">
            <Link href={buildDetailHref(row)} aria-label="Ver cliente">
              <Eye aria-hidden="true" />
              Ver
            </Link>
          </Button>
        ),
      },
    ];
    return base;
  }, []);

  const pagination: PaginationMeta = data?.pagination ?? {
    totalRecords: 0,
    page,
    recordsPerPage,
    totalPages: 0,
  };

  const rows = (data?.items ?? []) as Customer[];

  const handleSortChange = React.useCallback((next: SortState) => {
    // Solo aceptamos campos permitidos por el recurso (Req. 6.4).
    if (
      (CUSTOMER_SORT_FIELDS as readonly string[]).includes(next.field)
    ) {
      setSort(next);
      setPage(1);
    }
  }, []);

  const handleRecordsPerPageChange = React.useCallback((value: number) => {
    setRecordsPerPage(value);
    setPage(1);
  }, []);

  return (
    <div className="space-y-4">
      {/* Ancla invisible al tope de la vista para el botón flotante "Ir al inicio". */}
      <div ref={topRef} aria-hidden="true" />

      {/* Botón flotante (esquina inferior derecha) para volver al tope. */}
      <ScrollToTopButton onClick={scrollToTop} />

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
              <Label htmlFor="customer_search_kind">Buscar por</Label>
              <select
                id="customer_search_kind"
                value={searchKind}
                onChange={(event) => {
                  setSearchKind(event.target.value as SearchKind);
                }}
                className="h-10 w-full rounded-md border border-input bg-input px-3 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring sm:w-48"
              >
                {SEARCH_KINDS.map((k) => (
                  <option key={k.value} value={k.value}>
                    {k.label}
                  </option>
                ))}
              </select>
            </div>

            <div className="flex-1 space-y-1.5">
              <Label htmlFor="customer_search_value">{currentKind.label}</Label>
              <Input
                id="customer_search_value"
                value={searchDraft}
                onChange={(event) => setSearchDraft(event.target.value)}
                placeholder={currentKind.placeholder}
                type={searchKind === "email" ? "email" : "text"}
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
        <FeedbackBanner
          tone="error"
          message={
            error?.message ??
            "No se pudo cargar la lista de clientes. Intenta de nuevo."
          }
        />
      ) : null}

      <ResourceTable<Customer>
        columns={columns}
        rows={rows}
        rowKey={(row) =>
          String(row.external_id ?? row.email ?? JSON.stringify(row))
        }
        pagination={pagination}
        isLoading={isLoading}
        sort={sort}
        onSortChange={handleSortChange}
        onPageChange={setPage}
        onRecordsPerPageChange={handleRecordsPerPageChange}
        emptyMessage="Sin resultados"
      />
    </div>
  );
}

export default CustomersTable;
