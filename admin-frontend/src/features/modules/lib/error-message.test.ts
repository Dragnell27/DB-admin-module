// Test PURO (entorno 'node' por defecto, sin jsdom) de `safeModuleErrorMessage`
// (Punto 2 / Decisión D2). Verifica el saneado de errores de infraestructura y
// la conservación de los mensajes útiles del BFF, incluido el falso positivo
// evitado (422 con 'SQL'/'database' se CONSERVA).

import { describe, expect, it } from "vitest";

import type { ApiError } from "@/lib/api-client";
import {
  GENERIC_DATASOURCE_MESSAGE,
  safeModuleErrorMessage,
} from "@/features/modules/lib/error-message";

function apiError(partial: Partial<ApiError>): ApiError {
  return { isApiError: true, message: "", ...partial };
}

describe("safeModuleErrorMessage", () => {
  it("502 con 'dial tcp ...' → mensaje genérico", () => {
    const error = apiError({
      status: 502,
      message:
        "dial tcp 10.0.0.5:5432: connect: connection refused (user=app db=ott)",
    });
    expect(safeModuleErrorMessage(error)).toBe(GENERIC_DATASOURCE_MESSAGE);
  });

  it("500 → mensaje genérico", () => {
    const error = apiError({ status: 500, message: "Internal Server Error" });
    expect(safeModuleErrorMessage(error)).toBe(GENERIC_DATASOURCE_MESSAGE);
  });

  it("status undefined (red/timeout) → mensaje genérico", () => {
    const error = apiError({
      message: "No se pudo conectar con el servidor. Verifica tu conexión.",
    });
    expect(safeModuleErrorMessage(error)).toBe(GENERIC_DATASOURCE_MESSAGE);
  });

  it("422 de validación → conserva el mensaje del BFF", () => {
    const error = apiError({ status: 422, message: "El campo nombre es obligatorio" });
    expect(safeModuleErrorMessage(error)).toBe("El campo nombre es obligatorio");
  });

  it("404 → conserva el mensaje del BFF", () => {
    const error = apiError({ status: 404, message: "Recurso no encontrado" });
    expect(safeModuleErrorMessage(error)).toBe("Recurso no encontrado");
  });

  it("403 → conserva el mensaje del BFF", () => {
    const error = apiError({ status: 403, message: "No tienes permisos" });
    expect(safeModuleErrorMessage(error)).toBe("No tienes permisos");
  });

  it("message vacío → mensaje genérico", () => {
    const error = apiError({ status: 400, message: "   " });
    expect(safeModuleErrorMessage(error)).toBe(GENERIC_DATASOURCE_MESSAGE);
  });

  it("null → mensaje genérico", () => {
    expect(safeModuleErrorMessage(null)).toBe(GENERIC_DATASOURCE_MESSAGE);
  });

  it("undefined → mensaje genérico", () => {
    expect(safeModuleErrorMessage(undefined)).toBe(GENERIC_DATASOURCE_MESSAGE);
  });

  it("400 con 'connection refused' → mensaje genérico (patrón de infra)", () => {
    const error = apiError({ status: 400, message: "connection refused by host" });
    expect(safeModuleErrorMessage(error)).toBe(GENERIC_DATASOURCE_MESSAGE);
  });

  it("falso positivo evitado: 422 con 'SQL' → conserva", () => {
    const error = apiError({
      status: 422,
      message: "El identificador SQL no es válido",
    });
    expect(safeModuleErrorMessage(error)).toBe("El identificador SQL no es válido");
  });

  it("falso positivo evitado: 422 con 'database' → conserva", () => {
    const error = apiError({
      status: 422,
      message: "El registro ya existe en la database",
    });
    expect(safeModuleErrorMessage(error)).toBe("El registro ya existe en la database");
  });
});
