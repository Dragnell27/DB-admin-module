"use client";

/**
 * `ResourceTable` — tabla genérica y reutilizable del panel (tarea 14.2).
 *
 * Es un componente CONTROLADO: el estado de búsqueda, página, tamaño de página
 * y ordenamiento vive en el componente padre (las vistas de recurso de las
 * tareas 16.x) y se comunica hacia abajo por props, mientras que los cambios se
 * reportan hacia arriba por callbacks. Esto permite que el padre conecte la
 * tabla a los hooks de TanStack Query (tareas 15.2/16.x) sin que la tabla sepa
 * nada del origen de los datos.
 *
 * Características (Req. 6.3, 6.5, 6.6, 9.3, 13.4, 17.3, 20.4):
 * - Columnas y datos configurables mediante `columns` y `rows`.
 * - Buscador con _debounce_ que reporta el término por `onSearchChange`.
 * - Controles de paginación (anterior/siguiente + indicador) y selector de
 *   tamaño de página, respetando `page > 0` y `recordsPerPage <= 100`.
 * - Ordenamiento por columnas marcadas como `sortable`, alternando asc/desc y
 *   reportando `order_by`/`order` por `onSortChange`.
 * - Estado de carga y estado de lista vacía ("sin resultados") sin error.
 * - Muestra los metadatos de paginación `totalRecords`, `page`,
 *   `recordsPerPage` y `totalPages` devueltos por el backend.
 */
import * as React from "react";
import {
  ArrowDown,
  ArrowUp,
  ArrowUpDown,
  ChevronLeft,
  ChevronRight,
  Loader2,
  Search,
} from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { cn } from "@/lib/utils";

/** Límite máximo de registros por página impuesto por el contrato OTT. */
export const MAX_RECORDS_PER_PAGE = 100;

/** Dirección de ordenamiento aceptada por el contrato OTT. */
export type SortOrder = "asc" | "desc";

/**
 * Estado de ordenamiento actual. `field` corresponde a `order_by` (un campo
 * permitido por recurso) y `order` a la dirección.
 */
export interface SortState {
  readonly field: string;
  readonly order: SortOrder;
}

/**
 * Metadatos de paginación tal como los expone el BFF tras normalizar la
 * respuesta OTT (Req. 6.6, 20.4).
 */
export interface PaginationMeta {
  readonly totalRecords: number;
  readonly page: number;
  readonly recordsPerPage: number;
  readonly totalPages: number;
}

/**
 * Definición de una columna de la tabla, genérica sobre el tipo de fila `TRow`.
 */
export interface ResourceColumn<TRow> {
  /**
   * Identificador estable de la columna. Para columnas ordenables debe
   * coincidir con el campo `order_by` permitido del recurso.
   */
  readonly key: string;
  /** Encabezado visible de la columna. */
  readonly header: React.ReactNode;
  /**
   * Marca la columna como ordenable. Solo las columnas con `sortable: true`
   * muestran el control de orden y pueden emitir `onSortChange` (Req. 6.4).
   */
  readonly sortable?: boolean;
  /**
   * Render personalizado de la celda. Si se omite, se muestra
   * `row[key]` convertido a texto.
   */
  readonly render?: (row: TRow) => React.ReactNode;
  /** Clase adicional opcional para la celda y el encabezado. */
  readonly className?: string;
}

export interface ResourceTableProps<TRow> {
  /** Configuración de columnas. */
  readonly columns: ReadonlyArray<ResourceColumn<TRow>>;
  /** Filas a renderizar (ya paginadas por el backend). */
  readonly rows: ReadonlyArray<TRow>;
  /**
   * Clave única por fila. Puede ser el nombre de una propiedad de `TRow` o una
   * función que derive la clave. Por defecto usa el índice de la fila.
   */
  readonly rowKey?: keyof TRow | ((row: TRow) => React.Key);
  /** Metadatos de paginación. */
  readonly pagination: PaginationMeta;
  /** Indica si hay una carga en curso. */
  readonly isLoading?: boolean;

  /** Valor controlado del buscador. */
  readonly searchValue?: string;
  /** Se invoca (con _debounce_) cuando cambia el término de búsqueda. */
  readonly onSearchChange?: (value: string) => void;
  /** Texto de ayuda del buscador. */
  readonly searchPlaceholder?: string;
  /** Milisegundos de _debounce_ del buscador (por defecto 300). */
  readonly searchDebounceMs?: number;

  /** Estado de ordenamiento actual (controlado). */
  readonly sort?: SortState | null;
  /** Se invoca cuando el usuario cambia el ordenamiento (Req. 6.4). */
  readonly onSortChange?: (sort: SortState) => void;

  /** Se invoca cuando el usuario cambia de página (base 1, `page > 0`). */
  readonly onPageChange?: (page: number) => void;
  /** Opciones de tamaño de página ofrecidas. Todas deben ser `<= 100`. */
  readonly pageSizeOptions?: ReadonlyArray<number>;
  /** Se invoca cuando el usuario cambia el tamaño de página. */
  readonly onRecordsPerPageChange?: (recordsPerPage: number) => void;

  /** Mensaje mostrado cuando no hay filas. */
  readonly emptyMessage?: string;
  /** Clase adicional del contenedor. */
  readonly className?: string;
}

const DEFAULT_PAGE_SIZE_OPTIONS: ReadonlyArray<number> = [10, 25, 50, 100];
const DEFAULT_EMPTY_MESSAGE = "Sin resultados";
const DEFAULT_SEARCH_PLACEHOLDER = "Buscar...";
const DEFAULT_DEBOUNCE_MS = 300;

/** Resuelve la clave de React para una fila dada. */
function resolveRowKey<TRow>(
  row: TRow,
  index: number,
  rowKey?: keyof TRow | ((row: TRow) => React.Key),
): React.Key {
  if (typeof rowKey === "function") {
    return rowKey(row);
  }
  if (rowKey !== undefined) {
    const value = row[rowKey];
    if (typeof value === "string" || typeof value === "number") {
      return value;
    }
  }
  return index;
}

/** Convierte un valor de celda en algo renderizable de forma segura. */
function renderCellValue<TRow>(
  column: ResourceColumn<TRow>,
  row: TRow,
): React.ReactNode {
  if (column.render) {
    return column.render(row);
  }
  const value = (row as Record<string, unknown>)[column.key];
  if (value === null || value === undefined) {
    return "—";
  }
  if (typeof value === "string" || typeof value === "number") {
    return value;
  }
  if (typeof value === "boolean") {
    return value ? "Sí" : "No";
  }
  return String(value);
}

/** Icono de orden según el estado de la columna. */
function SortIcon({ active, order }: { active: boolean; order?: SortOrder }) {
  if (!active) {
    return <ArrowUpDown className="opacity-50" aria-hidden="true" />;
  }
  return order === "asc" ? (
    <ArrowUp aria-hidden="true" />
  ) : (
    <ArrowDown aria-hidden="true" />
  );
}

export function ResourceTable<TRow>({
  columns,
  rows,
  rowKey,
  pagination,
  isLoading = false,
  searchValue = "",
  onSearchChange,
  searchPlaceholder = DEFAULT_SEARCH_PLACEHOLDER,
  searchDebounceMs = DEFAULT_DEBOUNCE_MS,
  sort = null,
  onSortChange,
  onPageChange,
  pageSizeOptions = DEFAULT_PAGE_SIZE_OPTIONS,
  onRecordsPerPageChange,
  emptyMessage = DEFAULT_EMPTY_MESSAGE,
  className,
}: ResourceTableProps<TRow>) {
  // Estado local del buscador para aplicar _debounce_ sin perder el control del
  // padre: el input refleja lo que el usuario escribe de inmediato, pero el
  // callback se dispara tras `searchDebounceMs`.
  const [localSearch, setLocalSearch] = React.useState(searchValue);

  // Sincroniza cuando el padre cambia el valor controlado (p. ej. al limpiar).
  React.useEffect(() => {
    setLocalSearch(searchValue);
  }, [searchValue]);

  const onSearchChangeRef = React.useRef(onSearchChange);
  React.useEffect(() => {
    onSearchChangeRef.current = onSearchChange;
  }, [onSearchChange]);

  const handleSearchInput = React.useCallback(
    (event: React.ChangeEvent<HTMLInputElement>) => {
      setLocalSearch(event.target.value);
    },
    [],
  );

  // Debounce: emite el término al padre cuando se estabiliza.
  React.useEffect(() => {
    if (localSearch === searchValue) {
      return;
    }
    const handle = window.setTimeout(() => {
      onSearchChangeRef.current?.(localSearch);
    }, searchDebounceMs);
    return () => window.clearTimeout(handle);
  }, [localSearch, searchValue, searchDebounceMs]);

  const handleSort = React.useCallback(
    (column: ResourceColumn<TRow>) => {
      if (!column.sortable || !onSortChange) {
        return;
      }
      const isActive = sort?.field === column.key;
      // Alterna asc/desc sobre la misma columna; una nueva columna empieza asc.
      const nextOrder: SortOrder =
        isActive && sort?.order === "asc" ? "desc" : "asc";
      onSortChange({ field: column.key, order: nextOrder });
    },
    [onSortChange, sort],
  );

  const { page, totalPages, totalRecords, recordsPerPage } = pagination;
  const canGoPrev = page > 1;
  const canGoNext = totalPages > 0 && page < totalPages;

  const handlePrev = React.useCallback(() => {
    if (canGoPrev) {
      onPageChange?.(page - 1);
    }
  }, [canGoPrev, onPageChange, page]);

  const handleNext = React.useCallback(() => {
    if (canGoNext) {
      onPageChange?.(page + 1);
    }
  }, [canGoNext, onPageChange, page]);

  const handleRecordsPerPage = React.useCallback(
    (event: React.ChangeEvent<HTMLSelectElement>) => {
      const next = Number(event.target.value);
      if (Number.isFinite(next) && next > 0 && next <= MAX_RECORDS_PER_PAGE) {
        onRecordsPerPageChange?.(next);
      }
    },
    [onRecordsPerPageChange],
  );

  const columnCount = columns.length;
  const hasRows = rows.length > 0;

  return (
    <div className={cn("flex flex-col gap-4", className)}>
      {/* Buscador (Req. 6.1, 6.2) */}
      {onSearchChange ? (
        <div className="relative w-full max-w-sm">
          <Search
            className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
            aria-hidden="true"
          />
          <Input
            type="search"
            value={localSearch}
            onChange={handleSearchInput}
            placeholder={searchPlaceholder}
            className="pl-9"
            aria-label={searchPlaceholder}
          />
        </div>
      ) : null}

      {/* Tabla */}
      <div className="rounded-lg border border-border bg-card">
        <Table>
          <TableHeader>
            <TableRow>
              {columns.map((column) => {
                const isActive = sort?.field === column.key;
                if (column.sortable && onSortChange) {
                  return (
                    <TableHead key={column.key} className={column.className}>
                      <button
                        type="button"
                        onClick={() => handleSort(column)}
                        className="inline-flex items-center gap-1.5 whitespace-nowrap font-medium text-muted-foreground transition-colors hover:text-foreground [&_svg]:size-3.5 [&_svg]:shrink-0"
                        aria-label={`Ordenar por ${
                          typeof column.header === "string"
                            ? column.header
                            : column.key
                        }`}
                      >
                        {column.header}
                        <SortIcon active={isActive} order={sort?.order} />
                      </button>
                    </TableHead>
                  );
                }
                return (
                  <TableHead key={column.key} className={column.className}>
                    {column.header}
                  </TableHead>
                );
              })}
            </TableRow>
          </TableHeader>
          <TableBody>
            {isLoading ? (
              <TableRow>
                <TableCell colSpan={columnCount} className="h-32 text-center">
                  <span className="inline-flex items-center gap-2 text-muted-foreground">
                    <Loader2 className="size-4 animate-spin" aria-hidden="true" />
                    Cargando...
                  </span>
                </TableCell>
              </TableRow>
            ) : hasRows ? (
              rows.map((row, index) => (
                <TableRow key={resolveRowKey(row, index, rowKey)}>
                  {columns.map((column) => (
                    <TableCell
                      key={column.key}
                      className={column.className}
                    >
                      {renderCellValue(column, row)}
                    </TableCell>
                  ))}
                </TableRow>
              ))
            ) : (
              /* Estado de lista vacía sin error (Req. 6.5, 9.5, 13.5, 17.3) */
              <TableRow>
                <TableCell
                  colSpan={columnCount}
                  className="h-32 text-center text-muted-foreground"
                >
                  {emptyMessage}
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </div>

      {/* Pie: metadatos de paginación + controles (Req. 6.3, 6.6, 20.4) */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-sm text-muted-foreground">
          <span>
            {totalRecords}{" "}
            {totalRecords === 1 ? "registro" : "registros"}
          </span>
          {onRecordsPerPageChange ? (
            <label className="flex items-center gap-2">
              <span>Por página</span>
              <select
                value={recordsPerPage}
                onChange={handleRecordsPerPage}
                className="h-9 rounded-md border border-input bg-input px-2 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                aria-label="Registros por página"
              >
                {pageSizeOptions
                  .filter((size) => size > 0 && size <= MAX_RECORDS_PER_PAGE)
                  .map((size) => (
                    <option key={size} value={size}>
                      {size}
                    </option>
                  ))}
              </select>
            </label>
          ) : null}
        </div>

        <div className="flex items-center gap-3">
          <span className="text-sm text-muted-foreground">
            Página {totalPages === 0 ? 0 : page} de {totalPages}
          </span>
          <div className="flex items-center gap-1">
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={handlePrev}
              disabled={!canGoPrev || isLoading}
              aria-label="Página anterior"
            >
              <ChevronLeft aria-hidden="true" />
              Anterior
            </Button>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={handleNext}
              disabled={!canGoNext || isLoading}
              aria-label="Página siguiente"
            >
              Siguiente
              <ChevronRight aria-hidden="true" />
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}

export default ResourceTable;
