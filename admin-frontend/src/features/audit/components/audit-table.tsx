"use client";

/**
 * Tabla de la bitácora de auditoría (audit-log, tarea 10).
 *
 * Conecta el hook `useAuditLogs` (TanStack Query) con el componente controlado
 * `ResourceTable`, manteniendo en estado local la página, el tamaño de página y
 * los filtros aplicados. El filtro principal es el id del objeto operado
 * (`object_id`), presentado con el patrón "Buscar por" (un selector de criterio
 * + un campo) consistente con Customers/Cards; además acota por recurso, actor
 * y rango de fechas.
 *
 * Cada fila muestra fecha, actor, recurso, acción, id de objeto (monoespaciado),
 * país y resultado (éxito/error con color), y permite expandir el payload de
 * entrada registrado. El estado "sin resultados" lo maneja `ResourceTable` sin
 * error; un error real de red/servidor se muestra como banner inline.
 */
import * as React from "react";
import {
  ChevronDown,
  ChevronRight,
  CornerUpLeft,
  RefreshCcw,
  Search,
  X,
} from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent } from "@/components/ui/card";
import { ScrollToTopButton } from "@/components/scroll-to-top-button";
import { getScrollBehavior, useScrollToTop } from "@/lib/use-scroll-to-top";
import {
  ResourceTable,
  type PaginationMeta,
  type ResourceColumn,
} from "@/components/resource-table";
import { FeedbackBanner } from "@/components/feedback-banner";
import type { ListParams } from "@/types/api";
import {
  useAuditLogs,
  type AuditEntry,
  type AuditFilters,
} from "@/features/audit/hooks";
import type { Resource } from "@/lib/permissions";

const DEFAULT_RECORDS_PER_PAGE = 25;

/** Opciones del selector de recurso (incluye "todos"). */
const RESOURCE_OPTIONS: ReadonlyArray<{
  readonly value: Resource | "";
  readonly label: string;
}> = [
  { value: "", label: "Todos los recursos" },
  { value: "customers", label: "Customers" },
  { value: "plans", label: "Plans" },
  { value: "subscriptions", label: "Subscriptions" },
  { value: "cards", label: "Cards" },
  { value: "users", label: "Usuarios" },
  { value: "roles", label: "Roles" },
];

/** Etiquetas legibles de recurso para la columna de la tabla. */
const RESOURCE_LABELS: Readonly<Record<string, string>> = {
  customers: "Customers",
  plans: "Plans",
  subscriptions: "Subscriptions",
  cards: "Cards",
  users: "Usuarios",
  roles: "Roles",
  audit: "Auditoría",
};

/** Etiquetas legibles de acción. */
const ACTION_LABELS: Readonly<Record<string, string>> = {
  ver: "Ver",
  crear: "Crear",
  editar: "Editar",
  eliminar: "Eliminar",
  cambiar: "Cambiar",
  cancelar: "Cancelar",
  recargar: "Recargar",
};

/** Formatea un timestamp RFC3339 a fecha y hora local legible. */
function formatDateTime(value: string): string {
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) {
    return value;
  }
  return d.toLocaleString("es-MX", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

export function AuditTable() {
  const [page, setPage] = React.useState(1);
  const [recordsPerPage, setRecordsPerPage] = React.useState(
    DEFAULT_RECORDS_PER_PAGE,
  );

  // Borradores de filtro (lo que el usuario escribe) y filtros aplicados (lo que
  // viaja al hook) al pulsar "Aplicar".
  const [objectIdDraft, setObjectIdDraft] = React.useState("");
  const [resourceDraft, setResourceDraft] = React.useState<Resource | "">("");
  const [actorDraft, setActorDraft] = React.useState("");
  const [fromDraft, setFromDraft] = React.useState("");
  const [toDraft, setToDraft] = React.useState("");
  const [filters, setFilters] = React.useState<AuditFilters>({});

  // Fila expandida (payload visible) por id de entrada.
  const [expandedId, setExpandedId] = React.useState<string | null>(null);

  // Ancla + desplazamiento al tope de la vista ("Ir al inicio").
  const { topRef, scrollToTop } = useScrollToTop();

  // Refs para el enfoque B del payload:
  // - `payloadDetailRef`: contenedor de la Card "Payload registrado" (objetivo
  //   del auto-scroll al abrir).
  // - `payloadHeadingRef`: encabezado del payload (recibe el foco al abrir).
  // - `triggerRef`: botón "Payload" que abrió el detalle (para "Volver a la fila").
  const payloadDetailRef = React.useRef<HTMLDivElement | null>(null);
  const payloadHeadingRef = React.useRef<HTMLHeadingElement | null>(null);
  const triggerRef = React.useRef<HTMLButtonElement | null>(null);

  const params: ListParams = React.useMemo(
    () => ({ page, recordsPerPage }),
    [page, recordsPerPage],
  );

  const { data, isLoading, isFetching, isError, error, refetch } = useAuditLogs(
    params,
    filters,
    {
      placeholderData: (previous) => previous,
    },
  );

  const applyFilters = React.useCallback(() => {
    const next: {
      objectId?: string;
      resource?: Resource;
      actor?: string;
      from?: string;
      to?: string;
    } = {};
    const objectId = objectIdDraft.trim();
    const actor = actorDraft.trim();
    if (objectId !== "") {
      next.objectId = objectId;
    }
    if (resourceDraft !== "") {
      next.resource = resourceDraft;
    }
    if (actor !== "") {
      next.actor = actor;
    }
    if (fromDraft !== "") {
      next.from = fromDraft;
    }
    if (toDraft !== "") {
      next.to = toDraft;
    }
    setFilters(next);
    setPage(1);
    setExpandedId(null);
  }, [objectIdDraft, resourceDraft, actorDraft, fromDraft, toDraft]);

  const clearFilters = React.useCallback(() => {
    setObjectIdDraft("");
    setResourceDraft("");
    setActorDraft("");
    setFromDraft("");
    setToDraft("");
    setFilters({});
    setPage(1);
    setExpandedId(null);
  }, []);

  const hasActiveFilters = Object.keys(filters).length > 0;
  const hasDraft =
    objectIdDraft !== "" ||
    resourceDraft !== "" ||
    actorDraft !== "" ||
    fromDraft !== "" ||
    toDraft !== "";

  const toggleExpand = React.useCallback(
    (id: string, event: React.MouseEvent<HTMLButtonElement>) => {
      // Al ABRIR, guardamos el botón que disparó la expansión para poder
      // devolver el foco (y volver a la fila) desde la Card del payload.
      // `ResourceTable` no expone refs por fila, así que se captura del evento.
      const button = event.currentTarget;
      setExpandedId((current) => {
        if (current === id) {
          return null;
        }
        triggerRef.current = button;
        return id;
      });
    },
    [],
  );

  // Al abrir el payload, desplazar la vista hasta su Card y mover el foco al
  // encabezado para que lectores de pantalla anuncien el detalle. Respeta
  // `prefers-reduced-motion` vía `getScrollBehavior()`.
  React.useEffect(() => {
    if (expandedId === null) {
      return;
    }
    const behavior = getScrollBehavior();
    payloadDetailRef.current?.scrollIntoView({ block: "start", behavior });
    payloadHeadingRef.current?.focus();
  }, [expandedId]);

  const columns = React.useMemo<ReadonlyArray<ResourceColumn<AuditEntry>>>(
    () => [
      {
        key: "createdAt",
        header: "Fecha",
        className: "whitespace-nowrap",
        render: (row) => (
          <span className="text-xs text-muted-foreground">
            {formatDateTime(row.createdAt)}
          </span>
        ),
      },
      {
        key: "actorName",
        header: "Actor",
        render: (row) => row.actorName || "—",
      },
      {
        key: "resource",
        header: "Recurso",
        render: (row) => RESOURCE_LABELS[row.resource] ?? row.resource,
      },
      {
        key: "action",
        header: "Acción",
        render: (row) => ACTION_LABELS[row.action] ?? row.action,
      },
      {
        key: "objectId",
        header: "ID de objeto",
        render: (row) => (
          <span className="font-mono text-xs">{row.objectId || "—"}</span>
        ),
      },
      {
        key: "country",
        header: "País",
        render: (row) =>
          row.country ? row.country.toUpperCase() : "—",
      },
      {
        key: "outcome",
        header: "Resultado",
        render: (row) => {
          const isError = row.outcome === "error";
          return (
            <span
              className={
                isError
                  ? "inline-flex items-center rounded-md bg-destructive/15 px-2 py-0.5 text-xs font-medium text-destructive"
                  : "inline-flex items-center rounded-md bg-emerald-500/15 px-2 py-0.5 text-xs font-medium text-ok-text"
              }
            >
              {isError ? "Error" : "Éxito"}
              {typeof row.statusCode === "number" && row.statusCode > 0
                ? ` · ${row.statusCode}`
                : ""}
            </span>
          );
        },
      },
      {
        key: "__detail",
        header: "Detalle",
        className: "w-24 text-right",
        render: (row) => {
          const open = expandedId === row.id;
          return (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={(event) => toggleExpand(row.id, event)}
              aria-expanded={open}
              aria-label={open ? "Ocultar payload" : "Ver payload"}
            >
              {open ? (
                <ChevronDown aria-hidden="true" />
              ) : (
                <ChevronRight aria-hidden="true" />
              )}
              Payload
            </Button>
          );
        },
      },
    ],
    [expandedId, toggleExpand],
  );

  const pagination: PaginationMeta = data?.pagination ?? {
    totalRecords: 0,
    page,
    recordsPerPage,
    totalPages: 0,
  };

  const rows = (data?.items ?? []) as AuditEntry[];
  const expandedEntry =
    expandedId !== null ? rows.find((r) => r.id === expandedId) : undefined;

  const handleRecordsPerPageChange = React.useCallback((value: number) => {
    setRecordsPerPage(value);
    setPage(1);
    setExpandedId(null);
  }, []);

  return (
    <div className="space-y-4">
      {/* Ancla invisible al tope de la vista para el botón flotante "Ir al inicio". */}
      <div ref={topRef} aria-hidden="true" />

      {/* Botón flotante (esquina inferior derecha) para volver al tope de la
          vista sin tener que desplazarse manualmente. */}
      <ScrollToTopButton onClick={scrollToTop} />

      {/* Barra de acciones: refresco manual de la bitácora. El historial crece
          con escrituras hechas en otras vistas que no invalidan esta query, por
          lo que el botón fuerza una consulta al estado actual. */}
      <div className="flex justify-end">
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => {
            void refetch();
          }}
          disabled={isFetching}
          aria-label="Actualizar la bitácora"
        >
          <RefreshCcw
            aria-hidden="true"
            className={isFetching ? "animate-spin" : undefined}
          />
          {isFetching ? "Actualizando..." : "Actualizar"}
        </Button>
      </div>

      {/* Filtros de la bitácora. El id de objeto es el filtro principal. */}
      <Card>
        <CardContent className="pt-6">
          <form
            className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3"
            onSubmit={(event) => {
              event.preventDefault();
              applyFilters();
            }}
          >
            <div className="space-y-1.5 lg:col-span-2">
              <Label htmlFor="audit_object_id">ID de objeto</Label>
              <Input
                id="audit_object_id"
                value={objectIdDraft}
                onChange={(event) => setObjectIdDraft(event.target.value)}
                placeholder="ID del customer, plan, suscripción, etc."
                autoComplete="off"
              />
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="audit_resource">Recurso</Label>
              <select
                id="audit_resource"
                value={resourceDraft}
                onChange={(event) =>
                  setResourceDraft(event.target.value as Resource | "")
                }
                className="h-10 w-full rounded-md border border-input-border bg-input px-3 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                {RESOURCE_OPTIONS.map((opt) => (
                  <option key={opt.value || "all"} value={opt.value}>
                    {opt.label}
                  </option>
                ))}
              </select>
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="audit_actor">Usuario</Label>
              <Input
                id="audit_actor"
                value={actorDraft}
                onChange={(event) => setActorDraft(event.target.value)}
                placeholder="Nombre o id del usuario"
                autoComplete="off"
              />
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="audit_from">Desde</Label>
              <input
                id="audit_from"
                type="date"
                value={fromDraft}
                onChange={(event) => setFromDraft(event.target.value)}
                className="flex h-10 w-full rounded-md border border-input-border bg-input px-3 py-2 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              />
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="audit_to">Hasta</Label>
              <input
                id="audit_to"
                type="date"
                value={toDraft}
                onChange={(event) => setToDraft(event.target.value)}
                className="flex h-10 w-full rounded-md border border-input-border bg-input px-3 py-2 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              />
            </div>

            <div className="flex items-end gap-2 sm:col-span-2 lg:col-span-3">
              <Button type="submit" variant="secondary">
                <Search aria-hidden="true" />
                Aplicar filtros
              </Button>
              {hasActiveFilters || hasDraft ? (
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
            "No se pudo cargar la bitácora de auditoría. Intenta de nuevo."
          }
        />
      ) : null}

      <ResourceTable<AuditEntry>
        columns={columns}
        rows={rows}
        rowKey={(row) => row.id}
        pagination={pagination}
        isLoading={isLoading}
        onPageChange={setPage}
        onRecordsPerPageChange={handleRecordsPerPageChange}
        pageSizeOptions={[25, 50, 100]}
        emptyMessage="Sin registros de auditoría"
      />

      {/* Detalle del payload de la entrada expandida. */}
      {expandedEntry ? (
        <Card ref={payloadDetailRef}>
          <CardContent className="space-y-3 pt-6">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <h2
                ref={payloadHeadingRef}
                tabIndex={-1}
                className="text-sm font-semibold text-foreground outline-none"
              >
                Payload registrado
              </h2>
              <div className="flex items-center gap-2">
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onClick={() => {
                    // Regresa al botón "Payload" que abrió el detalle y le
                    // devuelve el foco (respeta prefers-reduced-motion).
                    const behavior = getScrollBehavior();
                    triggerRef.current?.scrollIntoView({
                      block: "center",
                      behavior,
                    });
                    triggerRef.current?.focus();
                  }}
                  aria-label="Volver a la fila del registro"
                >
                  <CornerUpLeft aria-hidden="true" />
                  Volver a la fila
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onClick={() => setExpandedId(null)}
                  aria-label="Cerrar detalle"
                >
                  <X aria-hidden="true" />
                  Cerrar
                </Button>
              </div>
            </div>
            {expandedEntry.message ? (
              <p className="text-sm text-muted-foreground">
                {expandedEntry.message}
              </p>
            ) : null}
            {expandedEntry.errorCode ? (
              <p className="text-xs text-muted-foreground">
                Código de error:{" "}
                <span className="font-mono">{expandedEntry.errorCode}</span>
              </p>
            ) : null}
            {expandedEntry.payload &&
            Object.keys(expandedEntry.payload).length > 0 ? (
              <pre className="max-h-96 overflow-auto rounded-md border border-border bg-muted/40 p-3 text-xs text-foreground">
                {JSON.stringify(expandedEntry.payload, null, 2)}
              </pre>
            ) : (
              <p className="text-sm text-muted-foreground">
                Esta operación no registró payload de entrada.
              </p>
            )}
          </CardContent>
        </Card>
      ) : null}
    </div>
  );
}

export default AuditTable;
