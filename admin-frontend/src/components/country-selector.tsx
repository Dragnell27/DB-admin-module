"use client";

/**
 * Selector de país del panel (multi-país).
 *
 * Permite al administrador elegir sobre qué país OTT consultar. El país elegido
 * se persiste (lib/country) y el cliente HTTP lo envía como `X-Country` en cada
 * petición al BFF. Al cambiar de país se invalidan todas las queries para que
 * las vistas (Customers/Plans/Subscriptions/Cards) se recarguen con los datos
 * del país seleccionado.
 *
 * Las opciones provienen de `GET /meta/countries` (solo países con credenciales
 * configuradas en el backend) y se restringen además a los países para los que
 * el usuario tiene al menos un permiso (`allowedCountries`, Req. 9.4). Si tras
 * el filtrado queda un único país, el selector se muestra deshabilitado como
 * indicador. Si aún no hay país seleccionado, se adopta el país activo por
 * defecto que informa el backend cuando esté permitido, o el primer país
 * permitido en caso contrario. Si el país seleccionado deja de estar permitido,
 * se repliega al primer país permitido. El país elegido se persiste (lib/country)
 * y el cliente HTTP lo envía como `X-Country` (Req. 9.5).
 */
import * as React from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Globe } from "lucide-react";

import {
  countryLabel,
  getCountry,
  setCountry,
  subscribeCountry,
} from "@/lib/country";
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

export function CountrySelector() {
  const queryClient = useQueryClient();
  const selected = useSelectedCountry();
  const { data, isLoading, isError } = useCountriesMeta();
  const perms = usePermissions();

  // Evita el desajuste de hidratación: el selector depende de datos que solo
  // existen en el cliente (localStorage del país + la query a /meta/countries),
  // por lo que el render del servidor y el primer render del cliente podrían
  // diferir. Hasta que el componente se monta en el cliente, renderizamos un
  // marcador neutro idéntico al del servidor; tras montar, mostramos el selector.
  const [mounted, setMounted] = React.useState(false);
  React.useEffect(() => {
    setMounted(true);
  }, []);

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

  const handleChange = React.useCallback(
    (event: React.ChangeEvent<HTMLSelectElement>) => {
      const next = event.target.value;
      if (!next || next === selected) {
        return;
      }
      setCountry(next);
      // Recargar los datos del nuevo país: invalida todas las queries de recurso.
      void queryClient.invalidateQueries();
    },
    [queryClient, selected],
  );

  // Antes de montar en el cliente (y durante la carga inicial) mostramos un
  // marcador neutro estable. El servidor renderiza exactamente esto, de modo
  // que la hidratación coincide y no hay desajuste.
  if (!mounted || isLoading) {
    return (
      <div
        className="flex items-center gap-2 text-sm text-muted-foreground"
        suppressHydrationWarning
      >
        <Globe className="size-4" aria-hidden="true" />
        <span>Cargando países…</span>
      </div>
    );
  }

  if (isError || countries.length === 0) {
    return null;
  }

  // El valor mostrado debe ser siempre una de las opciones renderizadas (los
  // países permitidos). Si la selección aún no está en el conjunto permitido
  // (el efecto de normalización se ejecuta tras el render), se usa un país
  // permitido como valor para que el <select> sea controlado y coherente.
  const value =
    selected && countries.includes(selected)
      ? selected
      : data?.active && countries.includes(data.active)
        ? data.active
        : countries[0];
  const onlyOne = countries.length === 1;

  return (
    <div className="flex items-center gap-2">
      <Globe className="size-4 text-muted-foreground" aria-hidden="true" />
      <label htmlFor="country-selector" className="sr-only">
        País
      </label>
      <select
        id="country-selector"
        value={value}
        onChange={handleChange}
        disabled={onlyOne}
        aria-label="Seleccionar país"
        className="h-9 rounded-md border border-input bg-input px-2 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-70"
      >
        {countries.map((code) => (
          <option key={code} value={code}>
            {countryLabel(code)}
          </option>
        ))}
      </select>
    </div>
  );
}

export default CountrySelector;
