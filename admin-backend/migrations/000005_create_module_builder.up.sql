-- Feature: module-builder
-- Migration 000005 (up): definiciones de módulos low-code, sus orígenes de
-- datos (con credenciales cifradas) y sus acciones. La configuración flexible
-- vive en columnas JSONB; las invariantes consultables (slug único, tipo de
-- origen, lista blanca de Postgres) viven en columnas relacionales.

CREATE EXTENSION IF NOT EXISTS pgcrypto;  -- gen_random_uuid()

-- 1) Definición del módulo. slug kebab-case único = recurso dinámico mod:<slug>.
CREATE TABLE IF NOT EXISTS module_definitions (
    id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    slug            VARCHAR(64)  NOT NULL UNIQUE,          -- kebab-case; Req. 1.2, 1.3
    name            VARCHAR(150) NOT NULL,
    source_type     VARCHAR(16)  NOT NULL,                 -- 'api' | 'postgres' (Req. 3, 4)
    country_scope   VARCHAR(16)  NOT NULL DEFAULT 'none',  -- 'none' | 'multi' (Req. 11.7)
    fetch_mapping   JSONB        NOT NULL DEFAULT '{}',     -- Req. 5
    display_mapping JSONB        NOT NULL DEFAULT '{}',     -- Req. 6
    list_params     JSONB        NOT NULL DEFAULT '{}',     -- paginación/orden/filtros declarados (Req. 9.3, 9.4, 9.5)
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- 2) Origen de datos del módulo (1:1 con el módulo). Credenciales cifradas aparte.
CREATE TABLE IF NOT EXISTS module_data_sources (
    id            UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    module_id     UUID         NOT NULL UNIQUE REFERENCES module_definitions (id) ON DELETE CASCADE,
    source_type   VARCHAR(16)  NOT NULL,                 -- redundante con el módulo, por claridad del executor
    base_url      TEXT,                                  -- API: URL base (Req. 3.1)
    auth_scheme   VARCHAR(24),                           -- 'none'|'api_key'|'bearer'|'oauth_cc' (Req. 3.2)
    auth_config   JSONB        NOT NULL DEFAULT '{}',     -- no sensible: nombre de cabecera, URL de token... (Req. 3.3-3.5)
    secrets_enc   BYTEA,                                 -- AES-GCM: api key/token/client_secret/DSN (Req. 3.6, 4.3, 13.5)
    pg_whitelist  JSONB        NOT NULL DEFAULT '{}',     -- Postgres: tablas -> columnas permitidas (Req. 4.5)
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- 3) Acciones del módulo (CRUD + personalizadas), cada una con su botón.
CREATE TABLE IF NOT EXISTS module_actions (
    id           UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    module_id    UUID         NOT NULL REFERENCES module_definitions (id) ON DELETE CASCADE,
    action_key   VARCHAR(64)  NOT NULL,                 -- estable por módulo (p. ej. 'list','create','custom:activar')
    kind         VARCHAR(16)  NOT NULL,                 -- 'list'|'create'|'edit'|'delete'|'custom' (Req. 7.1)
    label        VARCHAR(120) NOT NULL,                 -- etiqueta del botón (Req. 8.2)
    placement    VARCHAR(8)   NOT NULL,                 -- 'row' | 'header' (Req. 8.1, 8.4, 8.5)
    destructive  BOOLEAN      NOT NULL DEFAULT false,   -- confirma antes de ejecutar (Req. 8.3, 10.5)
    perm_action  VARCHAR(32)  NOT NULL,                 -- acción de permiso RBAC (ver/crear/editar/eliminar/custom:*)
    is_write     BOOLEAN      NOT NULL DEFAULT false,   -- determina auditoría de escritura (Req. 12.1)
    -- API: endpoint/método; Postgres: operación. Campos de entrada en JSONB.
    http_method  VARCHAR(8),                            -- GET/POST/PUT/DELETE (Req. 7.2)
    endpoint     TEXT,                                  -- API (Req. 7.2)
    sql_op       VARCHAR(8),                            -- SELECT/INSERT/UPDATE/DELETE (Req. 7.3)
    input_fields JSONB        NOT NULL DEFAULT '[]',     -- [{name,type,required,location}] (Req. 7.5, 10.3)
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT module_actions_unique_key UNIQUE (module_id, action_key)
);

CREATE INDEX IF NOT EXISTS idx_module_actions_module_id ON module_actions (module_id);
