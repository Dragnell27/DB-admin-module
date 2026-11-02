"use client";

/**
 * Hook de TanStack Query de la bitácora de auditoría (audit-log, tarea 10).
 *
 * Mapea a la ruta del BFF `GET /audit-logs`, protegida por el permiso
 * `audit:ver`. A diferencia de los recursos OTT, el handler de auditoría del BFF
 * anida la colección y la paginación bajo `data` (`{ ok, data: { items,
 * pagination }, message }`), por lo que este hook desenvuelve ese `data`
 * directamente en lugar de reutilizar `fetchList` (que espera `pagination` en la
 * raíz del envoltorio).
 *
 * Cada entrada registra quién hizo qué sobre qué objeto, cuándo, en qué país y
 * con qué resultado (éxito o error), más el payload de entrada redaccionado. Los
 * filtros se traducen a los nombres de query del handler (`object_id`,
 * `resource`, `actor`, `action`, `from`, `to`) y la paginación a
 * `page`/`records_per_page`. Un listado sin resultados devuelve `items: []` sin
 * error.
 */
import {
  useQuery,
  type UseQueryOptions,
  type UseQueryResult,
} from "@tanstack/react-query";

import { apiClient, type ApiError } from "@/lib/api-client";
import type { Resource } from "@/lib/permissions";
import {
  type ApiEnvelope,
  EMPTY_PAGINATION,
  type ListParams,
  type ListResult,
  type Pagination,
} from "@/types/api";

/** Ruta de la bitácora de auditoría en el BFF. */
const RESOURCE_URL = "/audit-logs";

/** Resultado de una operación auditada. Espejo de `OutcomeSuccess`/`OutcomeError`. */
export type AuditOutcome = "success" | "error";

/**
 * Una entrada de la bitácora tal como la serializa el BFF (`audit.Entry`). Los
 * campos opcionales reflejan columnas que pueden venir vacías (actor anónimo,
 * sin código de error, sin payload). `payload` es el cuerpo de entrada
 * redaccionado, ya parseado a objeto JSON por el BFF.
 */
export interface AuditEntry {
  readonly id: string;
  readonly actorId?: string;
  readonly actorName?: string;
  readonly resource: string;
  readonly action: string;
  readonly objectId?: string;
  readonly country?: string;
  readonly outcome: AuditOutcome;
  readonly statusCode?: number;
  readonly errorCode?: string;
  readonly message?: string;
  readonly payload?: Record<string, unknown> | null;
  readonly createdAt: string;
}

/**
 * Filtros opcionales del listado de bitácora. `objectId` es el filtro principal
 * (el id del objeto operado: customer, plan, etc.); los demás acotan por recurso
 * OTT/panel, actor (nombre o id) y rango de fechas (`YYYY-MM-DD` o RFC3339).
 */
export interface AuditFilters {
  readonly objectId?: string;
  readonly resource?: Resource | "";
  readonly actor?: string;
  readonly action?: string;
  readonly from?: string;
  readonly to?: string;
}

/** Forma de `data` en la respuesta de `GET /audit-logs`. */
interface AuditListData {
  readonly items?: ReadonlyArray<AuditEntry>;
  readonly pagination?: Pagination;
}

/** Fábrica de `queryKey`s de la bitácora. */
export const auditKeys = {
  all: ["audit-logs"] as const,
  lists: () => [...auditKeys.all, "list"] as const,
  list: (params: ListParams, filters: AuditFilters) =>
    [...auditKeys.lists(), params, filters] as const,
};

/**
 * Ejecuta el `GET /audit-logs` y desenvuelve `data.items` / `data.pagination`.
 * Un cuerpo sin `items` (sin resultados) produce `items: []` con metadatos en
 * cero, sin lanzar error.
 */
async function fetchAuditLogs(
  params: ListParams,
  filters: AuditFilters,
): Promise<ListResult<AuditEntry>> {
  const query: Record<string, string | number> = {};
  if (typeof params.page === "number") {
    query.page = params.page;
  }
  if (typeof params.recordsPerPage === "number") {
    query.records_per_page = params.recordsPerPage;
  }
  const filterEntries: ReadonlyArray<readonly [string, string | undefined]> = [
    ["object_id", filters.objectId],
    ["resource", filters.resource ? filters.resource : undefined],
    ["actor", filters.actor],
    ["action", filters.action],
    ["from", filters.from],
    ["to", filters.to],
  ];
  for (const [key, value] of filterEntries) {
    if (value !== undefined && value.trim() !== "") {
      query[key] = value.trim();
    }
  }

  const response = await apiClient.get<ApiEnvelope<AuditListData>>(
    RESOURCE_URL,
    { params: query },
  );
  const data = response.data?.data;
  return {
    items: data?.items ?? [],
    pagination: data?.pagination ?? EMPTY_PAGINATION,
  };
}

/**
 * Lista las entradas de la bitácora con paginación y filtros opcionales
 * (object_id, resource, actor, action, from, to). Un listado sin resultados
 * devuelve `items: []` sin error.
 */
export function useAuditLogs(
  params: ListParams = {},
  filters: AuditFilters = {},
  options?: Omit<
    UseQueryOptions<ListResult<AuditEntry>, ApiError>,
    "queryKey" | "queryFn"
  >,
): UseQueryResult<ListResult<AuditEntry>, ApiError> {
  return useQuery<ListResult<AuditEntry>, ApiError>({
    queryKey: auditKeys.list(params, filters),
    queryFn: () => fetchAuditLogs(params, filters),
    // La bitácora es un historial que crece con cada escritura hecha en
    // cualquier otra vista; esas mutaciones no invalidan esta query. Para que
    // la vista no muestre datos obsoletos tras un cambio reciente, se marca
    // siempre como "stale" (staleTime 0) y se refresca al montar la vista
    // (refetchOnMount "always"), de modo que entrar a Auditoría siempre
    // consulte el estado actual. El refresco manual lo cubre el botón de la
    // tabla (refetch). Estos valores pueden sobreescribirse vía `options`.
    staleTime: 0,
    refetchOnMount: "always",
    ...options,
  });
}
