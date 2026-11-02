/**
 * Cliente HTTP único del admin-frontend (Req. 2.3, 2.5, 20.1).
 *
 * El frontend habla ÚNICAMENTE con el admin-backend (BFF); nunca llama a las
 * APIs OTT directamente. Toda petición de la aplicación debe pasar por esta
 * instancia de axios para heredar:
 *
 * - baseURL leída de `NEXT_PUBLIC_ADMIN_API_URL` (vía `env.adminApiUrl`).
 * - Interceptor de request que adjunta `Authorization: Bearer <JWT sesión>`
 *   cuando existe una sesión (Req. 2.3).
 * - Interceptor de response que, ante 401, borra el token y redirige a `/login`
 *   (Req. 2.5), y normaliza cualquier error de la API a una forma uniforme
 *   (`ApiError`) para que las vistas y hooks (tareas 15.x/16.x) muestren el
 *   mensaje del backend sin inspeccionar la estructura cruda de axios.
 *
 * El acceso al token se hace a través del seam `src/lib/session.ts`, que la
 * tarea 13.1 completará. No se implementa aquí login ni hooks de recurso.
 */
import axios, {
  AxiosError,
  type AxiosInstance,
  type InternalAxiosRequestConfig,
} from "axios";

import { env } from "@/lib/env";
import { clearSessionToken, getSessionToken } from "@/lib/session";
import { getCountry } from "@/lib/country";

/**
 * Error normalizado de la API del BFF. Es lo que recibe el `catch`/`onError`
 * de cualquier llamada hecha con este cliente, de modo que la capa de UI pueda
 * depender de una forma estable.
 */
export interface ApiError {
  /** Marca de tipo para discriminar en los consumidores. */
  readonly isApiError: true;
  /** Código HTTP de la respuesta, si la hubo. */
  readonly status?: number;
  /**
   * Código de error de negocio devuelto por el BFF/OTT (p. ej. `S401SEC`),
   * cuando el cuerpo lo expone.
   */
  readonly errorCode?: string;
  /** Mensaje legible (en español) listo para mostrar en un toast. */
  readonly message: string;
  /** Cuerpo de error devuelto por el servidor, para diagnósticos avanzados. */
  readonly data?: unknown;
}

/** Ruta pública de login a la que se redirige cuando la sesión expira. */
const LOGIN_PATH = "/login";

/** Mensaje por defecto cuando el servidor no proporciona uno. */
const DEFAULT_ERROR_MESSAGE = "Ocurrió un error inesperado. Intenta de nuevo.";

/** Mensaje cuando no hubo respuesta del servidor (red/timeout). */
const NETWORK_ERROR_MESSAGE =
  "No se pudo conectar con el servidor. Verifica tu conexión.";

/**
 * Comprueba, de forma segura ante tipos desconocidos, si un valor es un objeto
 * plano con propiedades consultables.
 */
function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

/**
 * Extrae el mensaje más adecuado del cuerpo de error del BFF. El normalizador
 * del backend produce un envoltorio uniforme `{ ok, data, errorCode, message }`
 * (ver design.md), pero también toleramos variantes (`error`, `resultMessage`).
 */
function extractMessage(data: unknown): string | undefined {
  if (typeof data === "string" && data.trim() !== "") {
    return data;
  }
  if (!isRecord(data)) {
    return undefined;
  }
  const candidate =
    data.message ?? data.error ?? data.resultMessage ?? data.detail;
  return typeof candidate === "string" && candidate.trim() !== ""
    ? candidate
    : undefined;
}

/** Extrae el código de error de negocio del cuerpo, si existe. */
function extractErrorCode(data: unknown): string | undefined {
  if (!isRecord(data)) {
    return undefined;
  }
  const candidate = data.errorCode ?? data.error_code;
  return typeof candidate === "string" ? candidate : undefined;
}

/**
 * Convierte cualquier error (de axios o no) en el `ApiError` uniforme que
 * consumen las capas superiores.
 */
export function normalizeApiError(error: unknown): ApiError {
  if (axios.isAxiosError(error)) {
    const axiosError = error as AxiosError;
    const status = axiosError.response?.status;
    const responseData = axiosError.response?.data;

    if (axiosError.response) {
      return {
        isApiError: true,
        status,
        errorCode: extractErrorCode(responseData),
        message: extractMessage(responseData) ?? DEFAULT_ERROR_MESSAGE,
        data: responseData,
      };
    }

    // Sin respuesta: error de red, timeout o cancelación.
    return {
      isApiError: true,
      message: NETWORK_ERROR_MESSAGE,
    };
  }

  if (error instanceof Error) {
    return { isApiError: true, message: error.message || DEFAULT_ERROR_MESSAGE };
  }

  return { isApiError: true, message: DEFAULT_ERROR_MESSAGE };
}

/** Type guard para que los consumidores reconozcan un `ApiError`. */
export function isApiError(value: unknown): value is ApiError {
  return isRecord(value) && value.isApiError === true;
}

/**
 * Redirige al login tras una sesión inválida/expirada. Se separa para poder
 * ejecutarse sólo en el navegador y evitar romper en SSR.
 */
function redirectToLogin(): void {
  if (typeof window === "undefined") {
    return;
  }
  // Evita un bucle de redirección si ya estamos en la pantalla de login.
  if (window.location.pathname === LOGIN_PATH) {
    return;
  }
  window.location.assign(LOGIN_PATH);
}

/** Instancia única de axios apuntando al BFF. */
export const apiClient: AxiosInstance = axios.create({
  baseURL: env.adminApiUrl,
  headers: {
    "Content-Type": "application/json",
  },
});

// Interceptor de request: adjunta el JWT de sesión del admin (Req. 2.3).
apiClient.interceptors.request.use(
  (config: InternalAxiosRequestConfig) => {
    const token = getSessionToken();
    if (token) {
      config.headers.set("Authorization", `Bearer ${token}`);
    }
    // Selector de país: adjunta X-Country con el país seleccionado. Si no hay
    // selección (null), no se envía y el BFF usa su país por defecto.
    const country = getCountry();
    if (country) {
      config.headers.set("X-Country", country);
    }
    return config;
  },
  (error: unknown) => Promise.reject(normalizeApiError(error)),
);

// Interceptor de response: maneja 401 (sesión expirada) y normaliza errores (Req. 2.5, 20.1).
apiClient.interceptors.response.use(
  (response) => response,
  (error: unknown) => {
    if (axios.isAxiosError(error) && error.response?.status === 401) {
      // Sesión ausente/expirada/ inválida: borrar token y volver al login.
      clearSessionToken();
      redirectToLogin();
    }
    return Promise.reject(normalizeApiError(error));
  },
);

export default apiClient;
