"use client";

/**
 * Hooks de TanStack Query del recurso Plan (tarea 15.2).
 *
 * Mapean a las rutas del BFF (ver design.md, "API/Endpoints"):
 * - `GET /plans`    → listado paginado o detalle por `plan_id`.
 * - `POST /plans`   → crear plan (Req. 10); el BFF valida `country` vs token.
 * - `PUT /plans`    → editar por `plan_id` (Req. 11).
 * - `DELETE /plans` → eliminar por `plan_id` (Req. 12).
 *
 * Las mutaciones invalidan los listados tras un éxito (Req. 9.1, 9.2, 10.14,
 * 11.4, 12.4, 20.1). Los errores se propagan como `ApiError` normalizado.
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

import type { ApiError } from "@/lib/api-client";
import {
  deleteResource,
  fetchList,
  postResource,
  putResource,
} from "@/features/shared";
import type { PlanCreateInput, PlanEditInput } from "@/features/plans/schema";
import type { ListParams, ListResult } from "@/types/api";

/** Ruta del recurso Plan en el BFF. */
const RESOURCE_URL = "/plans";

/**
 * Representación mínima de un Plan tal como lo devuelve el BFF. Se mantiene laxa
 * (índice de propiedades); las vistas (tarea 16.2) definen el detalle exacto.
 */
export interface Plan {
  /** Identificador del plan. El contrato OTT lo devuelve como `id`; algunas
   *  respuestas/entradas usan `plan_id`. Usa `planId(plan)` para resolverlo. */
  readonly id?: string;
  readonly plan_id?: string;
  readonly name?: string;
  readonly external_id?: string;
  readonly country?: string;
  readonly currency?: string;
  readonly amount?: number;
  readonly [key: string]: unknown;
}

/**
 * Resuelve el identificador de un Plan con independencia de si el backend lo
 * entrega como `id` (contrato OTT) o `plan_id`. Devuelve "" si no hay ninguno.
 */
export function planId(plan: Pick<Plan, "id" | "plan_id">): string {
  if (typeof plan.id === "string" && plan.id !== "") {
    return plan.id;
  }
  if (typeof plan.plan_id === "string" && plan.plan_id !== "") {
    return plan.plan_id;
  }
  return "";
}

/** Fábrica de `queryKey`s del recurso Plan. */
export const planKeys = {
  all: ["plans"] as const,
  lists: () => [...planKeys.all, "list"] as const,
  list: (params: ListParams) => [...planKeys.lists(), params] as const,
  details: () => [...planKeys.all, "detail"] as const,
  detail: (planId: string) => [...planKeys.details(), planId] as const,
};

/**
 * Lista de planes con paginación/orden/búsqueda (Req. 9.1, 9.2). Un listado sin
 * resultados devuelve `items: []` sin error (Req. 9.5).
 */
/** Filtros opcionales del listado de planes (resueltos por el BFF). */
export interface PlanFilters {
  /** Coincidencia exacta por Plan ID (id o plan_id). */
  readonly plan_id?: string;
  /** Coincidencia exacta por External ID. */
  readonly external_id?: string;
  /** Subcadena por nombre. */
  readonly name?: string;
}

export function usePlans(
  params: ListParams = {},
  filters: PlanFilters = {},
  options?: Omit<
    UseQueryOptions<ListResult<Plan>, ApiError>,
    "queryKey" | "queryFn"
  >,
): UseQueryResult<ListResult<Plan>, ApiError> {
  return useQuery<ListResult<Plan>, ApiError>({
    queryKey: [...planKeys.list(params), filters] as const,
    queryFn: () =>
      fetchList<Plan>(RESOURCE_URL, params, {
        plan_id: filters.plan_id,
        external_id: filters.external_id,
        name: filters.name,
      }),
    ...options,
  });
}

/**
 * Detalle de un plan localizado por `plan_id`. El BFF resuelve `plan_id` sobre
 * el caché y responde un LISTADO; se toma el primer elemento como detalle. Si no
 * hay coincidencias, se devuelve un objeto vacío (las vistas caen al id).
 */
export function usePlan(
  planId: string | undefined,
  options?: Omit<
    UseQueryOptions<Plan, ApiError>,
    "queryKey" | "queryFn" | "enabled"
  > & { enabled?: boolean },
): UseQueryResult<Plan, ApiError> {
  return useQuery<Plan, ApiError>({
    queryKey: planKeys.detail(planId ?? ""),
    queryFn: async () => {
      const result = await fetchList<Plan>(
        RESOURCE_URL,
        { page: 1, recordsPerPage: 1 },
        { plan_id: planId },
      );
      return result.items[0] ?? ({} as Plan);
    },
    enabled: Boolean(planId) && (options?.enabled ?? true),
    ...options,
  });
}

/** Crea un plan (Req. 10) e invalida los listados tras el éxito. */
export function useCreatePlan(
  options?: UseMutationOptions<Plan, ApiError, PlanCreateInput>,
): UseMutationResult<Plan, ApiError, PlanCreateInput> {
  const queryClient = useQueryClient();
  return useMutation<Plan, ApiError, PlanCreateInput>({
    mutationFn: (input) =>
      postResource<PlanCreateInput, Plan>(RESOURCE_URL, input),
    ...options,
    onSuccess: (...args) => {
      void queryClient.invalidateQueries({ queryKey: planKeys.lists() });
      return options?.onSuccess?.(...args);
    },
  });
}

/** Variables de edición de Plan: `plan_id` (query) + datos editables (Req. 11). */
export interface UpdatePlanVariables {
  readonly planId: string;
  readonly data: PlanEditInput;
}

/** Edita un plan por `plan_id` (Req. 11); invalida listados y su detalle. */
export function useUpdatePlan(
  options?: UseMutationOptions<Plan, ApiError, UpdatePlanVariables>,
): UseMutationResult<Plan, ApiError, UpdatePlanVariables> {
  const queryClient = useQueryClient();
  return useMutation<Plan, ApiError, UpdatePlanVariables>({
    mutationFn: ({ planId, data }) =>
      putResource<PlanEditInput, Plan>(RESOURCE_URL, data, {
        params: { plan_id: planId },
      }),
    ...options,
    onSuccess: (...args) => {
      const [, variables] = args;
      void queryClient.invalidateQueries({ queryKey: planKeys.lists() });
      void queryClient.invalidateQueries({
        queryKey: planKeys.detail(variables.planId),
      });
      return options?.onSuccess?.(...args);
    },
  });
}

/** Elimina un plan por `plan_id` (Req. 12); invalida los listados. */
export function useDeletePlan(
  options?: UseMutationOptions<unknown, ApiError, string>,
): UseMutationResult<unknown, ApiError, string> {
  const queryClient = useQueryClient();
  return useMutation<unknown, ApiError, string>({
    mutationFn: (planId) =>
      deleteResource(RESOURCE_URL, { plan_id: planId }),
    ...options,
    onSuccess: (...args) => {
      void queryClient.invalidateQueries({ queryKey: planKeys.lists() });
      return options?.onSuccess?.(...args);
    },
  });
}
