"use client";

/**
 * Asistente de creación/edición de un módulo del constructor (module-builder,
 * tareas 13.1, 13.2). Rediseño de UX (navegación por COLAPSO): en vez de una
 * BARRA DE PESTAÑAS horizontal que muestra UNA sección a la vez, las cuatro
 * secciones se apilan como un ACORDEÓN (todas presentes, cada una colapsable).
 * Cada encabezado muestra el estado de validez (icono completa/pendiente) a la
 * izquierda y un RESUMEN legible a la derecha cuando está cerrada (conteos
 * derivados del WizardState). El acordeón es NO EXCLUSIVO (varias secciones
 * abiertas a la vez): en formularios largos conviene poder comparar secciones y
 * no perder el foco al abrir otra. El estado de apertura se recuerda dentro de
 * la sesión de edición. La BARRA DE ACCIONES (Cancelar / Guardar) se mantiene
 * ARRIBA, sobre el acordeón, siempre visible.
 *
 * Secciones:
 *   1. Origen y conexión (slug kebab-case + nombre + plantilla + variables).
 *   2. Tablas y relaciones (tabla principal + foráneas + FKs).
 *   3. Columnas (campo, etiqueta, orden, formato; visible/PK/editable).
 *   4. Acciones (CRUD/personalizadas con su operación SQL).
 *
 * Al guardar valida TODAS las secciones y, si alguna falla, AUTO-EXPANDE la
 * PRIMERA inválida y dice CUÁL corregir (sin cambiar las reglas de validez por
 * sección). El pago útil (`toDefinitionInput`) y las reglas de validez son
 * equivalentes en comportamiento a la versión anterior. Un slug duplicado se
 * muestra como toast (E409). Tema claro/oscuro, con primitivas del proyecto. La
 * seguridad real la aplica el backend.
 */
import * as React from "react";
import { useRouter } from "next/navigation";
import { CheckCircle2, Circle, Loader2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import { CollapsibleRow } from "@/components/ui/collapsible-row";
import { FeedbackBanner } from "@/components/feedback-banner";
import { toast } from "@/lib/use-toast";
import { usePageTitle } from "@/lib/page-chrome";
import type { ApiError } from "@/lib/api-client";
import {
  useCreateModuleDef,
  useUpdateModuleDef,
} from "@/features/modules/hooks";
import { useConnectionTemplates } from "@/features/connections/hooks";
import {
  actionIsComplete,
  dynamicFieldError,
  dynamicSourceError,
  emptyWizardState,
  fromCopyableDefinition,
  hasAtLeastOneColumn,
  tablesMissingPrimaryKey,
  moduleBaseSchema,
  selectFieldError,
  selectOptionsError,
  toDefinitionInput,
  type WizardState,
} from "@/features/modules/schema";
import {
  ActionsSection,
  ColumnsSection,
  CopyConfigPicker,
  OriginSection,
  TablesSection,
} from "@/features/modules/components/wizard-sections";

/** Identificadores estables de cada sección del asistente. */
type SectionId = "origin" | "tables" | "columns" | "actions";

interface SectionMeta {
  readonly id: SectionId;
  readonly title: string;
  readonly description: string;
}

/** Orden y metadatos de las secciones del acordeón. */
const SECTIONS: readonly SectionMeta[] = [
  { id: "origin", title: "Origen y conexión", description: "Nombre, identificador y conexión del módulo." },
  { id: "tables", title: "Tablas y relaciones", description: "Tabla principal, foráneas y claves foráneas." },
  { id: "columns", title: "Columnas", description: "Etiquetas, formato y visibilidad de cada columna." },
  { id: "actions", title: "Acciones", description: "Botones CRUD y acciones personalizadas." },
];

/** Resultado de validar una sección: válida (ok) o con el primer error. */
interface SectionCheck {
  readonly ok: boolean;
  readonly error: string | null;
}

/**
 * Deriva el mensaje de error de GUARDAR a partir del error de la mutación.
 * Función PURA (sin React ni DOM) para poder unit-testear ambas ramas:
 *   - `E409` (slug duplicado) → mensaje específico en español.
 *   - cualquier otro → el mensaje del backend o el genérico de respaldo.
 */
export function deriveSaveErrorMessage(err: ApiError | undefined): string {
  const code = (err as { errorCode?: string } | undefined)?.errorCode;
  if (code === "E409") {
    return "El identificador del módulo ya está en uso. Elige otro slug.";
  }
  return err?.message ?? "No se pudo guardar el módulo.";
}

/**
 * Resumen legible de una sección para mostrar en su encabezado cuando interesa
 * el CONTENIDO (tablas/columnas/acciones). Función pura sobre el WizardState:
 *   - tables   → «N tablas · N relaciones».
 *   - columns  → «N columnas».
 *   - actions  → «N acciones».
 * La sección «origin» usa el estado de validez (completo/incompleto), no un
 * conteo, así que no tiene resumen de conteo aquí.
 */
export function sectionSummary(id: SectionId, state: WizardState): string | null {
  if (id === "tables") {
    const tableCount = (state.pgTable.trim() !== "" ? 1 : 0) + state.foreignTables.filter((t) => t.trim() !== "").length;
    const relCount = state.relationships.length;
    const tablesLabel = `${tableCount} ${tableCount === 1 ? "tabla" : "tablas"}`;
    const relLabel = `${relCount} ${relCount === 1 ? "relación" : "relaciones"}`;
    return `${tablesLabel} · ${relLabel}`;
  }
  if (id === "columns") {
    const n = state.columns.length;
    return `${n} ${n === 1 ? "columna" : "columnas"}`;
  }
  if (id === "actions") {
    const n = state.actions.length;
    return `${n} ${n === 1 ? "acción" : "acciones"}`;
  }
  return null;
}

export interface ModuleWizardProps {
  /** Estado inicial (edición); si se omite, es creación. */
  readonly initial?: WizardState;
  /** Slug a editar; si se provee, el asistente usa PUT en lugar de POST. */
  readonly editSlug?: string;
  /** Título mostrado en la CABECERA del layout de tres zonas (lo pasa la página). */
  readonly title?: string;
}

export function ModuleWizard({ initial, editSlug, title }: ModuleWizardProps) {
  const router = useRouter();
  // El título de la vista se declara en la TOPBAR (bloque «título en header»):
  // la topbar renderiza el único <h1>; el asistente ya no lo pinta en el cuerpo.
  usePageTitle(title ?? null);
  const [state, setState] = React.useState<WizardState>(
    initial ?? emptyWizardState(),
  );
  // Estado de apertura por sección (acordeón NO exclusivo: varias abiertas a la
  // vez). Al montar (crear o editar) se abre la primera sección, «Origen y
  // conexión»; el resto arranca cerrado. Se RECUERDA durante la sesión de
  // edición (vive mientras el componente esté montado).
  const [openSections, setOpenSections] = React.useState<Record<SectionId, boolean>>({
    origin: true,
    tables: false,
    columns: false,
    actions: false,
  });
  const [saveAttempted, setSaveAttempted] = React.useState(false);

  const createMut = useCreateModuleDef();
  const updateMut = useUpdateModuleDef(editSlug ?? "");
  const isEdit = Boolean(editSlug);
  const mut = isEdit ? updateMut : createMut;

  const { data: templates } = useConnectionTemplates();
  const selectedTemplate = (templates ?? []).find(
    (t) => t.id === state.connectionTemplateId,
  );

  const patch = React.useCallback((p: Partial<WizardState>) => {
    setState((prev) => ({ ...prev, ...p }));
  }, []);

  const toggleSection = React.useCallback((id: SectionId) => {
    setOpenSections((prev) => ({ ...prev, [id]: !prev[id] }));
  }, []);

  /**
   * Valida UNA sección devolviendo {ok, error}. Es el MISMO conjunto de reglas
   * del antiguo `validateStep` (origen↔paso 0, tablas↔paso 1, columnas↔paso 2,
   * acciones↔paso 3), reexpresado por sección sin cambiar qué cuenta como
   * válido. Función pura respecto a los argumentos capturados.
   */
  const checkSection = React.useCallback(
    (id: SectionId): SectionCheck => {
      if (id === "origin") {
        const parsed = moduleBaseSchema.safeParse({ slug: state.slug, name: state.name });
        if (!parsed.success) {
          return { ok: false, error: parsed.error.issues[0]?.message ?? "Datos de origen inválidos" };
        }
        if (state.connectionTemplateId.trim() === "") {
          return { ok: false, error: "Selecciona una plantilla de conexión" };
        }
        // Al crear, los campos VARIABLE requeridos deben tener valor. Al editar
        // los valores llegan vacíos del backend (vacío = conservar).
        if (!isEdit && selectedTemplate) {
          for (const f of selectedTemplate.fields) {
            if (f.kind === "variable" && f.required !== false) {
              if ((state.variableValues[f.key] ?? "").trim() === "") {
                return { ok: false, error: `El campo "${f.key}" es obligatorio` };
              }
            }
          }
        }
        return { ok: true, error: null };
      }
      if (id === "tables") {
        if (state.pgTable.trim() === "") {
          return { ok: false, error: "Declara la tabla de origen" };
        }
        return { ok: true, error: null };
      }
      if (id === "columns") {
        if (!hasAtLeastOneColumn(state)) {
          return { ok: false, error: "Agrega al menos una columna visible" };
        }
        // Fuente de valores por columna: si es estática/dinámica, debe ser
        // válida (mismas reglas que los campos de acción) antes de guardar.
        for (const col of state.columns) {
          const vs = col.valueSource;
          const colLabel = col.label || col.field;
          if (vs.kind === "static") {
            const err = selectOptionsError(vs.options, colLabel);
            if (err) {
              return { ok: false, error: err };
            }
          }
          if (vs.kind === "dynamic") {
            const err = dynamicSourceError(vs.dynamicSource, colLabel);
            if (err) {
              return { ok: false, error: err };
            }
          }
        }
        return { ok: true, error: null };
      }
      // id === "actions"
      {
        const sinPK = tablesMissingPrimaryKey(state);
        if (sinPK.length > 0) {
          // Nombra en qué tabla(s) falta la llave. En single-table es una sola;
          // en multi-tabla puede ser más de una.
          const detalle =
            sinPK.length === 1
              ? `la tabla «${sinPK[0]}»`
              : `las tablas ${sinPK.map((t) => `«${t}»`).join(", ")}`;
          return {
            ok: false,
            error: `Marca al menos una columna como llave primaria en ${detalle} para las acciones de editar o eliminar.`,
          };
        }
      }
      for (const a of state.actions) {
        if (!actionIsComplete(a)) {
          return { ok: false, error: `La acción "${a.label || a.actionKey}" está incompleta` };
        }
        for (const f of a.inputFields) {
          if (f.source === "select") {
            const err = selectFieldError(f);
            if (err) {
              return { ok: false, error: err };
            }
          }
          if (f.source === "dynamic") {
            const err = dynamicFieldError(f);
            if (err) {
              return { ok: false, error: err };
            }
          }
        }
      }
      return { ok: true, error: null };
    },
    [state, isEdit, selectedTemplate],
  );

  // Estado por sección para los indicadores de validez del acordeón.
  const checks = React.useMemo(() => {
    const map = {} as Record<SectionId, SectionCheck>;
    for (const s of SECTIONS) {
      map[s.id] = checkSection(s.id);
    }
    return map;
  }, [checkSection]);

  const save = () => {
    setSaveAttempted(true);
    // Valida TODAS las secciones; si alguna falla, AUTO-EXPANDE la PRIMERA
    // inválida (sin cerrar las demás) y dice CUÁL corregir.
    const firstInvalid = SECTIONS.find((s) => !checks[s.id].ok);
    if (firstInvalid) {
      setOpenSections((prev) => ({ ...prev, [firstInvalid.id]: true }));
      return;
    }
    const input = toDefinitionInput(state);
    mut.mutate(input, {
      onSuccess: () => router.push("/module-defs?saved=1"),
      // El error de guardar se muestra como TOAST flotante (siempre visible sin
      // importar en qué sección esté el scroll), no como banner inline.
      onError: (err) => toast.error(deriveSaveErrorMessage(err)),
    });
  };

  const firstInvalidTitle = SECTIONS.find((s) => !checks[s.id].ok)?.title ?? null;

  return (
    // Layout de TRES ZONAS a altura completa (cabecera / cuerpo con scroll /
    // pie), anclado POR ESTRUCTURA (flex column) y no por sticky/fixed: así la
    // cabecera y el pie nunca se desplazan respecto al nav lateral de la app y
    // el scroll vive SOLO en el cuerpo.
    <div className="flex h-full min-h-0 flex-col">
      {/* CABECERA no desplazable: barra de acciones del asistente (el título
          vive en la topbar del panel). Los botones Guardar/Cancelar van AQUÍ,
          en la zona de trabajo del asistente, SIEMPRE visibles sobre el
          acordeón; no se meten dentro de ninguna sección colapsable. */}
      <div className="shrink-0">
        <div className="mb-3 flex items-center justify-end gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => router.push("/module-defs")}
          >
            Cancelar
          </Button>
          <Button type="button" size="sm" onClick={save} disabled={mut.isPending}>
            {mut.isPending ? (
              <>
                <Loader2 className="size-4 animate-spin" aria-hidden="true" />
                Guardando...
              </>
            ) : isEdit ? (
              "Guardar cambios"
            ) : (
              "Crear módulo"
            )}
          </Button>
        </div>
      </div>

      {/* CUERPO: ÚNICA zona con scroll (`flex-1 min-h-0 overflow-y-auto`).
          Contiene el selector de copiar configuración (solo al crear), el aviso
          de sección inválida y el ACORDEÓN con todas las secciones apiladas. */}
      <div className="min-w-0 flex-1 min-h-0 space-y-4 overflow-y-auto pb-6">
        {/* Copiar configuración de otro módulo: SOLO al crear. */}
        {!isEdit ? (
          <CopyConfigPicker onCopy={(def) => setState(fromCopyableDefinition(def))} />
        ) : null}

        {/* Aviso de sección inválida al intentar guardar: la primera inválida se
            auto-expande; este banner recuerda CUÁL revisar. */}
        {saveAttempted && firstInvalidTitle ? (
          <FeedbackBanner
            tone="error"
            message={`Revisa la sección «${firstInvalidTitle}» para poder guardar.`}
          />
        ) : null}

        {/* Acordeón de secciones: todas apiladas, cada una colapsable. El
            encabezado lleva el icono de validez (completa/pendiente), el título
            y, cerrada, un resumen a la derecha. */}
        <div className="space-y-3">
          {SECTIONS.map((s) => {
            const complete = checks[s.id].ok;
            const open = openSections[s.id];
            const summary = sectionSummary(s.id, state);
            // Si la sección está abierta y es inválida tras intentar guardar,
            // muestra su error contextual dentro del panel.
            const inlineError = saveAttempted && !complete ? checks[s.id].error : null;
            const panelId = `w_section_${s.id}`;
            return (
              <CollapsibleRow
                key={s.id}
                panelId={panelId}
                open={open}
                onToggle={() => toggleSection(s.id)}
                leading={
                  complete ? (
                    <CheckCircle2 className="size-4 text-primary" />
                  ) : (
                    <Circle className="size-4 text-muted-foreground" />
                  )
                }
                header={
                  <span className="flex min-w-0 items-center gap-2">
                    <span className="truncate text-sm font-medium text-foreground">{s.title}</span>
                    <span className="sr-only">{complete ? " (completa)" : " (pendiente)"}</span>
                    {!open ? (
                      <span className="ml-auto truncate pl-2 text-xs text-muted-foreground">
                        {summary ?? (complete ? "Completa" : "Pendiente")}
                      </span>
                    ) : null}
                  </span>
                }
                panelClassName="space-y-4"
              >
                <p className="text-sm text-muted-foreground">{s.description}</p>
                {inlineError ? <FeedbackBanner tone="error" message={inlineError} /> : null}
                {s.id === "origin" ? (
                  <OriginSection
                    state={state}
                    patch={patch}
                    isEdit={isEdit}
                    templates={templates ?? []}
                    selectedTemplate={selectedTemplate}
                  />
                ) : null}
                {s.id === "tables" ? <TablesSection state={state} patch={patch} /> : null}
                {s.id === "columns" ? <ColumnsSection state={state} patch={patch} /> : null}
                {s.id === "actions" ? <ActionsSection state={state} patch={patch} /> : null}
              </CollapsibleRow>
            );
          })}
        </div>
      </div>
    </div>
  );
}

export default ModuleWizard;
