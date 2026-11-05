"use client";

/**
 * SearchSelect: combobox ligero con búsqueda incremental, construido con
 * primitivas existentes (Input + lista) sin dependencias nuevas.
 *
 * Es controlado: recibe el valor seleccionado (`value`, un identificador) y la
 * lista de opciones (`options`, cada una con `value` y `label` legible). El
 * usuario escribe para filtrar por `label` (y opcionalmente por `value`), y al
 * elegir una opción se reporta su `value` por `onChange`. Pensado para
 * integrarse con react-hook-form vía Controller.
 */
import * as React from "react";
import { Check, ChevronsUpDown, Loader2 } from "lucide-react";

import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";

export interface SearchSelectOption {
  readonly value: string;
  readonly label: string;
  /** Texto secundario opcional (p. ej. monto/país del plan). */
  readonly hint?: string;
}

export interface SearchSelectProps {
  readonly id?: string;
  /** Valor seleccionado (identificador). "" = nada seleccionado. */
  readonly value: string;
  readonly onChange: (value: string) => void;
  readonly onBlur?: () => void;
  readonly options: ReadonlyArray<SearchSelectOption>;
  readonly placeholder?: string;
  readonly disabled?: boolean;
  /** Muestra un spinner cuando las opciones aún se están cargando. */
  readonly loading?: boolean;
  /** Texto cuando no hay coincidencias. */
  readonly emptyText?: string;
  readonly "aria-invalid"?: boolean | "true" | "false";
  /**
   * Permite ESCRIBIR LIBRE en el disparador (modo combobox editable): el texto
   * tecleado se reporta por `onChange` tal cual (no solo al elegir una opción) y
   * sirve de filtro. La validación de "opción válida" la hace el consumidor al
   * confirmar. Cuando es false (por defecto) el disparador es un botón y solo se
   * reporta un value al elegir una opción de la lista.
   */
  readonly allowCustomValue?: boolean;
}

export function SearchSelect({
  id,
  value,
  onChange,
  onBlur,
  options,
  placeholder = "Buscar...",
  disabled,
  loading,
  emptyText = "Sin coincidencias",
  "aria-invalid": ariaInvalid,
  allowCustomValue = false,
}: SearchSelectProps) {
  const [open, setOpen] = React.useState(false);
  const [query, setQuery] = React.useState("");
  // Índice activo para navegación con teclado (flechas/enter). -1 = ninguno.
  const [activeIndex, setActiveIndex] = React.useState(-1);
  const rootRef = React.useRef<HTMLDivElement | null>(null);
  const listboxId = React.useId();

  // Etiqueta de la opción seleccionada (para mostrarla cuando el panel está cerrado).
  const selected = React.useMemo(
    () => options.find((o) => o.value === value),
    [options, value],
  );

  // Cerrar al hacer click fuera.
  React.useEffect(() => {
    if (!open) {
      return;
    }
    function onDocClick(e: MouseEvent) {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) {
        setOpen(false);
        onBlur?.();
      }
    }
    document.addEventListener("mousedown", onDocClick);
    return () => document.removeEventListener("mousedown", onDocClick);
  }, [open, onBlur]);

  // En modo editable, el texto visible del disparador es, si hay opción
  // seleccionada, su etiqueta; si no, el value crudo (texto libre tecleado). El
  // filtro de la lista usa `query` (editable) o `value` (texto libre en curso).
  const filterText = allowCustomValue
    ? (selected ? query : value)
    : query;

  const filtered = React.useMemo(() => {
    const q = filterText.trim().toLowerCase();
    if (q === "") {
      return options.slice(0, 50);
    }
    return options
      .filter(
        (o) =>
          o.label.toLowerCase().includes(q) ||
          o.value.toLowerCase().includes(q),
      )
      .slice(0, 50);
  }, [options, filterText]);

  function commitOption(o: SearchSelectOption) {
    onChange(o.value);
    setOpen(false);
    setQuery("");
    setActiveIndex(-1);
    onBlur?.();
  }

  function onListKeyDown(e: React.KeyboardEvent) {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      if (!open) {
        setOpen(true);
        return;
      }
      setActiveIndex((i) => Math.min(i + 1, filtered.length - 1));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setActiveIndex((i) => Math.max(i - 1, 0));
    } else if (e.key === "Enter") {
      if (open && activeIndex >= 0 && activeIndex < filtered.length) {
        e.preventDefault();
        commitOption(filtered[activeIndex]);
      }
    } else if (e.key === "Escape") {
      if (open) {
        e.preventDefault();
        setOpen(false);
        setActiveIndex(-1);
      }
    }
  }

  // Texto del disparador NO editable (modo botón).
  const displayText = selected ? selected.label : "";

  const triggerCommon =
    "flex h-10 w-full items-center justify-between rounded-md border border-input-border bg-[var(--input-surface,var(--input))] px-3 py-2 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50";

  return (
    <div ref={rootRef} className="relative">
      {allowCustomValue ? (
        // Modo combobox EDITABLE: el usuario escribe libre; el texto se reporta
        // por onChange tal cual y filtra la lista. Elegir una opción fija su
        // value (y muestra su etiqueta). La validez la comprueba el consumidor.
        <div className="relative">
          <input
            id={id}
            type="text"
            role="combobox"
            aria-expanded={open}
            aria-controls={listboxId}
            aria-autocomplete="list"
            aria-invalid={ariaInvalid}
            disabled={disabled || loading}
            value={selected ? selected.label : value}
            placeholder={loading ? "Cargando..." : placeholder}
            onChange={(e) => {
              setOpen(true);
              setActiveIndex(-1);
              setQuery(e.target.value);
              onChange(e.target.value);
            }}
            onFocus={() => setOpen(true)}
            onKeyDown={onListKeyDown}
            className={cn(triggerCommon, "pr-9")}
          />
          <span className="pointer-events-none absolute inset-y-0 right-0 flex items-center pr-3">
            {loading ? (
              <Loader2 className="size-4 animate-spin" aria-hidden />
            ) : (
              <ChevronsUpDown className="size-4 opacity-60" aria-hidden />
            )}
          </span>
        </div>
      ) : (
        <button
          id={id}
          type="button"
          disabled={disabled}
          data-invalid={ariaInvalid ? "true" : undefined}
          aria-haspopup="listbox"
          aria-expanded={open}
          aria-controls={listboxId}
          onClick={() => setOpen((v) => !v)}
          onKeyDown={onListKeyDown}
          className={cn(triggerCommon, !selected && "text-muted-foreground")}
        >
          <span className="truncate">
            {loading ? "Cargando..." : displayText || placeholder}
          </span>
          {loading ? (
            <Loader2 className="size-4 shrink-0 animate-spin" aria-hidden />
          ) : (
            <ChevronsUpDown className="size-4 shrink-0 opacity-60" aria-hidden />
          )}
        </button>
      )}

      {open && !disabled ? (
        <div
          data-popover
          className="absolute z-50 mt-1 w-full rounded-md border border-border bg-card shadow-lg"
        >
          {allowCustomValue ? null : (
            <div className="p-2">
              <Input
                autoFocus
                value={query}
                onChange={(e) => {
                  setQuery(e.target.value);
                  setActiveIndex(-1);
                }}
                onKeyDown={onListKeyDown}
                placeholder={placeholder}
                aria-label="Buscar"
              />
            </div>
          )}
          <ul
            id={listboxId}
            role="listbox"
            className="max-h-60 overflow-auto p-1"
            aria-label="Opciones"
          >
            {filtered.length === 0 ? (
              <li className="px-3 py-2 text-sm text-muted-foreground">
                {emptyText}
              </li>
            ) : (
              filtered.map((o, i) => {
                const isSel = o.value === value;
                const isActive = i === activeIndex;
                return (
                  <li key={o.value}>
                    <button
                      type="button"
                      role="option"
                      aria-selected={isSel}
                      onMouseEnter={() => setActiveIndex(i)}
                      onClick={() => commitOption(o)}
                      className={cn(
                        "flex w-full items-start gap-2 rounded-sm px-3 py-2 text-left text-sm hover:bg-accent",
                        (isSel || isActive) && "bg-accent/60",
                      )}
                    >
                      <Check
                        className={cn(
                          "mt-0.5 size-4 shrink-0",
                          isSel ? "opacity-100" : "opacity-0",
                        )}
                        aria-hidden
                      />
                      <span className="flex-1">
                        <span className="block">{o.label}</span>
                        {o.hint ? (
                          <span className="block text-xs text-muted-foreground">
                            {o.hint}
                          </span>
                        ) : null}
                      </span>
                    </button>
                  </li>
                );
              })
            )}
          </ul>
        </div>
      ) : null}
    </div>
  );
}

export default SearchSelect;
