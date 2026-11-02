"use client";

import * as React from "react";
import { Check, Copy } from "lucide-react";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

/**
 * Muestra de forma destacada el identificador de un recurso (customer_id,
 * plan_id, subscription_id) en una superficie elevada, con una etiqueta y un
 * botón para copiarlo al portapapeles. Pensado para la cabecera de las páginas
 * de detalle (Req. 1.4, 1.5).
 */
export interface CopyableIdProps {
  /** Etiqueta descriptiva (p. ej. "Customer ID"). */
  readonly label: string;
  /** Valor del identificador. */
  readonly value: string;
  readonly className?: string;
}

export function CopyableId({ label, value, className }: CopyableIdProps) {
  const [copied, setCopied] = React.useState(false);

  const handleCopy = React.useCallback(async () => {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2000);
    } catch {
      // Si el navegador bloquea el portapapeles, el usuario puede copiar a mano.
    }
  }, [value]);

  return (
    <div
      className={cn(
        "flex items-center justify-between gap-3 rounded-md border border-border bg-popover px-4 py-3",
        className,
      )}
    >
      <div className="min-w-0 space-y-1">
        <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
          {label}
        </p>
        <p className="truncate font-mono text-sm text-foreground" title={value}>
          {value}
        </p>
      </div>
      <Button
        type="button"
        variant="outline"
        size="sm"
        onClick={handleCopy}
        aria-label={`Copiar ${label}`}
      >
        {copied ? <Check aria-hidden="true" /> : <Copy aria-hidden="true" />}
        {copied ? "Copiado" : "Copiar"}
      </Button>
    </div>
  );
}

export default CopyableId;
