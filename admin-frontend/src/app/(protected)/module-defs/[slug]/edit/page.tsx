"use client";

/**
 * Página de edición de un módulo (module-builder): carga la Definición por slug,
 * la convierte al estado del asistente y lo monta en modo edición (PUT).
 */
import * as React from "react";
import { useParams } from "next/navigation";
import { Loader2 } from "lucide-react";

import { FeedbackBanner } from "@/components/feedback-banner";
import { ModuleWizard } from "@/features/modules/components/module-wizard";
import { useModuleDef } from "@/features/modules/hooks";
import { fromDefinition } from "@/features/modules/schema";

export default function EditModulePage() {
  const params = useParams<{ slug: string }>();
  const slug = typeof params.slug === "string" ? params.slug : "";
  const { data: def, isLoading, isError, error } = useModuleDef(slug);

  return (
    // Contenedor a altura completa: el asistente es un layout de tres zonas y
    // necesita llenar el alto. El título dinámico vive ahora en la CABECERA del
    // asistente (prop `title`); los estados de carga/error se conservan. El tope
    // de ancho es generoso (`max-w-6xl`, ~1152px) para aprovechar el ancho
    // disponible sin líneas absurdamente largas en monitores ultra anchos.
    <div className="mx-auto flex h-full min-h-0 w-full max-w-6xl flex-col">
      {isLoading ? (
        <div className="flex items-center gap-2 text-muted-foreground">
          <Loader2 className="size-4 animate-spin" aria-hidden="true" />
          Cargando módulo...
        </div>
      ) : isError || !def ? (
        <FeedbackBanner tone="error" message={error?.message ?? "No se pudo cargar el módulo."} />
      ) : (
        <ModuleWizard
          editSlug={slug}
          initial={fromDefinition(def)}
          title={def?.name ? `Editar módulo · ${def.name}` : "Editar módulo"}
        />
      )}
    </div>
  );
}
