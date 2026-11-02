"use client";

/**
 * Hook de refresco manual del caché de un recurso cacheable del BFF
 * (customers/plans/subscriptions).
 *
 * Invoca `POST /{resource}/refresh`, que fuerza al BFF a recargar el conjunto
 * completo desde OTT para el país seleccionado (el `apiClient` ya adjunta la
 * cabecera X-Country). Al completarse con éxito, invalida en TanStack Query
 * TODAS las queries de ese recurso (clave raíz `[resource]`), de modo que el
 * listado visible se vuelva a pedir y muestre los datos recién refrescados.
 */
import {
  useMutation,
  useQueryClient,
  type UseMutationResult,
} from "@tanstack/react-query";

import { apiClient, type ApiError } from "@/lib/api-client";

/** Recursos cacheables que soportan refresco manual. */
export type RefreshableResource = "customers" | "plans" | "subscriptions";

/** Datos devueltos por el endpoint de refresco del BFF. */
export interface RefreshResult {
  readonly resource: string;
  readonly country: string;
  readonly count: number;
  readonly refreshedAt: string;
}

/** Forma del envoltorio normalizado del BFF para la respuesta de refresco. */
interface RefreshEnvelope {
  readonly data?: Partial<RefreshResult>;
}

/**
 * Devuelve una mutación que refresca el caché del recurso dado y luego invalida
 * sus listados. No recibe variables: el país lo resuelve el `apiClient` desde la
 * selección actual.
 */
export function useRefreshResource(
  resource: RefreshableResource,
): UseMutationResult<RefreshResult, ApiError, void> {
  const queryClient = useQueryClient();

  return useMutation<RefreshResult, ApiError, void>({
    mutationFn: async () => {
      const response = await apiClient.post<RefreshEnvelope>(
        `/${resource}/refresh`,
      );
      const data = response.data?.data ?? {};
      return {
        resource: typeof data.resource === "string" ? data.resource : resource,
        country: typeof data.country === "string" ? data.country : "",
        count: typeof data.count === "number" ? data.count : 0,
        refreshedAt:
          typeof data.refreshedAt === "string" ? data.refreshedAt : "",
      };
    },
    onSuccess: () => {
      // Invalida TODAS las queries del recurso (listas y detalles) para que la
      // vista actual vuelva a pedir los datos ya refrescados.
      void queryClient.invalidateQueries({ queryKey: [resource] });
    },
  });
}
