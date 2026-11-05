"use client";

/**
 * Cuerpos de las SECCIONES del constructor de módulos (refactor de presentación
 * de `module-wizard.tsx`). Cada sección es un bloque de edición autónomo que se
 * renderiza en el área de trabajo cuando el usuario la selecciona en la barra
 * lateral:
 *   - OriginSection  → Origen y conexión (nombre, slug, plantilla, variables).
 *   - TablesSection  → Tablas y relaciones (principal, foráneas, FKs).
 *   - ColumnsSection → Columnas (tarjetas de dos filas con switches semánticos).
 *   - ActionsSection → Acciones y botones (se aloja tal cual; sin rediseño).
 *
 * Este archivo NO cambia la lógica de negocio: reutiliza `WizardState` y los
 * helpers de `schema.ts`. El rediseño afecta solo al layout y a los controles
 * (switches accesibles en lugar de checkboxes nativos en las columnas). La
 * invariante «Llave primaria ⇒ Editable off» se conserva exactamente.
 */
import * as React from "react";
import { ChevronDown, Loader2, Plus, Search, Trash2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent } from "@/components/ui/card";
import { Switch } from "@/components/ui/switch";
import { CollapsibleRow } from "@/components/ui/collapsible-row";
import { Pill } from "@/components/ui/pill";
import { FeedbackBanner } from "@/components/feedback-banner";
import type { ApiError } from "@/lib/api-client";
import { useModuleDef, useVisibleModules } from "@/features/modules/hooks";
import type {
  ConnectionTemplate,
  TemplateField,
} from "@/features/connections/types";
import type {
  ActionKind,
  AttributeMap,
  DynamicSource,
  FieldType,
  FillStrategyKind,
  FkJump,
  OrderDirection,
  SelectOption,
} from "@/features/modules/types";
import {
  actionRowKey,
  buildColumnOptions,
  columnOptionValue,
  datalistActionKeys,
  declaredCaptureKeys,
  defaultPlacementForKind,
  deriveActionShape,
  effectiveSqlOp,
  emptyStrategyFormState,
  emptyUniquenessFormState,
  fieldRowKey,
  fieldTargetColumnError,
  nextUid,
  parseColumnOptionValue,
  permActionForKind,
  parseColumnList,
  slugifyActionKey,
  writableColumns,
  deleteOrder,
  treeError,
  type ActionFormState,
  type ColumnFormState,
  type ColumnValueSourceFormState,
  type ConditionFormState,
  type FieldFormState,
  type RelationshipFormState,
  type StrategyFormState,
  type WizardState,
} from "@/features/modules/schema";

/** Clase compartida de los `<select>`/inputs nativos. Borde --input-border
 *  (más marcado en claro) para que el campo se lea sobre la tarjeta. */
export const inputClass =
  "h-10 w-full rounded-md border border-input-border bg-[var(--input-surface,var(--input))] px-3 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";

/** Indica si `field` corresponde a una columna definida en Columnas. */
function isKnownColumn(state: WizardState, field: string): boolean {
  return field !== "" && state.columns.some((c) => c.field === field);
}

/**
 * Entrada de la UNICIDAD: lista de columnas separada por comas. Mantiene el
 * TEXTO CRUDO en estado local (igual que `ForeignTableRow`), de modo que
 * escribir comas y espacios funcione: el parseo a array (`parseColumnList`) se
 * hace en un efecto, NO en cada tecla. Antes el input se controlaba sobre el
 * array ya parseado (`.join(", ")` ↔ `split`/`filter`), lo que borraba la coma
 * recién tecleada y hacía imposible declarar varias columnas. Patrón omitempty:
 * texto vacío ⇒ `columns=[]` (la unicidad no se emite; lo garantiza
 * `toUniqueness`).
 */
function UniquenessColumnsInput({
  columns,
  onColumnsChange,
}: {
  readonly columns: string[];
  readonly onColumnsChange: (columns: string[]) => void;
}) {
  const [text, setText] = React.useState(columns.join(", "));
  // Última lista que ESTE componente emitió por onColumnsChange a raíz del
  // tecleo. El <input> es la ÚNICA fuente de verdad del texto en curso; esta
  // guarda distingue los cambios de `columns` originados por el propio tecleo
  // (NO se resincroniza el texto: lo pisaría a media palabra) de los EXTERNOS
  // (hidratación / remonte de la sección al cambiar la plantilla de conexión:
  // SÍ se resincroniza). Se siembra con el valor inicial para que el primer
  // render no dispare un resync espurio.
  const lastEmittedRef = React.useRef<string[]>(columns);

  // Parsea el texto crudo a array y lo propaga al estado del asistente. Solo
  // reacciona a lo que el usuario teclea (text), no al array de vuelta. Antes de
  // propagar, registra el array emitido en lastEmittedRef para que el efecto de
  // resync distinga este cambio (propio) de uno externo.
  React.useEffect(() => {
    const parsed = parseColumnList(text);
    const changed =
      parsed.length !== columns.length ||
      parsed.some((c, i) => c !== columns[i]);
    if (changed) {
      lastEmittedRef.current = parsed;
      onColumnsChange(parsed);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [text]);

  // Resincroniza el texto SOLO cuando `columns` cambia por fuera (remonte de
  // sección al navegar entre pestañas, hidratación, cambio de plantilla de
  // conexión). Bug #2: al cambiar la plantilla la sección "Acciones" se
  // desmonta/remonta y el texto local quedaba desacoplado del estado (que SÍ
  // conserva las columnas en el reducer) ⇒ hay que reseed el texto.
  //
  // CRÍTICO (Bug corrupción de texto): si el cambio de `columns` proviene del
  // PROPIO tecleo (coincide con lastEmittedRef), NO se reescribe `text`: hacerlo
  // creaba una carrera bidireccional text↔columns que normalizaba el espaciado a
  // media palabra y comía caracteres. Solo se reseed cuando `columns` difiere de
  // lo último que emitimos (cambio externo real).
  React.useEffect(() => {
    const key = columns.join("\u0000");
    if (key === lastEmittedRef.current.join("\u0000")) {
      return; // cambio originado por el tecleo: el <input> manda, no se pisa.
    }
    if (parseColumnList(text).join("\u0000") !== key) {
      setText(columns.join(", "));
    }
    lastEmittedRef.current = columns;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [columns.join("\u0000")]);

  return (
    <Input
      value={text}
      onChange={(e) => setText(e.target.value)}
      placeholder="p. ej. padr_numero, depe_numero"
    />
  );
}

/**
 * Indica si una acción es de ESCRITURA sobre la tabla (crear/editar con
 * INSERT/UPDATE): en ese caso sus campos de entrada son COLUMNAS OBJETIVO (su
 * `name` debe ser una columna editable no-PK) y el formulario se genera
 * combinando columnas + acción. Eliminar y listar, y las acciones custom, no
 * aplican este modo.
 */
function isWriteColumnAction(a: ActionFormState): boolean {
  // El sqlOp se deriva del tipo, así que create/edit implican INSERT/UPDATE por
  // construcción. Depender solo del kind mantiene el modo columna aunque una
  // definición vieja venga con sqlOp incoherente.
  return a.kind === "create" || a.kind === "edit";
}

/** Etiqueta legible del tipo de un campo de entrada (para resúmenes). */
const FIELD_TYPE_LABEL: Record<FieldType, string> = {
  string: "Texto",
  number: "Número",
  boolean: "Booleano",
  date: "Fecha",
};

/** Etiqueta legible del origen («se llena con») de un campo (para resúmenes). */
const FIELD_SOURCE_LABEL: Record<FieldFormState["source"], string> = {
  input: "Captura del usuario",
  row: "Campo de la fila",
  constant: "Valor constante",
  select: "Selección de lista",
  dynamic: "Lista desde tabla",
  fillStrategy: "Estrategia de llenado",
};

/**
 * Resumen legible de un CAMPO de acción cuando está colapsado:
 * «<columna objetivo> · <tipo> · <se llena con>». Sin columna aún, usa un
 * marcador. Función pura; solo presentación.
 */
function fieldSummary(f: FieldFormState): string {
  const target = f.name.trim() !== "" ? f.name : "(sin columna)";
  return `${target} · ${FIELD_TYPE_LABEL[f.type]} · ${FIELD_SOURCE_LABEL[f.source]}`;
}

/**
 * Resumen legible de una RELACIÓN cuando está colapsada:
 * «<tabla_hija> → <tabla_padre> · <INNER|LEFT>». Sin tablas aún, usa marcadores.
 * Función pura; solo presentación.
 */
function relationshipSummary(r: RelationshipFormState): string {
  const child = r.childTable.trim() !== "" ? r.childTable : "(hija)";
  const parent = r.parentTable.trim() !== "" ? r.parentTable : "(padre)";
  const join = r.joinType === "left" ? "LEFT" : "INNER";
  return `${child} → ${parent} · ${join}`;
}

/**
 * Campo "Clave" de una acción: un input de texto libre (para escribir o
 * autoderivar la clave) con un botón de flecha a la derecha que despliega las
 * claves de OTRAS acciones. Elegir una la reutiliza (agrupa ambas acciones bajo
 * el mismo permiso). Se prefiere a un <datalist> (que el navegador filtra por lo
 * ya escrito y abre de forma inconsistente) y a chips sueltos. Construido con
 * primitivas del proyecto, sin dependencias nuevas; cierra al hacer click fuera.
 */
function ActionKeyInput({
  value,
  suggestions,
  onChange,
}: {
  readonly value: string;
  readonly suggestions: string[];
  readonly onChange: (next: string) => void;
}) {
  const [open, setOpen] = React.useState(false);
  const rootRef = React.useRef<HTMLDivElement | null>(null);

  React.useEffect(() => {
    if (!open) {
      return;
    }
    function onDocClick(e: MouseEvent) {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    }
    document.addEventListener("mousedown", onDocClick);
    return () => document.removeEventListener("mousedown", onDocClick);
  }, [open]);

  const hasSuggestions = suggestions.length > 0;

  return (
    <div ref={rootRef} className="relative">
      <Input
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder="list, create, activar"
        className={hasSuggestions ? "pr-9" : undefined}
      />
      {hasSuggestions ? (
        <button
          type="button"
          aria-haspopup="listbox"
          aria-expanded={open}
          aria-label="Reutilizar la clave de otra acción"
          onClick={() => setOpen((v) => !v)}
          className="absolute inset-y-0 right-0 flex w-9 items-center justify-center text-muted-foreground transition-colors hover:text-foreground"
        >
          <ChevronDown className="size-4" aria-hidden="true" />
        </button>
      ) : null}

      {open && hasSuggestions ? (
        <div className="absolute z-50 mt-1 w-full rounded-md border border-border bg-card shadow-lg">
          <p className="border-b border-border/60 px-3 py-1.5 text-xs text-muted-foreground">
            Reutilizar la clave de otra acción (agrupa el permiso)
          </p>
          <ul role="listbox" className="max-h-60 overflow-auto p-1" aria-label="Claves existentes">
            {suggestions.map((k) => (
              <li key={k}>
                <button
                  type="button"
                  role="option"
                  aria-selected={k === value}
                  onClick={() => {
                    onChange(k);
                    setOpen(false);
                  }}
                  className="w-full rounded-sm px-3 py-2 text-left text-sm text-foreground hover:bg-accent"
                >
                  {k}
                </button>
              </li>
            ))}
          </ul>
        </div>
      ) : null}
    </div>
  );
}

export interface SectionProps {
  readonly state: WizardState;
  readonly patch: (p: Partial<WizardState>) => void;
}

/**
 * Selector opcional "Copiar configuración de otro módulo" (solo modo creación).
 *
 * Lista los módulos VISIBLES del usuario (`useVisibleModules`, el mismo origen
 * que el sidebar/inicio, ya filtrado por RBAC). Al elegir uno, obtiene su
 * definición completa con el hook del modo edición (`useModuleDef` →
 * GET /module-defs/:slug) y llama a `onCopy` con ella; el asistente la mapea con
 * `fromCopyableDefinition`, que precarga campos/acciones/mapeos/listParams/
 * condiciones/ámbito de país/menuPath y DEJA sin copiar la conexión, el slug y
 * el nombre. Reemplaza la configuración actual del formulario.
 */
export function CopyConfigPicker({
  onCopy,
}: {
  readonly onCopy: (def: NonNullable<ReturnType<typeof useModuleDef>["data"]>) => void;
}) {
  const [sourceSlug, setSourceSlug] = React.useState("");
  // Evita volver a aplicar la misma definición en renders posteriores.
  const appliedFor = React.useRef<string | null>(null);

  const { data: modules } = useVisibleModules();
  const {
    data: def,
    isFetching,
    isError,
    error,
  } = useModuleDef(sourceSlug, { enabled: sourceSlug !== "" });

  React.useEffect(() => {
    if (sourceSlug !== "" && def && appliedFor.current !== sourceSlug) {
      appliedFor.current = sourceSlug;
      onCopy(def);
    }
  }, [sourceSlug, def, onCopy]);

  const handleChange = (slug: string) => {
    // Permite recopiar la misma fuente si se reselecciona tras cambiarla.
    appliedFor.current = null;
    setSourceSlug(slug);
  };

  const loadErrorMessage = isError
    ? (error as ApiError | undefined)?.message ??
      "No se pudo cargar la configuración del módulo origen."
    : null;

  return (
    <div className="space-y-2 rounded-lg border border-border bg-card p-3">
      <div className="space-y-1.5">
        <Label htmlFor="w_copy_source">Copiar configuración de otro módulo (opcional)</Label>
        <select
          id="w_copy_source"
          className={inputClass}
          value={sourceSlug}
          onChange={(e) => handleChange(e.target.value)}
        >
          <option value="">— Sin copiar —</option>
          {(modules ?? []).map((m) => (
            <option key={m.slug} value={m.slug}>
              {m.name}
            </option>
          ))}
        </select>
        <p className="text-xs text-muted-foreground">
          Precarga campos, acciones y mapeos del módulo elegido y reemplaza la
          configuración actual. La conexión, el identificador y el nombre NO se
          copian: debes definirlos para el módulo nuevo.
        </p>
      </div>
      {isFetching ? (
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <Loader2 className="size-4 animate-spin" aria-hidden="true" />
          Cargando configuración…
        </div>
      ) : null}
      {loadErrorMessage ? (
        <FeedbackBanner tone="error" message={loadErrorMessage} />
      ) : null}
    </div>
  );
}

/* === Sección: Origen y conexión ========================================== */

export function OriginSection({
  state,
  patch,
  isEdit,
  templates,
  selectedTemplate,
}: SectionProps & {
  isEdit: boolean;
  templates: ReadonlyArray<ConnectionTemplate>;
  selectedTemplate?: ConnectionTemplate;
}) {
  const variableFields: TemplateField[] = (selectedTemplate?.fields ?? []).filter(
    (f) => f.kind === "variable",
  );
  const setVariable = (key: string, value: string) => {
    patch({ variableValues: { ...state.variableValues, [key]: value } });
  };
  return (
    <div className="space-y-4">
      {/* Con el contenedor más ancho, los datos de origen aprovechan hasta 3
          columnas en pantallas grandes (1 en móvil, 2 desde sm). */}
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        <div className="space-y-1.5">
          <Label htmlFor="w_name">Nombre del módulo</Label>
          <Input id="w_name" value={state.name} onChange={(e) => patch({ name: e.target.value })} placeholder="Facturas" />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="w_slug">Identificador (slug)</Label>
          <Input
            id="w_slug"
            value={state.slug}
            onChange={(e) => patch({ slug: e.target.value })}
            placeholder="facturas"
            disabled={isEdit}
            autoComplete="off"
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="w_source">Tipo de origen</Label>
          <Input id="w_source" value="PostgreSQL" readOnly disabled />
        </div>
      </div>

      <div className="space-y-1.5">
        <Label htmlFor="w_menu_path">Ruta de menú (opcional)</Label>
        <Input
          id="w_menu_path"
          value={state.menuPath}
          onChange={(e) => patch({ menuPath: e.target.value })}
          placeholder="CENAM/Cuentas de recaudo/GT"
          autoComplete="off"
        />
        <p className="text-xs text-muted-foreground">
          Organiza el módulo en el menú con segmentos separados por «/». Vacío = raíz.
        </p>
      </div>

      <div className="space-y-1.5">
        <Label htmlFor="w_template">Plantilla de conexión</Label>
        <select
          id="w_template"
          className={inputClass}
          value={state.connectionTemplateId}
          onChange={(e) => {
            const id = e.target.value;
            const tmpl = templates.find((t) => t.id === id);
            // Prellena los valores sugeridos (NO sensibles) de los campos
            // variables; editables por el usuario (D-2).
            const prefill: Record<string, string> = {};
            for (const f of tmpl?.fields ?? []) {
              if (f.kind === "variable" && f.suggested) {
                prefill[f.key] = f.suggested;
              }
            }
            patch({ connectionTemplateId: id, variableValues: prefill });
          }}
        >
          <option value="">Selecciona una plantilla…</option>
          {templates.map((t) => (
            <option key={t.id} value={t.id}>
              {t.name}
            </option>
          ))}
        </select>
      </div>

      {selectedTemplate && variableFields.length > 0 ? (
        <div className="space-y-3 rounded-lg border border-border bg-card p-3">
          <h3 className="text-sm font-medium text-foreground">Valores de la conexión</h3>
          {/* Los valores variables se reparten en una rejilla responsiva (1 col
              en móvil, 2 desde sm, 3 en pantallas grandes) para aprovechar el
              ancho disponible. */}
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {variableFields.map((f) => {
              const current = state.variableValues[f.key] ?? "";
              const required = f.required !== false;
              return (
                <div key={f.key} className="space-y-1.5">
                  <Label htmlFor={`w_var_${f.key}`}>
                    {f.key}
                    {required ? " *" : " (opcional)"}
                    {isEdit ? " (vacío = sin cambios)" : ""}
                  </Label>
                  <Input
                    id={`w_var_${f.key}`}
                    value={current}
                    onChange={(e) => setVariable(f.key, e.target.value)}
                    autoComplete="off"
                  />
                </div>
              );
            })}
          </div>
        </div>
      ) : selectedTemplate ? (
        <p className="text-sm text-muted-foreground">
          Esta plantilla no tiene campos variables: todos sus valores son fijos.
        </p>
      ) : null}
    </div>
  );
}

/* === Sección: Tablas y relaciones ======================================== */

export function TablesSection({ state, patch }: SectionProps) {
  const warning = treeError(state);
  return (
    <div className="space-y-6">
      <PrincipalTableEditor state={state} patch={patch} />
      <ForeignTablesEditor state={state} patch={patch} />
      <RelationshipsEditor state={state} patch={patch} />
      {warning ? (
        <FeedbackBanner tone="error" message={`Revisa las relaciones: ${warning}`} />
      ) : null}
    </div>
  );
}

/** Editor de la tabla PRINCIPAL y su lista blanca. */
function PrincipalTableEditor({ state, patch }: SectionProps) {
  const [table, setTable] = React.useState(state.pgTable);
  const [cols, setCols] = React.useState((state.pgWhitelist[state.pgTable] ?? []).join(", "));

  React.useEffect(() => {
    const columns = cols.split(",").map((c) => c.trim()).filter(Boolean);
    const prevTable = state.pgTable;
    const nextWhitelist: Record<string, string[]> = { ...state.pgWhitelist };
    // Si cambió el nombre de la principal, migra su entrada de lista blanca.
    if (prevTable && prevTable !== table) {
      delete nextWhitelist[prevTable];
    }
    if (table) {
      nextWhitelist[table] = columns;
    }
    patch({ pgTable: table, pgWhitelist: nextWhitelist });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [table, cols]);

  return (
    <div className="space-y-4 rounded-md border border-border/60 p-4">
      <p className="text-sm font-medium">Tabla principal</p>
      <div className="space-y-1.5">
        <Label htmlFor="w_table">Tabla</Label>
        <Input id="w_table" value={table} onChange={(e) => setTable(e.target.value)} placeholder="facturas" />
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="w_cols">Columnas permitidas (lista blanca)</Label>
        <Input id="w_cols" value={cols} onChange={(e) => setCols(e.target.value)} placeholder="id, numero, monto, estado" />
        <p className="text-xs text-muted-foreground">
          Separadas por comas. Solo estas columnas podrán leerse o escribirse.
        </p>
      </div>
    </div>
  );
}

/**
 * Editor de TABLAS FORÁNEAS (multi-tabla): una sección por tabla foránea con su
 * nombre y su lista blanca. La sourceTable de cada columna queda IMPLÍCITA por
 * la tabla en la que vive (el admin no teclea prefijos).
 */
function ForeignTablesEditor({ state, patch }: SectionProps) {
  const addForeign = () => {
    patch({ foreignTables: [...state.foreignTables, ""] });
  };
  const updateForeign = (i: number, name: string) => {
    const prev = state.foreignTables[i];
    const foreignTables = state.foreignTables.slice();
    foreignTables[i] = name;
    const whitelist = { ...state.pgWhitelist };
    if (prev && prev !== name) {
      whitelist[name] = whitelist[prev] ?? [];
      delete whitelist[prev];
    } else if (name && whitelist[name] === undefined) {
      whitelist[name] = [];
    }
    patch({ foreignTables, pgWhitelist: whitelist });
  };
  const removeForeign = (i: number) => {
    const name = state.foreignTables[i];
    const foreignTables = state.foreignTables.filter((_, idx) => idx !== i);
    const whitelist = { ...state.pgWhitelist };
    if (name) {
      delete whitelist[name];
    }
    // Descarta relaciones que referencian la tabla eliminada.
    const relationships = state.relationships.filter(
      (r) => r.childTable !== name && r.parentTable !== name,
    );
    patch({ foreignTables, pgWhitelist: whitelist, relationships });
  };
  const updateForeignCols = (name: string, columns: string[]) => {
    patch({ pgWhitelist: { ...state.pgWhitelist, [name]: columns } });
  };

  return (
    <div className="space-y-4 rounded-md border border-border/60 p-4">
      <div className="flex items-center justify-between">
        <p className="text-sm font-medium">Tablas foráneas</p>
        <Button type="button" variant="outline" size="sm" onClick={addForeign}>
          Agregar tabla foránea
        </Button>
      </div>
      {state.foreignTables.length === 0 ? (
        <p className="text-xs text-muted-foreground">
          Sin tablas foráneas: el módulo es de una sola tabla.
        </p>
      ) : null}
      {state.foreignTables.map((name, i) => (
        <ForeignTableRow
          key={i}
          name={name}
          columns={state.pgWhitelist[name] ?? []}
          onNameChange={(n) => updateForeign(i, n)}
          onColumnsChange={(cols) => updateForeignCols(name, cols)}
          onRemove={() => removeForeign(i)}
        />
      ))}
    </div>
  );
}

/**
 * Fila de una tabla foránea. Mantiene el TEXTO CRUDO de la lista blanca en
 * estado local (igual que PrincipalTableEditor), de modo que escribir comas y
 * espacios funcione: el parseo a array (split por comas) se hace en un efecto,
 * NO en cada tecla. Antes el input se controlaba sobre el array ya parseado
 * (`.join(", ")` ↔ `split(",").filter(Boolean)`), lo que borraba la coma recién
 * tecleada y hacía imposible declarar varias columnas.
 */
function ForeignTableRow({
  name,
  columns,
  onNameChange,
  onColumnsChange,
  onRemove,
}: {
  readonly name: string;
  readonly columns: string[];
  readonly onNameChange: (name: string) => void;
  readonly onColumnsChange: (columns: string[]) => void;
  readonly onRemove: () => void;
}) {
  const [colsText, setColsText] = React.useState(columns.join(", "));

  // Parsea el texto crudo a array y lo propaga al estado del asistente. Solo
  // reacciona a lo que el usuario teclea (colsText), no al array de vuelta.
  React.useEffect(() => {
    const parsed = colsText.split(",").map((c) => c.trim()).filter(Boolean);
    const current = columns;
    const changed =
      parsed.length !== current.length ||
      parsed.some((c, i) => c !== current[i]);
    if (changed) {
      onColumnsChange(parsed);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [colsText]);

  return (
    <div className="space-y-2 rounded-md border border-border/40 p-3">
      <div className="flex items-end gap-2">
        <div className="flex-1 space-y-1.5">
          <Label>Tabla foránea</Label>
          <Input value={name} onChange={(e) => onNameChange(e.target.value)} placeholder="clientes" />
        </div>
        <Button type="button" variant="destructive" size="sm" onClick={onRemove}>
          Quitar
        </Button>
      </div>
      <div className="space-y-1.5">
        <Label>Columnas permitidas (lista blanca)</Label>
        <Input
          value={colsText}
          onChange={(e) => setColsText(e.target.value)}
          placeholder="id, nombre"
          disabled={!name}
        />
      </div>
    </div>
  );
}

/**
 * Editor de RELACIONES FK dirigidas: filas [tabla hija (posee FK)] [tabla padre]
 * [par ON] [tipo JOIN]. La distinción hijo/padre es explícita porque de ella se
 * deriva el orden de borrado (la hija se borra primero).
 */
function RelationshipsEditor({ state, patch }: SectionProps) {
  const tables = [state.pgTable, ...state.foreignTables].filter(Boolean);
  const addRel = () => {
    patch({
      relationships: [
        ...state.relationships,
        { childTable: "", parentTable: "", on: [{ child: "", parent: "" }], onConstants: [], joinType: "inner" },
      ],
    });
  };
  const updateRel = (i: number, p: Partial<RelationshipFormState>) => {
    const rels = state.relationships.slice();
    rels[i] = { ...rels[i], ...p };
    patch({ relationships: rels });
  };
  const removeRel = (i: number) => {
    patch({ relationships: state.relationships.filter((_, idx) => idx !== i) });
  };
  const updatePair = (ri: number, pi: number, p: Partial<{ child: string; parent: string }>) => {
    const rels = state.relationships.slice();
    const on = rels[ri].on.slice();
    on[pi] = { ...on[pi], ...p };
    rels[ri] = { ...rels[ri], on };
    patch({ relationships: rels });
  };
  const addPair = (ri: number) => {
    const rels = state.relationships.slice();
    rels[ri] = { ...rels[ri], on: [...rels[ri].on, { child: "", parent: "" }] };
    patch({ relationships: rels });
  };
  // Condiciones constantes del ON (tabla.col = valor). Por defecto la tabla es la
  // padre (la recién incorporada por el JOIN, caso típico list_numero = 10001).
  const addConstant = (ri: number) => {
    const rels = state.relationships.slice();
    const r = rels[ri];
    rels[ri] = {
      ...r,
      onConstants: [...r.onConstants, { table: r.parentTable || r.childTable || "", column: "", value: "" }],
    };
    patch({ relationships: rels });
  };
  const updateConstant = (ri: number, ci: number, p: Partial<{ table: string; column: string; value: string }>) => {
    const rels = state.relationships.slice();
    const onConstants = rels[ri].onConstants.slice();
    onConstants[ci] = { ...onConstants[ci], ...p };
    rels[ri] = { ...rels[ri], onConstants };
    patch({ relationships: rels });
  };
  const removeConstant = (ri: number, ci: number) => {
    const rels = state.relationships.slice();
    rels[ri] = { ...rels[ri], onConstants: rels[ri].onConstants.filter((_, idx) => idx !== ci) };
    patch({ relationships: rels });
  };

  if (state.foreignTables.length === 0) {
    return null;
  }

  return (
    <div className="space-y-4 rounded-md border border-border/60 p-4">
      <div className="flex items-center justify-between">
        <p className="text-sm font-medium">Relaciones (claves foráneas)</p>
        <Button type="button" variant="outline" size="sm" onClick={addRel}>
          Agregar relación
        </Button>
      </div>
      <p className="text-xs text-muted-foreground">
        La tabla hija es la que tiene la columna FK y se borra primero; la padre
        es la referenciada. Los JOIN solo se usan para LEER.
      </p>
      {state.relationships.map((r, i) => (
        <RelationshipRow
          key={i}
          index={i}
          rel={r}
          tables={tables}
          onUpdateRel={(p) => updateRel(i, p)}
          onRemove={() => removeRel(i)}
          onUpdatePair={(pi, p) => updatePair(i, pi, p)}
          onAddPair={() => addPair(i)}
          onAddConstant={() => addConstant(i)}
          onUpdateConstant={(ci, p) => updateConstant(i, ci, p)}
          onRemoveConstant={(ci) => removeConstant(i, ci)}
        />
      ))}
    </div>
  );
}

/**
 * Fila COLAPSABLE de una relación FK. Colapsada muestra el resumen
 * «<tabla_hija> → <tabla_padre> · <INNER|LEFT>»; expandida edita tablas, JOIN,
 * pares ON y condiciones constantes. Una relación NUEVA (sin tablas) arranca
 * EXPANDIDA. Estado de colapso LOCAL; el botón de borrar vive en el encabezado.
 * No cambia el modelo ni la serialización: solo presentación.
 */
function RelationshipRow({
  index,
  rel: r,
  tables,
  onUpdateRel,
  onRemove,
  onUpdatePair,
  onAddPair,
  onAddConstant,
  onUpdateConstant,
  onRemoveConstant,
}: {
  readonly index: number;
  readonly rel: RelationshipFormState;
  readonly tables: string[];
  readonly onUpdateRel: (p: Partial<RelationshipFormState>) => void;
  readonly onRemove: () => void;
  readonly onUpdatePair: (pi: number, p: Partial<{ child: string; parent: string }>) => void;
  readonly onAddPair: () => void;
  readonly onAddConstant: () => void;
  readonly onUpdateConstant: (ci: number, p: Partial<{ table: string; column: string; value: string }>) => void;
  readonly onRemoveConstant: (ci: number) => void;
}) {
  const i = index;
  const [open, setOpen] = React.useState(
    r.childTable.trim() === "" && r.parentTable.trim() === "",
  );
  const panelId = `w_rel_${i}`;

  return (
    <CollapsibleRow
      panelId={panelId}
      open={open}
      onToggle={() => setOpen((v) => !v)}
      header={
        <span className="truncate text-sm font-medium text-foreground">
          {relationshipSummary(r)}
        </span>
      }
      actions={
        <Button type="button" variant="ghost" size="sm" onClick={onRemove} aria-label="Quitar relación">
          <Trash2 aria-hidden="true" />
        </Button>
      }
      panelClassName="space-y-2"
    >
      <div className="grid items-end gap-2 sm:grid-cols-[1fr_1fr_120px]">
        <div className="space-y-1">
          <Label>Tabla hija (posee FK)</Label>
          <select className={inputClass} value={r.childTable} onChange={(e) => onUpdateRel({ childTable: e.target.value })}>
            <option value="">—</option>
            {tables.map((t) => (
              <option key={t} value={t}>{t}</option>
            ))}
          </select>
        </div>
        <div className="space-y-1">
          <Label>Tabla padre (referenciada)</Label>
          <select className={inputClass} value={r.parentTable} onChange={(e) => onUpdateRel({ parentTable: e.target.value })}>
            <option value="">—</option>
            {tables.map((t) => (
              <option key={t} value={t}>{t}</option>
            ))}
          </select>
        </div>
        <div className="space-y-1">
          <Label>Tipo JOIN</Label>
          <select className={inputClass} value={r.joinType} onChange={(e) => onUpdateRel({ joinType: e.target.value as RelationshipFormState["joinType"] })}>
            <option value="inner">INNER</option>
            <option value="left">LEFT</option>
          </select>
        </div>
      </div>
      <div className="space-y-2">
        <Label className="text-xs">Columnas de unión (ON): fk_hijo = clave_padre</Label>
        {r.on.map((p, pi) => (
          <div key={pi} className="grid items-center gap-2 sm:grid-cols-[1fr_auto_1fr]">
            <Input value={p.child} onChange={(e) => onUpdatePair(pi, { child: e.target.value })} placeholder="cliente_id" />
            <span className="text-center text-sm text-muted-foreground">=</span>
            <Input value={p.parent} onChange={(e) => onUpdatePair(pi, { parent: e.target.value })} placeholder="id" />
          </div>
        ))}
        <Button type="button" variant="outline" size="sm" onClick={onAddPair}>
          Agregar par ON
        </Button>
      </div>
      <div className="space-y-2">
        <Label className="text-xs">Condiciones constantes (ON): columna = valor</Label>
        <p className="text-xs text-muted-foreground">
          Se evalúa dentro del JOIN (ON), no en el filtro. Útil para fijar un
          valor como <code>list_numero = 10001</code> sin descartar filas sin coincidencia.
        </p>
        {r.onConstants.map((c, ci) => (
          <div key={ci} className="grid items-center gap-2 sm:grid-cols-[1fr_1fr_auto_1fr_auto]">
            <select
              className={inputClass}
              value={c.table}
              onChange={(e) => onUpdateConstant(ci, { table: e.target.value })}
            >
              <option value="">—</option>
              {[r.childTable, r.parentTable].filter(Boolean).map((t) => (
                <option key={t} value={t}>{t}</option>
              ))}
            </select>
            <Input value={c.column} onChange={(e) => onUpdateConstant(ci, { column: e.target.value })} placeholder="list_numero" />
            <span className="text-center text-sm text-muted-foreground">=</span>
            <Input value={c.value} onChange={(e) => onUpdateConstant(ci, { value: e.target.value })} placeholder="10001" />
            <Button type="button" variant="destructive" size="sm" onClick={() => onRemoveConstant(ci)}>
              Quitar
            </Button>
          </div>
        ))}
        <Button type="button" variant="outline" size="sm" onClick={onAddConstant}>
          Agregar condición constante
        </Button>
      </div>
    </CollapsibleRow>
  );
}

/**
 * Editor de la FUENTE DE VALORES de una columna: un `<select>` de tipo
 * (Ninguna / Lista estática / Lista dinámica) y, según el tipo, el editor
 * estático (`StaticOptionsEditor`) sobre `vs.options` o el editor dinámico
 * (`DynamicSourceEditor`, solo filtros de igualdad) sobre `vs.dynamicSource` +
 * `vs.dynamicFilters`. Opera de forma INMUTABLE sobre `vs` y propaga el objeto
 * completo con `onChange`. UI en español, tema oscuro.
 */
function ColumnValueSourceEditor({
  vs,
  onChange,
}: {
  readonly vs: ColumnValueSourceFormState;
  readonly onChange: (next: ColumnValueSourceFormState) => void;
}) {
  const addOption = () => {
    onChange({ ...vs, options: [...vs.options, { label: "", value: "" }] });
  };
  const updateOption = (oi: number, p: Partial<SelectOption>) => {
    const options = vs.options.slice();
    options[oi] = { ...options[oi], ...p };
    onChange({ ...vs, options });
  };
  const removeOption = (oi: number) => {
    onChange({ ...vs, options: vs.options.filter((_, idx) => idx !== oi) });
  };

  return (
    <div className="space-y-2 border-t border-border/40 pt-2">
      <div className="space-y-1">
        <Label>Fuente de valores</Label>
        <select
          className={inputClass}
          value={vs.kind}
          onChange={(e) =>
            onChange({ ...vs, kind: e.target.value as ColumnValueSourceFormState["kind"] })
          }
        >
          <option value="none">Ninguna</option>
          <option value="static">Lista estática</option>
          <option value="dynamic">Lista dinámica</option>
        </select>
        <p className="text-xs text-muted-foreground">
          Resuelve id→etiqueta al listar y puebla el desplegable al crear o editar.
        </p>
      </div>

      {vs.kind === "static" ? (
        <StaticOptionsEditor
          options={vs.options}
          onAddOption={addOption}
          onUpdateOption={updateOption}
          onRemoveOption={removeOption}
          showDefault={false}
        />
      ) : null}

      {vs.kind === "dynamic" ? (
        <DynamicSourceEditor
          dynamicSource={vs.dynamicSource}
          dynamicFilters={vs.dynamicFilters}
          onPatch={(p) => onChange({ ...vs, ...p })}
        />
      ) : null}
    </div>
  );
}

/* === Sección: Columnas (rediseñada) ====================================== */

export function ColumnsSection({ state, patch }: SectionProps) {
  // Las columnas son la ÚNICA fuente de verdad y se derivan de la lista blanca
  // (sección «Tablas y relaciones»). La SINCRONIZACIÓN con la lista blanca se
  // eleva a `ModuleWizard` (efecto global), de modo que `state.columns` queda al
  // día aunque el usuario edite la lista blanca y salte a otra sección sin pasar
  // por aquí. Esta sección solo RENDERIZA y edita `state.columns`.
  const multiTable = state.foreignTables.length > 0;

  const updateColumn = (i: number, p: Partial<ColumnFormState>) => {
    const copy = state.columns.slice();
    // Invariante PK ⇒ no editable (R4.3): marcar PK fuerza editable=false.
    const merged = { ...copy[i], ...p };
    if (p.isPrimaryKey === true) {
      merged.editable = false;
    }
    copy[i] = merged;
    patch({ columns: copy });
  };

  // Cada columna conserva su ÍNDICE GLOBAL en state.columns (el que consume
  // updateColumn). Al agrupar por tabla y filtrar por búsqueda se arrastra ese
  // índice para no romper la edición.
  const indexed = state.columns.map((col, index) => ({ col, index }));

  // Agrupa por tabla de origen PRESERVANDO el orden de primera aparición de
  // cada tabla (que ya viene principal-primero desde la sincronización). La
  // tabla efectiva de una columna sin sourceTable es la principal.
  const groups = React.useMemo(() => {
    const order: string[] = [];
    const byTable = new Map<string, { col: ColumnFormState; index: number }[]>();
    for (const item of indexed) {
      const table = item.col.sourceTable || state.pgTable || "";
      if (!byTable.has(table)) {
        byTable.set(table, []);
        order.push(table);
      }
      byTable.get(table)!.push(item);
    }
    return order.map((table) => ({ table, items: byTable.get(table)! }));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [state.columns, state.pgTable]);

  // Buscador por nombre de columna (campo o etiqueta, sin distinguir acentos no;
  // simple contains en minúsculas).
  const [query, setQuery] = React.useState("");
  const q = query.trim().toLowerCase();
  const matches = React.useCallback(
    (col: ColumnFormState) =>
      q === "" ||
      col.field.toLowerCase().includes(q) ||
      col.label.toLowerCase().includes(q),
    [q],
  );

  // Estado de expansión por tabla. Default: principal abierta, foráneas
  // colapsadas. `null` = aún sin override del usuario (usa el default).
  const [openByTable, setOpenByTable] = React.useState<Record<string, boolean>>({});
  const isOpen = (table: string) =>
    openByTable[table] ?? table === state.pgTable;
  const toggleTable = (table: string) =>
    setOpenByTable((prev) => ({ ...prev, [table]: !isOpen(table) }));
  const setAll = (open: boolean) =>
    setOpenByTable(Object.fromEntries(groups.map((g) => [g.table, open])));

  const renderCard = (col: ColumnFormState, i: number) => (
    <ColumnCard
      key={col.field || i}
      col={col}
      index={i}
      // Dentro de un grupo el "Campo" muestra solo el nombre de columna; en
      // módulos de una tabla no hay grupo, así que también va sin prefijo.
      fieldDisplay={col.field}
      onUpdate={updateColumn}
    />
  );

  return (
    <div className="space-y-3">
      <p className="text-xs text-muted-foreground">
        Derivadas de la lista blanca de «Tablas y relaciones». Edita etiqueta,
        orden y formato; marca visible, llave primaria o editable.
      </p>

      {state.columns.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          Aún no hay columnas. Declara columnas en la lista blanca de «Tablas y
          relaciones».
        </p>
      ) : (
        <>
          {/* Barra de herramientas: buscador por nombre + expandir/colapsar todo
              (estos dos solo tienen sentido con grupos, es decir, multi-tabla). */}
          <div className="flex flex-wrap items-center gap-2">
            <div className="relative min-w-[14rem] flex-1">
              <Search
                className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
                aria-hidden="true"
              />
              <Input
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="Buscar columna por nombre"
                aria-label="Buscar columna por nombre"
                className="pl-9"
              />
            </div>
            {multiTable ? (
              <div className="flex items-center gap-2">
                <Button type="button" variant="outline" size="sm" onClick={() => setAll(true)}>
                  Expandir todo
                </Button>
                <Button type="button" variant="outline" size="sm" onClick={() => setAll(false)}>
                  Colapsar todo
                </Button>
              </div>
            ) : null}
          </div>

          {/* N=1 (una sola tabla): render PLANO sin encabezado de grupo. */}
          {!multiTable ? (
            <div className="space-y-2">
              {indexed
                .filter(({ col }) => matches(col))
                .map(({ col, index }) => renderCard(col, index))}
              {indexed.every(({ col }) => !matches(col)) ? (
                <p className="text-sm text-muted-foreground">
                  Ninguna columna coincide con «{query}».
                </p>
              ) : null}
            </div>
          ) : (
            <div className="space-y-2">
              {groups.map((group) => (
                <ColumnTableGroup
                  key={group.table}
                  table={group.table}
                  isPrincipal={group.table === state.pgTable}
                  items={group.items}
                  matches={matches}
                  open={isOpen(group.table)}
                  onToggle={() => toggleTable(group.table)}
                  renderCard={renderCard}
                  query={query}
                />
              ))}
            </div>
          )}
        </>
      )}

      <ConditionsEditor state={state} patch={patch} />
    </div>
  );
}

/**
 * Grupo colapsable (accordion) de las columnas de UNA tabla en multi-tabla. El
 * encabezado muestra el nombre de la tabla (mono), si es principal/foránea y
 * pills de resumen («N columnas», «N PK»). El cuerpo contiene las tarjetas de
 * columna. Si la búsqueda está activa, el grupo se fuerza ABIERTO cuando tiene
 * coincidencias y se OCULTA cuando no tiene ninguna, para que el buscador sea
 * útil sin tener que expandir a mano.
 */
function ColumnTableGroup({
  table,
  isPrincipal,
  items,
  matches,
  open,
  onToggle,
  renderCard,
  query,
}: {
  readonly table: string;
  readonly isPrincipal: boolean;
  readonly items: { col: ColumnFormState; index: number }[];
  readonly matches: (col: ColumnFormState) => boolean;
  readonly open: boolean;
  readonly onToggle: () => void;
  readonly renderCard: (col: ColumnFormState, index: number) => React.ReactNode;
  readonly query: string;
}) {
  const searching = query.trim() !== "";
  const visibleItems = items.filter(({ col }) => matches(col));
  // Con búsqueda activa, oculta grupos sin coincidencias y abre los que sí.
  if (searching && visibleItems.length === 0) {
    return null;
  }
  const expanded = searching ? true : open;
  const pkCount = items.filter(({ col }) => col.isPrimaryKey).length;
  const panelId = `w_col_group_${table}`;

  return (
    <div className="rounded-lg border border-border bg-card shadow-elevation">
      <button
        type="button"
        onClick={onToggle}
        aria-expanded={expanded}
        aria-controls={panelId}
        className="flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-left transition-colors hover:bg-accent-hover disabled:cursor-default"
        disabled={searching}
      >
        <ChevronDown
          className={`size-4 shrink-0 text-muted-foreground transition-transform ${expanded ? "" : "-rotate-90"}`}
          aria-hidden="true"
        />
        <span className="font-mono text-sm font-medium text-foreground">{table}</span>
        <Pill variant={isPrincipal ? "principal" : "foranea"}>
          {isPrincipal ? "Principal" : "Foránea"}
        </Pill>
        <span className="ml-auto flex items-center gap-2">
          <Pill variant="count">
            {items.length} {items.length === 1 ? "columna" : "columnas"}
          </Pill>
          {pkCount > 0 ? <Pill variant="pk">{pkCount} PK</Pill> : null}
        </span>
      </button>
      {expanded ? (
        <div id={panelId} className="space-y-2 border-t border-border-soft p-2.5">
          {visibleItems.map(({ col, index }) => renderCard(col, index))}
        </div>
      ) : null}
    </div>
  );
}

/**
 * Tarjeta de edición de UNA columna: metadatos (campo, etiqueta, orden, formato)
 * en la fila superior y los switches Visible / Llave primaria / Editable debajo,
 * más el editor de Fuente de valores. Conserva la invariante PK ⇒ Editable off
 * (y Editable deshabilitado mientras sea PK). `index` es el índice GLOBAL en
 * state.columns que consume `onUpdate`. `fieldDisplay` es lo que se muestra en
 * "Campo" (dentro de un grupo, solo el nombre de la columna, sin prefijo tabla).
 */
function ColumnCard({
  col,
  index,
  fieldDisplay,
  onUpdate,
}: {
  readonly col: ColumnFormState;
  readonly index: number;
  readonly fieldDisplay: string;
  readonly onUpdate: (i: number, p: Partial<ColumnFormState>) => void;
}) {
  const i = index;
  // Las columnas no visibles se ATENÚAN (quedan presentes y editables).
  const attenuated = !col.visible;
  return (
    <div
      className={`space-y-2 rounded-md border border-border/60 bg-background p-2.5 transition-opacity ${
        attenuated ? "opacity-60" : ""
      }`}
    >
      <div className="grid w-full items-end gap-2 sm:grid-cols-[1fr_1fr_70px_1fr]">
        <div className="space-y-1">
          <Label>Campo</Label>
          <Input value={fieldDisplay} readOnly disabled />
        </div>
        <div className="space-y-1">
          <Label htmlFor={`w_col_label_${i}`}>Etiqueta</Label>
          <Input
            id={`w_col_label_${i}`}
            value={col.label}
            onChange={(e) => onUpdate(i, { label: e.target.value })}
            placeholder="Monto"
          />
        </div>
        <div className="space-y-1">
          <Label htmlFor={`w_col_order_${i}`}>Orden</Label>
          <Input
            id={`w_col_order_${i}`}
            type="number"
            value={col.order}
            onChange={(e) => onUpdate(i, { order: Number(e.target.value) })}
          />
        </div>
        <div className="space-y-1">
          <Label htmlFor={`w_col_format_${i}`}>Formato</Label>
          <select
            id={`w_col_format_${i}`}
            className={inputClass}
            value={col.format}
            onChange={(e) => onUpdate(i, { format: e.target.value })}
          >
            <option value="">Texto</option>
            <option value="date">Fecha</option>
            <option value="datetime">Fecha y hora</option>
          </select>
        </div>
      </div>
      <div className="flex flex-wrap items-center gap-x-5 gap-y-2">
        <SwitchField
          id={`w_col_visible_${i}`}
          label="Visible"
          title="Visible al listar"
          tone="primary"
          checked={col.visible}
          onCheckedChange={(v) => onUpdate(i, { visible: v })}
        />
        <SwitchField
          id={`w_col_pk_${i}`}
          label="Llave primaria"
          title="Llave primaria (WHERE)"
          tone="accent"
          checked={col.isPrimaryKey}
          onCheckedChange={(v) => onUpdate(i, { isPrimaryKey: v })}
        />
        <SwitchField
          id={`w_col_editable_${i}`}
          label="Editable"
          title="Editable (SET del UPDATE)"
          tone="primary"
          checked={col.editable}
          disabled={col.isPrimaryKey}
          onCheckedChange={(v) => onUpdate(i, { editable: v })}
        />
      </div>
      <ColumnValueSourceEditor
        vs={col.valueSource}
        onChange={(next) => onUpdate(i, { valueSource: next })}
      />
    </div>
  );
}

/** Switch etiquetado accesible: asocia el texto visible al control por `id`. */
function SwitchField({
  id,
  label,
  title,
  tone,
  checked,
  disabled,
  onCheckedChange,
}: {
  readonly id: string;
  readonly label: string;
  readonly title: string;
  readonly tone: "primary" | "accent";
  readonly checked: boolean;
  readonly disabled?: boolean;
  readonly onCheckedChange: (checked: boolean) => void;
}) {
  return (
    <div className="flex items-center gap-2 text-xs" title={title}>
      <Switch
        id={id}
        aria-labelledby={`${id}_label`}
        tone={tone}
        checked={checked}
        disabled={disabled}
        onCheckedChange={onCheckedChange}
      />
      <label
        id={`${id}_label`}
        htmlFor={id}
        className={disabled ? "cursor-not-allowed opacity-70" : "cursor-pointer"}
      >
        {label}
      </label>
    </div>
  );
}

/**
 * Cuerpo visual REUTILIZABLE de un editor de condiciones: filas columna +
 * valores (`valuesText`, separados por comas). Opera sobre un
 * `ConditionFormState[]` y su setter.
 */
function ConditionRowsEditor({
  conditions,
  onChange,
  addLabel,
  definedColumns,
}: {
  readonly conditions: ConditionFormState[];
  readonly onChange: (next: ConditionFormState[]) => void;
  readonly addLabel: string;
  /**
   * OPT-IN: si se pasa, la celda "Columna" es un desplegable de estas columnas
   * definidas (etiqueta tabla.columna). Si se omite (filtros de fuente
   * dinámica), la "Columna" sigue siendo texto libre. Así el cambio NO afecta a
   * los filtros dinámicos, que referencian columnas de la tabla de catálogo.
   */
  readonly definedColumns?: ColumnFormState[];
}) {
  const addCondition = () => {
    onChange([...conditions, { field: "", valuesText: "" }]);
  };
  const updateCondition = (i: number, p: Partial<ConditionFormState>) => {
    const copy = conditions.slice();
    copy[i] = { ...copy[i], ...p };
    onChange(copy);
  };
  const removeCondition = (i: number) => {
    onChange(conditions.filter((_, idx) => idx !== i));
  };

  return (
    <>
      <div className="flex justify-end">
        <Button type="button" variant="outline" size="sm" onClick={addCondition}>
          <Plus aria-hidden="true" /> {addLabel}
        </Button>
      </div>
      {conditions.map((cond, i) => (
        <div key={i} className="grid items-end gap-2 sm:grid-cols-[1fr_2fr_auto]">
          <div className="space-y-1">
            <Label>Columna</Label>
            {definedColumns ? (
              <select
                className={inputClass}
                value={columnOptionValue(cond.field, cond.sourceTable ?? "")}
                onChange={(e) => {
                  const { field, sourceTable } = parseColumnOptionValue(e.target.value);
                  updateCondition(i, { field, sourceTable });
                }}
              >
                <option value={columnOptionValue("", "")} disabled>
                  — Selecciona columna —
                </option>
                {buildColumnOptions(definedColumns, cond.field, cond.sourceTable ?? "").map((o) => (
                  <option key={o.value} value={o.value}>
                    {o.label}
                  </option>
                ))}
              </select>
            ) : (
              <Input
                value={cond.field}
                onChange={(e) => updateCondition(i, { field: e.target.value })}
                placeholder="status"
              />
            )}
          </div>
          <div className="space-y-1">
            <Label>Valores permitidos (separados por comas)</Label>
            <Input
              value={cond.valuesText}
              onChange={(e) => updateCondition(i, { valuesText: e.target.value })}
              placeholder="active, pending"
            />
          </div>
          <Button type="button" variant="ghost" size="sm" onClick={() => removeCondition(i)} aria-label="Quitar condición">
            <Trash2 aria-hidden="true" />
          </Button>
        </div>
      ))}
    </>
  );
}

/**
 * Editor de condiciones fijas de listado: pares columna + valores esperados.
 * Solo se muestran las filas cuya columna coincide (exacto, sin distinguir
 * mayúsculas) con alguno de los valores. Es opcional. Envoltorio delgado sobre
 * `ConditionRowsEditor`.
 */
function ConditionsEditor({ state, patch }: SectionProps) {
  return (
    <div className="space-y-3 border-t border-border pt-4">
      <div>
        <p className="text-sm font-medium text-foreground">Condiciones de listado (opcional)</p>
        <p className="text-xs text-muted-foreground">
          Muestra solo las filas cuya columna tenga uno de los valores indicados. Varias condiciones se combinan (todas deben cumplirse).
        </p>
      </div>
      <ConditionRowsEditor
        conditions={state.conditions}
        onChange={(next) => patch({ conditions: next })}
        addLabel="Condición"
        definedColumns={state.columns}
      />
    </div>
  );
}

/**
 * Editor de la fuente DINÁMICA de un campo (source==="dynamic"): tabla de
 * catálogo + columna id + columna etiqueta + orden + límite + filtros. Reutiliza
 * `ConditionRowsEditor` para los filtros.
 */
function DynamicSourceEditor({
  dynamicSource,
  dynamicFilters,
  onPatch,
}: {
  readonly dynamicSource: DynamicSource;
  readonly dynamicFilters: ConditionFormState[];
  readonly onPatch: (p: {
    dynamicSource?: DynamicSource;
    dynamicFilters?: ConditionFormState[];
  }) => void;
}) {
  const ds = dynamicSource;
  const order = ds.order ?? { column: "", direction: "asc" as OrderDirection };
  const patchSource = (p: Partial<DynamicSource>) => {
    onPatch({ dynamicSource: { ...ds, ...p } });
  };
  const patchOrder = (p: Partial<{ column: string; direction: OrderDirection }>) => {
    onPatch({ dynamicSource: { ...ds, order: { ...order, ...p } } });
  };

  return (
    <div className="space-y-3 rounded-md border border-border/60 p-2">
      <span className="text-xs font-medium text-muted-foreground">Fuente dinámica (tabla de catálogo)</span>
      <div className="grid gap-2 sm:grid-cols-3">
        <div className="space-y-1">
          <Label>Tabla</Label>
          <Input
            value={ds.table}
            onChange={(e) => patchSource({ table: e.target.value })}
            placeholder="planes"
          />
        </div>
        <div className="space-y-1">
          <Label>Columna ID</Label>
          <Input
            value={ds.idColumn}
            onChange={(e) => patchSource({ idColumn: e.target.value })}
            placeholder="id"
          />
        </div>
        <div className="space-y-1">
          <Label>Columna etiqueta</Label>
          <Input
            value={ds.labelColumn}
            onChange={(e) => patchSource({ labelColumn: e.target.value })}
            placeholder="nombre"
          />
        </div>
      </div>

      <div className="grid items-end gap-2 sm:grid-cols-3">
        <div className="space-y-1">
          <Label>Orden: columna (opcional)</Label>
          <Input
            value={order.column}
            onChange={(e) => patchOrder({ column: e.target.value })}
            placeholder="nombre"
          />
        </div>
        <div className="space-y-1">
          <Label>Dirección</Label>
          <select
            className={inputClass}
            value={order.direction}
            onChange={(e) => patchOrder({ direction: e.target.value as OrderDirection })}
          >
            <option value="asc">Ascendente</option>
            <option value="desc">Descendente</option>
          </select>
        </div>
        <div className="space-y-1">
          <Label>Límite (opcional)</Label>
          <Input
            type="number"
            value={ds.limit ?? ""}
            onChange={(e) => {
              const n = Number(e.target.value);
              patchSource({ limit: e.target.value === "" || Number.isNaN(n) ? undefined : n });
            }}
            placeholder="500"
          />
        </div>
      </div>
      <p className="text-xs text-muted-foreground">
        Máximo efectivo en v1: 500. Si la tabla tiene más filas, el desplegable
        solo mostrará las primeras 500 hasta que exista búsqueda/typeahead.
      </p>

      <div className="space-y-2 border-t border-border/60 pt-2">
        <p className="text-xs font-medium text-muted-foreground">Filtros (opcional)</p>
        <ConditionRowsEditor
          conditions={dynamicFilters}
          onChange={(next) => onPatch({ dynamicFilters: next })}
          addLabel="Filtro"
        />
      </div>
    </div>
  );
}

/** Etiqueta legible de cada tipo de estrategia de llenado (selector del editor). */
const STRATEGY_KIND_LABEL: Record<FillStrategyKind, string> = {
  "capture-from-catalog": "Capturar de catálogo",
  "derive-from-option": "Derivar de una opción",
  "derive-via-fk": "Derivar por relación (FK)",
  constant: "Constante",
  autogenerated: "Autogenerada",
};

/**
 * Editor de la ESTRATEGIA DE LLENADO de una columna objetivo (source===
 * "fillStrategy"). Un desplegable elige el `kind` y según él se muestran los
 * sub-campos:
 *   - capture-from-catalog: reutiliza `DynamicSourceEditor` para el catálogo +
 *     columna a guardar (`storeColumn`) + clave de captura (`captureKey`). Es la
 *     única estrategia que el usuario final ve como selector.
 *   - derive-from-option: captura de origen (`sourceCaptureKey`, entre las
 *     declaradas) + atributo de su fila (`attribute`).
 *   - derive-via-fk: salto FK (tabla/columna/captura/atributo locales) + lista de
 *     pares atributo→columna (`mapping`).
 *   - constant / autogenerated: sin sub-campos (la constante usa la ruta
 *     existente de valor constante; la autogenerada no emite la columna).
 * Solo propaga el patch de `StrategyFormState`; el estado vive en el campo.
 */
function StrategyEditor({
  strategy: s,
  captureKeys,
  onPatch,
}: {
  readonly strategy: StrategyFormState;
  readonly captureKeys: string[];
  readonly onPatch: (p: Partial<StrategyFormState>) => void;
}) {
  const patchJump = (p: Partial<FkJump>) => {
    onPatch({ jump: { ...s.jump, ...p } });
  };
  const updateMapping = (mi: number, p: Partial<AttributeMap>) => {
    const next = s.mapping.slice();
    next[mi] = { ...next[mi], ...p };
    onPatch({ mapping: next });
  };
  const addMapping = () => {
    onPatch({ mapping: [...s.mapping, { attribute: "", targetColumn: "" }] });
  };
  const removeMapping = (mi: number) => {
    onPatch({ mapping: s.mapping.filter((_, i) => i !== mi) });
  };

  return (
    <div className="space-y-3 rounded-md border border-border/60 p-2">
      <div className="space-y-1">
        <Label>Estrategia de llenado</Label>
        <select
          className={inputClass}
          value={s.kind}
          onChange={(e) => onPatch({ kind: e.target.value as FillStrategyKind })}
        >
          {(Object.keys(STRATEGY_KIND_LABEL) as FillStrategyKind[]).map((k) => (
            <option key={k} value={k}>
              {STRATEGY_KIND_LABEL[k]}
            </option>
          ))}
        </select>
      </div>

      {/* capture-from-catalog: catálogo + columna a guardar + clave de captura. */}
      {s.kind === "capture-from-catalog" ? (
        <>
          <DynamicSourceEditor
            dynamicSource={s.catalog}
            dynamicFilters={s.catalogFilters}
            onPatch={(p) =>
              onPatch({
                catalog: p.dynamicSource ?? s.catalog,
                catalogFilters: p.dynamicFilters ?? s.catalogFilters,
              })
            }
          />
          <div className="grid gap-2 sm:grid-cols-2">
            <div className="space-y-1">
              <Label>Columna a guardar (opcional)</Label>
              <Input
                value={s.storeColumn}
                onChange={(e) => onPatch({ storeColumn: e.target.value })}
                placeholder="por defecto: columna ID"
              />
            </div>
            <div className="space-y-1">
              <Label>Clave de captura</Label>
              <Input
                value={s.captureKey}
                onChange={(e) => onPatch({ captureKey: e.target.value })}
                placeholder="p. ej. comercio"
              />
            </div>
          </div>
          <p className="text-xs text-muted-foreground">
            Esta captura se muestra como selector al usuario final. Las
            derivaciones la referencian por su clave de captura.
          </p>
        </>
      ) : null}

      {/* derive-from-option: relee la fila de la captura de origen y toma un
          atributo. */}
      {s.kind === "derive-from-option" ? (
        <div className="grid gap-2 sm:grid-cols-2">
          <div className="space-y-1">
            <Label>Captura de origen</Label>
            <CaptureKeySelect
              value={s.sourceCaptureKey}
              captureKeys={captureKeys}
              onChange={(v) => onPatch({ sourceCaptureKey: v })}
            />
          </div>
          <div className="space-y-1">
            <Label>Atributo</Label>
            <Input
              value={s.attribute}
              onChange={(e) => onPatch({ attribute: e.target.value })}
              placeholder="p. ej. list_numero"
            />
          </div>
        </div>
      ) : null}

      {/* derive-via-fk: salto 1-a-1 por FK + lista de atributo→columna. */}
      {s.kind === "derive-via-fk" ? (
        <div className="space-y-3">
          <div className="grid gap-2 sm:grid-cols-2">
            <div className="space-y-1">
              <Label>Tabla relacionada</Label>
              <Input
                value={s.jump.relatedTable}
                onChange={(e) => patchJump({ relatedTable: e.target.value })}
                placeholder="gt_valores_padre"
              />
            </div>
            <div className="space-y-1">
              <Label>Columna relacionada (ON)</Label>
              <Input
                value={s.jump.relatedColumn}
                onChange={(e) => patchJump({ relatedColumn: e.target.value })}
                placeholder="valor_objeto"
              />
            </div>
            <div className="space-y-1">
              <Label>Captura local</Label>
              <CaptureKeySelect
                value={s.jump.localCaptureKey}
                captureKeys={captureKeys}
                onChange={(v) => patchJump({ localCaptureKey: v })}
              />
            </div>
            <div className="space-y-1">
              <Label>Atributo local (ON)</Label>
              <Input
                value={s.jump.localAttribute}
                onChange={(e) => patchJump({ localAttribute: e.target.value })}
                placeholder="come_numero"
              />
            </div>
          </div>
          <div className="space-y-2 border-t border-border/60 pt-2">
            <p className="text-xs font-medium text-muted-foreground">
              Atributos → columnas destino
            </p>
            {s.mapping.map((m, mi) => (
              <div key={mi} className="grid items-end gap-2 sm:grid-cols-[1fr_1fr_auto]">
                <div className="space-y-1">
                  <Label>Atributo</Label>
                  <Input
                    value={m.attribute}
                    onChange={(e) => updateMapping(mi, { attribute: e.target.value })}
                    placeholder="padr_numero"
                  />
                </div>
                <div className="space-y-1">
                  <Label>Columna destino</Label>
                  <Input
                    value={m.targetColumn}
                    onChange={(e) => updateMapping(mi, { targetColumn: e.target.value })}
                    placeholder="padr_numero"
                  />
                </div>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onClick={() => removeMapping(mi)}
                  aria-label="Quitar mapeo"
                >
                  <Trash2 aria-hidden="true" />
                </Button>
              </div>
            ))}
            <Button type="button" variant="outline" size="sm" onClick={addMapping}>
              <Plus aria-hidden="true" /> Mapeo
            </Button>
          </div>
        </div>
      ) : null}

      {s.kind === "constant" ? (
        <p className="text-xs text-muted-foreground">
          La constante se define con el origen &quot;Valor constante&quot;. Déjala
          configurada en el campo correspondiente.
        </p>
      ) : null}
      {s.kind === "autogenerated" ? (
        <p className="text-xs text-muted-foreground">
          La columna no se envía: la base de datos la genera (serial / valor por
          defecto).
        </p>
      ) : null}
    </div>
  );
}

/**
 * Desplegable de una `captureKey` existente con opción para escribirla a mano
 * ("Otra…"). Tolerante: si `value` no está entre `captureKeys` muestra el input
 * libre precargado, para no perder una config previa.
 */
function CaptureKeySelect({
  value,
  captureKeys,
  onChange,
}: {
  readonly value: string;
  readonly captureKeys: string[];
  readonly onChange: (v: string) => void;
}) {
  const known = value !== "" && captureKeys.includes(value);
  return (
    <>
      <select
        className={inputClass}
        value={known ? value : "__other__"}
        onChange={(e) => {
          const v = e.target.value;
          onChange(v === "__other__" ? "" : v);
        }}
      >
        <option value="" disabled>
          — Selecciona captura —
        </option>
        {captureKeys.map((k) => (
          <option key={k} value={k}>
            {k}
          </option>
        ))}
        <option value="__other__">Otra…</option>
      </select>
      {!known ? (
        <Input
          value={value}
          onChange={(e) => onChange(e.target.value)}
          placeholder="clave de captura"
        />
      ) : null}
    </>
  );
}

/**
 * Editor REUTILIZABLE de una lista ESTÁTICA de opciones etiqueta/valor con
 * opción preseleccionada opcional. API plana: opera sobre `options` +
 * `defaultValue` y propaga cambios con los handlers recibidos. Lo comparten los
 * campos de acción source==="select" (ActionsSection) y las columnas con fuente
 * estática (ColumnsSection). El render es idéntico al bloque original.
 */
function StaticOptionsEditor({
  options,
  defaultValue,
  onAddOption,
  onUpdateOption,
  onRemoveOption,
  onDefaultChange,
  showDefault = true,
}: {
  readonly options: SelectOption[];
  readonly defaultValue?: string;
  readonly onAddOption: () => void;
  readonly onUpdateOption: (oi: number, p: Partial<SelectOption>) => void;
  readonly onRemoveOption: (oi: number) => void;
  readonly onDefaultChange?: (value: string) => void;
  readonly showDefault?: boolean;
}) {
  return (
    <div className="space-y-2 rounded-md border border-border/60 p-2">
      <div className="flex items-center justify-between">
        <span className="text-xs font-medium text-muted-foreground">Opciones de la lista</span>
        <Button type="button" variant="ghost" size="sm" onClick={onAddOption}>
          <Plus aria-hidden="true" /> Opción
        </Button>
      </div>
      {options.map((opt, oi) => (
        <div key={oi} className="grid items-end gap-2 sm:grid-cols-[1fr_1fr_auto]">
          <div className="space-y-1">
            <Label>Etiqueta</Label>
            <Input
              value={opt.label}
              onChange={(e) => onUpdateOption(oi, { label: e.target.value })}
              placeholder="Guatemala"
            />
          </div>
          <div className="space-y-1">
            <Label>Valor</Label>
            <Input
              value={opt.value}
              onChange={(e) => onUpdateOption(oi, { value: e.target.value })}
              placeholder="gt"
            />
          </div>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => onRemoveOption(oi)}
            aria-label="Quitar opción"
          >
            <Trash2 aria-hidden="true" />
          </Button>
        </div>
      ))}
      {options.length === 0 ? (
        <p className="text-xs text-muted-foreground">Aún no hay opciones. Agrega al menos una.</p>
      ) : null}
      {showDefault && onDefaultChange ? (
        <div className="space-y-1">
          <Label>Opción por defecto</Label>
          <select
            className={inputClass}
            value={defaultValue ?? ""}
            onChange={(e) => onDefaultChange(e.target.value)}
          >
            <option value="">— Sin preselección —</option>
            {options.map((opt, oi) => (
              <option key={oi} value={opt.value}>
                {opt.label || opt.value}
              </option>
            ))}
          </select>
        </div>
      ) : null}
    </div>
  );
}

/* === Sección: Acciones y botones (alojada sin rediseño) ================== */

export function ActionsSection({ state, patch }: SectionProps) {
  const addAction = () => {
    const kind: ActionKind = "custom";
    const next: ActionFormState = {
      actionKey: "",
      kind,
      label: "",
      placement: "row",
      destructive: false,
      permAction: permActionForKind(kind, ""),
      isWrite: true,
      sqlOp: "",
      inputFields: [],
      uniqueness: emptyUniquenessFormState(),
      keyTouched: false,
      // uid EFÍMERO de runtime: identidad estable de la fila (key de React) que
      // sobrevive a updateAction/removeAction y no se serializa.
      uid: nextUid("a"),
    };
    patch({ actions: [...state.actions, next] });
  };
  const updateAction = (i: number, p: Partial<ActionFormState>) => {
    const copy = state.actions.slice();
    const merged = { ...copy[i], ...p };
    // Autoderivar la clave desde la etiqueta mientras no se haya tocado a mano.
    if (p.label !== undefined && !merged.keyTouched) {
      merged.actionKey = slugifyActionKey(p.label);
      merged.permAction = permActionForKind(merged.kind, merged.actionKey);
    }
    // Escribir en la clave a mano detiene la autoderivación desde el label.
    if (p.actionKey !== undefined) {
      merged.keyTouched = true;
    }
    if (p.kind) {
      // Cambiar el Tipo es la única fuente de verdad: recalcula permiso,
      // sqlOp/isWrite (derivados del tipo) y placement (default del tipo, pisa
      // el ajuste manual previo — borde 2).
      merged.permAction = permActionForKind(p.kind, merged.actionKey);
      const shape = deriveActionShape(p.kind, merged.sqlOp);
      merged.sqlOp = shape.sqlOp;
      merged.isWrite = shape.isWrite;
      merged.placement = defaultPlacementForKind(p.kind);
    } else if (p.sqlOp !== undefined && merged.kind === "custom") {
      // custom: cambiar la Operación SQL (sin cambiar tipo) recalcula isWrite.
      merged.isWrite = deriveActionShape("custom", merged.sqlOp).isWrite;
    }
    copy[i] = merged;
    patch({ actions: copy });
  };
  const removeAction = (i: number) => {
    patch({ actions: state.actions.filter((_, idx) => idx !== i) });
  };
  const addField = (ai: number) => {
    const copy = state.actions.slice();
    copy[ai] = {
      ...copy[ai],
      inputFields: [
        ...copy[ai].inputFields,
        {
          name: "",
          type: "string",
          required: false,
          source: "input",
          rowField: "",
          constValue: "",
          options: [],
          defaultValue: "",
          dynamicSource: { table: "", idColumn: "", labelColumn: "", order: { column: "", direction: "asc" } },
          dynamicFilters: [],
          hidden: false,
          label: "",
          captureOnly: false,
          strategy: emptyStrategyFormState(),
          // uid EFÍMERO de runtime: sobrevive al spread de updateField, de modo
          // que elegir la columna (name:"" -> "x") NO remonta ni colapsa la fila.
          uid: nextUid("f"),
        },
      ],
    };
    patch({ actions: copy });
  };
  const updateField = (ai: number, fi: number, p: Partial<ActionFormState["inputFields"][number]>) => {
    const copy = state.actions.slice();
    const fields = copy[ai].inputFields.slice();
    fields[fi] = { ...fields[fi], ...p };
    copy[ai] = { ...copy[ai], inputFields: fields };
    patch({ actions: copy });
  };
  const removeField = (ai: number, fi: number) => {
    const copy = state.actions.slice();
    copy[ai] = { ...copy[ai], inputFields: copy[ai].inputFields.filter((_, idx) => idx !== fi) };
    patch({ actions: copy });
  };
  // Handlers inmutables del editor de opciones de un campo source==="select".
  const addOption = (ai: number, fi: number) => {
    const copy = state.actions.slice();
    const fields = copy[ai].inputFields.slice();
    fields[fi] = { ...fields[fi], options: [...fields[fi].options, { label: "", value: "" }] };
    copy[ai] = { ...copy[ai], inputFields: fields };
    patch({ actions: copy });
  };
  const updateOption = (
    ai: number,
    fi: number,
    oi: number,
    p: Partial<{ label: string; value: string }>,
  ) => {
    const copy = state.actions.slice();
    const fields = copy[ai].inputFields.slice();
    const options = fields[fi].options.slice();
    options[oi] = { ...options[oi], ...p };
    fields[fi] = { ...fields[fi], options };
    copy[ai] = { ...copy[ai], inputFields: fields };
    patch({ actions: copy });
  };
  const removeOption = (ai: number, fi: number, oi: number) => {
    const copy = state.actions.slice();
    const fields = copy[ai].inputFields.slice();
    const removed = fields[fi].options[oi];
    const options = fields[fi].options.filter((_, idx) => idx !== oi);
    // Si la opción quitada era la preseleccionada, limpiar el default.
    const defaultValue =
      removed && removed.value === fields[fi].defaultValue ? "" : fields[fi].defaultValue;
    fields[fi] = { ...fields[fi], options, defaultValue };
    copy[ai] = { ...copy[ai], inputFields: fields };
    patch({ actions: copy });
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <p className="text-sm text-muted-foreground">Acciones del módulo y sus botones.</p>
        <Button type="button" variant="outline" size="sm" onClick={addAction}>
          <Plus aria-hidden="true" /> Acción
        </Button>
      </div>

      {state.actions.map((a, ai) => (
        <Card key={actionRowKey(a, ai)}>
          <CardContent className="space-y-3 pt-6">
            <div className="grid gap-2 sm:grid-cols-3">
              <div className="space-y-1">
                <Label>Clave</Label>
                <ActionKeyInput
                  value={a.actionKey}
                  suggestions={datalistActionKeys(state.actions, ai)}
                  onChange={(next) => updateAction(ai, { actionKey: next })}
                />
                <p className="text-xs text-muted-foreground">
                  Reutiliza la clave de otra acción para agrupar ambas bajo el
                  mismo permiso.
                </p>
              </div>
              <div className="space-y-1">
                <Label>Etiqueta del botón</Label>
                <Input value={a.label} onChange={(e) => updateAction(ai, { label: e.target.value })} />
              </div>
              <div className="space-y-1">
                <Label>Tipo</Label>
                <select className={inputClass} value={a.kind} onChange={(e) => updateAction(ai, { kind: e.target.value as ActionKind })}>
                  <option value="list">Listar</option>
                  <option value="search">Buscar</option>
                  <option value="create">Crear</option>
                  <option value="edit">Editar</option>
                  <option value="delete">Eliminar</option>
                  <option value="custom">Personalizada</option>
                </select>
              </div>
            </div>

            <div className="grid gap-2 sm:grid-cols-3">
              <div className="space-y-1">
                <Label>Botón en</Label>
                <select className={inputClass} value={a.placement} onChange={(e) => updateAction(ai, { placement: e.target.value as ActionFormState["placement"] })}>
                  <option value="header">Cabecera</option>
                  <option value="row">Fila</option>
                </select>
              </div>
              {/* La Operación SQL solo es editable en 'Personalizada'; en el
                  resto se deriva del Tipo (fuente única de verdad). */}
              {a.kind === "custom" ? (
                <div className="space-y-1">
                  <Label>Operación SQL</Label>
                  <select className={inputClass} value={a.sqlOp} onChange={(e) => updateAction(ai, { sqlOp: e.target.value })}>
                    <option value="">—</option>
                    <option value="SELECT">SELECT</option>
                    <option value="INSERT">INSERT</option>
                    <option value="UPDATE">UPDATE</option>
                    <option value="DELETE">DELETE</option>
                  </select>
                </div>
              ) : null}
            </div>

            {state.foreignTables.length > 0 && a.kind === "delete" ? (
              <div className="space-y-1">
                <Label>Eliminar hasta la tabla</Label>
                <select
                  className={inputClass}
                  value={a.deleteTopTable || state.pgTable}
                  onChange={(e) => updateAction(ai, { deleteTopTable: e.target.value })}
                >
                  {[state.pgTable, ...state.foreignTables].filter(Boolean).map((t) => (
                    <option key={t} value={t}>{t}</option>
                  ))}
                </select>
                <div className="rounded-md border border-border/50 bg-muted/20 px-2 py-1 text-xs text-muted-foreground">
                  Se eliminarán (hijo → padre):{" "}
                  {deleteOrder(state.relationships, a.deleteTopTable || state.pgTable).join(" → ")}
                  . El borrado alcanza la tabla tope y lo que cuelga de ella por FK, no el módulo completo.
                </div>
              </div>
            ) : null}

            {state.foreignTables.length > 0 && a.kind === "create" ? (
              <div className="space-y-1">
                <Label>Tabla objetivo del INSERT</Label>
                <select
                  className={inputClass}
                  value={a.targetTable || state.pgTable}
                  onChange={(e) => updateAction(ai, { targetTable: e.target.value })}
                >
                  {[state.pgTable, ...state.foreignTables].filter(Boolean).map((t) => (
                    <option key={t} value={t}>{t}</option>
                  ))}
                </select>
              </div>
            ) : null}

            {/* MULTI-TABLA (create): unicidad parametrizable. Columnas de la tabla
                objetivo (por comas) que deben ser únicas + mensaje de negocio. Se
                emite solo si hay al menos una columna (patrón omitempty). */}
            {a.kind === "create" ? (
              <div className="grid gap-2 rounded-md border border-border/60 p-2 sm:grid-cols-2">
                <div className="space-y-1">
                  <Label>Unicidad: columnas (opcional)</Label>
                  <UniquenessColumnsInput
                    columns={a.uniqueness.columns}
                    onColumnsChange={(cols) =>
                      updateAction(ai, {
                        uniqueness: { ...a.uniqueness, columns: cols },
                      })
                    }
                  />
                </div>
                <div className="space-y-1">
                  <Label>Mensaje al chocar (opcional)</Label>
                  <Input
                    value={a.uniqueness.message}
                    onChange={(e) =>
                      updateAction(ai, {
                        uniqueness: { ...a.uniqueness, message: e.target.value },
                      })
                    }
                    placeholder="La combinación ya existe"
                  />
                </div>
              </div>
            ) : null}

            <label className="flex items-center gap-2 text-sm">
              <input type="checkbox" checked={a.destructive} onChange={(e) => updateAction(ai, { destructive: e.target.checked })} className="size-4" />
              Acción destructiva (pide confirmación)
            </label>

            {/* Campos de entrada */}
            <div className="space-y-2 rounded-md border border-border p-3">
              <div className="flex items-center justify-between">
                <span className="text-xs font-medium text-muted-foreground">Campos de entrada</span>
                <Button type="button" variant="ghost" size="sm" onClick={() => addField(ai)}>
                  <Plus aria-hidden="true" /> Campo
                </Button>
              </div>
              {a.inputFields.map((f, fi) => (
                <ActionFieldRow
                  key={fieldRowKey(f, fi)}
                  index={fi}
                  field={f}
                  action={a}
                  state={state}
                  onUpdateField={(p) => updateField(ai, fi, p)}
                  onRemoveField={() => removeField(ai, fi)}
                  onAddOption={() => addOption(ai, fi)}
                  onUpdateOption={(oi, p) => updateOption(ai, fi, oi, p)}
                  onRemoveOption={(oi) => removeOption(ai, fi, oi)}
                />
              ))}
            </div>

            <div className="flex justify-end">
              <Button type="button" variant="ghost" size="sm" onClick={() => removeAction(ai)}>
                <Trash2 aria-hidden="true" /> Quitar acción
              </Button>
            </div>
          </CardContent>
        </Card>
      ))}
      {state.actions.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          Sin acciones aún. Agrega al menos una acción &quot;Listar&quot; para poder ver datos.
        </p>
      ) : null}
    </div>
  );
}

/**
 * Fila COLAPSABLE de un CAMPO de entrada de una acción. Colapsada muestra el
 * resumen «<columna objetivo> · <tipo> · <se llena con>»; expandida edita todos
 * sus controles (columna/tipo/obligatorio, origen del valor, opciones estáticas
 * o fuente dinámica). Un campo NUEVO (sin columna objetivo) arranca EXPANDIDO.
 * Estado de colapso LOCAL; el botón de borrar vive en el encabezado. No cambia
 * el modelo ni la serialización: solo presentación.
 */
function ActionFieldRow({
  index,
  field: f,
  action: a,
  state,
  onUpdateField,
  onRemoveField,
  onAddOption,
  onUpdateOption,
  onRemoveOption,
}: {
  readonly index: number;
  readonly field: FieldFormState;
  readonly action: ActionFormState;
  readonly state: WizardState;
  readonly onUpdateField: (p: Partial<FieldFormState>) => void;
  readonly onRemoveField: () => void;
  readonly onAddOption: () => void;
  readonly onUpdateOption: (oi: number, p: Partial<SelectOption>) => void;
  readonly onRemoveOption: (oi: number) => void;
}) {
  const [open, setOpen] = React.useState(f.name.trim() === "");
  const panelId = `w_action_field_${a.actionKey || "n"}_${index}`;
  // Validación EN VIVO de la "columna objetivo" vacía (Arreglo 2). Solo se marca
  // en rojo tras interacción (`touched`), nunca pre-interacción en un campo
  // recién creado. El touched local es fiable porque el Arreglo 1 evita que la
  // fila se remonte al elegir la columna (key por uid, no por name). El BLOQUEO
  // del guardado es independiente de touched (vía actionsSectionError).
  const [touched, setTouched] = React.useState(false);
  const colObjInvalid = fieldTargetColumnError(f) !== null && touched;
  const colObjErrorId = `${panelId}_colobj_error`;
  // MEJORA 4: un campo auxiliar de SOLO CAPTURA no escribe columna de la tabla
  // objetivo; su `name` es la clave de captura. En ese modo NO se muestra el
  // selector de "Columna objetivo" ni su validación.
  const captureOnly = Boolean(f.captureOnly);

  return (
    <CollapsibleRow
      panelId={panelId}
      open={open}
      onToggle={() => setOpen((v) => !v)}
      header={
        <span className="truncate text-sm font-medium text-foreground">
          {fieldSummary(f)}
        </span>
      }
      actions={
        <Button type="button" variant="ghost" size="sm" onClick={onRemoveField} aria-label="Quitar campo">
          <Trash2 aria-hidden="true" />
        </Button>
      }
      panelClassName="space-y-2"
    >
                  <div className="grid items-end gap-2 sm:grid-cols-[1fr_1fr_auto]">
                    <div className="space-y-1">
                      {captureOnly ? (
                        // Campo solo-captura: no hay columna objetivo; el `name` es
                        // la clave de captura que alimenta las derivaciones.
                        <>
                          <Label htmlFor={`${panelId}_capkey`}>Clave de captura (name)</Label>
                          <Input
                            id={`${panelId}_capkey`}
                            className={inputClass}
                            value={f.name}
                            placeholder="p. ej. __capture_comercio"
                            onChange={(e) => onUpdateField({ name: e.target.value })}
                          />
                        </>
                      ) : (
                        <>
                      <Label>Columna objetivo</Label>
                      {isWriteColumnAction(a) ? (
                        // Acción de escritura: el campo ES una columna editable no-PK
                        // (invariante name == columna). Selector de columnas definidas
                        // (etiqueta tabla.columna) que además fija sourceTable para
                        // desambiguar homónimos.
                        <select
                          className={inputClass}
                          value={columnOptionValue(f.name, f.sourceTable ?? "")}
                          onBlur={() => setTouched(true)}
                          aria-invalid={colObjInvalid || undefined}
                          aria-describedby={colObjInvalid ? colObjErrorId : undefined}
                          onChange={(e) => {
                            const { field, sourceTable } = parseColumnOptionValue(e.target.value);
                            onUpdateField({ name: field, sourceTable });
                          }}
                        >
                          <option value="" disabled>
                            — Selecciona columna —
                          </option>
                          {buildColumnOptions(writableColumns(state), f.name, f.sourceTable ?? "").map((o) => (
                            <option key={o.value} value={o.value}>
                              {o.label}
                            </option>
                          ))}
                        </select>
                      ) : (
                        // Resto de acciones (delete/custom/…): la columna objetivo es
                        // un desplegable de las columnas definidas del módulo con
                        // etiqueta tabla.columna. Fija name + sourceTable.
                        <select
                          className={inputClass}
                          value={columnOptionValue(f.name, f.sourceTable ?? "")}
                          onBlur={() => setTouched(true)}
                          aria-invalid={colObjInvalid || undefined}
                          aria-describedby={colObjInvalid ? colObjErrorId : undefined}
                          onChange={(e) => {
                            const { field, sourceTable } = parseColumnOptionValue(e.target.value);
                            onUpdateField({ name: field, sourceTable });
                          }}
                        >
                          <option value={columnOptionValue("", "")} disabled>
                            — Selecciona columna —
                          </option>
                          {buildColumnOptions(state.columns, f.name, f.sourceTable ?? "").map((o) => (
                            <option key={o.value} value={o.value}>
                              {o.label}
                            </option>
                          ))}
                        </select>
                      )}
                      {colObjInvalid ? (
                        <p className="text-xs text-destructive" role="alert" id={colObjErrorId}>
                          Falta la columna objetivo
                        </p>
                      ) : null}
                        </>
                      )}
                    </div>
                    <div className="space-y-1">
                      <Label>Tipo</Label>
                      <select className={inputClass} value={f.type} onChange={(e) => onUpdateField({ type: e.target.value as FieldType })}>
                        <option value="string">Texto</option>
                        <option value="number">Número</option>
                        <option value="boolean">Booleano</option>
                        <option value="date">Fecha</option>
                      </select>
                    </div>
                    <label className="flex items-center gap-1 pb-2 text-xs">
                      <input type="checkbox" checked={f.required} onChange={(e) => onUpdateField({ required: e.target.checked })} className="size-4" />
                      Oblig.
                    </label>
                  </div>

                  {/* MEJORA 3 + 4: etiqueta visible del formulario de ejecución y
                      marca de campo auxiliar de solo captura (no escribe columna). */}
                  <div className="grid items-start gap-2 sm:grid-cols-2">
                    <div className="space-y-1">
                      <Label htmlFor={`${panelId}_label`}>Etiqueta a mostrar (opcional)</Label>
                      <Input
                        id={`${panelId}_label`}
                        className={inputClass}
                        value={f.label}
                        placeholder="Etiqueta legible en el formulario"
                        onChange={(e) => onUpdateField({ label: e.target.value })}
                      />
                    </div>
                    <label className="flex items-start gap-2 pt-6 text-xs">
                      <input
                        type="checkbox"
                        checked={captureOnly}
                        onChange={(e) => onUpdateField({ captureOnly: e.target.checked })}
                        className="mt-0.5 size-4"
                      />
                      <span>Campo auxiliar: solo captura (no escribe a columna)</span>
                    </label>
                  </div>

                  {/* Origen del valor: capturado del usuario, tomado de la fila
                      o un valor literal constante definido aquí. */}
                  <div className="grid items-end gap-2 sm:grid-cols-2">
                    <div className="space-y-1">
                      <Label>Se llena con</Label>
                      <select
                        className={inputClass}
                        value={f.source}
                        onChange={(e) => onUpdateField({ source: e.target.value as FieldFormState["source"] })}
                      >
                        <option value="input">Captura del usuario</option>
                        {/* "Campo de la fila" NO aplica a escrituras Postgres: la
                            PK/WHERE se resuelve por columnas, no por un campo row. */}
                        {isWriteColumnAction(a) ? null : (
                          <option value="row">Campo de la fila</option>
                        )}
                        <option value="constant">Valor constante</option>
                        <option value="select">Selección de lista</option>
                        <option value="dynamic">Lista desde tabla</option>
                        {/* Estrategia de llenado: SOLO acciones INSERT (crear /
                            custom INSERT) multi-tabla. Resuelve el valor de la
                            columna objetivo en el backend (captura/derivación). */}
                        {effectiveSqlOp(a) === "INSERT" ? (
                          <option value="fillStrategy">Estrategia de llenado</option>
                        ) : null}
                      </select>
                    </div>
                    {/* Coincidencia del filtro: SOLO acciones SELECT (filtro de
                        listado). "Exacta" ⇒ igualdad; "Parcial" ⇒ contiene (LIKE).
                        Se ignora en INSERT/UPDATE/DELETE. Default exacta. */}
                    {effectiveSqlOp(a) === "SELECT" ? (
                      <div className="space-y-1">
                        <Label>Coincidencia</Label>
                        <select
                          className={inputClass}
                          value={f.matchType ?? "exact"}
                          onChange={(e) =>
                            onUpdateField({
                              matchType: e.target.value === "partial" ? "partial" : "exact",
                            })
                          }
                        >
                          <option value="exact">Exacta</option>
                          <option value="partial">Parcial (contiene)</option>
                        </select>
                      </div>
                    ) : null}
                    {/* Valor literal fijo que se envía siempre (p. ej. un header
                        country: gt). Análogo al bloque de source==='row'. */}
                    {f.source === "constant" ? (
                      <div className="space-y-1">
                        <Label>Valor constante</Label>
                        <Input
                          value={f.constValue}
                          onChange={(e) => onUpdateField({ constValue: e.target.value })}
                          placeholder="p. ej. gt"
                        />
                        {/* Solo para escrituras: ocultar la columna del formulario
                            (flujo "Inactivar"). Su valor lo fija el backend. */}
                        {isWriteColumnAction(a) ? (
                          <label className="flex items-center gap-1 pt-1 text-xs">
                            <input
                              type="checkbox"
                              checked={f.hidden}
                              onChange={(e) => onUpdateField({ hidden: e.target.checked })}
                              className="size-4"
                            />
                            Ocultar en el formulario
                          </label>
                        ) : null}
                      </div>
                    ) : null}
                    {f.source === "row" ? (
                      <div className="space-y-1">
                        <Label>Campo de la fila</Label>
                        <select
                          className={inputClass}
                          value={isKnownColumn(state, f.rowField) ? f.rowField : "__other__"}
                          onChange={(e) => {
                            const v = e.target.value;
                            onUpdateField({ rowField: v === "__other__" ? "" : v });
                          }}
                        >
                          {state.columns.map((c) => (
                            <option key={c.field} value={c.field}>
                              {c.label || c.field} ({c.field})
                            </option>
                          ))}
                          <option value="__other__">Otro campo…</option>
                        </select>
                        {!isKnownColumn(state, f.rowField) ? (
                          <Input
                            value={f.rowField}
                            onChange={(e) => onUpdateField({ rowField: e.target.value })}
                            placeholder="nombre del campo en la fila (p. ej. id)"
                          />
                        ) : null}
                      </div>
                    ) : null}
                  </div>

                  {/* source==="select": lista estática de opciones (etiqueta/
                      valor) que el usuario elige en tiempo de ejecución, con una
                      opción preseleccionada opcional. */}
                  {f.source === "select" ? (
                    <StaticOptionsEditor
                      options={f.options}
                      defaultValue={f.defaultValue}
                      onAddOption={onAddOption}
                      onUpdateOption={onUpdateOption}
                      onRemoveOption={onRemoveOption}
                      onDefaultChange={(value) => onUpdateField({ defaultValue: value })}
                    />
                  ) : null}

                  {/* source==="dynamic": lista cuyas opciones [{id,label}] se
                      cargan en ejecución desde una tabla de catálogo. */}
                  {f.source === "dynamic" ? (
                    <DynamicSourceEditor
                      dynamicSource={f.dynamicSource}
                      dynamicFilters={f.dynamicFilters}
                      onPatch={(p) => onUpdateField(p)}
                    />
                  ) : null}

                  {/* source==="fillStrategy": estrategia de llenado de la columna
                      objetivo (captura de catálogo / derivación / constante /
                      autogenerada). La captura es la ÚNICA visible en runtime. */}
                  {f.source === "fillStrategy" ? (
                    <StrategyEditor
                      strategy={f.strategy}
                      captureKeys={declaredCaptureKeys(a, index)}
                      onPatch={(p) => onUpdateField({ strategy: { ...f.strategy, ...p } })}
                    />
                  ) : null}
    </CollapsibleRow>
  );
}
