/**
 * Tipos compartidos del contrato que el admin-frontend consume del admin-backend
 * (BFF). El BFF normaliza las respuestas OTT a un envoltorio uniforme
 * `{ ok, data, pagination?, errorCode?, message }` (ver design.md, "Error
 * Handling" y el normalizador). Estos tipos son el contrato que usan los hooks
 * de TanStack Query (tarea 15.2) y las vistas (tareas 16.x).
 */

/**
 * Metadatos de paginación tal como los expone el BFF tras normalizar la
 * respuesta OTT (Req. 6.6, 20.4). Las mismas claves que consume
 * `ResourceTable` (`PaginationMeta`).
 */
export interface Pagination {
  readonly totalRecords: number;
  readonly page: number;
  readonly recordsPerPage: number;
  readonly totalPages: number;
}

/**
 * Envoltorio uniforme devuelto por el BFF para cualquier operación. `data`
 * contiene la carga útil del recurso (un objeto para detalle/mutación o una
 * colección para listados) y `pagination` solo está presente en listados.
 *
 * @typeParam T Forma de `data` para la operación concreta.
 */
export interface ApiEnvelope<T> {
  /** Indica éxito lógico de la operación según el normalizador del BFF. */
  readonly ok: boolean;
  /** Carga útil normalizada. */
  readonly data: T;
  /** Metadatos de paginación en respuestas de listado. */
  readonly pagination?: Pagination;
  /** Código de error de negocio (p. ej. `S401SEC`) cuando aplica. */
  readonly errorCode?: string;
  /** Mensaje legible en español (para toasts). */
  readonly message?: string;
}

/**
 * Resultado de un listado ya desenvuelto para los consumidores: la colección de
 * elementos y sus metadatos de paginación. Un listado "sin resultados" (el BFF
 * normaliza el 404 de OTT) se representa como `items: []` con metadatos en cero,
 * nunca como un error (Req. 6.5, 9.5, 13.5, 17.3).
 *
 * @typeParam TItem Tipo de cada elemento de la colección.
 */
export interface ListResult<TItem> {
  readonly items: ReadonlyArray<TItem>;
  readonly pagination: Pagination;
}

/** Dirección de ordenamiento aceptada por el contrato OTT (Req. 6.4). */
export type SortOrder = "asc" | "desc";

/**
 * Parámetros de listado comunes a todos los recursos (Req. 6.1-6.4, 9.1-9.4,
 * 13.1-13.4, 17.1). Son opcionales; el BFF aplica sus valores por defecto y la
 * validación espejo (`page > 0`, `records_per_page <= 100`, `order_by` por
 * recurso). Las claves usan la convención del front (`recordsPerPage`,
 * `orderBy`) y se traducen a los nombres de query del BFF al construir la
 * petición.
 */
export interface ListParams {
  /** Página base 1 (`page > 0`). */
  readonly page?: number;
  /** Tamaño de página (`<= 100`). */
  readonly recordsPerPage?: number;
  /** Campo de ordenamiento permitido por el recurso. */
  readonly orderBy?: string;
  /** Dirección de ordenamiento. */
  readonly order?: SortOrder;
  /** Término de búsqueda libre. */
  readonly search?: string;
}

/** Pagination vacía usada como respaldo cuando el BFF no la provee. */
export const EMPTY_PAGINATION: Pagination = {
  totalRecords: 0,
  page: 1,
  recordsPerPage: 0,
  totalPages: 0,
};
