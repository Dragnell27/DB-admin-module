// @vitest-environment jsdom

// Pruebas de `useGlobalRefresh` (botón GLOBAL de la topbar): compone la
// reemisión del token (`/auth/refresh-permissions` → `setSessionToken`) con la
// invalidación de la query del menú lateral (`moduleDefKeys.visible`).
// - Éxito: persiste el token nuevo e invalida `["modules","visible"]`.
// - Error (401): NO persiste el token y NO invalida ninguna query.

import * as React from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

import { apiClient } from "@/lib/api-client";
import { setSessionToken } from "@/lib/session";
import { useGlobalRefresh } from "@/features/modules/hooks";

vi.mock("@/lib/api-client", () => ({
  apiClient: { post: vi.fn() },
}));

vi.mock("@/lib/session", () => ({
  setSessionToken: vi.fn(),
}));

const postMock = vi.mocked(apiClient.post);
const setSessionTokenMock = vi.mocked(setSessionToken);

/**
 * Provee un `QueryClientProvider` sin reintentos y expone el cliente para
 * espiar `invalidateQueries`.
 */
function createWrapper() {
  const client = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  });
  const invalidateSpy = vi.spyOn(client, "invalidateQueries");
  function Wrapper({ children }: { children: React.ReactNode }) {
    return React.createElement(QueryClientProvider, { client }, children);
  }
  return { Wrapper, invalidateSpy };
}

beforeEach(() => {
  postMock.mockReset();
  setSessionTokenMock.mockReset();
});

afterEach(() => {
  vi.clearAllMocks();
});

describe("useGlobalRefresh", () => {
  it("en éxito persiste el token nuevo e invalida la query del menú", async () => {
    postMock.mockResolvedValue({ data: { data: { token: "nuevo-token" } } });
    const { Wrapper, invalidateSpy } = createWrapper();

    const { result } = renderHook(() => useGlobalRefresh(), {
      wrapper: Wrapper,
    });

    await result.current.refresh();

    expect(postMock).toHaveBeenCalledWith("/auth/refresh-permissions");
    expect(setSessionTokenMock).toHaveBeenCalledWith("nuevo-token");
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ["modules", "visible"],
    });
  });

  it("en error (401) NO persiste el token NI invalida queries", async () => {
    postMock.mockRejectedValue({ isApiError: true, status: 401 });
    const { Wrapper, invalidateSpy } = createWrapper();

    const { result } = renderHook(() => useGlobalRefresh(), {
      wrapper: Wrapper,
    });

    await expect(result.current.refresh()).rejects.toBeTruthy();

    await waitFor(() => expect(setSessionTokenMock).not.toHaveBeenCalled());
    expect(invalidateSpy).not.toHaveBeenCalled();
  });
});
