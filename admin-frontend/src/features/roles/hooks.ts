"use client";

/**
 * Hooks de TanStack Query del recurso Role (tarea 18.3).
 *
 * Mapean a las rutas del BFF (ver design.md, "API/Endpoints"):
 * - `GET /roles`        → listado de roles con sus permisos (Req. 11.1).
 * - `GET /roles/:id`    → detalle de un rol por id (path param) (Req. 11.2).
 * - `POST /roles`       → crear rol `{name, permissions}` → `{role}` (Req. 11.1).
 * - `PUT /roles`        → editar rol `{id, name, permissions}` → `{role}`.
 * - `DELETE /roles/:id` → eliminar un rol por id (path param).
 *
 * El detalle y el borrado usan path params (`/roles/${id}`), que las utilidades
 * de `shared.ts` no soportan (envían identificadores por query), por lo que se
 * resuelven con `apiClient` directo. La creación/edición sí reutilizan
 * `postResource`/`putResource` y luego desanidan la clave `role` del envoltorio.
 *
 * Las mutaciones invalidan los listados tras un éxito (y el detalle en la
 * edición), siguiendo el patrón de los demás recursos (Req. 16.1). Los errores
 * se propagan como `ApiError` normalizado por `apiClient`.
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
import { fetchList, postResource, putResource } from "@/features/shared";
import type { PermissionInput } from "@/features/roles/schema";
import type { ApiEnvelope, ListParams, ListResult } from "@/types/api";

/** Ruta del recurso Role en el BFF. */
const RESOURCE_URL = "/roles";

/**
 * Representación de un Role tal como lo devuelve el BFF (Req. 11.1, 11.2). Los
 * permisos son la lista de duplas recurso × acción. Se usa el shape de
 * `Permission` (de `@/lib/permissions`) relajado a strings para no acoplar la
 * capa de datos al catálogo cerrado: la respuesta del BFF puede, en teoría,
 * traer valores que la UI aún deba tolerar.
 */
export interface Role {
  readonly id: string;
  readonly name: string;
  readonly permissions: ReadonlyArray<{
    readonly resource: string;
    readonly action: string;
  }>;
}

/**
 * Entrada de creación de un rol (`POST /roles`). Los permisos usan el shape de
 * `Permission` del catálogo (recurso/acción tipados, sin país).
 */
export interface RoleCreateInput {
  readonly name: string;
  readonly permissions: ReadonlyArray<PermissionInput>;
}

/** Entrada de edición de un rol (`PUT /roles`): añade el `id` del rol. */
export interface RoleUpdateInput extends RoleCreateInput {
  readonly id: string;
}

/**
 * Un ítem del catálogo de módulos dinámicos (GET /roles/module-catalog), que la
 * pantalla de Roles usa para pintar los permisos `mod:<slug>` junto a los fijos.
 * `actions` SIEMPRE empieza por "ver" seguido del resto en orden alfabético.
 */
export interface ModuleCatalogItem {
  readonly slug: string;
  readonly name: string;
  readonly actions: ReadonlyArray<string>;
  /**
   * Mapa opcional de acciones de permiso custom (`custom:<x>`) a su etiqueta
   * legible. El backend lo envía de forma retrocompatible (omitido si no hay
   * acciones custom con label); la UI lo consume de forma tolerante y degrada a
   * capitalizar la clave si falta.
   */
  readonly actionLabels?: Readonly<Record<string, string>>;
}

/** Fábrica de `queryKey`s del recurso Role. */
export const roleKeys = {
  all: ["roles"] as const,
  lists: () => [...roleKeys.all, "list"] as const,
  list: (params: ListParams) => [...roleKeys.lists(), params] as const,
  details: () => [...roleKeys.all, "detail"] as const,
  detail: (id: string) => [...roleKeys.details(), id] as const,
  moduleCatalog: () => [...roleKeys.all, "module-catalog"] as const,
};

/**
 * Lista el catálogo de módulos dinámicos (GET /roles/module-catalog), para que
 * la pantalla de Roles ofrezca sus permisos `mod:<slug>` junto a los fijos. NO
 * usa `throwOnError`: el fallo se expone vía `isError` para una degradación no
 * bloqueante en la UI (los permisos fijos siguen editables). Sigue el patrón de
 * `useVisibleModules`/`useModuleDefs`.
 */
export function useModuleCatalog(
  options?: Omit<
    UseQueryOptions<ReadonlyArray<ModuleCatalogItem>, ApiError>,
    "queryKey" | "queryFn"
  >,
): UseQueryResult<ReadonlyArray<ModuleCatalogItem>, ApiError> {
  return useQuery<ReadonlyArray<ModuleCatalogItem>, ApiError>({
    queryKey: roleKeys.moduleCatalog(),
    queryFn: async () => {
      const res = await apiClient.get<
        ApiEnvelope<{ items?: ModuleCatalogItem[] }>
      >("/roles/module-catalog");
      return res.data?.data?.items ?? [];
    },
    ...options,
  });
}

/**
 * Desanida la clave `role` del envoltorio `data` cuando el BFF devuelve el rol
 * envuelto (`{ role: { ... } }`). Si ya viene plano se devuelve tal cual.
 */
function unwrapRole(data: unknown): Role {
  if (data && typeof data === "object" && !Array.isArray(data)) {
    const nested = (data as { role?: unknown }).role;
    if (nested && typeof nested === "object" && !Array.isArray(nested)) {
      return nested as Role;
    }
  }
  return data as Role;
}

/**
 * Lista de roles con sus permisos (Req. 11.1). Un listado sin resultados
 * devuelve `items: []` sin error.
 */
export function useRoles(
  params: ListParams = {},
  options?: Omit<
    UseQueryOptions<ListResult<Role>, ApiError>,
    "queryKey" | "queryFn"
  >,
): UseQueryResult<ListResult<Role>, ApiError> {
  return useQuery<ListResult<Role>, ApiError>({
    queryKey: roleKeys.list(params),
    queryFn: () => fetchList<Role>(RESOURCE_URL, params),
    ...options,
  });
}

/**
 * Detalle de un rol por id (Req. 11.2). Usa el path param `/roles/${id}`, que
 * `shared.fetchDetail` no soporta, por lo que se resuelve con `apiClient`
 * directo y se desanida la posible clave `role` del envoltorio. El nombre es
 * `useRoleDetail` para evitar colisión con `useRole()` de `@/lib/use-role`.
 */
export function useRoleDetail(
  id: string | undefined,
  options?: Omit<
    UseQueryOptions<Role, ApiError>,
    "queryKey" | "queryFn" | "enabled"
  > & { enabled?: boolean },
): UseQueryResult<Role, ApiError> {
  return useQuery<Role, ApiError>({
    queryKey: roleKeys.detail(id ?? ""),
    queryFn: async () => {
      const response = await apiClient.get<ApiEnvelope<unknown>>(
        `${RESOURCE_URL}/${id}`,
      );
      return unwrapRole(response.data.data);
    },
    enabled: Boolean(id) && (options?.enabled ?? true),
    ...options,
  });
}

/** Crea un rol (`POST /roles` → `{role}`, Req. 11.1); invalida los listados. */
export function useCreateRole(
  options?: UseMutationOptions<Role, ApiError, RoleCreateInput>,
): UseMutationResult<Role, ApiError, RoleCreateInput> {
  const queryClient = useQueryClient();
  return useMutation<Role, ApiError, RoleCreateInput>({
    mutationFn: async (input) => {
      const data = await postResource<RoleCreateInput, unknown>(
        RESOURCE_URL,
        input,
      );
      return unwrapRole(data);
    },
    ...options,
    onSuccess: (...args) => {
      void queryClient.invalidateQueries({ queryKey: roleKeys.lists() });
      return options?.onSuccess?.(...args);
    },
  });
}

/**
 * Edita un rol (`PUT /roles` con `{id, name, permissions}` → `{role}`); invalida
 * los listados y el detalle del rol editado.
 */
export function useUpdateRole(
  options?: UseMutationOptions<Role, ApiError, RoleUpdateInput>,
): UseMutationResult<Role, ApiError, RoleUpdateInput> {
  const queryClient = useQueryClient();
  return useMutation<Role, ApiError, RoleUpdateInput>({
    mutationFn: async (input) => {
      const data = await putResource<RoleUpdateInput, unknown>(
        RESOURCE_URL,
        input,
      );
      return unwrapRole(data);
    },
    ...options,
    onSuccess: (...args) => {
      const [, variables] = args;
      void queryClient.invalidateQueries({ queryKey: roleKeys.lists() });
      void queryClient.invalidateQueries({
        queryKey: roleKeys.detail(variables.id),
      });
      return options?.onSuccess?.(...args);
    },
  });
}

/**
 * Elimina un rol por id (`DELETE /roles/:id`); invalida los listados. Usa el
 * path param `/roles/${id}` con `apiClient` directo, ya que `deleteResource`
 * de `shared.ts` envía el identificador por query y no por path.
 */
export function useDeleteRole(
  options?: UseMutationOptions<unknown, ApiError, string>,
): UseMutationResult<unknown, ApiError, string> {
  const queryClient = useQueryClient();
  return useMutation<unknown, ApiError, string>({
    mutationFn: async (id) => {
      const response = await apiClient.delete<ApiEnvelope<unknown>>(
        `${RESOURCE_URL}/${id}`,
      );
      return response.data.data;
    },
    ...options,
    onSuccess: (...args) => {
      void queryClient.invalidateQueries({ queryKey: roleKeys.lists() });
      return options?.onSuccess?.(...args);
    },
  });
}
