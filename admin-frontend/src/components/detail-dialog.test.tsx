// @vitest-environment jsdom

// Pruebas de componente de `DetailDialog` para los cambios de la tarea de
// navegación "Editar" con retorno desde el detalle:
// - Muestra la fila "Plan ID".
// - El botón "Editar plan" aparece solo con permiso `plans/editar/<país>` y, al
//   pulsarlo, navega a `/plans/edit?plan_id=...&from=...`.
// - El botón "Editar cliente" aparece solo con permiso y con un localizador
//   (`email`/`external_id`), y navega conservando `from`.

import "@testing-library/jest-dom/vitest";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import type { EffectivePermissions } from "@/lib/permissions";

// Ternas de permisos efectivos devueltas por `usePermissions` en cada test.
let perms: EffectivePermissions = [];

// `router.push` capturado por los tests.
const push = vi.fn();

// Mock de next/navigation: `useRouter` captura `push`; `usePathname` devuelve el
// origen simulado (`/subscriptions`).
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push }),
  usePathname: () => "/subscriptions",
}));

// Mock de los hooks de datos; cada test ajusta el valor devuelto.
const usePlanMock = vi.fn();
const useCustomerMock = vi.fn();

vi.mock("@/features/plans/hooks", () => ({
  // Se conserva `planId` real (resuelve `id`/`plan_id`).
  planId: (plan: { id?: string; plan_id?: string }) =>
    plan.id ?? plan.plan_id ?? "",
  usePlan: (...args: unknown[]) => usePlanMock(...args),
}));

vi.mock("@/features/customers/hooks", () => ({
  useCustomer: (...args: unknown[]) => useCustomerMock(...args),
}));

// Mock de permisos; cada test define las ternas efectivas vía la variable `perms`.
vi.mock("@/lib/use-permissions", () => ({
  usePermissions: () => perms,
}));

// Mock de país: selector fijo en "gt"; `subscribeCountry` es noop.
vi.mock("@/lib/country", () => ({
  getCountry: () => "gt",
  subscribeCountry: () => () => {},
}));

import { DetailDialog } from "@/components/detail-dialog";

beforeEach(() => {
  push.mockReset();
  usePlanMock.mockReset();
  useCustomerMock.mockReset();
  perms = [];
});

afterEach(() => {
  cleanup();
});

describe("DetailDialog — plan", () => {
  function mockPlan() {
    usePlanMock.mockReturnValue({
      data: {
        id: "plan-123",
        name: "Plan Oro",
        country: "gt",
        currency: "GTQ",
        amount: 100,
      },
      isLoading: false,
      isError: false,
    });
  }

  it("renderiza la fila 'Plan ID'", () => {
    mockPlan();
    render(
      <DetailDialog open kind="plan" id="plan-123" onClose={vi.fn()} />,
    );

    expect(screen.getByText("Plan ID")).toBeInTheDocument();
    expect(screen.getByText("plan-123")).toBeInTheDocument();
  });

  it("con permiso plans/editar/gt muestra 'Editar plan' y navega con from", async () => {
    const user = userEvent.setup({ delay: null });
    mockPlan();
    perms = [{ resource: "plans", action: "editar", country: "gt" }];
    const onClose = vi.fn();

    render(<DetailDialog open kind="plan" id="plan-123" onClose={onClose} />);

    await user.click(screen.getByRole("button", { name: "Editar plan" }));

    expect(onClose).toHaveBeenCalledTimes(1);
    expect(push).toHaveBeenCalledTimes(1);
    const target = push.mock.calls[0][0] as string;
    expect(target).toContain("/plans/edit?");
    expect(target).toContain("plan_id=plan-123");
    expect(target).toContain("from=%2Fsubscriptions");
  });

  it("sin permiso no muestra 'Editar plan'", () => {
    mockPlan();
    perms = [];

    render(<DetailDialog open kind="plan" id="plan-123" onClose={vi.fn()} />);

    expect(
      screen.queryByRole("button", { name: "Editar plan" }),
    ).not.toBeInTheDocument();
  });
});

describe("DetailDialog — cliente", () => {
  function mockCustomer(data: Record<string, unknown>) {
    useCustomerMock.mockReturnValue({
      data,
      isLoading: false,
      isError: false,
    });
  }

  it("con permiso customers/editar/gt y localizador muestra 'Editar cliente' y navega con from", async () => {
    const user = userEvent.setup({ delay: null });
    mockCustomer({ email: "a@b.com", external_id: "ext-9", first_name: "Ana" });
    perms = [{ resource: "customers", action: "editar", country: "gt" }];
    const onClose = vi.fn();

    render(
      <DetailDialog open kind="customer" id="cust-1" onClose={onClose} />,
    );

    await user.click(screen.getByRole("button", { name: "Editar cliente" }));

    expect(onClose).toHaveBeenCalledTimes(1);
    expect(push).toHaveBeenCalledTimes(1);
    const target = push.mock.calls[0][0] as string;
    expect(target).toContain("/customers/edit?");
    expect(target).toContain("email=a%40b.com");
    expect(target).toContain("external_id=ext-9");
    expect(target).toContain("from=%2Fsubscriptions");
  });

  it("sin email ni external_id no muestra 'Editar cliente'", () => {
    mockCustomer({ first_name: "Ana" });
    perms = [{ resource: "customers", action: "editar", country: "gt" }];

    render(<DetailDialog open kind="customer" id="cust-1" onClose={vi.fn()} />);

    expect(
      screen.queryByRole("button", { name: "Editar cliente" }),
    ).not.toBeInTheDocument();
  });

  it("sin permiso no muestra 'Editar cliente'", () => {
    mockCustomer({ email: "a@b.com" });
    perms = [];

    render(<DetailDialog open kind="customer" id="cust-1" onClose={vi.fn()} />);

    expect(
      screen.queryByRole("button", { name: "Editar cliente" }),
    ).not.toBeInTheDocument();
  });
});
