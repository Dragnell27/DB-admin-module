// @vitest-environment jsdom
// Pruebas del store SSR-safe de notificaciones flotantes (toasts). Cubren:
// agregar, estabilidad de snapshot (para no hacer bucle en useSyncExternalStore),
// descartar, el tope de pila (pushCapped descarta el más viejo), la guarda de
// mensaje vacío, los datos de auto-descarte por tono y el bus de suscriptores.

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  __resetToastsForTests,
  addToast,
  AUTO_DISMISS_MS,
  clearToasts,
  dismissToast,
  getToasts,
  pushCapped,
  subscribeToasts,
  TOAST_CAP,
  toast,
  type Toast,
} from "@/lib/toast-store";

describe("store de toasts", () => {
  beforeEach(() => {
    __resetToastsForTests();
  });
  afterEach(() => {
    __resetToastsForTests();
  });

  it("add: toast.success agrega un toast con tono, mensaje e id", () => {
    toast.success("Guardado");
    const list = getToasts();
    expect(list).toHaveLength(1);
    expect(list[0].tone).toBe("success");
    expect(list[0].message).toBe("Guardado");
    expect(list[0].id).not.toBe("");
  });

  it("estabilidad del snapshot: getToasts devuelve la misma referencia sin mutación", () => {
    const a = getToasts();
    const b = getToasts();
    expect(a).toBe(b);
    addToast("error", "x");
    // Tras mutar, la referencia cambia (el snapshot es inmutable).
    expect(getToasts()).not.toBe(a);
  });

  it("dismiss: agregar y luego descartar deja la lista vacía", () => {
    const id = addToast("error", "falló");
    expect(getToasts()).toHaveLength(1);
    dismissToast(id);
    expect(getToasts()).toHaveLength(0);
  });

  it("tope de pila: con 6 toasts quedan 5 y se descarta el MÁS VIEJO", () => {
    for (let i = 0; i < TOAST_CAP + 1; i++) {
      addToast("success", `m${i}`);
    }
    const list = getToasts();
    expect(list).toHaveLength(TOAST_CAP);
    // El primero (m0) fue descartado; el último (m5) está presente.
    expect(list.some((t) => t.message === "m0")).toBe(false);
    expect(list[list.length - 1].message).toBe(`m${TOAST_CAP}`);
  });

  it("pushCapped (puro): agrega al final y descarta los más viejos sobre el tope", () => {
    const mk = (n: number): Toast => ({
      id: `id${n}`,
      tone: "success",
      message: `m${n}`,
      createdAt: n,
    });
    let list: readonly Toast[] = [];
    for (let i = 0; i < 7; i++) {
      list = pushCapped(list, mk(i), 5);
    }
    expect(list).toHaveLength(5);
    expect(list[0].message).toBe("m2"); // m0 y m1 descartados
    expect(list[4].message).toBe("m6");
  });

  it("guarda de mensaje vacío: addToast con '' no crea toast y devuelve ''", () => {
    const id = addToast("error", "");
    expect(id).toBe("");
    expect(getToasts()).toHaveLength(0);
  });

  it("datos de tono: error persiste (null), éxito/aviso se auto-descartan (número)", () => {
    expect(AUTO_DISMISS_MS.error).toBeNull();
    expect(typeof AUTO_DISMISS_MS.success).toBe("number");
    expect(typeof AUTO_DISMISS_MS.warning).toBe("number");
  });

  it("subscribe/notify: el suscriptor se dispara al agregar y descartar, no tras desuscribir", () => {
    const fn = vi.fn();
    const unsub = subscribeToasts(fn);
    const id = addToast("success", "a");
    expect(fn).toHaveBeenCalledTimes(1);
    dismissToast(id);
    expect(fn).toHaveBeenCalledTimes(2);
    unsub();
    addToast("success", "b");
    expect(fn).toHaveBeenCalledTimes(2);
  });

  it("clearToasts vacía la lista", () => {
    addToast("success", "a");
    addToast("error", "b");
    clearToasts();
    expect(getToasts()).toHaveLength(0);
  });
});
