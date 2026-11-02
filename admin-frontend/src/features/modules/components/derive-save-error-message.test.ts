// Prueba de la lógica de disparo migrada del asistente de módulos (sitio #2 de
// la tabla de clasificación). `deriveSaveErrorMessage` es una función PURA
// extraída del IIFE anterior para unit-testear ambas ramas sin renderizar: el
// asistente llama `toast.error(deriveSaveErrorMessage(err))` en el `onError` de
// la mutación de guardar, en lugar de mostrar un banner inline.

import { describe, expect, it } from "vitest";

import { deriveSaveErrorMessage } from "@/features/modules/components/module-wizard";
import type { ApiError } from "@/lib/api-client";

describe("deriveSaveErrorMessage", () => {
  it("E409 (slug duplicado) → mensaje específico en español", () => {
    const err = { errorCode: "E409", message: "conflict" } as unknown as ApiError;
    expect(deriveSaveErrorMessage(err)).toBe(
      "El identificador del módulo ya está en uso. Elige otro slug.",
    );
  });

  it("otro error → mensaje del backend si existe", () => {
    const err = { message: "Boom del servidor" } as unknown as ApiError;
    expect(deriveSaveErrorMessage(err)).toBe("Boom del servidor");
  });

  it("sin error / sin mensaje → genérico de respaldo", () => {
    expect(deriveSaveErrorMessage(undefined)).toBe("No se pudo guardar el módulo.");
  });
});
