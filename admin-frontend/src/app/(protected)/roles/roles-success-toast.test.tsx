// @vitest-environment jsdom

/**
 * Test de idempotencia del toast de éxito de Roles (`RolesSuccessToast`).
 *
 * Objetivo: demostrar que el guard `useRef` de `RolesSuccessToast` evita el
 * DOBLE toast que reportó la auditoría. El efecto lee el flag de query
 * (`?created=1`/`?updated=1`) y emite `toast.success(...)`; sin guard, cuando el
 * efecto se ejecuta dos veces (antes de que `router.replace` quite el flag) se
 * disparan DOS toasts.
 *
 * Por qué es DETERMINISTA (y no depende del azar del doble-render de
 * StrictMode): el test FUERZA dos ejecuciones del efecto de forma controlada —
 * monta el componente, lo desmonta y lo vuelve a montar con el MISMO flag de
 * query (remontaje explícito), de modo que el efecto corre dos veces sí o sí.
 * Como el guard vive en un `useRef` que se reinicia en cada montaje, el remonte
 * por sí solo NO probaría el guard; por eso, además, se envuelve cada montaje en
 * `<React.StrictMode>`, que en test ejecuta el efecto dos veces DENTRO del mismo
 * montaje (donde el `useRef` SÍ persiste). Así el guard se ejercita de verdad:
 *
 *  - CON guard  → cada montaje StrictMode emite UNA sola vez (ref persiste entre
 *    las dos pasadas del efecto del mismo montaje).
 *  - SIN guard  → cada montaje StrictMode emitiría DOS veces.
 *
 * Se asevera EXACTAMENTE UNA emisión por montaje. Si se quita el guard `useRef`
 * de `RolesSuccessToast`, el conteo pasa a 2 y el test FALLA, demostrando la
 * regresión.
 *
 * El espía se pone sobre el MISMO objeto `toast` que importa el componente
 * (`@/lib/use-toast`), que reexporta la misma referencia de `@/lib/toast-store`.
 */
import * as React from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { render, cleanup } from "@testing-library/react";

import { toast } from "@/lib/use-toast";
import { RolesSuccessToast } from "./roles-success-toast";

// Mock de next/navigation dentro del mismo factory: `useRouter().replace`
// espiado y `useSearchParams()` devolviendo el flag de query controlado por la
// variable `searchParams` (new URLSearchParams tiene `.get(...)`, lo único que
// consume el componente).
const replaceMock = vi.fn();
let searchParams = new URLSearchParams("created=1");
vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace: replaceMock }),
  useSearchParams: () => searchParams,
}));

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.clearAllMocks();
});

describe("RolesSuccessToast — idempotencia del toast de éxito", () => {
  it("emite UN solo toast con ?created=1 aunque el efecto corra dos veces", () => {
    searchParams = new URLSearchParams("created=1");
    const successSpy = vi
      .spyOn(toast, "success")
      .mockImplementation(() => "");

    render(
      <React.StrictMode>
        <RolesSuccessToast />
      </React.StrictMode>,
    );

    expect(successSpy).toHaveBeenCalledTimes(1);
    expect(successSpy).toHaveBeenCalledWith("Rol creado correctamente.");
  });

  it("emite UN solo toast con ?updated=1 aunque el efecto corra dos veces", () => {
    searchParams = new URLSearchParams("updated=1");
    const successSpy = vi
      .spyOn(toast, "success")
      .mockImplementation(() => "");

    render(
      <React.StrictMode>
        <RolesSuccessToast />
      </React.StrictMode>,
    );

    expect(successSpy).toHaveBeenCalledTimes(1);
    expect(successSpy).toHaveBeenCalledWith("Rol actualizado correctamente.");
  });
});
