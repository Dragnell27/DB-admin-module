// @vitest-environment jsdom

// Pruebas de componente del editor de plantillas de conexión
// (connection-templates, Req. 10.3): marcar un campo FIJO muestra un input de
// valor; marcarlo VARIABLE oculta el valor y ofrece required/suggested; el
// desplegable de rol ofrece el conjunto cerrado + option. Se usa fireEvent
// (síncrono) para evitar el cuelgue de userEvent en jsdom/Windows.

import "@testing-library/jest-dom/vitest";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";

// Mocks de router y hooks para no requerir QueryClientProvider ni red.
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn() }),
}));

const noopMut = { mutate: vi.fn(), isPending: false, isError: false, error: undefined };
vi.mock("@/features/connections/hooks", () => ({
  useCreateConnectionTemplate: () => noopMut,
  useUpdateConnectionTemplate: () => noopMut,
}));

import { TemplateEditor } from "@/features/connections/components/template-editor";

beforeEach(() => {
  vi.clearAllMocks();
});
afterEach(() => cleanup());

describe("TemplateEditor — marcar campos FIJO/VARIABLE", () => {
  it("un campo nuevo es VARIABLE por defecto: muestra 'Valor sugerido' y no input de valor", () => {
    render(<TemplateEditor />);
    fireEvent.click(screen.getByRole("button", { name: "Añadir campo" }));

    // Por defecto kind = variable.
    expect(screen.getByLabelText("Tipo")).toHaveValue("variable");
    // Hay input de valor sugerido (no de valor fijo).
    expect(screen.getByLabelText("Valor sugerido (opcional)")).toBeInTheDocument();
    expect(screen.queryByLabelText("Valor")).not.toBeInTheDocument();
    // Ofrece el checkbox de obligatorio.
    expect(
      screen.getByRole("checkbox", { name: /Obligatorio al crear el módulo/ }),
    ).toBeInTheDocument();
  });

  it("marcar FIJO muestra un input de valor y oculta sugerido/required", () => {
    render(<TemplateEditor />);
    fireEvent.click(screen.getByRole("button", { name: "Añadir campo" }));

    fireEvent.change(screen.getByLabelText("Tipo"), { target: { value: "fixed" } });

    expect(screen.getByLabelText("Valor")).toBeInTheDocument();
    expect(screen.queryByLabelText("Valor sugerido (opcional)")).not.toBeInTheDocument();
    expect(
      screen.queryByRole("checkbox", { name: /Obligatorio al crear el módulo/ }),
    ).not.toBeInTheDocument();
  });

  it("renderiza el pie con la acción de guardar desde el primer montaje", () => {
    render(<TemplateEditor title="Nueva plantilla de conexión" />);

    // El pie (FormActionBar en modo estructural) está presente desde el inicio.
    expect(screen.getByRole("button", { name: "Crear plantilla" })).toBeInTheDocument();
  });

  it("el desplegable de rol ofrece el conjunto cerrado + option", () => {
    render(<TemplateEditor />);
    fireEvent.click(screen.getByRole("button", { name: "Añadir campo" }));

    const roleSelect = screen.getByLabelText("Rol en el DSN");
    const options = within(roleSelect as HTMLElement)
      .getAllByRole("option")
      .map((o) => (o as HTMLOptionElement).value);
    for (const r of ["host", "port", "user", "password", "dbname", "sslmode", "option"]) {
      expect(options).toContain(r);
    }
    // Incluye la opción automática (role vacío).
    expect(options).toContain("");
  });
});
