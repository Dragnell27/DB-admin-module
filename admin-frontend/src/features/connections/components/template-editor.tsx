"use client";

/**
 * Editor de una plantilla de conexión (connection-templates). Permite:
 *  - nombrar la plantilla,
 *  - añadir campos con clave libre, marcarlos FIJO/VARIABLE y asignar un `role`
 *    del conjunto cerrado (+ option),
 *  - capturar el VALOR de los campos FIJOS (se cifra server-side; al editar los
 *    inputs llegan vacíos y dejar vacío conserva el valor previo),
 *  - para los VARIABLE: marcar required y un valor sugerido NO sensible.
 *
 * Antes de guardar cambios en una plantilla usada por módulos, avisa el radio de
 * impacto con `usedByModules` (D-4). La seguridad real la aplica el backend.
 */
import * as React from "react";
import { useRouter } from "next/navigation";
import { Loader2, Plus, Trash2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { CollapsibleRow } from "@/components/ui/collapsible-row";
import { FeedbackBanner } from "@/components/feedback-banner";
import { FormActionBar } from "@/components/form-action-bar";
import { toast } from "@/lib/use-toast";
import type { ApiError } from "@/lib/api-client";
import {
  useCreateConnectionTemplate,
  useUpdateConnectionTemplate,
} from "@/features/connections/hooks";
import { connectionTemplateSchema } from "@/features/connections/schema";
import {
  FIELD_ROLES,
  type FieldKind,
  type FieldRole,
  type TemplateField,
} from "@/features/connections/types";

const inputClass =
  "h-10 w-full rounded-md border border-input-border bg-[var(--input-surface,var(--input))] px-3 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";

/** Estado de un campo en el editor: estructura + valor FIJO en claro. */
export interface FieldRow {
  key: string;
  kind: FieldKind;
  role: FieldRole;
  required: boolean;
  suggested: string;
  /** Valor FIJO en claro (solo relevante si kind === "fixed"). */
  fixedValue: string;
}

/** Estado completo del editor. */
export interface EditorState {
  name: string;
  fields: FieldRow[];
}

/** Estado vacío (creación). */
export function emptyEditorState(): EditorState {
  return { name: "", fields: [] };
}

/**
 * Resumen legible de un campo cuando está COLAPSADO: «<key> · <variable|fijo>»
 * y, si tiene rol asignado, « · <role>». Función pura (unit-testeable). Solo se
 * usa para presentación; no toca el modelo ni la serialización.
 */
export function fieldRowSummary(f: FieldRow): string {
  const kindLabel = f.kind === "fixed" ? "fijo" : "variable";
  const parts = [f.key, kindLabel];
  if (f.role) {
    parts.push(f.role);
  }
  return parts.join(" · ");
}

export interface TemplateEditorProps {
  /** Estado inicial (edición); si se omite, es creación. */
  readonly initial?: EditorState;
  /** Id a editar; si se provee, el editor usa PUT en lugar de POST. */
  readonly editId?: string;
  /** Módulos que usan la plantilla (radio de impacto, D-4). */
  readonly usedByModules?: number;
  /** Título mostrado en la CABECERA del layout de tres zonas (lo pasa la página). */
  readonly title?: string;
}

export function TemplateEditor({ initial, editId, usedByModules = 0, title }: TemplateEditorProps) {
  const router = useRouter();
  const isEdit = Boolean(editId);
  const [state, setState] = React.useState<EditorState>(initial ?? emptyEditorState());
  const [formError, setFormError] = React.useState<string | null>(null);

  const createMut = useCreateConnectionTemplate();
  const updateMut = useUpdateConnectionTemplate(editId ?? "");
  const mut = isEdit ? updateMut : createMut;

  const patchField = React.useCallback((i: number, p: Partial<FieldRow>) => {
    setState((prev) => {
      const fields = prev.fields.slice();
      fields[i] = { ...fields[i], ...p };
      return { ...prev, fields };
    });
  }, []);

  const addField = React.useCallback(() => {
    setState((prev) => ({
      ...prev,
      fields: [
        ...prev.fields,
        { key: "", kind: "variable", role: "", required: true, suggested: "", fixedValue: "" },
      ],
    }));
  }, []);

  const removeField = React.useCallback((i: number) => {
    setState((prev) => ({ ...prev, fields: prev.fields.filter((_, idx) => idx !== i) }));
  }, []);

  const handleSave = React.useCallback(() => {
    setFormError(null);
    const fields: TemplateField[] = state.fields.map((f) => ({
      key: f.key.trim(),
      kind: f.kind,
      role: f.role || undefined,
      required: f.kind === "variable" ? f.required : undefined,
      suggested: f.kind === "variable" && f.suggested !== "" ? f.suggested : undefined,
    }));
    const fixedValues: Record<string, string> = {};
    for (const f of state.fields) {
      if (f.kind === "fixed" && f.fixedValue.trim() !== "") {
        fixedValues[f.key.trim()] = f.fixedValue;
      }
    }

    const parsed = connectionTemplateSchema(!isEdit).safeParse({
      name: state.name,
      fields,
      fixedValues,
    });
    if (!parsed.success) {
      setFormError(parsed.error.issues[0]?.message ?? "La plantilla es inválida");
      return;
    }

    mut.mutate(
      { name: state.name.trim(), fields, fixedValues },
      {
        onSuccess: () => router.push("/connection-templates?saved=1"),
        // El error de guardar se muestra como TOAST flotante (siempre visible
        // sin importar el scroll del editor), no como banner inline.
        onError: (err) =>
          toast.error(
            (err as ApiError | undefined)?.message ??
              "No se pudo guardar la plantilla.",
          ),
      },
    );
  }, [state, isEdit, mut, router]);

  return (
    // Layout de TRES ZONAS a altura completa (cabecera / cuerpo con scroll /
    // pie), anclado POR ESTRUCTURA (flex column) y no por sticky/fixed: así la
    // cabecera y el pie nunca se desplazan respecto al nav lateral de la app y
    // el scroll vive SOLO en el cuerpo.
    <div className="flex h-full min-h-0 flex-col">
      {/* CABECERA no desplazable: título del editor (lo pasa la página). */}
      {title ? (
        <div className="shrink-0">
          <h1 className="mb-3 text-2xl font-semibold text-foreground">{title}</h1>
        </div>
      ) : null}

      {/* CUERPO: ÚNICA zona con scroll (`flex-1 min-h-0 overflow-y-auto`).
          Contiene los banners, el nombre y la lista de campos. `pb-6` da aire al
          final; ya NO se usa `pb-20` porque el pie es un bloque estructural. */}
      <div className="min-h-0 flex-1 space-y-6 overflow-y-auto pb-6">
        {/* `formError` (validación de esquema previa al envío) y el aviso de
            `usedByModules` (advertencia contextual mientras se edita) se quedan
            INLINE. El error de GUARDAR, en cambio, es ahora un toast flotante. */}
        {formError ? <FeedbackBanner tone="error" message={formError} /> : null}
        {isEdit && usedByModules > 0 ? (
          <FeedbackBanner
            tone="warning"
            message={`${usedByModules} módulos usan esta plantilla: cambiar sus campos fijos afectará a todos.`}
          />
        ) : null}

        <div className="space-y-1.5">
          <Label htmlFor="t_name">Nombre de la plantilla</Label>
          <Input
            id="t_name"
            value={state.name}
            onChange={(e) => setState((prev) => ({ ...prev, name: e.target.value }))}
            placeholder="conexion-recaudo"
          />
        </div>

        <div className="space-y-3">
        <div className="flex items-center justify-between">
          <h2 className="text-sm font-medium text-foreground">Campos</h2>
          <Button type="button" variant="outline" size="sm" onClick={addField}>
            <Plus aria-hidden="true" />
            Añadir campo
          </Button>
        </div>

        {state.fields.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            Aún no hay campos. Añade host, puerto, usuario, password, nombreBD…
          </p>
        ) : null}

        {state.fields.map((f, i) => (
          <FieldRowEditor
            key={i}
            index={i}
            field={f}
            isEdit={isEdit}
            onPatch={(p) => patchField(i, p)}
            onRemove={() => removeField(i)}
          />
        ))}
        </div>
      </div>

      {/* PIE no desplazable: acciones SIEMPRE visibles (mismo FormActionBar que
          el asistente de módulos, por consistencia) en modo estructural
          `sticky={false}`, último hijo flex del layout de tres zonas. Mismos
          handlers, labels, spinner y estado `disabled`; el payload/validación no
          cambian. */}
      <FormActionBar sticky={false}>
        <Button type="button" variant="outline" onClick={() => router.push("/connection-templates")}>
          Cancelar
        </Button>
        <Button type="button" onClick={handleSave} disabled={mut.isPending}>
          {mut.isPending ? (
            <>
              <Loader2 className="size-4 animate-spin" aria-hidden="true" />
              Guardando...
            </>
          ) : isEdit ? (
            "Guardar cambios"
          ) : (
            "Crear plantilla"
          )}
        </Button>
      </FormActionBar>
    </div>
  );
}

/**
 * Fila COLAPSABLE de un campo de la plantilla. Un campo con `key` NO vacía se
 * muestra colapsado con un resumen («<key> · <variable|fijo>[ · <role>]») y se
 * expande para editar sus controles; un campo NUEVO (key vacía) arranca
 * EXPANDIDO para capturarlo de inmediato. El estado de colapso es LOCAL (no se
 * persiste): se inicializa según la key y luego lo gobierna el usuario. El botón
 * de eliminar vive en el encabezado de la fila (fuera del botón de toggle). No
 * cambia el modelo de datos ni la serialización: es solo presentación.
 */
function FieldRowEditor({
  index,
  field: f,
  isEdit,
  onPatch,
  onRemove,
}: {
  readonly index: number;
  readonly field: FieldRow;
  readonly isEdit: boolean;
  readonly onPatch: (p: Partial<FieldRow>) => void;
  readonly onRemove: () => void;
}) {
  const i = index;
  // Nuevo (key vacía) ⇒ abierto; existente ⇒ colapsado.
  const [open, setOpen] = React.useState(f.key.trim() === "");
  const panelId = `f_panel_${i}`;
  const titled = f.key.trim() !== "";

  return (
    <CollapsibleRow
      panelId={panelId}
      open={open}
      onToggle={() => setOpen((v) => !v)}
      header={
        <span className="truncate text-sm font-medium text-foreground">
          {titled ? fieldRowSummary(f) : "Campo nuevo"}
        </span>
      }
      actions={
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={onRemove}
          aria-label="Eliminar campo"
        >
          <Trash2 aria-hidden="true" />
        </Button>
      }
      panelClassName="grid gap-3 sm:grid-cols-2 lg:grid-cols-3"
    >
      <div className="space-y-1.5">
        <Label htmlFor={`f_key_${i}`}>Clave</Label>
        <Input
          id={`f_key_${i}`}
          value={f.key}
          onChange={(e) => onPatch({ key: e.target.value })}
          placeholder="host"
        />
      </div>
      <div className="space-y-1.5">
        <Label htmlFor={`f_kind_${i}`}>Tipo</Label>
        <select
          id={`f_kind_${i}`}
          className={inputClass}
          value={f.kind}
          onChange={(e) => onPatch({ kind: e.target.value as FieldKind })}
        >
          <option value="fixed">Fijo</option>
          <option value="variable">Variable</option>
        </select>
      </div>
      <div className="space-y-1.5">
        <Label htmlFor={`f_role_${i}`}>Rol en el DSN</Label>
        <select
          id={`f_role_${i}`}
          className={inputClass}
          value={f.role}
          onChange={(e) => onPatch({ role: e.target.value as FieldRole })}
        >
          <option value="">(automático / option)</option>
          {FIELD_ROLES.map((r) => (
            <option key={r} value={r}>
              {r}
            </option>
          ))}
        </select>
      </div>

      {f.kind === "fixed" ? (
        <div className="space-y-1.5">
          <Label htmlFor={`f_fixed_${i}`}>
            Valor {isEdit ? "(vacío = sin cambios)" : ""}
          </Label>
          <Input
            id={`f_fixed_${i}`}
            type="password"
            value={f.fixedValue}
            onChange={(e) => onPatch({ fixedValue: e.target.value })}
            autoComplete="off"
            placeholder={isEdit ? "••••••" : "valor"}
          />
        </div>
      ) : (
        <div className="space-y-1.5">
          <Label htmlFor={`f_suggested_${i}`}>Valor sugerido (opcional)</Label>
          <Input
            id={`f_suggested_${i}`}
            value={f.suggested}
            onChange={(e) => onPatch({ suggested: e.target.value })}
            placeholder="(prellenado editable)"
          />
        </div>
      )}

      <div className="flex items-center justify-between sm:col-span-2 lg:col-span-3">
        {f.kind === "variable" ? (
          <label className="flex items-center gap-2 text-sm text-foreground">
            <input
              type="checkbox"
              checked={f.required}
              onChange={(e) => onPatch({ required: e.target.checked })}
            />
            Obligatorio al crear el módulo
          </label>
        ) : (
          <span className="text-xs text-muted-foreground">
            Los campos fijos son siempre obligatorios y se cifran.
          </span>
        )}
      </div>
    </CollapsibleRow>
  );
}
