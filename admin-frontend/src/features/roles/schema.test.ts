// Pruebas del `permissionSchema` de Roles (FEAT-002): acepta recursos del
// catálogo fijo y recursos de módulo `mod:<slug>` con acciones de texto libre
// (incluidas `custom:<x>`), y rechaza slugs mal formados y recursos vacíos.

import { describe, expect, it } from "vitest";

import { permissionSchema } from "@/features/roles/schema";

describe("permissionSchema (recursos de módulo mod:<slug>)", () => {
  it("acepta un permiso de módulo con acción custom", () => {
    const result = permissionSchema.safeParse({
      resource: "mod:afiliaciones",
      action: "custom:activar",
    });
    expect(result.success).toBe(true);
  });

  it("acepta un permiso fijo del catálogo cerrado", () => {
    const result = permissionSchema.safeParse({
      resource: "customers",
      action: "ver",
    });
    expect(result.success).toBe(true);
  });

  it("rechaza un slug de módulo mal formado (mayúsculas/guion bajo)", () => {
    const result = permissionSchema.safeParse({
      resource: "mod:Mal_Slug",
      action: "ver",
    });
    expect(result.success).toBe(false);
  });

  it("rechaza un recurso vacío", () => {
    const result = permissionSchema.safeParse({
      resource: "",
      action: "ver",
    });
    expect(result.success).toBe(false);
  });
});

describe("permissionSchema (recurso fijo module-builder)", () => {
  // El Constructor de módulos es un recurso del catálogo cerrado que el backend
  // acepta vía auth.ValidateRolePermission con acciones ver/crear/editar/eliminar.
  // El schema del cliente debe aceptarlo (antes lo rechazaba por no estar en
  // RESOURCE_VALUES ni casar el regex mod:<slug>).
  it.each(["ver", "crear", "editar", "eliminar"])(
    "acepta module-builder con acción %s",
    (action) => {
      const result = permissionSchema.safeParse({
        resource: "module-builder",
        action,
      });
      expect(result.success).toBe(true);
    },
  );
});

describe("permissionSchema (recurso fijo connections)", () => {
  // Plantillas de conexión: recurso del catálogo cerrado que el backend acepta
  // con acciones ver/crear/editar/eliminar; el schema del cliente debe aceptarlo.
  it.each(["ver", "crear", "editar", "eliminar"])(
    "acepta connections con acción %s",
    (action) => {
      const result = permissionSchema.safeParse({
        resource: "connections",
        action,
      });
      expect(result.success).toBe(true);
    },
  );
});
