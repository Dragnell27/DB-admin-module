// @vitest-environment jsdom

// Pruebas de `useRefreshPermissions` (FEAT-002): con `apiClient` mockeado, un
// 200 persiste el token (spy sobre `setSessionToken`); un 401 (error) NO
// persiste el token.

import * as React from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

import { apiClient } from "@/lib/api-client";
import { setSessionToken } from "@/lib/session";
import { useRefreshPermissions } from "@/features/modules/hooks";

vi.mock("@/lib/api-client", () => ({
  apiClient: { post: vi.fn() },
}));

vi.mock("@/lib/session", () => ({
  setSessionToken: vi.fn(),
}));

const postMock = vi.mocked(apiClient.post);
const setSessionTokenMock = vi.mocked(setSessionToken);

/** Provee un `QueryClientProvider` sin reintentos para el hook de mutación. */
function createWrapper() {
  const client = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  });
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return React.createElement(QueryClientProvider, { client }, children);
  };
}

beforeEach(() => {
  postMock.mockReset();
  setSessionTokenMock.mockReset();
});

afterEach(() => {
  vi.clearAllMocks();
});

describe("useRefreshPermissions", () => {
  it("un 200 persiste el token con setSessionToken", async () => {
    postMock.mockResolvedValue({ data: { data: { token: "nuevo-token" } } });

    const { result } = renderHook(() => useRefreshPermissions(), {
      wrapper: createWrapper(),
    });

    result.current.mutate();

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(postMock).toHaveBeenCalledWith("/auth/refresh-permissions");
    expect(setSessionTokenMock).toHaveBeenCalledWith("nuevo-token");
    expect(result.current.data).toBe("nuevo-token");
  });

  it("un 401 (error) NO persiste el token", async () => {
    postMock.mockRejectedValue({ isApiError: true, status: 401 });

    const { result } = renderHook(() => useRefreshPermissions(), {
      wrapper: createWrapper(),
    });

    result.current.mutate();

    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(setSessionTokenMock).not.toHaveBeenCalled();
  });
});
