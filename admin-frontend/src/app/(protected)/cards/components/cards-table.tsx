"use client";

/**
 * Tabla de tarjetas de un cliente (tarea 16.4; Req. 17.1, 17.3, 19.1-19.5).
 *
 * Conecta `ResourceTable` (controlado) con el hook `useCards`, que solo se
 * ejecuta cuando el selector trae `customer_id` o `email` (Req. 17.1). Un
 * listado sin tarjetas se muestra como estado vacío sin error (Req. 17.3).
 *
 * Para el rol `admin` cada fila ofrece eliminar, que abre una confirmación
 * inline (no un diálogo compartido) antes de llamar a `useDeleteCard`
 * (Req. 19.1-19.3). El resultado se reporta al contenedor por callbacks para
 * el banner de éxito/error (Req. 19.4, 19.5).
 */
import * as React from "react";
import { Loader2, Trash2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  ResourceTable,
  type PaginationMeta,
  type ResourceColumn,
} from "@/components/resource-table";
import { isApiError } from "@/lib/api-client";
import {
  useCards,
  useDeleteCard,
  type Card as CardType,
  type CardCustomerSelector,
} from "@/features/cards/hooks";
import type { ListParams } from "@/types/api";

/** Página inicial y tamaño por defecto para el listado. */
const DEFAULT_PAGE = 1;
const DEFAULT_RECORDS_PER_PAGE = 25;

/** Pagination vacía usada mientras no hay datos. */
const EMPTY_PAGINATION: PaginationMeta = {
  totalRecords: 0,
  page: DEFAULT_PAGE,
  recordsPerPage: DEFAULT_RECORDS_PER_PAGE,
  totalPages: 0,
};

/** Devuelve un texto seguro para una propiedad desconocida de la tarjeta. */
function text(value: unknown): string {
  if (value === null || value === undefined || value === "") {
    return "—";
  }
  return String(value);
}

export interface CardsTableProps {
  /** Cliente cuyas tarjetas se listan (Req. 17.1). */
  readonly selector: CardCustomerSelector;
  /** Indica si el usuario puede eliminar (rol admin) (Req. 5.1, 5.2). */
  readonly canWrite: boolean;
  /** Se invoca tras una eliminación exitosa (Req. 19.4). */
  readonly onDeleteSuccess?: () => void;
  /** Se invoca ante un error de eliminación (Req. 19.5). */
  readonly onDeleteError?: (message: string) => void;
}

export function CardsTable({
  selector,
  canWrite,
  onDeleteSuccess,
  onDeleteError,
}: CardsTableProps) {
  const [page, setPage] = React.useState(DEFAULT_PAGE);
  const [recordsPerPage, setRecordsPerPage] = React.useState(
    DEFAULT_RECORDS_PER_PAGE,
  );
  // `card_id` de la fila en confirmación de borrado (null = ninguna).
  const [pendingDeleteId, setPendingDeleteId] = React.useState<string | null>(
    null,
  );

  // Reinicia a la primera página cuando cambia el cliente seleccionado.
  React.useEffect(() => {
    setPage(DEFAULT_PAGE);
    setPendingDeleteId(null);
  }, [selector.customer_id, selector.email]);

  const params: ListParams = { page, recordsPerPage };
  const query = useCards(selector, params);

  const deleteCard = useDeleteCard({
    onSuccess: () => {
      setPendingDeleteId(null);
      onDeleteSuccess?.();
    },
    onError: (error) => {
      setPendingDeleteId(null);
      onDeleteError?.(
        isApiError(error)
          ? error.message
          : "No se pudo eliminar la tarjeta. Intenta de nuevo.",
      );
    },
  });

  const rows = React.useMemo(
    () => (query.data?.items ?? []) as ReadonlyArray<CardType>,
    [query.data],
  );
  const pagination = query.data?.pagination ?? EMPTY_PAGINATION;

  const columns = React.useMemo<ReadonlyArray<ResourceColumn<CardType>>>(
    () => [
      {
        key: "card_number",
        header: "Tarjeta",
        render: (row) => text(row.card_number ?? row.card_id),
      },
      {
        key: "card_franchise",
        header: "Franquicia",
        render: (row) => text(row.card_franchise),
      },
      {
        key: "payment_method_name",
        header: "Método de pago",
        render: (row) => text(row.payment_method_name),
      },
      {
        key: "expiration_date",
        header: "Expiración",
        render: (row) => text(row.expiration_date),
      },
      ...(canWrite
        ? [
            {
              key: "actions",
              header: <span className="sr-only">Acciones</span>,
              className: "w-px text-right",
              render: (row: CardType) => {
                const cardId =
                  typeof row.card_id === "string" ? row.card_id : undefined;
                if (!cardId) {
                  return "—";
                }
                const isConfirming = pendingDeleteId === cardId;
                const isDeleting =
                  deleteCard.isPending && pendingDeleteId === cardId;

                if (isConfirming) {
                  // Confirmación inline (Req. 19.1-19.3): sin diálogo compartido.
                  return (
                    <div className="flex items-center justify-end gap-2">
                      <span className="text-sm text-muted-foreground">
                        ¿Eliminar?
                      </span>
                      <Button
                        type="button"
                        variant="destructive"
                        size="sm"
                        disabled={isDeleting}
                        onClick={() => deleteCard.mutate(cardId)}
                      >
                        {isDeleting ? (
                          <>
                            <Loader2
                              className="animate-spin"
                              aria-hidden="true"
                            />
                            Eliminando...
                          </>
                        ) : (
                          "Confirmar"
                        )}
                      </Button>
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        disabled={isDeleting}
                        onClick={() => setPendingDeleteId(null)}
                      >
                        Cancelar
                      </Button>
                    </div>
                  );
                }

                return (
                  <div className="flex items-center justify-end">
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      className="text-destructive hover:text-destructive"
                      onClick={() => setPendingDeleteId(cardId)}
                      aria-label="Eliminar tarjeta"
                    >
                      <Trash2 aria-hidden="true" />
                      Eliminar
                    </Button>
                  </div>
                );
              },
            } satisfies ResourceColumn<CardType>,
          ]
        : []),
    ],
    [canWrite, deleteCard, pendingDeleteId],
  );

  // Un error de carga real (no un listado vacío) se expone sobre la tabla.
  const loadError =
    query.isError && query.error
      ? isApiError(query.error)
        ? query.error.message
        : "No se pudieron cargar las tarjetas."
      : null;

  return (
    <div className="space-y-3">
      {loadError ? (
        <div
          role="alert"
          className="rounded-md border border-destructive/50 bg-destructive/10 px-3 py-2 text-sm text-destructive"
        >
          {loadError}
        </div>
      ) : null}

      <ResourceTable<CardType>
        columns={columns}
        rows={rows}
        rowKey={(row) =>
          typeof row.card_id === "string" ? row.card_id : JSON.stringify(row)
        }
        pagination={pagination}
        isLoading={query.isLoading || query.isFetching}
        onPageChange={setPage}
        onRecordsPerPageChange={(next) => {
          setRecordsPerPage(next);
          setPage(DEFAULT_PAGE);
        }}
        emptyMessage="Este cliente no tiene tarjetas registradas"
      />
    </div>
  );
}

export default CardsTable;
