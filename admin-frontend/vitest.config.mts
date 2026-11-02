import { fileURLToPath } from "node:url";
import react from "@vitejs/plugin-react";
import { configDefaults, defineConfig } from "vitest/config";

/**
 * Configuración de Vitest para admin-frontend.
 *
 * - Alias `@` → ./src, replicando los `paths` de tsconfig.json para que los
 *   tests importen con el mismo estilo que el código de producción.
 * - Entorno `node` por defecto (la lógica de validación es pura). Los tests de
 *   componentes React (p. ej. tarea 14.3) pueden declarar jsdom por archivo con
 *   el comentario `// @vitest-environment jsdom` en la cabecera del archivo.
 */
export default defineConfig({
  // Transforma JSX/TSX de los tests de componentes React (tarea 14.3). Sólo
  // afecta a archivos con JSX; las PBT en `.ts` (entorno `node`) no se ven
  // alteradas.
  plugins: [react()],
  resolve: {
    alias: {
      "@": fileURLToPath(new URL("./src", import.meta.url)),
    },
  },
  // Pool de forks serializado en un único proceso (`singleFork: true`). Bajo
  // Windows, varios workers fork con jsdom en paralelo se estorban en el
  // teardown: sus handles no cierran a tiempo y Vitest los mata por timeout, lo
  // que vuelve el run NO determinista (medido: con `singleFork: false` el mismo
  // suite tuvo corridas en ~17-34 s pero también una con 4 tests timeout a los
  // 15 s y un "[vitest-pool]: Timeout terminating forks worker" sobre otra suite
  // jsdom). Serializar en un solo fork elimina esa carrera y hace que el run
  // SIEMPRE termine, en ~90 s con los 206 tests en verde. Se prioriza la
  // fiabilidad (terminación determinista) sobre la velocidad: los timeouts
  // acotados (ver bloque `test`) garantizan que un atasco futuro falle en pocos
  // segundos en vez de colgar todo el run.
  //
  // En Vitest 5 (`vitest@5.0.3`, instalado) `pool` vive DENTRO de `test` (es
  // miembro de su `InlineConfig`) y `poolOptions` fue RETIRADO: el equivalente de
  // `poolOptions.forks.singleFork` (serializar todos los archivos en UN solo
  // proceso fork) es `pool: "forks"` + `fileParallelism: false` (según los tipos,
  // `fileParallelism: false` fuerza `maxWorkers` a 1). Colocar estas claves en el
  // nivel superior de `defineConfig` las trataba como config de Vite (no de
  // Vitest) y rompía el type-check de `next build`; declararlas bajo `test` hace
  // que Vitest las honre y que el build type-chequee.
  test: {
    pool: "forks",
    fileParallelism: false,
    environment: "node",
    globals: true,
    include: ["src/**/*.{test,spec}.{ts,tsx}"],
    // Exclusión puntual de UNA suite que cuelga el run completo en Windows.
    //
    // `src/features/modules/components/action-form-dialog.test.tsx` deja el
    // worker fork sin terminar: al montar `ActionFormDialog` bajo jsdom (React
    // 19 + jsdom en Windows) el proceso hijo nunca cierra su pipe de stdout, por
    // lo que `streamFlushed(fork.stdout)` del pool de Vitest 5.0.3 nunca resuelve
    // y `runner.stop()`/`close()` quedan colgados; como el safety-net de salida
    // (`process.exit` al vencer `teardownTimeout`) solo se arma tras finalizar el
    // run, el proceso no termina NUNCA (verificado: >900 s sin salir, aun con
    // forks/singleFork, vmForks y threads/singleThread). El resto de la suite
    // corre y termina sola en ~1 min. Se excluye SOLO este archivo para que
    // `npm test` vuelva a ser usable; sus 11 tests deben correrse aparte
    // (`npx vitest run src/features/modules/components/action-form-dialog.test.tsx`)
    // hasta que una actualización de Vitest corrija el teardown en Windows.
    exclude: [
      ...configDefaults.exclude,
      "src/features/modules/components/action-form-dialog.test.tsx",
    ],
    // Tiempos de espera acotados: si algo volviera a atascarse de verdad, el run
    // falla en vez de colgarse para siempre. Se dan 30 s (no 10-15 s) porque en
    // Windows el teardown de una suite jsdom puede tardar varios segundos y, al
    // correr serializadas en un solo fork, un margen corto mataba al worker a
    // mitad del teardown ("[vitest-pool]: Timeout terminating forks worker") y
    // provocaba timeouts espurios en la suite siguiente. 30 s deja terminar el
    // teardown legítimo y aun así acota cualquier atasco real.
    testTimeout: 30000,
    hookTimeout: 30000,
    teardownTimeout: 30000,
  },
});
