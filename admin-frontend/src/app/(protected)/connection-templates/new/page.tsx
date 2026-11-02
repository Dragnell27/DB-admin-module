"use client";

/** Crear una plantilla de conexión nueva (connection-templates). */
import { TemplateEditor } from "@/features/connections/components/template-editor";

export default function NewConnectionTemplatePage() {
  return (
    // Contenedor a altura completa: el editor es un layout de tres zonas y
    // necesita llenar el alto del contenedor. El título vive ahora en la
    // CABECERA del editor (prop `title`). El tope de ancho es generoso
    // (`max-w-6xl`, ~1152px) para aprovechar el ancho disponible sin líneas
    // absurdamente largas en monitores ultra anchos.
    <div className="mx-auto flex h-full min-h-0 w-full max-w-6xl flex-col">
      <TemplateEditor title="Nueva plantilla de conexión" />
    </div>
  );
}
