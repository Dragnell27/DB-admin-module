/**
 * Rutas de RECURSOS OTT del panel, es decir, las pantallas que operan datos de
 * un país concreto y por tanto necesitan el SELECTOR DE PAÍS en la topbar (y
 * envían la cabecera `X-Country`): clientes, tarjetas, planes y suscripciones.
 *
 * El constructor de módulos de BD (`/module-defs`), las conexiones
 * (`/connection-templates`), los roles (`/roles`), los usuarios (`/users`), la
 * auditoría (`/audit`), los módulos dinámicos (`/modules/*`) y el inicio (`/`)
 * NO son recursos OTT por país: ahí el selector de país no aplica y se oculta.
 *
 * Se decide por la ruta (prefijo del `pathname`), no por estado de app, para que
 * sea puro y testeable sin React.
 */

/** Prefijos de ruta de los recursos OTT que usan el selector de país. */
export const OTT_RESOURCE_PREFIXES = [
  "/customers",
  "/cards",
  "/plans",
  "/subscriptions",
] as const;

/**
 * Indica si en `pathname` debe mostrarse el selector de país: true solo cuando
 * la ruta es (o cuelga de) uno de los recursos OTT por país. Coincidencia por
 * segmento: `/customers` y `/customers/new` cuentan, pero una ruta hipotética
 * `/customers-algo` NO (evita falsos positivos por prefijo textual). Función
 * pura; `pathname` suele venir de `usePathname()`.
 */
export function showsCountrySelector(pathname: string | null): boolean {
  if (!pathname) {
    return false;
  }
  return OTT_RESOURCE_PREFIXES.some(
    (prefix) => pathname === prefix || pathname.startsWith(`${prefix}/`),
  );
}
