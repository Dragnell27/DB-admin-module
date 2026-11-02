/**
 * Modelo de permisos del panel de administración (RBAC) en el cliente.
 *
 * Este módulo es el espejo en el front del dominio de permisos del backend
 * (`internal/auth`). Define el catálogo cerrado de recursos y acciones, el tipo
 * de permisos efectivos y las funciones puras de evaluación que la UI usa para
 * decidir qué mostrar u ocultar.
 *
 * Importante: la autorización REAL la aplica siempre el backend (BFF). Lo que
 * hay aquí es sólo para visibilidad de la UI (ocultar botones, módulos y filtrar
 * el selector de país). No es una barrera de seguridad.
 *
 * El JWT de sesión transporta un claim `perms` en forma compacta
 * (`CompactPerms`): país → código de recurso (2 letras) → bitmask de acciones.
 * Este módulo sabe decodificarlo (`decodeCompactPerms`) para que `session.ts`
 * reconstruya los permisos efectivos sin tocar la red. Alternativamente, el
 * backend expone en `/auth/login` y `/auth/me` una lista explícita de ternas
 * que `permissionsFromList` normaliza al mismo tipo.
 */

/** Recursos del catálogo cerrado (Req. 1.2). Deben coincidir con el backend. */
export type Resource =
  | "customers"
  | "plans"
  | "subscriptions"
  | "cards"
  | "users"
  | "roles"
  | "audit"
  // El constructor de módulos (module-builder) es un recurso del catálogo
  // cerrado del backend (auth.ResourceModuleBuilder) que se otorga desde la
  // matriz de Roles, pero NO viaja en el bitmask `perms`: el backend lo enruta
  // al claim `modperms` bajo el slug reservado `__defs__`. Por eso aparece en
  // `type Resource` (para que la tabla fija de Roles y su schema lo acepten),
  // pero NO en RESOURCES/CODE_TO_RESOURCE/BIT_TO_ACTION (decodificación del
  // bitmask), ni en can()/allowedCountries(), que operan sobre `perms`.
  | "module-builder"
  // Plantillas de conexión (connection-templates): igual que module-builder, es
  // un recurso del catálogo cerrado que NO viaja en el bitmask `perms`; el
  // backend lo enruta al claim `modperms` bajo el slug reservado
  // `__connections__`. Aparece en `type Resource` para que la matriz de Roles y
  // su schema lo acepten, pero NO en RESOURCES/CODE_TO_RESOURCE/BIT_TO_ACTION.
  | "connections";

/** Acciones del catálogo cerrado (Req. 1.3). Deben coincidir con el backend. */
export type Action =
  | "ver"
  | "crear"
  | "editar"
  | "eliminar"
  | "cambiar"
  | "cancelar"
  | "recargar";

/**
 * Permiso atómico: la terna Recurso × Acción × País (Req. 1.1). El país se
 * almacena siempre normalizado a minúsculas.
 */
export interface Permission {
  resource: Resource;
  action: Action;
  country: string;
}

/**
 * Conjunto de permisos efectivos del usuario de la sesión. Se modela como una
 * simple lista de ternas de sólo lectura: es suficiente para el front y evita
 * estructuras internas complejas. Puede contener la unión de varios roles.
 */
export type EffectivePermissions = ReadonlyArray<Permission>;

/**
 * Catálogo de recursos válidos como conjunto, para validar entradas crudas.
 * Mantener sincronizado con el tipo `Resource` y con el backend.
 */
const RESOURCES: ReadonlySet<string> = new Set<Resource>([
  "customers",
  "plans",
  "subscriptions",
  "cards",
  "users",
  "roles",
  "audit",
]);

/**
 * Catálogo de acciones válidas como conjunto, para validar entradas crudas.
 * Mantener sincronizado con el tipo `Action` y con el backend.
 */
const ACTIONS: ReadonlySet<string> = new Set<Action>([
  "ver",
  "crear",
  "editar",
  "eliminar",
  "cambiar",
  "cancelar",
  "recargar",
]);

/**
 * Traducción de código de recurso compacto (2 letras) → Resource. Espejo exacto
 * de `codeToResource` del backend (`perm_codec.go`).
 */
const CODE_TO_RESOURCE: Readonly<Record<string, Resource>> = {
  cu: "customers",
  pl: "plans",
  su: "subscriptions",
  ca: "cards",
  us: "users",
  ro: "roles",
  au: "audit",
};

/**
 * Pares (bit, acción) del bitmask de acciones. Espejo exacto de `bitToAction`
 * del backend (`perm_codec.go`): ver=1, crear=2, editar=4, eliminar=8,
 * cambiar=16, cancelar=32.
 */
const BIT_TO_ACTION: ReadonlyArray<readonly [number, Action]> = [
  [1, "ver"],
  [2, "crear"],
  [4, "editar"],
  [8, "eliminar"],
  [16, "cambiar"],
  [32, "cancelar"],
  [64, "recargar"],
];

/** Normaliza un país a minúsculas y sin espacios sobrantes. */
function normalizeCountry(country: string): string {
  return country.trim().toLowerCase();
}

/** Comprueba si un valor crudo es un `Resource` válido del catálogo. */
function isResource(value: unknown): value is Resource {
  return typeof value === "string" && RESOURCES.has(value);
}

/** Comprueba si un valor crudo es un `Action` válido del catálogo. */
function isAction(value: unknown): value is Action {
  return typeof value === "string" && ACTIONS.has(value);
}

/** Comprueba si un valor es un objeto plano (no nulo, no array). */
function isPlainObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

/**
 * Decodifica el claim compacto `perms` del JWT a permisos efectivos (Req. 7.3).
 *
 * Recibe el valor CRUDO del claim (de tipo desconocido tras el parseo del JWT) y
 * lo expande a la lista de ternas, mapeando:
 *   - código de recurso de 2 letras → Resource (`CODE_TO_RESOURCE`),
 *   - cada bit activo del bitmask → Action (`BIT_TO_ACTION`).
 *
 * Es tolerante a entradas malformadas: cualquier parte no reconocida (país que
 * no es objeto, código de recurso desconocido, bitmask que no es número) se
 * ignora de forma segura; si la entrada completa es inválida devuelve `[]`.
 * El país se normaliza a minúsculas, consistente con el backend.
 *
 * Lo usa `session.ts` para construir `SessionUser.permissions`.
 */
export function decodeCompactPerms(perms: unknown): EffectivePermissions {
  if (!isPlainObject(perms)) {
    return [];
  }

  const result: Permission[] = [];

  for (const [rawCountry, byResource] of Object.entries(perms)) {
    // Cada país debe mapear a un objeto { códigoRecurso -> bitmask }.
    if (!isPlainObject(byResource)) {
      continue;
    }
    const country = normalizeCountry(rawCountry);

    for (const [code, rawMask] of Object.entries(byResource)) {
      const resource = CODE_TO_RESOURCE[code];
      if (resource === undefined) {
        continue; // código de recurso desconocido: se ignora
      }
      // El bitmask debe ser un entero finito; cualquier otra cosa se ignora.
      if (typeof rawMask !== "number" || !Number.isInteger(rawMask)) {
        continue;
      }
      const mask = rawMask;

      // Expande cada bit activo del bitmask a su acción.
      for (const [bit, action] of BIT_TO_ACTION) {
        if ((mask & bit) !== 0) {
          result.push({ resource, action, country });
        }
      }
    }
  }

  return result;
}

/**
 * Normaliza una lista explícita de permisos `[{resource, action, country}]` a
 * `EffectivePermissions` (p. ej. la que devuelven `/auth/login` o `/auth/me`).
 *
 * Valida cada entrada contra el catálogo cerrado: descarta las que no tengan un
 * `resource` y un `action` válidos o un `country` string. El país resultante se
 * normaliza a minúsculas. Ante una entrada que no sea array, devuelve `[]`.
 *
 * Útil cuando el front prefiere consumir la lista explícita en lugar del claim
 * compacto.
 */
export function permissionsFromList(list: unknown): EffectivePermissions {
  if (!Array.isArray(list)) {
    return [];
  }

  const result: Permission[] = [];

  for (const entry of list) {
    if (!isPlainObject(entry)) {
      continue;
    }
    const { resource, action, country } = entry;
    if (!isResource(resource) || !isAction(action) || typeof country !== "string") {
      continue; // entrada inválida: se descarta
    }
    result.push({ resource, action, country: normalizeCountry(country) });
  }

  return result;
}

/**
 * Evalúa si el usuario tiene permiso para la terna (recurso, acción, país).
 * Es el espejo en el front de `HasPermission` del backend (Req. 1.6, 9.1, 9.6).
 *
 * El país se compara en minúsculas. Devuelve `true` si existe un permiso con
 * exactamente ese recurso, esa acción y ese país.
 */
export function can(
  perms: EffectivePermissions,
  resource: Resource,
  action: Action,
  country: string,
): boolean {
  const target = normalizeCountry(country);
  return perms.some(
    (p) => p.resource === resource && p.action === action && p.country === target,
  );
}

/**
 * Devuelve los países de `configured` para los que el usuario tiene al menos un
 * permiso (intersección de los países configurados con los que aparecen en los
 * permisos efectivos) (Req. 9.4).
 *
 * El resultado está en minúsculas, ordenado alfabéticamente y sin duplicados.
 * Esto alimenta el filtrado de opciones del `CountrySelector`.
 */
export function allowedCountries(
  perms: EffectivePermissions,
  configured: string[],
): string[] {
  // Países con al menos un permiso (normalizados a minúsculas).
  const withPermission = new Set<string>();
  for (const p of perms) {
    withPermission.add(p.country);
  }

  // Intersección con los países configurados, sin duplicados.
  const allowed = new Set<string>();
  for (const code of configured) {
    const normalized = normalizeCountry(code);
    if (withPermission.has(normalized)) {
      allowed.add(normalized);
    }
  }

  return Array.from(allowed).sort();
}

/**
 * Reporta si el usuario tiene AL MENOS una acción sobre el recurso indicado.
 * Útil para ocultar módulos completos (p. ej. Usuarios/Roles) en el sidebar.
 *
 * Si se indica `country`, limita la comprobación a ese país (en minúsculas); si
 * se omite, basta con tener permiso sobre el recurso en cualquier país.
 */
export function hasAnyPermissionForResource(
  perms: EffectivePermissions,
  resource: Resource,
  country?: string,
): boolean {
  const target = country === undefined ? undefined : normalizeCountry(country);
  return perms.some(
    (p) =>
      p.resource === resource &&
      (target === undefined || p.country === target),
  );
}

// --- Permisos de módulos dinámicos (module-builder) ------------------------

/**
 * Permiso efectivo sobre un módulo dinámico creado con el constructor de
 * módulos (module-builder): la terna slug × acción × país. Es el espejo en el
 * front de `auth.ModulePermission` del backend. El país `"*"` denota un módulo
 * sin ámbito de país (country_scope = none); cualquier otro valor exige
 * coincidencia exacta.
 */
export interface ModulePermission {
  slug: string;
  action: string;
  country: string;
}

/** Conjunto de permisos de módulos dinámicos del usuario de la sesión. */
export type ModulePermissions = ReadonlyArray<ModulePermission>;

/** País canónico de un permiso de módulo sin ámbito (espejo de ModuleCountryAny). */
const MODULE_COUNTRY_ANY = "*";

/**
 * Decodifica el claim `modperms` del JWT (país → slug → lista de acciones) a la
 * lista plana de permisos de módulo. Espejo de `DecodeModulePermissions` del
 * backend. Es tolerante a entradas malformadas: cualquier parte no reconocida
 * se ignora de forma segura; una entrada inválida completa devuelve `[]`.
 */
export function decodeModulePerms(modperms: unknown): ModulePermissions {
  if (!isPlainObject(modperms)) {
    return [];
  }
  const result: ModulePermission[] = [];
  for (const [rawCountry, bySlug] of Object.entries(modperms)) {
    if (!isPlainObject(bySlug)) {
      continue;
    }
    // El país "*" se conserva tal cual; el resto se normaliza a minúsculas.
    const country =
      rawCountry === MODULE_COUNTRY_ANY ? MODULE_COUNTRY_ANY : normalizeCountry(rawCountry);
    for (const [slug, actions] of Object.entries(bySlug)) {
      if (!Array.isArray(actions)) {
        continue;
      }
      for (const action of actions) {
        if (typeof action === "string" && action !== "") {
          result.push({ slug, action, country });
        }
      }
    }
  }
  return result;
}

/**
 * Normaliza una lista explícita de permisos de módulo `[{slug, action, country}]`
 * (la que devuelven `/auth/login` y `/auth/me` en el campo `modules`) al tipo
 * `ModulePermissions`. Descarta entradas sin slug/action string. El país se
 * conserva ("*" para módulos sin ámbito; el resto en minúsculas).
 */
export function modulePermissionsFromList(list: unknown): ModulePermissions {
  if (!Array.isArray(list)) {
    return [];
  }
  const result: ModulePermission[] = [];
  for (const entry of list) {
    if (!isPlainObject(entry)) {
      continue;
    }
    const { slug, action, country } = entry;
    if (typeof slug !== "string" || slug === "" || typeof action !== "string" || action === "") {
      continue;
    }
    const rawCountry = typeof country === "string" ? country : "";
    result.push({
      slug,
      action,
      country: rawCountry === MODULE_COUNTRY_ANY ? MODULE_COUNTRY_ANY : normalizeCountry(rawCountry),
    });
  }
  return result;
}

/**
 * Evalúa si el usuario tiene permiso para (slug, acción, país) sobre un módulo
 * dinámico. Espejo de `HasModulePermission` del backend (Req. 11.5, 11.6):
 * un permiso almacenado como `"*"` (módulo sin ámbito de país) concede acceso
 * para cualquier país; en caso contrario, el país debe coincidir exactamente.
 * La autorización real la aplica el backend; esto es solo visibilidad de UI.
 */
export function canModule(
  perms: ModulePermissions,
  slug: string,
  action: string,
  country: string,
): boolean {
  const target = normalizeCountry(country);
  return perms.some(
    (p) =>
      p.slug === slug &&
      p.action === action &&
      (p.country === MODULE_COUNTRY_ANY || p.country === target),
  );
}

// --- Generador de contraseñas (espejo de la política del backend) ----------

/** Letras minúsculas. Espejo de `passwordLowercase` del backend. */
const PASSWORD_LOWERCASE = "abcdefghijklmnopqrstuvwxyz";
/** Letras mayúsculas. Espejo de `passwordUppercase` del backend. */
const PASSWORD_UPPERCASE = "ABCDEFGHIJKLMNOPQRSTUVWXYZ";
/** Dígitos decimales. Espejo de `passwordDigits` del backend. */
const PASSWORD_DIGITS = "0123456789";
/**
 * Conjunto de símbolos. Espejo de `passwordSymbols` del backend: evita comillas,
 * barras invertidas y espacios para reducir errores al copiar.
 */
const PASSWORD_SYMBOLS = "!@#$%^&*()-_=+[]{};:,.?";

/** Alfabeto combinado para el relleno. */
const PASSWORD_ALPHABET =
  PASSWORD_LOWERCASE + PASSWORD_UPPERCASE + PASSWORD_DIGITS + PASSWORD_SYMBOLS;

/**
 * Longitud por defecto de la contraseña generada. Debe ser siempre >= 16 según
 * la política; usamos 20 como en el backend.
 */
const GENERATED_PASSWORD_LENGTH = 20;

/**
 * Devuelve un índice aleatorio uniforme en [0, n) usando Web Crypto
 * (`crypto.getRandomValues`) como fuente de aleatoriedad.
 *
 * Para evitar el sesgo del módulo, se descartan los valores que caen fuera del
 * mayor múltiplo de `n` dentro del rango de 32 bits (muestreo por rechazo).
 */
function randomIndex(n: number): number {
  // Mayor múltiplo de n representable en 32 bits; valores por encima se rechazan.
  const limit = Math.floor(0x1_0000_0000 / n) * n;
  const buffer = new Uint32Array(1);
  let value = 0;
  do {
    crypto.getRandomValues(buffer);
    value = buffer[0];
  } while (value >= limit);
  return value % n;
}

/** Devuelve un carácter aleatorio del conjunto indicado. */
function randomChar(set: string): string {
  return set.charAt(randomIndex(set.length));
}

/**
 * Baraja un array en el sitio con Fisher-Yates usando `randomIndex`, de modo que
 * los caracteres obligatorios no queden siempre al principio.
 */
function shuffle(chars: string[]): void {
  for (let i = chars.length - 1; i > 0; i--) {
    const j = randomIndex(i + 1);
    [chars[i], chars[j]] = [chars[j], chars[i]];
  }
}

/**
 * Genera una contraseña aleatoria segura, espejo de la política del backend
 * (`GeneratePassword`): longitud `GENERATED_PASSWORD_LENGTH` (>= 16) y al menos
 * una minúscula, una mayúscula, un dígito y un símbolo (Req. 3.2, 13.1).
 *
 * Para garantizar que SIEMPRE cumple la política, toma primero un carácter de
 * cada clase obligatoria, rellena el resto desde el alfabeto combinado y por
 * último baraja el conjunto. Usa `crypto.getRandomValues` (Web Crypto),
 * disponible en el navegador, como fuente de aleatoriedad.
 */
export function generatePassword(): string {
  const classes = [
    PASSWORD_LOWERCASE,
    PASSWORD_UPPERCASE,
    PASSWORD_DIGITS,
    PASSWORD_SYMBOLS,
  ];

  const chars: string[] = [];

  // Un carácter de cada clase obligatoria.
  for (const set of classes) {
    chars.push(randomChar(set));
  }

  // Relleno del resto desde el alfabeto combinado.
  while (chars.length < GENERATED_PASSWORD_LENGTH) {
    chars.push(randomChar(PASSWORD_ALPHABET));
  }

  // Barajado para que las clases obligatorias no queden siempre al principio.
  shuffle(chars);

  return chars.join("");
}
