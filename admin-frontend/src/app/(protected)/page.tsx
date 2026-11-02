"use client";

import * as React from "react";
import Link from "next/link";

import {
  Card,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { usePermissions, useModulePermissions } from "@/lib/use-permissions";
import { getCountry, subscribeCountry } from "@/lib/country";
import { useVisibleModules } from "@/features/modules/hooks";
import { selectVisibleNavEntries } from "@/features/navigation/nav-items";
import { usePageTitle } from "@/lib/page-chrome";

/**
 * Vista principal del panel protegido (`/`).
 *
 * Da la bienvenida y ofrece accesos directos DINÁMICOS: una tarjeta por cada
 * recurso o módulo que el usuario puede ver, derivada de la MISMA lógica de
 * permisos que el menú lateral (`selectVisibleNavEntries`), de modo que el HOME
 * y el sidebar nunca discrepan. Incluye los recursos fijos del catálogo
 * (Customers, Plans, Subscriptions, Cards, Usuarios, Roles, Auditoría), las
 * entradas reservadas (Constructor de módulos y Conexiones) y los módulos
 * dinámicos del constructor, condicionados al país seleccionado. La seguridad
 * real la aplica siempre el backend; esto es sólo visibilidad de UI.
 */

/**
 * Lee el país seleccionado de forma reactiva y SSR-safe (igual que el selector
 * de país y el sidebar). En el servidor y el primer render del cliente devuelve
 * `""` para evitar desajustes de hidratación; tras montar refleja la selección.
 */
function useSelectedCountry(): string {
  return React.useSyncExternalStore(
    subscribeCountry,
    () => getCountry() ?? "",
    () => "",
  );
}

export default function Home() {
  usePageTitle("Panel Admin");
  const perms = usePermissions();
  const modperms = useModulePermissions();
  const country = useSelectedCountry();
  const { data: dynamicModules } = useVisibleModules();

  const entries = selectVisibleNavEntries({
    perms,
    modperms,
    country,
    modules: dynamicModules ?? [],
  });

  return (
    <div className="mx-auto w-full max-w-5xl space-y-8">
      <p className="text-sm text-muted-foreground">
        Accede a los módulos y recursos para los que tienes permiso.
      </p>

      {entries.length === 0 ? (
        <Card>
          <CardHeader>
            <CardDescription>
              No tienes módulos o recursos asignados.
            </CardDescription>
          </CardHeader>
        </Card>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2">
          {entries.map((entry) => {
            const Icon = entry.icon;
            return (
              <Link
                key={entry.key}
                href={entry.href}
                className="rounded-lg transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                <Card className="h-full transition-colors hover:bg-accent/40">
                  <CardHeader>
                    <div className="flex items-center gap-3">
                      <span className="flex size-9 items-center justify-center rounded-md bg-accent text-accent-foreground">
                        <Icon className="size-5" aria-hidden="true" />
                      </span>
                      <CardTitle>{entry.label}</CardTitle>
                    </div>
                    <CardDescription>{entry.description}</CardDescription>
                  </CardHeader>
                </Card>
              </Link>
            );
          })}
        </div>
      )}
    </div>
  );
}
