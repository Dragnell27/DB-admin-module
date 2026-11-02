/**
 * Script anti-FOUC del tema CLARO/OSCURO del panel (Entrega A).
 *
 * Es un IIFE en forma de STRING que el `RootLayout` inyecta como primer hijo
 * del `<head>` (vía `dangerouslySetInnerHTML`) para que se ejecute de forma
 * SÍNCRONA antes del primer paint, evitando el parpadeo (FOUC) al recargar.
 *
 * Lee la preferencia de `localStorage['panel-admin-ott.theme']`, la normaliza a
 * `"light"`/`"dark"` con DEFECTO `"dark"` (no sigue `prefers-color-scheme`),
 * aplica/retira la clase `dark` en `<html>` y fija `color-scheme`. Si el
 * almacenamiento está bloqueado, cae al defecto oscuro.
 *
 * Se mantiene como string (y en su propio módulo) para poder testearlo —que
 * contiene la clave y el defecto— sin ejecutarlo, y para no mezclar el IIFE con
 * el JSX del layout. La clave DEBE coincidir con `THEME_KEY` de `lib/theme.ts`.
 */
export const THEME_INIT = `
(function () {
  try {
    var KEY = 'panel-admin-ott.theme';
    var stored = window.localStorage.getItem(KEY);
    // Defecto OSCURO: si no hay preferencia guardada, se usa 'dark'.
    var theme = (stored === 'light' || stored === 'dark') ? stored : 'dark';
    var root = document.documentElement;
    root.classList.toggle('dark', theme === 'dark');
    root.style.colorScheme = theme === 'dark' ? 'dark' : 'light';
  } catch (e) {
    // Almacenamiento bloqueado: caer al defecto oscuro.
    document.documentElement.classList.add('dark');
    document.documentElement.style.colorScheme = 'dark';
  }
})();
`;
