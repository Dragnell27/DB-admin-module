"use client";

/**
 * Éxito tras crear/editar un rol: las páginas de formulario (`roles/new`,
 * `roles/edit`) redirigen a `/roles?created=1`/`?updated=1`. Este componente lee
 * ese flag de query y dispara UN toast de éxito (no un banner inline, que
 * quedaba fuera de pantalla si la lista estaba scrolleada), luego limpia el
 * parámetro con `router.replace`. No renderiza nada.
 *
 * Vive en su propio módulo (y no dentro de `page.tsx`) porque Next.js solo
 * permite exports concretos en un `page.tsx`; extraerlo aquí permite importarlo
 * desde la página y, además, testearlo aislado (`roles-success-toast.test.tsx`).
 *
 * Usa `useSearchParams`, por lo que el consumidor lo envuelve en <Suspense>.
 */
import * as React from "react";
import { useRouter, useSearchParams } from "next/navigation";

import { toast } from "@/lib/use-toast";

export function RolesSuccessToast() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const created = searchParams.get("created") === "1";
  const updated = searchParams.get("updated") === "1";

  // Guard de idempotencia: en React Strict Mode (dev) el efecto se ejecuta dos
  // veces, y `toast.success` se dispara antes de que `router.replace` quite el
  // `?created=1`/`?updated=1`, lo que mostraba el toast DUPLICADO. El ref
  // asegura una sola emisión aunque el efecto corra más de una vez (mismo patrón
  // que `SavedToast` de Conexiones y Módulos).
  const emitted = React.useRef(false);
  React.useEffect(() => {
    if ((created || updated) && !emitted.current) {
      emitted.current = true;
      toast.success(
        created
          ? "Rol creado correctamente."
          : "Rol actualizado correctamente.",
      );
      router.replace("/roles");
    }
  }, [created, updated, router]);

  return null;
}

export default RolesSuccessToast;
