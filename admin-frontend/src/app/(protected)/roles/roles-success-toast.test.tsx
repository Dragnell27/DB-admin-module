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
 * Por qué es DETERMINISTA (y NO depende del doble-efecto de React Strict Mode,
 * que solo ocurre en las builds de desarrollo de React): el test FUERZA dos
 * ejecuciones del efecto de forma controlada SIN StrictMode. El guard vive en un
 * `useRef`, que persiste mientras NO se remonte el componente; por eso las dos
 * ejecuciones tienen que ocurrir en la MISMA instancia montada (un remonte
 * reiniciaría el ref y no probaría el guard).
 *
 * Mecanismo: el mock de `useRouter` devuelve un objeto NUEVO en cada llamada
 * (`{ replace: replaceMock }` recién creado). Como el efecto del componente
 * declara `router` en sus dependencias (`[created, updated, router]`), al hacer
 * `rerender(...)` sobre la MISMA instancia el `router` cambia de referencia, de
 * modo que React vuelve a ejecutar el efecto. El `useRef` persiste entre esos dos
 * renders. Así el efecto corre dos veces SÍ o SÍ, por control explícito del
 * test, no por el modo dev de React:
 *
 *  - CON guard  → la 2ª ejecución ve `emitted.current === true` y NO emite: UNA
 *    sola llamada a `toast.success`.
 *  - SIN guard  → la 2ª ejecución emite de nuevo: DOS llamadas → el test FALLA.
 *
 * Se asevera EXACTAMENTE UNA emisión. Si se quita el guard `useRef` de
 * `RolesSuccessToast`, el conteo pasa a 2 y el test FALLA, demostrando la
 * regresión — y lo hace igual en build de producción, porque el segundo efecto
 * lo fuerza el cambio de dependencia del `rerender`, no StrictMode.
 *
 * El espía se pone sobre el MISMO objeto `toast` que importa el componente
 * (`@/lib/use-toast`), que reexporta la misma referencia de `@/lib/toast-store`.
 */
import * as React from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { render, cleanup } from "@testing-library/react";

import { toast } from "@/lib/use-toast";
import { RolesSuccessToast } from "./roles-success-toast";

// Mock de next/navigation dentro del mismo factory. `useRouter()` devuelve un
// objeto NUEVO en cada invocación para que la dependencia `router` del efecto
// cambie entre renders y fuerce una segunda ejecución del efecto en la misma
// instancia (ver cabecera). `useSearchParams()` devuelve el flag de query
// controlado por `searchParams` (new URLSearchParams tiene `.get(...)`, lo único
// que consume el componente).
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

    // Primer render: el efecto corre una vez (emisión esperada).
    const { rerender } = render(<RolesSuccessToast />);
    // Segundo render de la MISMA instancia: `useRouter()` devuelve un objeto
    // nuevo, cambia la dependencia `router` y el efecto vuelve a ejecutarse. El
    // `useRef` del guard persiste, así que NO debe emitir de nuevo.
    rerender(<RolesSuccessToast />);

    expect(successSpy).toHaveBeenCalledTimes(1);
    expect(successSpy).toHaveBeenCalledWith("Rol creado correctamente.");
  });

  it("emite UN solo toast con ?updated=1 aunque el efecto corra dos veces", () => {
    searchParams = new URLSearchParams("updated=1");
    const successSpy = vi
      .spyOn(toast, "success")
      .mockImplementation(() => "");

    const { rerender } = render(<RolesSuccessToast />);
    rerender(<RolesSuccessToast />);

    expect(successSpy).toHaveBeenCalledTimes(1);
    expect(successSpy).toHaveBeenCalledWith("Rol actualizado correctamente.");
  });
});
