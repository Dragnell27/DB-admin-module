-- Feature: admin-rbac
-- Migration 000002 (up): introduce RBAC (roles, permisos granulares y asignación
-- usuario-rol) y evolucionar `users` para la convivencia con el esquema legacy.
-- Esquema per design.md Data Models ("Migración golang-migrate (000002_create_rbac)").
-- Requirements 12.1, 12.4.

-- Needed for gen_random_uuid(); available in PostgreSQL 13+ core, kept explicit for older versions.
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- 1) Catálogo de roles. `name` es el identificador legible y debe ser único.
CREATE TABLE IF NOT EXISTS roles (
    id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(100) NOT NULL UNIQUE,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- 2) Permisos de cada rol como ternas (recurso, acción, país). El catálogo de
-- recursos y acciones se valida en Go; aquí basta la restricción de unicidad
-- para evitar ternas duplicadas por rol.
CREATE TABLE IF NOT EXISTS role_permissions (
    id        UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    role_id   UUID        NOT NULL REFERENCES roles (id) ON DELETE CASCADE,
    resource  VARCHAR(32) NOT NULL,
    action    VARCHAR(32) NOT NULL,
    country   VARCHAR(8)  NOT NULL,
    CONSTRAINT role_permissions_unique_triple UNIQUE (role_id, resource, action, country)
);

-- Índice de apoyo para resolver permisos por rol (unión de permisos en login).
CREATE INDEX IF NOT EXISTS idx_role_permissions_role_id ON role_permissions (role_id);

-- 3) Relación muchos-a-muchos entre usuarios y roles. PK compuesta evita
-- asignaciones duplicadas; ON DELETE CASCADE limpia asignaciones al borrar
-- el usuario o el rol.
CREATE TABLE IF NOT EXISTS user_roles (
    user_id  UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role_id  UUID NOT NULL REFERENCES roles (id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, role_id)
);

-- 4) Evolución de `users`: estado de activación.
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_active BOOLEAN NOT NULL DEFAULT true;

-- 5) Relajar el `role` legacy: se mantiene (nullable) durante la convivencia
-- para no romper login ni pruebas existentes, pero deja de ser obligatorio y
-- se elimina el CHECK binario admin/viewer.
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;
ALTER TABLE users ALTER COLUMN role DROP NOT NULL;
