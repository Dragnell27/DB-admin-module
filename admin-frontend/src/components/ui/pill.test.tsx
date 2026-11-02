// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";

import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";

import { Pill } from "@/components/ui/pill";

describe("Pill", () => {
  afterEach(() => {
    cleanup();
  });

  it("muestra el texto hijo y la superficie azul en la variante principal", () => {
    render(<Pill variant="principal">Principal</Pill>);
    const pill = screen.getByText("Principal");
    expect(pill).toHaveClass("bg-pill-principal-bg");
    // La variante principal es la única que antepone el punto indicador.
    expect(pill.querySelector("[aria-hidden='true']")).not.toBeNull();
  });

  it("muestra el texto hijo y la superficie neutra en la variante foranea sin punto", () => {
    render(<Pill variant="foranea">Foránea</Pill>);
    const pill = screen.getByText("Foránea");
    expect(pill).toHaveClass("bg-border-soft");
    expect(pill.querySelector("[aria-hidden='true']")).toBeNull();
  });

  it("muestra el texto hijo y la superficie neutra en la variante count sin punto", () => {
    render(<Pill variant="count">3 columnas</Pill>);
    const pill = screen.getByText("3 columnas");
    expect(pill).toHaveClass("bg-border-soft");
    expect(pill.querySelector("[aria-hidden='true']")).toBeNull();
  });

  it("muestra el texto hijo y la superficie púrpura en la variante pk sin punto", () => {
    render(<Pill variant="pk">2 PK</Pill>);
    const pill = screen.getByText("2 PK");
    expect(pill).toHaveClass("bg-pill-pk-bg");
    expect(pill.querySelector("[aria-hidden='true']")).toBeNull();
  });
});
