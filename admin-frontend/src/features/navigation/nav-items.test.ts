// Pruebas puras de `selectVisibleNavEntries` (HOME dinámico): el HOME deriva sus
// tarjetas de la MISMA lógica de permisos que el sidebar, así que aquí se valida
// que un recurso fijo con permiso `ver` aparece, un módulo dinámico visible
// aparece enlazado a /modules/<slug>, y que los que NO tienen `ver` se ocultan
// (Req. 9.2, 9.3). El país fija la visibilidad de los recursos fijos.

import { describe, expect, it } from "vitest";

import type {
  EffectivePermissions,
  ModulePermissions,
} from "@/lib/permissions";
import type { ModuleSummary } from "@/features/modules/types";
import {
  selectVisibleNavEntries,
  type NavEntry,
} from "@/features/navigation/nav-items";

/** Permiso `ver` sobre un recurso fijo en el país indicado. */
function verFijo(resource: string, country: string): EffectivePermissions[number] {
  return {
    resource: resource as EffectivePermissions[number]["resource"],
    action: "ver",
    country,
  };
}

/** Permiso `ver` sobre un slug de módulo (o slug reservado) en un país. */
function verModulo(slug: string, country: string): ModulePermissions[number] {
  return { slug, action: "ver", country };
}

/** Localiza una tarjeta por su href. */
function byHref(entries: readonly NavEntry[], href: string): NavEntry | undefined {
  return entries.find((e) => e.href === href);
}

describe("selectVisibleNavEntries", () => {
  it("incluye un recurso fijo con permiso `ver` en el país activo", () => {
    const entries = selectVisibleNavEntries({
      perms: [verFijo("customers", "gt")],
      modperms: [],
      country: "gt",
      modules: [],
    });
    expect(byHref(entries, "/customers")).toMatchObject({ label: "Customers" });
  });

  it("oculta un recurso fijo sin permiso `ver`", () => {
    const entries = selectVisibleNavEntries({
      perms: [verFijo("customers", "gt")],
      modperms: [],
      country: "gt",
      modules: [],
    });
    // `plans` no tiene permiso: no debe aparecer.
    expect(byHref(entries, "/plans")).toBeUndefined();
  });

  it("oculta un recurso fijo cuyo permiso es de OTRO país", () => {
    const entries = selectVisibleNavEntries({
      perms: [verFijo("customers", "sv")],
      modperms: [],
      country: "gt",
      modules: [],
    });
    expect(byHref(entries, "/customers")).toBeUndefined();
  });

  it("incluye un módulo dinámico visible enlazado a /modules/<slug>", () => {
    const modules: ModuleSummary[] = [
      { slug: "recaudo-gt", name: "Cuentas de recaudo GT", menuPath: "CENAM/Recaudo/GT" },
    ];
    const entries = selectVisibleNavEntries({
      perms: [],
      modperms: [verModulo("recaudo-gt", "gt")],
      country: "gt",
      modules,
    });
    expect(byHref(entries, "/modules/recaudo-gt")).toMatchObject({
      label: "Cuentas de recaudo GT",
    });
  });

  it("oculta un módulo dinámico sin permiso `ver` en el país activo", () => {
    const modules: ModuleSummary[] = [
      { slug: "recaudo-gt", name: "Cuentas de recaudo GT" },
    ];
    const entries = selectVisibleNavEntries({
      perms: [],
      modperms: [], // sin canModule(..., "ver", "gt")
      country: "gt",
      modules,
    });
    expect(byHref(entries, "/modules/recaudo-gt")).toBeUndefined();
  });

  it("incluye las entradas reservadas (Constructor y Conexiones) solo con su permiso", () => {
    const entries = selectVisibleNavEntries({
      perms: [],
      modperms: [verModulo("__defs__", "gt"), verModulo("__connections__", "gt")],
      country: "gt",
      modules: [],
    });
    expect(byHref(entries, "/module-defs")).toMatchObject({ label: "Módulos" });
    expect(byHref(entries, "/connection-templates")).toMatchObject({
      label: "Conexiones",
    });
  });

  it("devuelve vacío cuando el usuario no puede ver nada (estado vacío del HOME)", () => {
    const entries = selectVisibleNavEntries({
      perms: [],
      modperms: [],
      country: "gt",
      modules: [{ slug: "x", name: "X" }],
    });
    expect(entries).toHaveLength(0);
  });

  it("ordena: recursos fijos, luego reservadas, luego módulos por nombre", () => {
    const modules: ModuleSummary[] = [
      { slug: "b", name: "Beta" },
      { slug: "a", name: "Alfa" },
    ];
    const entries = selectVisibleNavEntries({
      perms: [verFijo("customers", "gt")],
      modperms: [
        verModulo("__defs__", "gt"),
        verModulo("a", "gt"),
        verModulo("b", "gt"),
      ],
      country: "gt",
      modules,
    });
    expect(entries.map((e) => e.href)).toEqual([
      "/customers",
      "/module-defs",
      "/modules/a",
      "/modules/b",
    ]);
  });
});
