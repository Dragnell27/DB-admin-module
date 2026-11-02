"use client";

/**
 * Wrapper cliente que monta la inicialización de país para TODA el área
 * protegida. El layout `(protected)` es un Server Component y no puede usar
 * hooks de cliente; este componente minúsculo ejecuta `useCountryBootstrap` y no
 * pinta nada (devuelve `null`), de modo que el árbol SSR y el primer render de
 * cliente son idénticos (sin desajuste de hidratación).
 */
import { useCountryBootstrap } from "@/features/meta/use-country-bootstrap";

export function CountryBootstrap(): null {
  useCountryBootstrap();
  return null;
}

export default CountryBootstrap;
