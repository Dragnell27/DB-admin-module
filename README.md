# panel-admin-ott

Panel web de administración (en español, tema oscuro) para gestionar los recursos
expuestos por las APIs OTT del backend existente `hub-services-ott`
(Customers, Plans, Subscriptions y Cards).

La solución se compone de dos aplicaciones nuevas que conviven en este monorepo
**sin modificar** el backend OTT existente:

- **admin-frontend** — interfaz web en Next.js + TypeScript (Tailwind + shadcn/ui,
  TanStack Query, React Hook Form + Zod). Se comunica **únicamente** con el
  admin-backend.
- **admin-backend** — capa intermedia (BFF/proxy) en Go + Gin + GORM + PostgreSQL.
  Autentica al administrador, gestiona usuarios y roles propios (`admin` / `viewer`),
  emite su propio JWT de sesión y consume las APIs OTT en nombre del administrador,
  administrando las credenciales y el JWT OTT por país.

## Estructura del monorepo

```
ott-front/
├── admin-frontend/     # Next.js (BFF-only). Tema oscuro, español
├── admin-backend/      # BFF Go + Gin + GORM + PostgreSQL
├── hub-services-ott/   # Backend OTT existente — NO se modifica
├── docker-compose.yml  # db + admin-backend + admin-frontend
├── .env.example        # variables para Docker Compose (copiar a .env)
└── README.md
```

El admin-frontend nunca llama directamente a las APIs OTT: todas sus peticiones
pasan por el admin-backend, que aísla las credenciales OTT y centraliza la lógica
de integración.

## Prerrequisitos

- **Con Docker (recomendado):** Docker y Docker Compose.
- **Para desarrollo local sin Docker:** Go 1.26+, Node.js 20+ (con npm) y una
  instancia de PostgreSQL accesible.
- En ambos casos, una instancia del backend OTT (`hub-services-ott`) accesible y
  las credenciales OTT (`clientId` / `clientSecret`) del país que vayas a operar.

## Puertos expuestos

| Servicio        | Puerto | Descripción                                  |
|-----------------|--------|----------------------------------------------|
| admin-frontend  | 3000   | Panel web (abrir en el navegador)            |
| admin-backend   | 8081   | BFF / API (healthcheck en `GET /health`)     |
| db (PostgreSQL) | 5432   | Base de datos del admin-backend              |

## Inicio rápido con Docker Compose

1. Copia el archivo de ejemplo y completa los valores reales:

   ```bash
   cp .env.example .env
   # edita .env: credenciales de la DB, ADMIN_JWT_SECRET, SEED_ADMIN_*,
   # OTT_BASE_URL, OTT_ACTIVE_COUNTRY y las credenciales OTT del país.
   ```

2. Levanta los tres servicios (db, admin-backend, admin-frontend):

   ```bash
   docker compose up --build
   ```

   El admin-backend aplica las migraciones automáticamente al arrancar si el
   esquema no está presente, y espera a que la base de datos esté saludable.

3. Crea el usuario administrador inicial ejecutando el seed (idempotente: puede
   ejecutarse varias veces sin crear duplicados):

   ```bash
   docker compose run --rm admin-backend /app/seed
   ```

4. Abre el panel en <http://localhost:3000> e inicia sesión con las credenciales
   `SEED_ADMIN_USERNAME` / `SEED_ADMIN_PASSWORD` definidas en `.env`.

Para detener y limpiar:

```bash
docker compose down          # detiene los servicios
docker compose down -v       # además elimina el volumen de la base de datos
```

## Desarrollo local sin Docker

### admin-backend (Go)

Define las variables de entorno (ver `admin-backend/.env.example`) y, desde
`admin-backend/`:

```bash
go run ./cmd/api     # arranca el BFF (aplica migraciones al iniciar)
go run ./cmd/seed    # crea el administrador inicial (idempotente)
go test ./...        # ejecuta las pruebas (unitarias y PBT)
```

### admin-frontend (Next.js)

Define `NEXT_PUBLIC_ADMIN_API_URL` (ver `admin-frontend/.env.example`,
cópialo a `.env.local`) y, desde `admin-frontend/`:

```bash
npm install          # instala dependencias
npm run dev          # servidor de desarrollo en http://localhost:3000
npm run build        # build de producción
npm test             # ejecuta las pruebas
```

## Variables de entorno

El despliegue en otro ambiente requiere **únicamente** cambiar valores de
variables de entorno; no es necesario modificar código. El `.env` real está
excluido por `.gitignore`; solo se versionan los `.env.example`.

### admin-backend (`admin-backend/.env.example`)

| Variable                     | Descripción                                                       | Ejemplo                              |
|------------------------------|-------------------------------------------------------------------|--------------------------------------|
| `PORT`                       | Puerto HTTP del BFF                                               | `8081`                               |
| `CORS_ALLOWED_ORIGIN`        | Origen permitido para CORS (URL del admin-frontend)              | `http://localhost:3000`              |
| `DB_HOST`                    | Host de PostgreSQL (`db` dentro de Compose)                      | `localhost`                          |
| `DB_PORT`                    | Puerto de PostgreSQL                                             | `5432`                               |
| `DB_USER`                    | Usuario de la base de datos                                      | `admin_ott`                          |
| `DB_PASSWORD`                | Contraseña de la base de datos                                   | `changeme`                           |
| `DB_NAME`                    | Nombre de la base de datos                                       | `admin_ott`                          |
| `DB_SSLMODE`                 | Modo SSL de la conexión                                          | `disable`                            |
| `ADMIN_JWT_SECRET`           | Secreto para firmar el JWT de sesión (valor largo y aleatorio)   | `replace-with-a-long-random-secret`  |
| `ADMIN_JWT_EXPIRES_MINUTES`  | Expiración del JWT de sesión, en minutos                         | `480`                                |
| `BCRYPT_COST`                | Coste del hashing bcrypt                                         | `12`                                 |
| `SEED_ADMIN_USERNAME`        | Usuario del administrador inicial (seed)                         | `admin`                              |
| `SEED_ADMIN_PASSWORD`        | Contraseña del administrador inicial (seed)                      | `changeme`                           |
| `SEED_ADMIN_ROLE`            | Rol del administrador inicial                                    | `admin`                              |
| `OTT_BASE_URL`               | URL base de las APIs OTT                                         | `http://localhost:8080/api/v1`       |
| `OTT_ACTIVE_COUNTRY`         | País activo del BFF: uno de `gt`, `sv`, `ni`, `hn`, `cr`         | `gt`                                 |
| `OTT_<CC>_CLIENT_ID`         | `clientId` OTT por país (`GT`,`SV`,`NI`,`HN`,`CR`)               | `OTT_GT_CLIENT_ID=...`               |
| `OTT_<CC>_CLIENT_SECRET`     | `clientSecret` OTT por país                                      | `OTT_GT_CLIENT_SECRET=...`           |

`OTT_ACTIVE_COUNTRY` debe pertenecer a `{gt, sv, ni, hn, cr}` (validado al
arrancar). La moneda se deriva del país según el mapeo:

| País | `gt` | `sv` | `hn` | `ni` | `cr` |
|------|------|------|------|------|------|
| Moneda | GTQ | USD | HNL | NIO | CRC |

### admin-frontend (`admin-frontend/.env.example`)

| Variable                    | Descripción                                             | Ejemplo                  |
|-----------------------------|---------------------------------------------------------|--------------------------|
| `NEXT_PUBLIC_ADMIN_API_URL` | URL base del admin-backend (BFF) vista por el navegador | `http://localhost:8081`  |

### Docker Compose / base de datos (`.env` raíz, desde `.env.example`)

El `.env` de la raíz es el único archivo que lee `docker-compose.yml`. Además de
todas las variables del admin-backend y de `NEXT_PUBLIC_ADMIN_API_URL`, define las
credenciales de inicialización de PostgreSQL, que **deben coincidir** con las
`DB_*` del backend:

| Variable            | Descripción                                   | Debe coincidir con |
|---------------------|-----------------------------------------------|--------------------|
| `POSTGRES_USER`     | Usuario que inicializa el contenedor de la db | `DB_USER`          |
| `POSTGRES_PASSWORD` | Contraseña del contenedor de la db            | `DB_PASSWORD`      |
| `POSTGRES_DB`       | Nombre de la base de datos                    | `DB_NAME`          |

## Migraciones y seed

- **Migraciones (golang-migrate):** se aplican automáticamente al arrancar el
  admin-backend si el esquema aún no existe.
- **Seed del administrador inicial:** crea un usuario con rol `admin` a partir de
  `SEED_ADMIN_*`. Es idempotente (si el usuario ya existe, no lo duplica).
  - Con Docker: `docker compose run --rm admin-backend /app/seed`
  - Local: `go run ./cmd/seed` (desde `admin-backend/`)

## Arquitectura

- El **admin-frontend** habla **solo** con el admin-backend (BFF) a través de un
  cliente HTTP único con `NEXT_PUBLIC_ADMIN_API_URL` como URL base.
- El **admin-backend** gestiona la **sesión del admin** con JWT propio: contraseñas
  con **bcrypt** y token firmado con **HS256** (`ADMIN_JWT_SECRET`), incluyendo el
  rol en los claims. Un middleware exige rol `admin` para operaciones de escritura.
- La **autenticación OTT es por país**: el BFF obtiene y **cachea el JWT OTT** por
  país (reutilizándolo hasta su expiración) y ante un `401` de seguridad
  reautentica y **reintenta una sola vez**.
- El BFF realiza **validación espejo server-side** (paginación, ordenamiento,
  reglas de cada recurso y mapeo moneda↔país) antes de traducir la petición a OTT.
- Las respuestas OTT se **normalizan** a un envoltorio uniforme, exponiendo los
  metadatos de paginación (`totalRecords`, `page`, `recordsPerPage`, `totalPages`);
  un `404` en listados se interpreta como "sin resultados", no como error.

## Despliegue en otro ambiente

1. Copia `.env.example` a `.env` y ajusta los valores (DB, secretos, `OTT_BASE_URL`,
   `OTT_ACTIVE_COUNTRY`, credenciales OTT y `NEXT_PUBLIC_ADMIN_API_URL` con la URL
   pública real del BFF).
2. Ejecuta `docker compose up --build` y luego el seed una vez.
   No se requieren cambios de código para portar la solución a otro ambiente.

## Solución de problemas

- **Descarga de dependencias en contenedores dentro de la red CENAM:** la
  interceptación TLS corporativa puede hacer fallar la descarga de módulos
  (Go) o paquetes (npm) durante `docker compose up --build`. Si ves errores de
  certificado/TLS al construir las imágenes, construye detrás de la configuración
  de proxy/CA corporativa de tu red o realiza el build en una red sin
  interceptación, y reutiliza las imágenes resultantes.
