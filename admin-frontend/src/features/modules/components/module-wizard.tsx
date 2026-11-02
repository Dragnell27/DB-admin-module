"use client";

/**
 * Asistente de creación/edición de un módulo del constructor (module-builder,
 * tareas 13.1, 13.2). Rediseño de UX (fase 1 + refinamiento): en vez de un
 * stepper lineal de cuatro pasos con botón «Siguiente» forzado, presenta una
 * BARRA DE SECCIONES HORIZONTAL SUPERIOR (pestañas) + ÁREA DE TRABAJO a todo lo
 * ancho, con navegación LIBRE. Mover la navegación arriba (en vez de una
 * columna lateral) libera el ancho completo para los datos. Cada sección
 * muestra un indicador de estado (completa / pendiente) derivado de la MISMA
 * validación por paso de antes, ahora expresada por sección sin cambiar qué
 * cuenta como válido.
 *
 * Secciones:
 *   1. Origen y conexión (slug kebab-case + nombre + plantilla + variables).
 *   2. Tablas y relaciones (tabla principal + foráneas + FKs).
 *   3. Columnas (campo, etiqueta, orden, formato; visible/PK/editable).
 *   4. Acciones (CRUD/personalizadas con su operación SQL).
 *
 * Al guardar valida TODAS las secciones y, si alguna falla, indica CUÁL corregir
 * y la abre. El pago útil (`toDefinitionInput`) y las reglas de validez son
 * equivalentes en comportamiento a la versión anterior. Un slug duplicado se
 * muestra como banner (E409). Tema oscuro, con primitivas del proyecto. La
 * seguridad real la aplica el backend.
 */
import * as React from "react";
import { useRouter } from "next/navigation";
import { CheckCircle2, Circle, Loader2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { FeedbackBanner } from "@/components/feedback-banner";
import { cn } from "@/lib/utils";
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

/** Orden y metadatos de las secciones (pestañas de la barra superior). */
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
 * Se extrae del IIFE anterior sin cambiar la lógica: solo cambia la SUPERFICIE
 * de render (antes banner inline, ahora `toast.error`).
 */
export function deriveSaveErrorMessage(err: ApiError | undefined): string {
  const code = (err as { errorCode?: string } | undefined)?.errorCode;
  if (code === "E409") {
    return "El identificador del módulo ya está en uso. Elige otro slug.";
  }
  return err?.message ?? "No se pudo guardar el módulo.";
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
  const [active, setActive] = React.useState<SectionId>("origin");
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

  // Estado por sección para los indicadores de las pestañas superiores.
  const checks = React.useMemo(() => {
    const map = {} as Record<SectionId, SectionCheck>;
    for (const s of SECTIONS) {
      map[s.id] = checkSection(s.id);
    }
    return map;
  }, [checkSection]);

  const activeError = saveAttempted ? checks[active].error : null;

  const save = () => {
    setSaveAttempted(true);
    // Valida TODAS las secciones; si alguna falla, lleva a la PRIMERA inválida y
    // dice CUÁL corregir (sin cambiar las reglas de validez por sección).
    const firstInvalid = SECTIONS.find((s) => !checks[s.id].ok);
    if (firstInvalid) {
      setActive(firstInvalid.id);
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
  const activeMeta = SECTIONS.find((s) => s.id === active)!;

  return (
    // Layout de TRES ZONAS a altura completa (cabecera / cuerpo con scroll /
    // pie), anclado POR ESTRUCTURA (flex column) y no por sticky/fixed: así la
    // cabecera y el pie nunca se desplazan respecto al nav lateral de la app y
    // el scroll vive SOLO en el cuerpo.
    <div className="flex h-full min-h-0 flex-col">
      {/* Acciones Guardar/Cancelar PORTEADAS a la topbar (bloque «botones
          arriba»): siempre visibles, conservan sus handlers, labels, spinner y
          estado `disabled`. Si el slot aún no existe (SSR / primer render) no se
          portea; un efecto re-renderiza al resolverlo. */}
      {/* CABECERA no desplazable: barra de acciones del asistente + pestañas de
          secciones (el título vive en la topbar del panel). Los botones
          Guardar/Cancelar van AQUÍ, en la zona de trabajo del asistente (no en
          la topbar global), alineados a la derecha sobre las pestañas. */}
      <div className="shrink-0">
        {/* Barra de acciones: Cancelar / Guardar, arriba de las pestañas. */}
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
        {/* Barra de secciones HORIZONTAL: pestañas con navegación libre e
            indicador de estado por sección. Antes era una columna lateral que
            robaba ancho; ahora el área de trabajo ocupa todo el ancho. Ya NO
            necesita `sticky top-0` porque vive en esta cabecera no desplazable. */}
        <nav
          aria-label="Secciones del asistente"
          className="-mx-1 overflow-x-auto border-b border-border bg-background"
        >
        <ol className="flex min-w-max items-stretch gap-1 px-1">
          {SECTIONS.map((s) => {
            const isActive = s.id === active;
            const complete = checks[s.id].ok;
            return (
              <li key={s.id}>
                <button
                  type="button"
                  onClick={() => setActive(s.id)}
                  aria-current={isActive ? "step" : undefined}
                  title={complete ? "Completa" : "Pendiente"}
                  className={cn(
                    "flex items-center gap-2 whitespace-nowrap border-b-2 px-3 py-2 text-sm font-medium transition-colors",
                    isActive
                      ? "border-primary text-foreground"
                      : "border-transparent text-muted-foreground hover:text-foreground",
                  )}
                >
                  <span className="shrink-0" aria-hidden="true">
                    {complete ? (
                      <CheckCircle2 className="size-4 text-primary" />
                    ) : (
                      <Circle className="size-4 text-muted-foreground" />
                    )}
                  </span>
                  <span>{s.title}</span>
                  <span className="sr-only">{complete ? " (completa)" : " (pendiente)"}</span>
                </button>
              </li>
            );
          })}
          </ol>
        </nav>
      </div>

      {/* CUERPO: ÚNICA zona con scroll (`flex-1 min-h-0 overflow-y-auto`).
          Contiene el selector de copiar configuración (solo al crear), la
          descripción de ayuda, los banners, y la tarjeta con la sección activa.
          `pb-6` da aire al final; ya NO se usa `pb-20` porque el pie es un
          bloque estructural, no una barra fija que pueda tapar el contenido. */}
      <div className="min-w-0 flex-1 min-h-0 space-y-4 overflow-y-auto pb-6">
        {/* Copiar configuración de otro módulo: SOLO al crear. */}
        {!isEdit ? (
          <CopyConfigPicker onCopy={(def) => setState(fromCopyableDefinition(def))} />
        ) : null}

        <p className="text-sm text-muted-foreground">{activeMeta.description}</p>

        {/* Validación CONTEXTUAL por sección: se queda INLINE junto al paso
            que se edita (moverla a un toast perdería el contexto). El error de
            GUARDAR, en cambio, ahora es un toast flotante. */}
        {activeError ? <FeedbackBanner tone="error" message={activeError} /> : null}

        {/* Aviso de sección inválida al intentar guardar. Antes vivía en el pie
            (FormActionBar); al mover los botones a la topbar, el aviso se queda
            junto al cuerpo donde el usuario está trabajando. */}
        {saveAttempted && firstInvalidTitle ? (
          <FeedbackBanner
            tone="error"
            message={`Revisa la sección «${firstInvalidTitle}» para poder guardar.`}
          />
        ) : null}

        <Card>
          <CardContent className="space-y-4 pt-6">
            {active === "origin" ? (
              <OriginSection
                state={state}
                patch={patch}
                isEdit={isEdit}
                templates={templates ?? []}
                selectedTemplate={selectedTemplate}
              />
            ) : null}
            {active === "tables" ? <TablesSection state={state} patch={patch} /> : null}
            {active === "columns" ? <ColumnsSection state={state} patch={patch} /> : null}
            {active === "actions" ? <ActionsSection state={state} patch={patch} /> : null}
          </CardContent>
        </Card>
      </div>

    </div>
  );
}

export default ModuleWizard;
