"use client";

/**
 * Vista protegida de un módulo dinámico del constructor (module-builder). Toma
 * el `slug` de la ruta y delega el render al componente `ModuleView`, que carga
 * la Definición y materializa tabla, botones y formularios de forma dinámica.
 *
 * La visibilidad del módulo en el menú y la autorización de sus acciones las
 * gobierna el permiso `mod:<slug>` (canModule / backend); esta página solo
 * monta la vista.
 */
import * as React from "react";
import { useParams } from "next/navigation";

import { ModuleView } from "@/features/modules/components/module-view";

export default function ModulePage() {
  const params = useParams<{ slug: string }>();
  const slug = typeof params.slug === "string" ? params.slug : "";

  if (!slug) {
    return null;
  }

  return (
    <div className="mx-auto w-full max-w-6xl">
      <ModuleView slug={slug} />
    </div>
  );
}
