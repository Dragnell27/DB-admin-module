// @vitest-environment jsdom

// Pruebas de componente de `ResourceTable` (tarea 14.3).
// Validan el estado de lista vacía sin error (Req. 6.5), el estado de carga, el
// disparo de `onSortChange` desde los encabezados ordenables (Req. 6.4) y los
// controles de paginación anterior/siguiente respetando los límites (Req. 6.3).

import "@testing-library/jest-dom/vitest";
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import {
  ResourceTable,
  type PaginationMeta,
  type ResourceColumn,
} from "@/components/resource-table";

interface Row {
  readonly id: string;
  readonly name: string;
}

const columns: ReadonlyArray<ResourceColumn<Row>> = [
  { key: "name", header: "Nombre", sortable: true },
  { key: "id", header: "ID" },
];

function meta(overrides: Partial<PaginationMeta> = {}): PaginationMeta {
  return {
    totalRecords: 0,
    page: 1,
    recordsPerPage: 10,
    totalPages: 0,
    ...overrides,
  };
}

afterEach(() => {
  cleanup();
});

describe("ResourceTable — estado vacío y carga", () => {
  it("muestra \"Sin resultados\" cuando no hay filas y no está cargando", () => {
    render(
      <ResourceTable<Row>
        columns={columns}
        rows={[]}
        pagination={meta()}
        isLoading={false}
      />,
    );

    expect(screen.getByText("Sin resultados")).toBeInTheDocument();
  });

  it("respeta un mensaje de vacío personalizado", () => {
    render(
      <ResourceTable<Row>
        columns={columns}
        rows={[]}
        pagination={meta()}
        emptyMessage="No hay clientes"
      />,
    );

    expect(screen.getByText("No hay clientes")).toBeInTheDocument();
  });

  it("muestra el estado de carga y no el mensaje vacío", () => {
    render(
      <ResourceTable<Row>
        columns={columns}
        rows={[]}
        pagination={meta()}
        isLoading
      />,
    );

    expect(screen.getByText("Cargando...")).toBeInTheDocument();
    expect(screen.queryByText("Sin resultados")).not.toBeInTheDocument();
  });

  it("renderiza las filas cuando hay datos", () => {
    const rows: Row[] = [
      { id: "a1", name: "Ana" },
      { id: "b2", name: "Beto" },
    ];

    render(
      <ResourceTable<Row>
        columns={columns}
        rows={rows}
        rowKey="id"
        pagination={meta({ totalRecords: 2, totalPages: 1 })}
      />,
    );

    expect(screen.getByText("Ana")).toBeInTheDocument();
    expect(screen.getByText("Beto")).toBeInTheDocument();
    expect(screen.queryByText("Sin resultados")).not.toBeInTheDocument();
  });
});

describe("ResourceTable — ordenamiento", () => {
  it("invoca onSortChange con asc al pulsar un encabezado ordenable", async () => {
    const user = userEvent.setup({ delay: null });
    const onSortChange = vi.fn();

    render(
      <ResourceTable<Row>
        columns={columns}
        rows={[]}
        pagination={meta()}
        onSortChange={onSortChange}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Ordenar por Nombre" }));

    expect(onSortChange).toHaveBeenCalledTimes(1);
    expect(onSortChange).toHaveBeenCalledWith({ field: "name", order: "asc" });
  });

  it("alterna a desc cuando la columna ya está ordenada asc", async () => {
    const user = userEvent.setup({ delay: null });
    const onSortChange = vi.fn();

    render(
      <ResourceTable<Row>
        columns={columns}
        rows={[]}
        pagination={meta()}
        sort={{ field: "name", order: "asc" }}
        onSortChange={onSortChange}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Ordenar por Nombre" }));

    expect(onSortChange).toHaveBeenCalledWith({ field: "name", order: "desc" });
  });

  it("no expone control de orden en columnas no ordenables", () => {
    render(
      <ResourceTable<Row>
        columns={columns}
        rows={[]}
        pagination={meta()}
        onSortChange={vi.fn()}
      />,
    );

    expect(
      screen.queryByRole("button", { name: /Ordenar por ID/i }),
    ).not.toBeInTheDocument();
  });
});

describe("ResourceTable — paginación", () => {
  it("deshabilita \"Anterior\" en la primera página y habilita \"Siguiente\"", () => {
    render(
      <ResourceTable<Row>
        columns={columns}
        rows={[{ id: "a1", name: "Ana" }]}
        pagination={meta({ totalRecords: 30, page: 1, totalPages: 3 })}
        onPageChange={vi.fn()}
      />,
    );

    expect(screen.getByRole("button", { name: "Página anterior" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Página siguiente" })).toBeEnabled();
  });

  it("invoca onPageChange con la página siguiente", async () => {
    const user = userEvent.setup({ delay: null });
    const onPageChange = vi.fn();

    render(
      <ResourceTable<Row>
        columns={columns}
        rows={[{ id: "a1", name: "Ana" }]}
        pagination={meta({ totalRecords: 30, page: 1, totalPages: 3 })}
        onPageChange={onPageChange}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Página siguiente" }));

    expect(onPageChange).toHaveBeenCalledWith(2);
  });

  it("invoca onPageChange con la página anterior", async () => {
    const user = userEvent.setup({ delay: null });
    const onPageChange = vi.fn();

    render(
      <ResourceTable<Row>
        columns={columns}
        rows={[{ id: "a1", name: "Ana" }]}
        pagination={meta({ totalRecords: 30, page: 2, totalPages: 3 })}
        onPageChange={onPageChange}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Página anterior" }));

    expect(onPageChange).toHaveBeenCalledWith(1);
  });

  it("deshabilita \"Siguiente\" en la última página", async () => {
    const user = userEvent.setup({ delay: null });
    const onPageChange = vi.fn();

    render(
      <ResourceTable<Row>
        columns={columns}
        rows={[{ id: "a1", name: "Ana" }]}
        pagination={meta({ totalRecords: 30, page: 3, totalPages: 3 })}
        onPageChange={onPageChange}
      />,
    );

    const next = screen.getByRole("button", { name: "Página siguiente" });
    expect(next).toBeDisabled();

    await user.click(next);
    expect(onPageChange).not.toHaveBeenCalled();
  });

  it("no permite avanzar cuando no hay páginas (lista vacía)", () => {
    render(
      <ResourceTable<Row>
        columns={columns}
        rows={[]}
        pagination={meta({ totalRecords: 0, page: 1, totalPages: 0 })}
        onPageChange={vi.fn()}
      />,
    );

    expect(screen.getByRole("button", { name: "Página anterior" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Página siguiente" })).toBeDisabled();
  });
});
