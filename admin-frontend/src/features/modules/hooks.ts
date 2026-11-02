"use client";

/**
 * Hooks genéricos de TanStack Query del constructor de módulos (module-builder).
 *
 * A diferencia de los recursos fijos (customers, plans...), estos hooks están
 * parametrizados por el `slug` del módulo dinámico y sirven para CUALQUIER
 * módulo creado por el usuario:
 *
 * - `useModuleList(slug, params)` → `GET /modules/:slug/list`, que devuelve
 *   `{ ok, data: { items, pagination }, message }` (misma forma anidada que la
 *   bitácora de auditoría), por lo que se desenvuelve `data.items` /
 *   `data.pagination` directamente.
 * - `useModuleActionList(slug, actionKey, input, params)` →
 *   `POST /modules/:slug/actions/:actionId/list`, la ruta dedicada de LECTURA de
 *   una acción SELECT: lista las filas con la paginación en la query y los
 *   valores de filtro (campos LLENOS de la acción) en el cuerpo JSON. Misma forma
 *   de respuesta que `useModuleList`.
 * - `useModuleAction(slug, actionKey)` → `POST /modules/:slug/actions/:actionKey`,
 *   una mutación que invalida el listado del módulo tras una escritura.
 *
 * Todas las llamadas pasan por `apiClient` (hereda `Authorization` y
 * `X-Country`), consistente con que el backend es el único ejecutor (Req. 13.1).
 */
import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationOptions,
  type UseMutationResult,
  type UseQueryOptions,
  type UseQueryResult,
} from "@tanstack/react-query";

import { apiClient, type ApiError } from "@/lib/api-client";
import { setSessionToken } from "@/lib/session";
import {
  type ApiEnvelope,
  EMPTY_PAGINATION,
  type ListParams,
  type ListResult,
  type Pagination,
} from "@/types/api";
import type {
  ModuleDefinition,
  ModuleDefinitionInput,
  ModuleSummary,
} from "@/features/modules/types";

/** Una fila de un módulo dinámico: forma libre (depende del Display_Mapping). */
export type ModuleRow = Record<string, unknown>;

/** Forma de `data` en la respuesta de `GET /modules/:slug/list`. */
interface ModuleListData {
  readonly items?: ReadonlyArray<ModuleRow>;
  readonly pagination?: Pagination;
}

/** Fábrica de `queryKey`s de un módulo, namespaced por slug. */
export const moduleKeys = {
  all: (slug: string) => ["module", slug] as const,
  list: (slug: string, params: ListParams) =>
    ["module", slug, "list", params] as const,
  actionList: (
    slug: string,
    actionKey: string,
    input: Record<string, unknown>,
    params: ListParams,
  ) => ["module", slug, "action-list", actionKey, input, params] as const,
  options: (slug: string, field: string, actionKey: string) =>
    ["module", slug, "options", field, actionKey] as const,
};

/**
 * Una opción de una lista DINÁMICA tal como la devuelve el endpoint de opciones:
 * `id` (el valor que se almacena, la referencia FK) y `label` (lo que se
 * muestra). Llegan como `string | number` (un entero llega como número; el texto
 * ya viene normalizado a string); el front los trata SIEMPRE como string vía
 * `String(opt.id)` para el `value` del `<select>`.
 */
export interface FieldOption {
  readonly id: string | number;
  readonly label: string | number;
}

/** Forma de `data` en la respuesta de `GET /modules/:slug/options/:field`. */
interface ModuleOptionsData {
  readonly items?: ReadonlyArray<FieldOption>;
}

/**
 * Carga las opciones [{id,label}] de un campo dinámico
 * (`GET /modules/:slug/options/:field?action=<actionKey>`) y desenvuelve
 * `data.items`. SIEMPRE envía `?action=<actionKey>` (desambiguador del backend:
 * el mismo nombre de campo puede tener fuentes dinámicas distintas en acciones
 * distintas) y lo incluye en el `queryKey`, de modo que cada acción tenga su
 * propia entrada de caché. Se deshabilita si falta slug/field/actionKey o si
 * `enabled === false` (p. ej. el formulario aún no abrió). Mismo patrón que
 * `useModuleList`: `apiClient` hereda `Authorization` y `X-Country`.
 */
export function useModuleFieldOptions(
  slug: string,
  field: string,
  actionKey: string,
  options?: { enabled?: boolean },
): UseQueryResult<ReadonlyArray<FieldOption>, ApiError> {
  return useQuery<ReadonlyArray<FieldOption>, ApiError>({
    queryKey: moduleKeys.options(slug, field, actionKey),
    queryFn: async () => {
      const res = await apiClient.get<ApiEnvelope<ModuleOptionsData>>(
        `/modules/${encodeURIComponent(slug)}/options/${encodeURIComponent(field)}`,
        { params: { action: actionKey } },
      );
      return res.data?.data?.items ?? [];
    },
    enabled:
      Boolean(slug && field && actionKey) && (options?.enabled ?? true),
  });
}

/**
 * Traduce los `ListParams` del front a la query string del BFF
 * (`page`/`records_per_page`/`order_by`/`order`). Compartido por el listado base
 * y la ruta dedicada de acción SELECT, que transportan la paginación igual.
 */
function listParamsToQuery(params: ListParams): Record<string, string | number> {
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
  return query;
}

/**
 * Ejecuta `GET /modules/:slug/list` y desenvuelve `data.items`/`data.pagination`.
 * Traduce la paginación a `page`/`records_per_page`. Un cuerpo sin `items`
 * produce `items: []` con metadatos en cero, sin lanzar.
 */
async function fetchModuleList(
  slug: string,
  params: ListParams,
): Promise<ListResult<ModuleRow>> {
  const response = await apiClient.get<ApiEnvelope<ModuleListData>>(
    `/modules/${encodeURIComponent(slug)}/list`,
    { params: listParamsToQuery(params) },
  );
  const data = response.data?.data;
  return {
    items: data?.items ?? [],
    pagination: data?.pagination ?? EMPTY_PAGINATION,
  };
}

/**
 * Lista las filas de un módulo dinámico con paginación/orden. La query se
 * deshabilita si no hay slug.
 */
export function useModuleList(
  slug: string,
  params: ListParams = {},
  options?: Omit<
    UseQueryOptions<ListResult<ModuleRow>, ApiError>,
    "queryKey" | "queryFn" | "enabled"
  > & { enabled?: boolean },
): UseQueryResult<ListResult<ModuleRow>, ApiError> {
  return useQuery<ListResult<ModuleRow>, ApiError>({
    queryKey: moduleKeys.list(slug, params),
    queryFn: () => fetchModuleList(slug, params),
    enabled: Boolean(slug) && (options?.enabled ?? true),
    ...options,
  });
}

/**
 * Ejecuta la ruta dedicada de LECTURA de una acción SELECT
 * (`POST /modules/:slug/actions/:actionId/list`): la paginación/orden viaja por
 * la query string y los valores de filtro (campos llenos de la acción) por el
 * cuerpo JSON. Desenvuelve `data.items`/`data.pagination` igual que el listado
 * base; un cuerpo sin `items` produce `items: []`.
 */
async function fetchModuleActionList(
  slug: string,
  actionKey: string,
  input: Record<string, unknown>,
  params: ListParams,
): Promise<ListResult<ModuleRow>> {
  const response = await apiClient.post<ApiEnvelope<ModuleListData>>(
    `/modules/${encodeURIComponent(slug)}/actions/${encodeURIComponent(actionKey)}/list`,
    input,
    { params: listParamsToQuery(params) },
  );
  const data = response.data?.data;
  return {
    items: data?.items ?? [],
    pagination: data?.pagination ?? EMPTY_PAGINATION,
  };
}

/**
 * Lista las filas de un módulo aplicando una acción SELECT como filtro de
 * listado (ruta dedicada de lectura). Se deshabilita si falta slug/actionKey o
 * si `options.enabled === false` (p. ej. no hay acción activa). Mismo contrato
 * de respuesta que `useModuleList`.
 */
export function useModuleActionList(
  slug: string,
  actionKey: string,
  input: Record<string, unknown> = {},
  params: ListParams = {},
  options?: Omit<
    UseQueryOptions<ListResult<ModuleRow>, ApiError>,
    "queryKey" | "queryFn" | "enabled"
  > & { enabled?: boolean },
): UseQueryResult<ListResult<ModuleRow>, ApiError> {
  return useQuery<ListResult<ModuleRow>, ApiError>({
    queryKey: moduleKeys.actionList(slug, actionKey, input, params),
    queryFn: () => fetchModuleActionList(slug, actionKey, input, params),
    enabled: Boolean(slug && actionKey) && (options?.enabled ?? true),
    ...options,
  });
}

/** Variables de una ejecución de acción: el input del formulario autogenerado. */
export type ModuleActionInput = Record<string, unknown>;

/**
 * Ejecuta una acción de un módulo (`POST /modules/:slug/actions/:actionKey`) e
 * invalida el listado del módulo tras el éxito, para que la tabla se refresque.
 * Devuelve el `data` desenvuelto del envoltorio del BFF.
 */
export function useModuleAction(
  slug: string,
  actionKey: string,
  options?: UseMutationOptions<unknown, ApiError, ModuleActionInput>,
): UseMutationResult<unknown, ApiError, ModuleActionInput> {
  const queryClient = useQueryClient();
  return useMutation<unknown, ApiError, ModuleActionInput>({
    mutationFn: async (input) => {
      const response = await apiClient.post<ApiEnvelope<unknown>>(
        `/modules/${encodeURIComponent(slug)}/actions/${encodeURIComponent(actionKey)}`,
        input,
      );
      return response.data.data;
    },
    ...options,
    onSuccess: (...args) => {
      void queryClient.invalidateQueries({ queryKey: moduleKeys.all(slug) });
      return options?.onSuccess?.(...args);
    },
  });
}

// --- CRUD de definiciones y descubrimiento ---------------------------------
// Estos hooks alimentan el asistente de creación/edición (tarea 13) y el
// sidebar dinámico (tarea 14.4). Los tipos viven en ./types.

/** Fábrica de `queryKey`s de las definiciones de módulo. */
export const moduleDefKeys = {
  all: ["module-defs"] as const,
  lists: () => ["module-defs", "list"] as const,
  detail: (slug: string) => ["module-defs", "detail", slug] as const,
  visible: ["modules", "visible"] as const,
};

/** Lista todas las definiciones de módulo (GET /module-defs). */
export function useModuleDefs(
  options?: Omit<
    UseQueryOptions<ReadonlyArray<ModuleDefinition>, ApiError>,
    "queryKey" | "queryFn"
  >,
): UseQueryResult<ReadonlyArray<ModuleDefinition>, ApiError> {
  return useQuery<ReadonlyArray<ModuleDefinition>, ApiError>({
    queryKey: moduleDefKeys.lists(),
    queryFn: async () => {
      const res = await apiClient.get<ApiEnvelope<{ items?: ModuleDefinition[] }>>(
        "/module-defs",
      );
      return res.data?.data?.items ?? [];
    },
    ...options,
  });
}

/** Obtiene una definición por slug (GET /module-defs/:slug). */
export function useModuleDef(
  slug: string,
  options?: Omit<
    UseQueryOptions<ModuleDefinition, ApiError>,
    "queryKey" | "queryFn" | "enabled"
  > & { enabled?: boolean },
): UseQueryResult<ModuleDefinition, ApiError> {
  return useQuery<ModuleDefinition, ApiError>({
    queryKey: moduleDefKeys.detail(slug),
    queryFn: async () => {
      const res = await apiClient.get<ApiEnvelope<ModuleDefinition>>(
        `/module-defs/${encodeURIComponent(slug)}`,
      );
      return res.data.data;
    },
    enabled: Boolean(slug) && (options?.enabled ?? true),
    ...options,
  });
}

/** Crea una definición de módulo (POST /module-defs). Invalida los listados. */
export function useCreateModuleDef(
  options?: UseMutationOptions<{ slug: string }, ApiError, ModuleDefinitionInput>,
): UseMutationResult<{ slug: string }, ApiError, ModuleDefinitionInput> {
  const queryClient = useQueryClient();
  return useMutation<{ slug: string }, ApiError, ModuleDefinitionInput>({
    mutationFn: async (input) => {
      const res = await apiClient.post<ApiEnvelope<{ slug: string }>>(
        "/module-defs",
        input,
      );
      return res.data.data;
    },
    ...options,
    onSuccess: (...args) => {
      void queryClient.invalidateQueries({ queryKey: moduleDefKeys.all });
      void queryClient.invalidateQueries({ queryKey: moduleDefKeys.visible });
      return options?.onSuccess?.(...args);
    },
  });
}

/** Edita una definición (PUT /module-defs/:slug). Invalida listado y detalle. */
export function useUpdateModuleDef(
  slug: string,
  options?: UseMutationOptions<{ slug: string }, ApiError, ModuleDefinitionInput>,
): UseMutationResult<{ slug: string }, ApiError, ModuleDefinitionInput> {
  const queryClient = useQueryClient();
  return useMutation<{ slug: string }, ApiError, ModuleDefinitionInput>({
    mutationFn: async (input) => {
      const res = await apiClient.put<ApiEnvelope<{ slug: string }>>(
        `/module-defs/${encodeURIComponent(slug)}`,
        input,
      );
      return res.data.data;
    },
    ...options,
    onSuccess: (...args) => {
      void queryClient.invalidateQueries({ queryKey: moduleDefKeys.all });
      void queryClient.invalidateQueries({ queryKey: moduleDefKeys.detail(slug) });
      void queryClient.invalidateQueries({ queryKey: moduleDefKeys.visible });
      return options?.onSuccess?.(...args);
    },
  });
}

/** Elimina una definición (DELETE /module-defs/:slug). Invalida listados. */
export function useDeleteModuleDef(
  options?: UseMutationOptions<{ slug: string }, ApiError, string>,
): UseMutationResult<{ slug: string }, ApiError, string> {
  const queryClient = useQueryClient();
  return useMutation<{ slug: string }, ApiError, string>({
    mutationFn: async (slug) => {
      const res = await apiClient.delete<ApiEnvelope<{ slug: string }>>(
        `/module-defs/${encodeURIComponent(slug)}`,
      );
      return res.data.data;
    },
    ...options,
    onSuccess: (...args) => {
      void queryClient.invalidateQueries({ queryKey: moduleDefKeys.all });
      void queryClient.invalidateQueries({ queryKey: moduleDefKeys.visible });
      return options?.onSuccess?.(...args);
    },
  });
}

/**
 * Lista los módulos VISIBLES para el usuario (GET /modules), ya filtrados por
 * permiso `ver` en el backend. Alimenta el sidebar dinámico (tarea 14.4).
 */
export function useVisibleModules(
  options?: Omit<
    UseQueryOptions<ReadonlyArray<ModuleSummary>, ApiError>,
    "queryKey" | "queryFn"
  >,
): UseQueryResult<ReadonlyArray<ModuleSummary>, ApiError> {
  return useQuery<ReadonlyArray<ModuleSummary>, ApiError>({
    queryKey: moduleDefKeys.visible,
    queryFn: async () => {
      const res = await apiClient.get<ApiEnvelope<{ items?: ModuleSummary[] }>>(
        "/modules",
      );
      return res.data?.data?.items ?? [];
    },
    ...options,
  });
}

/**
 * Reemite el JWT de sesión con los permisos ACTUALES del usuario
 * (`POST /auth/refresh-permissions`) y lo persiste con `setSessionToken`, lo que
 * dispara `notifySessionChange` y refresca el estado in-memory de permisos sin
 * recargar la página. No invalida las queries de datos (no recarga la caché de
 * filas); un 401 lo maneja el `apiClient` (logout). Requisito explícito del
 * usuario: botón "Recargar permisos" por módulo, distinto de la acción de
 * permiso "Recargar" (que recarga la caché de datos).
 */
export function useRefreshPermissions(
  options?: UseMutationOptions<string, ApiError, void>,
): UseMutationResult<string, ApiError, void> {
  return useMutation<string, ApiError, void>({
    mutationFn: async () => {
      const res = await apiClient.post<ApiEnvelope<{ token: string }>>(
        "/auth/refresh-permissions",
      );
      const token = res.data.data.token;
      setSessionToken(token);
      return token;
    },
    ...options,
  });
}
