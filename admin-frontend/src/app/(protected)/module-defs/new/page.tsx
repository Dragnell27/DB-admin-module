"use client";

/**
 * Página de creación de un módulo nuevo (module-builder): monta el asistente
 * paso a paso en modo creación.
 */
import * as React from "react";

import { ModuleWizard } from "@/features/modules/components/module-wizard";

export default function NewModulePage() {
  return (
    // Contenedor a altura completa: el asistente es un layout de tres zonas y
    // necesita llenar el alto del contenedor. El título vive ahora en la
    // CABECERA del asistente (prop `title`). El tope de ancho es generoso
    // (`max-w-6xl`, ~1152px) para aprovechar el ancho disponible sin que en
    // monitores ultra anchos las líneas se vuelvan absurdamente largas.
    <div className="mx-auto flex h-full min-h-0 w-full max-w-6xl flex-col">
      <ModuleWizard title="Nuevo módulo" />
    </div>
  );
}
