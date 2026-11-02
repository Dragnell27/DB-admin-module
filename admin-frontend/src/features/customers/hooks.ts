"use client";

/**
 * Hooks de TanStack Query del recurso Customer (tarea 15.2).
 *
 * Mapean a las rutas del BFF (ver design.md, "API/Endpoints"):
 * - `GET /customers`  → listado paginado o detalle por `email`/`external_id`.
 * - `POST /customers` → crear cliente (Req. 7).
 * - `PUT /customers`  → editar cliente, localizado por `email`/`external_id`
 *   (Req. 8); `email`/`external_id` son inmutables.
 *
 * Las queries aceptan paginación/orden/búsqueda y usan `queryKey`s que incluyen
 * los parámetros para cachés independientes. Las mutaciones invalidan las
 * queries de lista tras un éxito para refrescar la tabla (Req. 6.1, 6.2, 7.11,
 * 20.1). Los errores se propagan como `ApiError` normalizado del `apiClient`.
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
  fetchDetail,
  fetchList,
  postResource,
  putResource,
} from "@/features/shared";
import type {
  CustomerCreateInput,
  CustomerEditInput,
} from "@/features/customers/schema";
import type { ListParams, ListResult } from "@/types/api";

/** Ruta del recurso Customer en el BFF. */
const RESOURCE_URL = "/customers";

/**
 * Representación mínima de un Customer tal como lo devuelve el BFF. Se mantiene
 * laxa (índice de propiedades) porque el detalle exacto lo definen las vistas
 * (tarea 16.1); los campos conocidos facilitan el consumo tipado habitual.
 */
export interface Customer {
  readonly external_id?: string;
  readonly email?: string;
  readonly first_name?: string;
  readonly last_name?: string;
  readonly [key: string]: unknown;
}

/** Clave para localizar a un cliente concreto (detalle/edición). */
export interface CustomerLocator {
  readonly email?: string;
  readonly external_id?: string;
  /**
   * `customer_id` interno de OTT. El contrato OTT no permite filtrar por este
   * id directamente; el BFF lo resuelve recorriendo el padrón (con índice
   * cacheado) y puede responder "no disponible" (OK=false) si el entorno no lo
   * soporta. Se usa para resolver el cliente asociado a una suscripción.
   */
  readonly customer_id?: string;
}

/** Fábrica de `queryKey`s del recurso Customer. */
export const customerKeys = {
  all: ["customers"] as const,
  lists: () => [...customerKeys.all, "list"] as const,
  list: (params: ListParams) => [...customerKeys.lists(), params] as const,
  details: () => [...customerKeys.all, "detail"] as const,
  detail: (locator: CustomerLocator) =>
    [...customerKeys.details(), locator] as const,
};

/** Filtros opcionales del listado de clientes (resueltos por el BFF). */
export interface CustomerFilters {
  readonly email?: string;
  readonly external_id?: string;
  /** Filtro por customer_id interno de OTT, resuelto por el BFF sobre el caché. */
  readonly customer_id?: string;
}

/**
 * Lista de clientes con paginación/orden/búsqueda y filtros opcionales por
 * email/external_id/customer_id (Req. 6.1, 6.2). Un listado sin resultados
 * devuelve `items: []` sin error (Req. 6.5).
 */
export function useCustomers(
  params: ListParams = {},
  filters: CustomerFilters = {},
  options?: Omit<
    UseQueryOptions<ListResult<Customer>, ApiError>,
    "queryKey" | "queryFn"
  >,
): UseQueryResult<ListResult<Customer>, ApiError> {
  return useQuery<ListResult<Customer>, ApiError>({
    queryKey: [...customerKeys.list(params), filters] as const,
    queryFn: () =>
      fetchList<Customer>(RESOURCE_URL, params, {
        email: filters.email,
        external_id: filters.external_id,
        customer_id: filters.customer_id,
      }),
    ...options,
  });
}

/**
 * Detalle de un cliente localizado por `email` o `external_id` (Req. 6.1). La
 * query se habilita solo cuando se provee al menos un identificador.
 */
export function useCustomer(
  locator: CustomerLocator,
  options?: Omit<
    UseQueryOptions<Customer, ApiError>,
    "queryKey" | "queryFn" | "enabled"
  > & { enabled?: boolean },
): UseQueryResult<Customer, ApiError> {
  const hasLocator = Boolean(
    locator.email || locator.external_id || locator.customer_id,
  );
  return useQuery<Customer, ApiError>({
    queryKey: customerKeys.detail(locator),
    queryFn: async () => {
      // Resolución por customer_id: el BFF (que lo filtra sobre el caché)
      // responde un LISTADO; se toma el primer elemento como detalle. Si no hay
      // coincidencias, se devuelve un objeto vacío (las vistas caen al id como
      // respaldo sin romperse).
      if (locator.customer_id) {
        const result = await fetchList<Customer>(
          RESOURCE_URL,
          { page: 1, recordsPerPage: 1 },
          { customer_id: locator.customer_id },
        );
        return result.items[0] ?? ({} as Customer);
      }
      // Resolución por email/external_id: OTT devuelve un objeto único.
      return fetchDetail<Customer>(RESOURCE_URL, {
        email: locator.email,
        external_id: locator.external_id,
      });
    },
    enabled: hasLocator && (options?.enabled ?? true),
    ...options,
  });
}

/** Crea un cliente (Req. 7) e invalida los listados tras el éxito. */
export function useCreateCustomer(
  options?: UseMutationOptions<Customer, ApiError, CustomerCreateInput>,
): UseMutationResult<Customer, ApiError, CustomerCreateInput> {
  const queryClient = useQueryClient();
  return useMutation<Customer, ApiError, CustomerCreateInput>({
    mutationFn: (input) =>
      postResource<CustomerCreateInput, Customer>(RESOURCE_URL, input),
    ...options,
    onSuccess: (...args) => {
      void queryClient.invalidateQueries({ queryKey: customerKeys.lists() });
      return options?.onSuccess?.(...args);
    },
  });
}

/** Variables de edición: datos editables + localizador inmutable (Req. 8). */
export interface UpdateCustomerVariables {
  readonly locator: CustomerLocator;
  readonly data: CustomerEditInput;
}

/**
 * Edita un cliente (Req. 8). El `email`/`external_id` del localizador viajan por
 * query para que el BFF ubique al cliente; `email`/`external_id` son inmutables.
 * Invalida listados y el detalle del cliente afectado.
 */
export function useUpdateCustomer(
  options?: UseMutationOptions<Customer, ApiError, UpdateCustomerVariables>,
): UseMutationResult<Customer, ApiError, UpdateCustomerVariables> {
  const queryClient = useQueryClient();
  return useMutation<Customer, ApiError, UpdateCustomerVariables>({
    mutationFn: ({ locator, data }) =>
      putResource<CustomerEditInput, Customer>(RESOURCE_URL, data, {
        params: {
          ...(locator.email ? { email: locator.email } : {}),
          ...(locator.external_id ? { external_id: locator.external_id } : {}),
        },
      }),
    ...options,
    onSuccess: (...args) => {
      const [, variables] = args;
      void queryClient.invalidateQueries({ queryKey: customerKeys.lists() });
      void queryClient.invalidateQueries({
        queryKey: customerKeys.detail(variables.locator),
      });
      return options?.onSuccess?.(...args);
    },
  });
}
