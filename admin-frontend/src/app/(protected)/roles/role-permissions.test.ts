// Pruebas de los helpers puros de la matriz de permisos (FEAT-002): round-trip
// de claves `mod:<slug>::<acción>` dado un catálogo, preservación de claves
// colgantes (slug fuera del catálogo) y continuidad de los permisos fijos.

import { describe, expect, it } from "vitest";

import {
  cellKey,
  selectedFromPermissions,
  selectedToPermissions,
  type ModuleCatalogEntry,
} from "@/app/(protected)/roles/role-permissions";

const CATALOG: ReadonlyArray<ModuleCatalogEntry> = [
  { slug: "afiliaciones", actions: ["ver", "custom:activar"] },
];

describe("selectedFromPermissions / selectedToPermissions", () => {
  it("round-trip de una clave de módulo conocida", () => {
    const perms = [{ resource: "mod:afiliaciones", action: "custom:activar" }];
    const selected = selectedFromPermissions(perms, CATALOG);

    expect(selected.has(cellKey("mod:afiliaciones", "custom:activar"))).toBe(true);

    const back = selectedToPermissions(selected, CATALOG);
    expect(back).toContainEqual({
      resource: "mod:afiliaciones",
      action: "custom:activar",
    });
  });

  it("preserva una acción de módulo conocido NO declarada (colgante)", () => {
    const perms = [{ resource: "mod:afiliaciones", action: "custom:no-existe" }];
    const selected = selectedFromPermissions(perms, CATALOG);

    // El módulo existe pero no declara la acción: se preserva (R3.4,
    // acción-no-declarada) para que la vista la muestre des-marcable, en vez de
    // descartarla en silencio.
    expect(selected.has(cellKey("mod:afiliaciones", "custom:no-existe"))).toBe(
      true,
    );

    const back = selectedToPermissions(selected, CATALOG);
    expect(back).toContainEqual({
      resource: "mod:afiliaciones",
      action: "custom:no-existe",
    });
  });

  it("preserva una clave colgante (slug fuera del catálogo)", () => {
    const perms = [{ resource: "mod:fantasma", action: "ver" }];
    const selected = selectedFromPermissions(perms, CATALOG);

    expect(selected.has(cellKey("mod:fantasma", "ver"))).toBe(true);

    const back = selectedToPermissions(selected, CATALOG);
    expect(back).toContainEqual({ resource: "mod:fantasma", action: "ver" });
  });

  it("mantiene los permisos fijos del catálogo cerrado", () => {
    const perms = [{ resource: "customers", action: "ver" }];
    const selected = selectedFromPermissions(perms, CATALOG);

    expect(selected.has(cellKey("customers", "ver"))).toBe(true);

    const back = selectedToPermissions(selected, CATALOG);
    expect(back).toContainEqual({ resource: "customers", action: "ver" });
  });

  it("ignora un permiso fijo con acción inválida para el recurso", () => {
    const perms = [{ resource: "cards", action: "editar" }];
    const selected = selectedFromPermissions(perms, CATALOG);
    expect(selected.size).toBe(0);
  });

  it("round-trip del recurso fijo connections (ver/crear/editar/eliminar)", () => {
    const perms = [
      { resource: "connections", action: "ver" },
      { resource: "connections", action: "crear" },
      { resource: "connections", action: "editar" },
      { resource: "connections", action: "eliminar" },
    ];
    const selected = selectedFromPermissions(perms, CATALOG);

    for (const action of ["ver", "crear", "editar", "eliminar"]) {
      expect(selected.has(cellKey("connections", action))).toBe(true);
    }

    const back = selectedToPermissions(selected, CATALOG);
    for (const action of ["ver", "crear", "editar", "eliminar"]) {
      expect(back).toContainEqual({ resource: "connections", action });
    }
  });

  it("round-trip del recurso fijo module-builder (ver/crear/editar/eliminar)", () => {
    const perms = [
      { resource: "module-builder", action: "ver" },
      { resource: "module-builder", action: "crear" },
      { resource: "module-builder", action: "editar" },
      { resource: "module-builder", action: "eliminar" },
    ];
    const selected = selectedFromPermissions(perms, CATALOG);

    for (const action of ["ver", "crear", "editar", "eliminar"]) {
      expect(selected.has(cellKey("module-builder", action))).toBe(true);
    }

    const back = selectedToPermissions(selected, CATALOG);
    for (const action of ["ver", "crear", "editar", "eliminar"]) {
      expect(back).toContainEqual({ resource: "module-builder", action });
    }
  });
});
