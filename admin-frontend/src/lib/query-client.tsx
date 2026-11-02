"use client";

/**
 * Proveedor de TanStack Query para toda la aplicación (tarea 15.2).
 *
 * Se monta en el layout raíz (`src/app/layout.tsx`) envolviendo el árbol, de
 * modo que cualquier vista protegida (tareas 16.x) pueda usar los hooks de
 * query/mutation por recurso. El cliente HTTP real (`apiClient`) y la
 * normalización de errores a `ApiError` viven en `src/lib/api-client.ts`; aquí
 * solo configuramos la caché y los valores por defecto de las queries.
 *
 * SSR-safe: el `QueryClient` se crea con `useState` para garantizar UNA sola
 * instancia por montaje del cliente (no una por render) y evitar compartir
 * caché entre peticiones en el servidor.
 */
import { useState } from "react";
import {
  QueryClient,
  QueryClientProvider,
  type QueryClientConfig,
} from "@tanstack/react-query";

/**
 * Configuración por defecto de las queries. `retry: 1` evita reintentos
 * agresivos ante errores del BFF (incluidos 4xx que no deben reintentarse de
 * forma indefinida); `refetchOnWindowFocus: false` reduce peticiones
 * innecesarias en un panel de administración.
 */
const defaultConfig: QueryClientConfig = {
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      retry: 1,
      refetchOnWindowFocus: false,
    },
    mutations: {
      retry: 0,
    },
  },
};

export interface QueryProviderProps {
  readonly children: React.ReactNode;
}

/**
 * Envuelve a sus hijos con el `QueryClientProvider`. Pensado para usarse una
 * sola vez, lo más arriba posible del árbol de la aplicación.
 */
export function QueryProvider({ children }: QueryProviderProps) {
  const [queryClient] = useState(() => new QueryClient(defaultConfig));

  return (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
}

export default QueryProvider;
