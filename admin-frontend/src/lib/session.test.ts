// @vitest-environment jsdom

// Pruebas del bus de sesión in-tab (FEAT-002): `subscribeSession` recibe la
// notificación al escribir/borrar el token (el evento `storage` del navegador
// no se dispara en la pestaña que escribe), y el `unsubscribe` devuelto deja de
// recibirla.

import { afterEach, describe, expect, it, vi } from "vitest";

import {
  clearSessionToken,
  setSessionToken,
  subscribeSession,
} from "@/lib/session";

afterEach(() => {
  window.localStorage.clear();
});

describe("subscribeSession (bus in-tab)", () => {
  it("notifica al llamar setSessionToken", () => {
    const listener = vi.fn();
    const unsub = subscribeSession(listener);

    setSessionToken("un-token");

    expect(listener).toHaveBeenCalledTimes(1);
    unsub();
  });

  it("notifica al llamar clearSessionToken", () => {
    const listener = vi.fn();
    const unsub = subscribeSession(listener);

    clearSessionToken();

    expect(listener).toHaveBeenCalledTimes(1);
    unsub();
  });

  it("el unsubscribe devuelto deja de recibir notificaciones", () => {
    const listener = vi.fn();
    const unsub = subscribeSession(listener);

    unsub();
    setSessionToken("otro-token");
    clearSessionToken();

    expect(listener).not.toHaveBeenCalled();
  });
});
