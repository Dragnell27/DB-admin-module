"use client";

/**
 * Hooks de TanStack Query del recurso User del panel de administración RBAC
 * (tarea 18.2).
 *
 * Mapean a las rutas del BFF (ver design.md, "Handlers de usuarios"):
 * - `GET  /users`                 → listado de usuarios (Req. 10.1, 10.2).
 * - `GET  /users/:id`             → detalle de un usuario por id.
 * - `POST /users`                 → crear usuario; el BFF genera y devuelve
 *   `generatedPassword` UNA sola vez (Req. 3.x, 16.1).
 * - `PUT  /users`                 → editar roles + estado activo (Req. 4.x).
 * - `POST /users/reset-password`  → regenerar contraseña UNA sola vez.
 *
 * Las mutaciones invalidan los listados (y el detalle, en edición) tras un
 * éxito, de modo que la UI refleje los cambios sin recargar. Los errores se
 * propagan como `ApiError` normalizado por `apiClient`.
 *
 * Nota: los tipos de entrada (`UserCreateInput`/`UserEditInput`) se definen
 * localmente aquí para no acoplar el build a `src/features/users/schema.ts`
 * (tarea 18.1), que puede no existir todavía. Cuando ese schema esté
 * disponible, estos tipos deben alinearse con los inferidos por zod.
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
import type { ApiEnvelope } from "@/types/api";

/** Rutas del recurso User en el BFF. */
const RESOURCE_URL = "/users";
const RESET_PASSWORD_URL = "/users/reset-password";

/** Rol asociado a un usuario, en su forma mínima (id + nombre). */
export interface UserRoleRef {
  readonly id: string;
  readonly name: string;
}

/**
 * Vista pública de un usuario del panel RBAC. Es exactamente el shape que el
 * BFF expone (sin el hash de contraseña): `{ id, username, isActive, roles }`.
 */
export interface AdminUser {
  readonly id: string;
  readonly username: string;
  readonly isActive: boolean;
  readonly roles: ReadonlyArray<UserRoleRef>;
  readonly countries: ReadonlyArray<string>;
}

/**
 * Entrada para crear un usuario (Req. 3.x, 16.1). El BFF genera la contraseña
 * inicial; aquí sólo viajan el `username` y los roles asignados.
 */
export interface UserCreateInput {
  readonly username: string;
  readonly roleIds: ReadonlyArray<string>;
  readonly countries: ReadonlyArray<string>;
}

/**
 * Entrada para editar un usuario (Req. 4.x). Reemplaza el conjunto de roles y
 * el estado activo/inactivo del usuario identificado por `id`.
 */
export interface UserEditInput {
  readonly id: string;
  readonly roleIds: ReadonlyArray<string>;
  readonly countries: ReadonlyArray<string>;
  readonly isActive: boolean;
}

/** Entrada para regenerar la contraseña de un usuario por `id`. */
export interface UserResetPasswordInput {
  readonly id: string;
}

/**
 * Resultado de crear un usuario: el usuario resultante y la contraseña generada
 * por el BFF, que sólo se devuelve en esta respuesta (Req. 3.x).
 */
export interface UserCreateResult {
  readonly user: AdminUser;
  readonly generatedPassword: string;
}

/** Resultado de editar un usuario: el usuario actualizado. */
export interface UserEditResult {
  readonly user: AdminUser;
}

/** Resultado de regenerar la contraseña: la nueva contraseña generada. */
export interface UserResetPasswordResult {
  readonly generatedPassword: string;
}

/** Fábrica de `queryKey`s del recurso User. */
export const userKeys = {
  all: ["users"] as const,
  lists: () => [...userKeys.all, "list"] as const,
  list: () => [...userKeys.lists()] as const,
  details: () => [...userKeys.all, "detail"] as const,
  detail: (id: string) => [...userKeys.details(), id] as const,
};

/**
 * Listado de usuarios del panel RBAC (Req. 10.1, 10.2). Un listado vacío llega
 * como `items: []` sin lanzar error.
 */
export function useUsers(
  options?: Omit<
    UseQueryOptions<ReadonlyArray<AdminUser>, ApiError>,
    "queryKey" | "queryFn"
  >,
): UseQueryResult<ReadonlyArray<AdminUser>, ApiError> {
  return useQuery<ReadonlyArray<AdminUser>, ApiError>({
    queryKey: userKeys.list(),
    // El contrato de usuarios devuelve la lista completa (sin paginación), por
    // lo que basta con desenvolver `items` del `ListResult`.
    queryFn: async () => {
      const result = await fetchList<AdminUser>(RESOURCE_URL);
      return result.items;
    },
    ...options,
  });
}

/**
 * Detalle de un usuario por `id` (ruta con path param `GET /users/:id`).
 *
 * `fetchDetail` de `shared.ts` sólo construye la URL con query params, por lo
 * que para esta ruta usamos `apiClient.get` directamente contra `/users/${id}`
 * y desenvolvemos el envoltorio uniforme del BFF (`{ ok, data, ... }`).
 */
export function useUser(
  id: string | undefined,
  options?: Omit<
    UseQueryOptions<AdminUser, ApiError>,
    "queryKey" | "queryFn" | "enabled"
  > & { enabled?: boolean },
): UseQueryResult<AdminUser, ApiError> {
  return useQuery<AdminUser, ApiError>({
    queryKey: userKeys.detail(id ?? ""),
    queryFn: async () => {
      const response = await apiClient.get<ApiEnvelope<AdminUser>>(
        `${RESOURCE_URL}/${id}`,
      );
      return response.data.data;
    },
    enabled: Boolean(id) && (options?.enabled ?? true),
    ...options,
  });
}

/**
 * Crea un usuario (Req. 3.x, 16.1) e invalida los listados. Devuelve el usuario
 * creado junto con la contraseña generada por el BFF (visible una sola vez).
 */
export function useCreateUser(
  options?: UseMutationOptions<UserCreateResult, ApiError, UserCreateInput>,
): UseMutationResult<UserCreateResult, ApiError, UserCreateInput> {
  const queryClient = useQueryClient();
  return useMutation<UserCreateResult, ApiError, UserCreateInput>({
    mutationFn: (input) =>
      postResource<UserCreateInput, UserCreateResult>(RESOURCE_URL, input),
    ...options,
    onSuccess: (...args) => {
      void queryClient.invalidateQueries({ queryKey: userKeys.lists() });
      return options?.onSuccess?.(...args);
    },
  });
}

/**
 * Edita un usuario (roles + estado activo, Req. 4.x) vía `PUT /users`; invalida
 * los listados y el detalle del usuario afectado.
 */
export function useUpdateUser(
  options?: UseMutationOptions<UserEditResult, ApiError, UserEditInput>,
): UseMutationResult<UserEditResult, ApiError, UserEditInput> {
  const queryClient = useQueryClient();
  return useMutation<UserEditResult, ApiError, UserEditInput>({
    mutationFn: (input) =>
      putResource<UserEditInput, UserEditResult>(RESOURCE_URL, input),
    ...options,
    onSuccess: (...args) => {
      // `args[1]` es el `variables` (UserEditInput) de la mutación.
      const variables = args[1];
      void queryClient.invalidateQueries({ queryKey: userKeys.lists() });
      void queryClient.invalidateQueries({
        queryKey: userKeys.detail(variables.id),
      });
      return options?.onSuccess?.(...args);
    },
  });
}

/**
 * Regenera la contraseña de un usuario (Req. 4.x) vía
 * `POST /users/reset-password`. Devuelve la nueva contraseña generada por el
 * BFF, visible una sola vez. No altera listados ni detalle.
 */
export function useResetUserPassword(
  options?: UseMutationOptions<
    UserResetPasswordResult,
    ApiError,
    UserResetPasswordInput
  >,
): UseMutationResult<UserResetPasswordResult, ApiError, UserResetPasswordInput> {
  return useMutation<UserResetPasswordResult, ApiError, UserResetPasswordInput>({
    mutationFn: (input) =>
      postResource<UserResetPasswordInput, UserResetPasswordResult>(
        RESET_PASSWORD_URL,
        input,
      ),
    ...options,
  });
}
