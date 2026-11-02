/**
 * Configuración leída desde variables de entorno (Req. 23.2).
 * El frontend habla ÚNICAMENTE con el admin-backend (BFF) a través de esta URL.
 *
 * `NEXT_PUBLIC_ADMIN_API_URL` debe exponerse con el prefijo NEXT_PUBLIC_ para
 * estar disponible en el navegador. Ver `.env.example`.
 */
export const env = {
  /** URL base del admin-backend (BFF). */
  adminApiUrl:
    process.env.NEXT_PUBLIC_ADMIN_API_URL ?? "http://localhost:8081",
} as const;
