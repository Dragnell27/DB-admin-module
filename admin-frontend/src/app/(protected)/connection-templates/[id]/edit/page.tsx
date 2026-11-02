"use client";

/**
 * Editar una plantilla de conexión (connection-templates). Carga la ESTRUCTURA
 * (claves/marcas/roles) sin valores fijos (el backend nunca los devuelve): los
 * inputs de valores fijos llegan vacíos y dejarlos vacíos conserva el previo.
 */
import { useParams } from "next/navigation";

import { FeedbackBanner } from "@/components/feedback-banner";
import { useConnectionTemplate } from "@/features/connections/hooks";
import {
  TemplateEditor,
  type EditorState,
} from "@/features/connections/components/template-editor";
import type { ConnectionTemplate } from "@/features/connections/types";

/** Convierte la plantilla cargada al estado del editor (sin valores fijos). */
function toEditorState(t: ConnectionTemplate): EditorState {
  return {
    name: t.name,
    fields: t.fields.map((f) => ({
      key: f.key,
      kind: f.kind,
      role: f.role ?? "",
      required: f.required ?? true,
      suggested: f.suggested ?? "",
      fixedValue: "", // nunca llega del backend; vacío = conservar
    })),
  };
}

export default function EditConnectionTemplatePage() {
  const params = useParams<{ id: string }>();
  const id = params.id;
  const { data: template, isLoading, isError, error } = useConnectionTemplate(id);

  return (
    // Contenedor a altura completa: el editor es un layout de tres zonas y
    // necesita llenar el alto. El título vive ahora en la CABECERA del editor
    // (prop `title`); los estados de carga/error se conservan. El tope de ancho
    // es generoso (`max-w-6xl`, ~1152px) para aprovechar el ancho disponible sin
    // líneas absurdamente largas en monitores ultra anchos.
    <div className="mx-auto flex h-full min-h-0 w-full max-w-6xl flex-col">
      {isError ? (
        <FeedbackBanner
          tone="error"
          message={error?.message ?? "No se pudo cargar la plantilla."}
        />
      ) : null}

      {isLoading ? (
        <p className="text-sm text-muted-foreground">Cargando...</p>
      ) : template ? (
        <TemplateEditor
          editId={id}
          initial={toEditorState(template)}
          usedByModules={template.usedByModules}
          title="Editar plantilla de conexión"
        />
      ) : null}
    </div>
  );
}
