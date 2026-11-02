"use client";

/**
 * Inicialización/normalización del país seleccionado a nivel del área protegida.
 *
 * El BFF opera por petición sobre un país OTT (cabecera `X-Country`). El home y
 * el sidebar filtran cada entrada por país con coincidencia exacta (`can()` /
 * `canModule()` de `lib/permissions`), de modo que si en el arranque no hay país
 * seleccionado (`getCountry()` devuelve `null`) todo desaparece aunque el usuario
 * tenga permisos. La única pieza que auto-seleccionaba el país era un `useEffect`
 * dentro de `CountrySelector`, que solo se monta en rutas OTT: en `/` nunca
 * corría y el dashboard quedaba vacío.
 *
 * Este hook EXTRAE esa lógica de auto-selección para poder montarla una vez en
 * el layout `(protected)` (vía `CountryBootstrap`) y que corra en TODAS las rutas
 * protegidas, incluida `/`, sin tener que renderizar el selector. No renderiza
 * nada: solo efectos. Es SSR-safe (el cambio de país ocurre dentro de un
 * `useEffect`, nunca durante el render) y no sobrescribe una selección existente.
 *
 * Mostrar el selector (solo en rutas OTT) y fijar el país por defecto quedan así
 * desacoplados.
 */
import * as React from "react";

import { getCountry, setCountry, subscribeCountry } from "@/lib/country";
import { useCountriesMeta } from "@/features/meta/hooks";
import { allowedCountries } from "@/lib/permissions";
import { usePermissions } from "@/lib/use-permissions";

/** Lee el país actual de forma reactiva y SSR-safe. */
function useSelectedCountry(): string | null {
  return React.useSyncExternalStore(
    subscribeCountry,
    () => getCountry(),
    () => null, // en el servidor no hay selección
  );
}

/**
 * Inicializa/normaliza el país seleccionado contra el conjunto permitido:
 *  - si aún no hay países permitidos (permisos sin cargar o usuario sin países)
 *    no hace nada, evitando forzar un país inválido;
 *  - si ya hay una selección válida, NO la sobrescribe;
 *  - si no, adopta el país activo por defecto del backend (si está permitido) o,
 *    en su defecto, el primer país permitido.
 */
export function useCountryBootstrap(): void {
  const selected = useSelectedCountry();
  const { data } = useCountriesMeta();
  const perms = usePermissions();

  // Países configurados por el backend (con credenciales OTT), restringidos a
  // los que el usuario puede ver según sus permisos efectivos (Req. 9.4).
  // `allowedCountries` normaliza a minúsculas, ordena y deduplica.
  const countries = React.useMemo(
    () => allowedCountries(perms, data?.countries ? [...data.countries] : []),
    [perms, data],
  );

  // Normalizar la selección frente al conjunto permitido:
  //  - si no hay selección aún, adoptar el país activo por defecto del backend
  //    (si está permitido) o el primer país permitido;
  //  - si la selección actual ya no está permitida (p. ej. cambió la sesión),
  //    replegarse al primer país permitido.
  // Mientras no haya países permitidos (sin permisos aún / cargando) no se hace
  // nada, evitando escrituras innecesarias.
  React.useEffect(() => {
    if (countries.length === 0) {
      return;
    }
    if (selected && countries.includes(selected)) {
      return;
    }
    const fallback =
      !selected && data?.active && countries.includes(data.active)
        ? data.active
        : countries[0];
    if (fallback && fallback !== selected) {
      setCountry(fallback);
    }
  }, [selected, countries, data?.active]);
}
