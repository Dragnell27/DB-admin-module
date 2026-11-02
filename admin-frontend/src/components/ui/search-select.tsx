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
}: SearchSelectProps) {
  const [open, setOpen] = React.useState(false);
  const [query, setQuery] = React.useState("");
  const rootRef = React.useRef<HTMLDivElement | null>(null);

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

  const filtered = React.useMemo(() => {
    const q = query.trim().toLowerCase();
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
  }, [options, query]);

  const displayText = selected ? selected.label : "";

  return (
    <div ref={rootRef} className="relative">
      <button
        id={id}
        type="button"
        disabled={disabled}
        data-invalid={ariaInvalid ? "true" : undefined}
        aria-haspopup="listbox"
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
        className={cn(
          "flex h-10 w-full items-center justify-between rounded-md border border-input bg-[var(--input-surface,var(--input))] px-3 py-2 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50",
          !selected && "text-muted-foreground",
        )}
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

      {open && !disabled ? (
        <div
          data-popover
          className="absolute z-50 mt-1 w-full rounded-md border border-border bg-card shadow-lg"
        >
          <div className="p-2">
            <Input
              autoFocus
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder={placeholder}
              aria-label="Buscar"
            />
          </div>
          <ul
            role="listbox"
            className="max-h-60 overflow-auto p-1"
            aria-label="Opciones"
          >
            {filtered.length === 0 ? (
              <li className="px-3 py-2 text-sm text-muted-foreground">
                {emptyText}
              </li>
            ) : (
              filtered.map((o) => {
                const isSel = o.value === value;
                return (
                  <li key={o.value}>
                    <button
                      type="button"
                      role="option"
                      aria-selected={isSel}
                      onClick={() => {
                        onChange(o.value);
                        setOpen(false);
                        setQuery("");
                        onBlur?.();
                      }}
                      className={cn(
                        "flex w-full items-start gap-2 rounded-sm px-3 py-2 text-left text-sm hover:bg-accent",
                        isSel && "bg-accent/60",
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
