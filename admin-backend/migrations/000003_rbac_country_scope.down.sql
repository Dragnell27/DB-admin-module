-- Migration 000003 (down): revierte el rediseño del modelo de permisos.
-- No restaura datos (la migración up era limpia).

-- Revierte la tabla de países del usuario.
DROP INDEX IF EXISTS idx_user_countries_user_id;
DROP TABLE IF EXISTS user_countries;

-- Revierte role_permissions al esquema con país (nullable, sin datos).
ALTER TABLE role_permissions DROP CONSTRAINT IF EXISTS role_permissions_unique_pair;
ALTER TABLE role_permissions ADD COLUMN IF NOT EXISTS country VARCHAR(8);
ALTER TABLE role_permissions
    ADD CONSTRAINT role_permissions_unique_triple UNIQUE (role_id, resource, action, country);
