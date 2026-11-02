"use client";

/**
 * Hooks de TanStack Query del recurso Subscription (tarea 15.2).
 *
 * Mapean a las rutas del BFF (ver design.md, "API/Endpoints"):
 * - `GET /subscriptions`         → listado/detalle/filtros por `customer_id`,
 *   `plan_id`, `subscription_id`.
 * - `POST /subscriptions`        → crear suscripción (Req. 14).
 * - `PUT /subscriptions/change`  → cambiar plan/método de pago (Req. 15).
 * - `POST /subscriptions/cancel` → cancelar (Req. 16).
 *
 * Las mutaciones invalidan los listados tras un éxito (Req. 13.1-13.3, 14.9,
 * 15.5, 16.5, 20.1). Los errores se propagan como `ApiError` normalizado.
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
import { fetchDetail, fetchList, postResource, putResource } from "@/features/shared";
import type {
  SubscriptionCancelInput,
  SubscriptionChangeInput,
  SubscriptionCreateInput,
} from "@/features/subscriptions/schema";
import type { ListParams, ListResult } from "@/types/api";

/** Rutas del recurso Subscription en el BFF. */
const RESOURCE_URL = "/subscriptions";
const CHANGE_URL = "/subscriptions/change";
const CANCEL_URL = "/subscriptions/cancel";

/**
 * Representación mínima de una Subscription devuelta por el BFF. Laxa; las
 * vistas (tarea 16.3) definen el detalle exacto.
 */
export interface Subscription {
  readonly subscription_id?: string;
  readonly customer_id?: string;
  readonly plan_id?: string;
  readonly status?: string;
  /** Fecha en que se activó la suscripción (RFC3339), si el OTT la provee. */
  readonly subscription_start_date?: string;
  readonly [key: string]: unknown;
}

/** Filtros opcionales de listado de suscripciones (Req. 13.2, 13.3). */
export interface SubscriptionFilters {
  readonly customer_id?: string;
  readonly plan_id?: string;
}

/** Fábrica de `queryKey`s del recurso Subscription. */
export const subscriptionKeys = {
  all: ["subscriptions"] as const,
  lists: () => [...subscriptionKeys.all, "list"] as const,
  list: (params: ListParams, filters: SubscriptionFilters) =>
    [...subscriptionKeys.lists(), params, filters] as const,
  details: () => [...subscriptionKeys.all, "detail"] as const,
  detail: (subscriptionId: string) =>
    [...subscriptionKeys.details(), subscriptionId] as const,
};

/**
 * Lista de suscripciones con paginación/orden/búsqueda y filtros opcionales por
 * `customer_id`/`plan_id` (Req. 13.1-13.3). Lista vacía sin error (Req. 13.5).
 */
export function useSubscriptions(
  params: ListParams = {},
  filters: SubscriptionFilters = {},
  options?: Omit<
    UseQueryOptions<ListResult<Subscription>, ApiError>,
    "queryKey" | "queryFn"
  >,
): UseQueryResult<ListResult<Subscription>, ApiError> {
  return useQuery<ListResult<Subscription>, ApiError>({
    queryKey: subscriptionKeys.list(params, filters),
    queryFn: () =>
      fetchList<Subscription>(RESOURCE_URL, params, {
        customer_id: filters.customer_id,
        plan_id: filters.plan_id,
      }),
    ...options,
  });
}

/** Detalle de una suscripción por `subscription_id` (Req. 13.1). */
export function useSubscription(
  subscriptionId: string | undefined,
  options?: Omit<
    UseQueryOptions<Subscription, ApiError>,
    "queryKey" | "queryFn" | "enabled"
  > & { enabled?: boolean },
): UseQueryResult<Subscription, ApiError> {
  return useQuery<Subscription, ApiError>({
    queryKey: subscriptionKeys.detail(subscriptionId ?? ""),
    queryFn: async () => {
      const detail = await fetchDetail<Subscription>(RESOURCE_URL, {
        subscription_id: subscriptionId,
      });
      // El OTT devuelve el detalle anidado bajo `subscription` junto con otros
      // campos (`datetime`, `timestamp`), por lo que el desanidado genérico no
      // aplica. Si viene así, se extrae el objeto de la suscripción.
      return unwrapSubscriptionDetail(detail);
    },
    enabled: Boolean(subscriptionId) && (options?.enabled ?? true),
    ...options,
  });
}

/**
 * Desanida el detalle de una suscripción cuando el BFF lo devuelve envuelto como
 * `{ datetime, subscription: { ... }, timestamp }`. Si ya viene plano (contiene
 * `subscription_id`/`customer_id` en el nivel superior), se devuelve tal cual.
 */
function unwrapSubscriptionDetail(detail: Subscription): Subscription {
  const nested = (detail as { subscription?: unknown }).subscription;
  if (nested && typeof nested === "object" && !Array.isArray(nested)) {
    return nested as Subscription;
  }
  return detail;
}

/** Crea una suscripción (Req. 14) e invalida los listados. */
export function useCreateSubscription(
  options?: UseMutationOptions<
    Subscription,
    ApiError,
    SubscriptionCreateInput
  >,
): UseMutationResult<Subscription, ApiError, SubscriptionCreateInput> {
  const queryClient = useQueryClient();
  return useMutation<Subscription, ApiError, SubscriptionCreateInput>({
    mutationFn: (input) =>
      postResource<SubscriptionCreateInput, Subscription>(RESOURCE_URL, input),
    ...options,
    onSuccess: (...args) => {
      void queryClient.invalidateQueries({
        queryKey: subscriptionKeys.lists(),
      });
      return options?.onSuccess?.(...args);
    },
  });
}

/**
 * Cambia una suscripción (plan/método de pago, Req. 15) vía
 * `PUT /subscriptions/change`; invalida los listados.
 */
export function useChangeSubscription(
  options?: UseMutationOptions<
    Subscription,
    ApiError,
    SubscriptionChangeInput
  >,
): UseMutationResult<Subscription, ApiError, SubscriptionChangeInput> {
  const queryClient = useQueryClient();
  return useMutation<Subscription, ApiError, SubscriptionChangeInput>({
    mutationFn: (input) =>
      putResource<SubscriptionChangeInput, Subscription>(CHANGE_URL, input),
    ...options,
    onSuccess: (...args) => {
      void queryClient.invalidateQueries({
        queryKey: subscriptionKeys.lists(),
      });
      return options?.onSuccess?.(...args);
    },
  });
}

/**
 * Cancela una suscripción (Req. 16) vía `POST /subscriptions/cancel`
 * (`customer_id`, `cancel_reason` opcional); invalida los listados.
 */
export function useCancelSubscription(
  options?: UseMutationOptions<
    Subscription,
    ApiError,
    SubscriptionCancelInput
  >,
): UseMutationResult<Subscription, ApiError, SubscriptionCancelInput> {
  const queryClient = useQueryClient();
  return useMutation<Subscription, ApiError, SubscriptionCancelInput>({
    mutationFn: (input) =>
      postResource<SubscriptionCancelInput, Subscription>(CANCEL_URL, input),
    ...options,
    onSuccess: (...args) => {
      void queryClient.invalidateQueries({
        queryKey: subscriptionKeys.lists(),
      });
      return options?.onSuccess?.(...args);
    },
  });
}
