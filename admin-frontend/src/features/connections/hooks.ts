"use client";

/**
 * Hooks TanStack Query de las plantillas de conexión (connection-templates),
 * espejo de features/modules/hooks.ts. Todas las llamadas pasan por apiClient
 * (hereda Authorization y X-Country). La autorización real la aplica el backend.
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
import { type ApiEnvelope } from "@/types/api";
import type {
  ConnectionTemplate,
  ConnectionTemplateInput,
} from "@/features/connections/types";

/** Fábrica de queryKeys de plantillas de conexión. */
export const connectionKeys = {
  all: ["connection-templates"] as const,
  lists: () => ["connection-templates", "list"] as const,
  detail: (id: string) => ["connection-templates", "detail", id] as const,
};

/** Lista todas las plantillas (GET /connection-templates). */
export function useConnectionTemplates(
  options?: Omit<
    UseQueryOptions<ReadonlyArray<ConnectionTemplate>, ApiError>,
    "queryKey" | "queryFn"
  >,
): UseQueryResult<ReadonlyArray<ConnectionTemplate>, ApiError> {
  return useQuery<ReadonlyArray<ConnectionTemplate>, ApiError>({
    queryKey: connectionKeys.lists(),
    queryFn: async () => {
      const res = await apiClient.get<ApiEnvelope<{ items?: ConnectionTemplate[] }>>(
        "/connection-templates",
      );
      return res.data?.data?.items ?? [];
    },
    ...options,
  });
}

/** Obtiene una plantilla por id (GET /connection-templates/:id). */
export function useConnectionTemplate(
  id: string,
  options?: Omit<
    UseQueryOptions<ConnectionTemplate, ApiError>,
    "queryKey" | "queryFn" | "enabled"
  > & { enabled?: boolean },
): UseQueryResult<ConnectionTemplate, ApiError> {
  return useQuery<ConnectionTemplate, ApiError>({
    queryKey: connectionKeys.detail(id),
    queryFn: async () => {
      const res = await apiClient.get<ApiEnvelope<ConnectionTemplate>>(
        `/connection-templates/${encodeURIComponent(id)}`,
      );
      return res.data.data;
    },
    enabled: Boolean(id) && (options?.enabled ?? true),
    ...options,
  });
}

/** Crea una plantilla (POST /connection-templates). */
export function useCreateConnectionTemplate(
  options?: UseMutationOptions<{ id: string }, ApiError, ConnectionTemplateInput>,
): UseMutationResult<{ id: string }, ApiError, ConnectionTemplateInput> {
  const queryClient = useQueryClient();
  return useMutation<{ id: string }, ApiError, ConnectionTemplateInput>({
    mutationFn: async (input) => {
      const res = await apiClient.post<ApiEnvelope<{ id: string }>>(
        "/connection-templates",
        input,
      );
      return res.data.data;
    },
    ...options,
    onSuccess: (...args) => {
      void queryClient.invalidateQueries({ queryKey: connectionKeys.all });
      return options?.onSuccess?.(...args);
    },
  });
}

/** Edita una plantilla (PUT /connection-templates/:id). */
export function useUpdateConnectionTemplate(
  id: string,
  options?: UseMutationOptions<{ id: string }, ApiError, ConnectionTemplateInput>,
): UseMutationResult<{ id: string }, ApiError, ConnectionTemplateInput> {
  const queryClient = useQueryClient();
  return useMutation<{ id: string }, ApiError, ConnectionTemplateInput>({
    mutationFn: async (input) => {
      const res = await apiClient.put<ApiEnvelope<{ id: string }>>(
        `/connection-templates/${encodeURIComponent(id)}`,
        input,
      );
      return res.data.data;
    },
    ...options,
    onSuccess: (...args) => {
      void queryClient.invalidateQueries({ queryKey: connectionKeys.all });
      void queryClient.invalidateQueries({ queryKey: connectionKeys.detail(id) });
      return options?.onSuccess?.(...args);
    },
  });
}

/** Elimina una plantilla (DELETE /connection-templates/:id). */
export function useDeleteConnectionTemplate(
  options?: UseMutationOptions<{ id: string }, ApiError, string>,
): UseMutationResult<{ id: string }, ApiError, string> {
  const queryClient = useQueryClient();
  return useMutation<{ id: string }, ApiError, string>({
    mutationFn: async (id) => {
      const res = await apiClient.delete<ApiEnvelope<{ id: string }>>(
        `/connection-templates/${encodeURIComponent(id)}`,
      );
      return res.data.data;
    },
    ...options,
    onSuccess: (...args) => {
      void queryClient.invalidateQueries({ queryKey: connectionKeys.all });
      return options?.onSuccess?.(...args);
    },
  });
}
