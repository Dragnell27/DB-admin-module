"use client";

/**
 * Hook de metadatos del BFF para poblar el selector de país.
 *
 * `GET /meta/countries` devuelve, en el envoltorio normalizado, los países que
 * el BFF puede consultar (tienen credenciales OTT) y el país activo por defecto:
 *   { ok, data: { countries: ["gt","ni"], active: "gt" }, ... }
 *
 * La respuesta cambia poco (depende de la configuración del backend), por lo que
 * se cachea con un staleTime alto.
 */
import { useQuery, type UseQueryResult } from "@tanstack/react-query";

import { apiClient, type ApiError } from "@/lib/api-client";
import type { ApiEnvelope } from "@/types/api";

/** Metadatos de países disponibles para el selector. */
export interface CountriesMeta {
  /** Países con credenciales OTT (códigos en minúsculas). */
  readonly countries: ReadonlyArray<string>;
  /** País por defecto cuando no se envía X-Country. */
  readonly active: string;
}

/** Clave de caché del endpoint de países. */
export const countriesMetaKey = ["meta", "countries"] as const;

/**
 * Obtiene la lista de países disponibles y el país activo por defecto del BFF.
 * Se cachea de forma agresiva porque depende de la configuración del servidor.
 */
export function useCountriesMeta(): UseQueryResult<CountriesMeta, ApiError> {
  return useQuery<CountriesMeta, ApiError>({
    queryKey: countriesMetaKey,
    queryFn: async () => {
      const response =
        await apiClient.get<ApiEnvelope<CountriesMeta>>("/meta/countries");
      const data = response.data?.data;
      return {
        countries: Array.isArray(data?.countries) ? data.countries : [],
        active: typeof data?.active === "string" ? data.active : "",
      };
    },
    staleTime: 5 * 60 * 1000,
    retry: 1,
  });
}
