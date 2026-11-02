"use client";

/**
 * Hooks de TanStack Query del recurso Card (tarea 15.2).
 *
 * Mapean a las rutas del BFF (ver design.md, "API/Endpoints"):
 * - `GET /cards?customer_id=|email=` → tarjetas de un cliente (Req. 17). Exige
 *   seleccionar cliente antes de consultar (Req. 17.1); la query se habilita
 *   solo cuando hay `customer_id` o `email`.
 * - `POST /cards`   → crear tarjeta (Req. 18).
 * - `DELETE /cards` → eliminar por `card_id` (Req. 19).
 *
 * Las mutaciones invalidan los listados de tarjetas del cliente afectado tras
 * un éxito (Req. 17.1, 18.9, 19.4, 20.1). Los errores se propagan como
 * `ApiError` normalizado.
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
import { deleteResource, fetchList, postResource } from "@/features/shared";
import type { CardCreateInput } from "@/features/cards/schema";
import type { ListParams, ListResult } from "@/types/api";

/** Ruta del recurso Card en el BFF. */
const RESOURCE_URL = "/cards";

/**
 * Representación mínima de una Card devuelta por el BFF. Laxa; la vista
 * (tarea 16.4) define el detalle exacto.
 */
export interface Card {
  readonly card_id?: string;
  readonly customer_id?: string;
  readonly card_franchise?: string;
  readonly [key: string]: unknown;
}

/**
 * Selector del cliente del que se consultan las tarjetas (Req. 17.1). Debe
 * proveerse `customer_id` o `email`.
 */
export interface CardCustomerSelector {
  readonly customer_id?: string;
  readonly email?: string;
}

/** Fábrica de `queryKey`s del recurso Card. */
export const cardKeys = {
  all: ["cards"] as const,
  lists: () => [...cardKeys.all, "list"] as const,
  list: (selector: CardCustomerSelector, params: ListParams) =>
    [...cardKeys.lists(), selector, params] as const,
};

/** Indica si el selector contiene un identificador de cliente utilizable. */
function hasSelector(selector: CardCustomerSelector): boolean {
  return Boolean(selector.customer_id || selector.email);
}

/**
 * Lista las tarjetas de un cliente (Req. 17). La query solo se ejecuta cuando el
 * selector tiene `customer_id` o `email` (Req. 17.1); una lista vacía (sin
 * tarjetas) no es un error (Req. 17.3).
 */
export function useCards(
  selector: CardCustomerSelector,
  params: ListParams = {},
  options?: Omit<
    UseQueryOptions<ListResult<Card>, ApiError>,
    "queryKey" | "queryFn" | "enabled"
  > & { enabled?: boolean },
): UseQueryResult<ListResult<Card>, ApiError> {
  return useQuery<ListResult<Card>, ApiError>({
    queryKey: cardKeys.list(selector, params),
    queryFn: () =>
      fetchList<Card>(RESOURCE_URL, params, {
        customer_id: selector.customer_id,
        email: selector.email,
      }),
    enabled: hasSelector(selector) && (options?.enabled ?? true),
    ...options,
  });
}

/**
 * Crea una tarjeta (Req. 18) e invalida los listados de tarjetas. Se invalidan
 * todos los listados de cards para refrescar el del cliente afectado.
 */
export function useCreateCard(
  options?: UseMutationOptions<Card, ApiError, CardCreateInput>,
): UseMutationResult<Card, ApiError, CardCreateInput> {
  const queryClient = useQueryClient();
  return useMutation<Card, ApiError, CardCreateInput>({
    mutationFn: (input) =>
      postResource<CardCreateInput, Card>(RESOURCE_URL, input),
    ...options,
    onSuccess: (...args) => {
      void queryClient.invalidateQueries({ queryKey: cardKeys.lists() });
      return options?.onSuccess?.(...args);
    },
  });
}

/** Elimina una tarjeta por `card_id` (Req. 19); invalida los listados. */
export function useDeleteCard(
  options?: UseMutationOptions<unknown, ApiError, string>,
): UseMutationResult<unknown, ApiError, string> {
  const queryClient = useQueryClient();
  return useMutation<unknown, ApiError, string>({
    mutationFn: (cardId) => deleteResource(RESOURCE_URL, { card_id: cardId }),
    ...options,
    onSuccess: (...args) => {
      void queryClient.invalidateQueries({ queryKey: cardKeys.lists() });
      return options?.onSuccess?.(...args);
    },
  });
}
