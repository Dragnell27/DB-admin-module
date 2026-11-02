"use client";

/**
 * Vista de Cards (tarea 16.4; actualizada en tarea 10.1).
 *
 * Flujo: el administrador debe seleccionar primero un cliente por `customer_id`
 * o `email` (Req. 17.1, 17.2). Hasta que confirme la selección, se muestra una
 * indicación en lugar de la tabla. Una vez seleccionado, se listan sus tarjetas
 * con `CardsTable` (Req. 17.1) y, para el rol `admin`, se habilitan la creación
 * (navegando a `/cards/new`, Req. 3.1) y la eliminación por fila (Req. 19).
 *
 * El botón "Nueva tarjeta" ya no abre un formulario embebido: navega a la ruta
 * propia `/cards/new`, propagando el cliente seleccionado por la query string
 * (`customer_id` o `email`) (Req. 3.1). Al volver con `?created=1` se muestra
 * un banner de éxito (Req. 7.1, 7.2, 7.4).
 *
 * La retroalimentación de éxito/error se da con banners inline (no un provider
 * de toasts compartido), como un alert simple (Req. 18.9, 18.10, 19.4, 19.5).
 */
import * as React from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { CreditCard, Plus, Search, X } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { FeedbackBanner } from "@/components/feedback-banner";
import { getCountry, subscribeCountry } from "@/lib/country";
import { can } from "@/lib/permissions";
import { usePermissions } from "@/lib/use-permissions";
import type { CardCustomerSelector } from "@/features/cards/hooks";

import { CardsTable } from "./components/cards-table";

/** Lee el país seleccionado de forma reactiva y SSR-safe (igual que el selector). */
function useSelectedCountry(): string | null {
  return React.useSyncExternalStore(
    subscribeCountry,
    () => getCountry(),
    () => null,
  );
}

/** Criterio de búsqueda del cliente. */
type SelectorKind = "customer_id" | "email";

/** Banner de retroalimentación inline. */
interface Feedback {
  readonly kind: "success" | "error";
  readonly message: string;
}

/**
 * Banner de éxito mostrado al volver de `/cards/new` con `?created=1`
 * (Req. 7.1, 7.2, 7.4). Lee `useSearchParams`, por lo que se renderiza dentro
 * de un <Suspense> (requisito de Next.js App Router).
 */
function CardsCreatedBanner() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const created = searchParams.get("created") === "1";

  if (!created) {
    return null;
  }

  return (
    <FeedbackBanner
      tone="success"
      message="Tarjeta creada correctamente."
      onDismiss={() => router.replace("/cards")}
    />
  );
}

export default function CardsPage() {
  const router = useRouter();
  const perms = usePermissions();
  const country = useSelectedCountry() ?? "";
  // Visibilidad de controles de escritura según permiso en el país seleccionado
  // (Req. 9.1, 9.6). Cards no tiene "editar". La seguridad real la aplica el BFF.
  const canCrear = can(perms, "cards", "crear", country);
  const canEliminar = can(perms, "cards", "eliminar", country);

  // Estado del formulario de selección (lo que el admin está escribiendo).
  const [kind, setKind] = React.useState<SelectorKind>("customer_id");
  const [value, setValue] = React.useState("");

  // Selector confirmado que alimenta la tabla (null = aún sin seleccionar).
  const [selector, setSelector] = React.useState<CardCustomerSelector | null>(
    null,
  );
  const [feedback, setFeedback] = React.useState<Feedback | null>(null);

  const trimmed = value.trim();
  const canApply = trimmed !== "";

  const applySelector = React.useCallback(
    (event: React.FormEvent) => {
      event.preventDefault();
      if (!canApply) {
        return;
      }
      setSelector(
        kind === "customer_id"
          ? { customer_id: trimmed }
          : { email: trimmed },
      );
      setFeedback(null);
    },
    [canApply, kind, trimmed],
  );

  const clearSelector = React.useCallback(() => {
    setSelector(null);
    setValue("");
    setFeedback(null);
  }, []);

  /**
   * Navega a la ruta de creación `/cards/new` propagando el cliente
   * seleccionado por la query string (Req. 3.1). Se prefiere `customer_id`;
   * si el selector es por correo, se envía `email`.
   */
  const goToCreate = React.useCallback(() => {
    if (!selector) {
      return;
    }
    const query = new URLSearchParams();
    if (selector.customer_id) {
      query.set("customer_id", selector.customer_id);
    } else if (selector.email) {
      query.set("email", selector.email);
    }
    router.push(`/cards/new?${query.toString()}`);
  }, [router, selector]);

  return (
    <div className="mx-auto w-full max-w-5xl space-y-6">
      <div className="space-y-1">
        <h1 className="text-2xl font-semibold text-foreground">Cards</h1>
        <p className="text-sm text-muted-foreground">
          Selecciona un cliente para revisar y administrar sus medios de pago.
        </p>
      </div>

      {/* Selector de cliente (Req. 17.1, 17.2) */}
      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Seleccionar cliente</CardTitle>
          <CardDescription>
            Indica el ID de cliente o el correo para consultar sus tarjetas.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form
            onSubmit={applySelector}
            className="flex flex-col gap-3 sm:flex-row sm:items-end"
          >
            <div className="space-y-1.5">
              <Label htmlFor="selector-kind">Buscar por</Label>
              <select
                id="selector-kind"
                value={kind}
                onChange={(event) =>
                  setKind(event.target.value as SelectorKind)
                }
                className="h-10 w-full rounded-md border border-input bg-input px-3 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring sm:w-44"
              >
                <option value="customer_id">ID de cliente</option>
                <option value="email">Correo electrónico</option>
              </select>
            </div>

            <div className="flex-1 space-y-1.5">
              <Label htmlFor="selector-value">
                {kind === "customer_id" ? "ID de cliente" : "Correo electrónico"}
              </Label>
              <Input
                id="selector-value"
                value={value}
                onChange={(event) => setValue(event.target.value)}
                placeholder={
                  kind === "customer_id"
                    ? "UUID del cliente"
                    : "cliente@correo.com"
                }
                type={kind === "email" ? "email" : "text"}
              />
            </div>

            <div className="flex items-center gap-2">
              <Button type="submit" disabled={!canApply}>
                <Search aria-hidden="true" />
                Consultar
              </Button>
              {selector ? (
                <Button
                  type="button"
                  variant="outline"
                  onClick={clearSelector}
                  aria-label="Limpiar selección de cliente"
                >
                  <X aria-hidden="true" />
                  Limpiar
                </Button>
              ) : null}
            </div>
          </form>
        </CardContent>
      </Card>

      {/* Banner de éxito al volver de /cards/new con ?created=1 (Req. 7.1, 7.2, 7.4) */}
      <React.Suspense fallback={null}>
        <CardsCreatedBanner />
      </React.Suspense>

      {/* Banner de retroalimentación inline (Req. 18.9/18.10, 19.4/19.5) */}
      {feedback ? (
        <div
          role="alert"
          className={
            feedback.kind === "success"
              ? "rounded-md border border-primary/40 bg-primary/10 px-3 py-2 text-sm text-foreground"
              : "rounded-md border border-destructive/50 bg-destructive/10 px-3 py-2 text-sm text-destructive"
          }
        >
          {feedback.message}
        </div>
      ) : null}

      {selector ? (
        <>
          {/* Acción de creación, visible solo con permiso `crear` (Req. 9.1) */}
          {canCrear ? (
            <div className="flex justify-end">
              <Button type="button" onClick={goToCreate}>
                <Plus aria-hidden="true" />
                Nueva tarjeta
              </Button>
            </div>
          ) : null}

          <CardsTable
            selector={selector}
            canWrite={canEliminar}
            onDeleteSuccess={() =>
              setFeedback({
                kind: "success",
                message: "Tarjeta eliminada correctamente.",
              })
            }
            onDeleteError={(message) =>
              setFeedback({ kind: "error", message })
            }
          />
        </>
      ) : (
        /* Prompt antes de seleccionar un cliente (Req. 17.2) */
        <div className="flex flex-col items-center gap-2 rounded-lg border border-dashed border-border bg-card/50 px-6 py-16 text-center">
          <CreditCard
            className="size-8 text-muted-foreground"
            aria-hidden="true"
          />
          <p className="text-sm font-medium text-foreground">
            Selecciona un cliente para ver sus tarjetas
          </p>
          <p className="text-sm text-muted-foreground">
            Ingresa un ID de cliente o un correo electrónico y pulsa
            &ldquo;Consultar&rdquo;.
          </p>
        </div>
      )}
    </div>
  );
}
