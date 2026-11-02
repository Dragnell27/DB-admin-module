/**
 * Utilidades compartidas por los hooks de TanStack Query de cada recurso
 * (tarea 15.2). Centraliza:
 *
 * - La traducción de `ListParams` (convención del front) a los parámetros de
 *   query del BFF (`page`, `records_per_page`, `order_by`, `order`, `search`).
 * - El desenvolvimiento del envoltorio uniforme del BFF
 *   (`{ ok, data, pagination?, ... }`) hacia un `ListResult` o un detalle.
 * - Fábricas de `queryKey` consistentes que incluyen los parámetros, de modo
 *   que cambios de página/orden/búsqueda produzcan cachés independientes.
 *
 * Todas las llamadas pasan por `apiClient`, por lo que heredan el interceptor de
 * sesión (Req. 2.3) y la normalización de errores a `ApiError` (Req. 20.1). Los
 * errores se propagan tal cual a `onError`/`error` de los hooks.
 */
import type { AxiosRequestConfig } from "axios";

import { apiClient } from "@/lib/api-client";
import {
  type ApiEnvelope,
  EMPTY_PAGINATION,
  type ListParams,
  type ListResult,
  type Pagination,
} from "@/types/api";

/**
 * Construye el objeto de parámetros de query del BFF a partir de `ListParams`.
 * Omite las claves no definidas o vacías para no enviar filtros espurios.
 */
export function buildListQueryParams(
  params: ListParams = {},
): Record<string, string | number> {
  const query: Record<string, string | number> = {};
  if (typeof params.page === "number") {
    query.page = params.page;
  }
  if (typeof params.recordsPerPage === "number") {
    query.records_per_page = params.recordsPerPage;
  }
  if (params.orderBy) {
    query.order_by = params.orderBy;
  }
  if (params.order) {
    query.order = params.order;
  }
  if (params.search && params.search.trim() !== "") {
    query.search = params.search.trim();
  }
  return query;
}

/** Normaliza los metadatos de paginación del envoltorio, con respaldo seguro. */
function resolvePagination(pagination?: Pagination): Pagination {
  if (!pagination) {
    return EMPTY_PAGINATION;
  }
  return pagination;
}

/**
 * Claves que corresponden a metadatos de paginación dentro del objeto `data`;
 * se ignoran al buscar la colección de elementos del listado.
 */
const PAGINATION_KEYS = new Set<string>([
  "page",
  "recordsPerPage",
  "records_per_page",
  "totalPages",
  "total_pages",
  "totalRecords",
  "total_records",
]);

/**
 * Extrae la colección de elementos del envoltorio de listado del BFF.
 *
 * El contrato OTT devuelve los listados de dos formas posibles bajo `data`:
 *   - como un arreglo directo (`data: [ ... ]`), o
 *   - como un objeto con la colección anidada bajo una clave con el nombre del
 *     recurso (`data: { page, plans: [ ... ], totalRecords, ... }`), junto con
 *     los metadatos de paginación.
 *
 * Esta función soporta ambas: si `data` es un arreglo lo devuelve; si es un
 * objeto, toma el primer valor que sea un arreglo ignorando las claves de
 * paginación. Un listado "sin resultados" devuelve `[]`.
 */
function resolveItems<TItem>(data: unknown): ReadonlyArray<TItem> {
  if (Array.isArray(data)) {
    return data as ReadonlyArray<TItem>;
  }
  if (data && typeof data === "object") {
    for (const [key, value] of Object.entries(data as Record<string, unknown>)) {
      if (PAGINATION_KEYS.has(key)) {
        continue;
      }
      if (Array.isArray(value)) {
        return value as ReadonlyArray<TItem>;
      }
    }
  }
  // Un listado "sin resultados" normalizado puede llegar como null/undefined.
  return [];
}

/**
 * Ejecuta un `GET` de listado y desenvuelve el envoltorio del BFF a un
 * `ListResult`. Un listado vacío (404 normalizado por el BFF) produce
 * `items: []` con metadatos en cero, sin lanzar error (Req. 6.5, 9.5, 13.5,
 * 17.3).
 */
export async function fetchList<TItem>(
  url: string,
  params: ListParams = {},
  extraQuery: Record<string, string | number | undefined> = {},
): Promise<ListResult<TItem>> {
  const query = { ...buildListQueryParams(params) };
  for (const [key, value] of Object.entries(extraQuery)) {
    if (value !== undefined && value !== "") {
      query[key] = value;
    }
  }
  const response = await apiClient.get<ApiEnvelope<TItem[]>>(url, {
    params: query,
  });
  const envelope = response.data;
  return {
    items: resolveItems<TItem>(envelope?.data),
    pagination: resolvePagination(envelope?.pagination),
  };
}

/**
 * Desenvuelve el objeto de detalle del envoltorio `data` del BFF.
 *
 * El contrato OTT devuelve el detalle de dos formas bajo `data`:
 *   - plano (`data: { first_name, ... }`), o
 *   - anidado bajo una única clave con el nombre del recurso
 *     (`data: { customer: { first_name, ... } }`, `{ plan: {...} }`, etc.).
 *
 * Si `data` es un objeto con EXACTAMENTE una propiedad cuyo valor es a su vez un
 * objeto (no arreglo), se asume el patrón anidado y se devuelve ese objeto
 * interno; en cualquier otro caso se devuelve `data` tal cual.
 */
function unwrapDetail<TItem>(data: unknown): TItem {
  if (data && typeof data === "object" && !Array.isArray(data)) {
    const entries = Object.entries(data as Record<string, unknown>);
    if (entries.length === 1) {
      const [, value] = entries[0];
      if (value && typeof value === "object" && !Array.isArray(value)) {
        return value as TItem;
      }
    }
  }
  return data as TItem;
}

/**
 * Ejecuta un `GET` de detalle y devuelve el objeto del recurso del envoltorio
 * del BFF, desenvolviendo el posible anidamiento bajo la clave del recurso
 * (p. ej. `data.customer`).
 */
export async function fetchDetail<TItem>(
  url: string,
  query: Record<string, string | number | undefined> = {},
): Promise<TItem> {
  const params: Record<string, string | number> = {};
  for (const [key, value] of Object.entries(query)) {
    if (value !== undefined && value !== "") {
      params[key] = value;
    }
  }
  const response = await apiClient.get<ApiEnvelope<TItem>>(url, { params });
  return unwrapDetail<TItem>(response.data.data);
}

/** Ejecuta un `POST` de creación y devuelve `data` del envoltorio del BFF. */
export async function postResource<TBody, TResult = unknown>(
  url: string,
  body: TBody,
  config?: AxiosRequestConfig,
): Promise<TResult> {
  const response = await apiClient.post<ApiEnvelope<TResult>>(
    url,
    body,
    config,
  );
  return response.data.data;
}

/** Ejecuta un `PUT` de edición y devuelve `data` del envoltorio del BFF. */
export async function putResource<TBody, TResult = unknown>(
  url: string,
  body: TBody,
  config?: AxiosRequestConfig,
): Promise<TResult> {
  const response = await apiClient.put<ApiEnvelope<TResult>>(url, body, config);
  return response.data.data;
}

/**
 * Ejecuta un `DELETE`. Los identificadores viajan por query o body según el
 * contrato OTT (ver design.md); por simplicidad enviamos por query, que es lo
 * que el BFF acepta para `plans`/`cards`.
 */
export async function deleteResource<TResult = unknown>(
  url: string,
  query: Record<string, string | number | undefined> = {},
): Promise<TResult> {
  const params: Record<string, string | number> = {};
  for (const [key, value] of Object.entries(query)) {
    if (value !== undefined && value !== "") {
      params[key] = value;
    }
  }
  const response = await apiClient.delete<ApiEnvelope<TResult>>(url, {
    params,
  });
  return response.data.data;
}
