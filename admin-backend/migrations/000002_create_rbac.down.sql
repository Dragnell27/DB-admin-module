-- Feature: admin-rbac
-- Migration 000002 (down): revierte la introducción de RBAC en orden inverso al
-- up, restaurando el esquema legacy de `users`. Requirements 12.1, 12.4.

-- 5') Restaurar el `role` legacy a su forma original: obligatorio y restringido
-- a los valores binarios admin/viewer.
ALTER TABLE users ALTER COLUMN role SET NOT NULL;
ALTER TABLE users ADD CONSTRAINT users_role_check CHECK (role IN ('admin', 'viewer'));

-- 4') Eliminar la columna de activación.
ALTER TABLE users DROP COLUMN IF EXISTS is_active;

-- 3'..1') Eliminar las tablas en orden que respeta las claves foráneas
-- (user_roles y role_permissions dependen de roles).
DROP TABLE IF EXISTS user_roles;
DROP INDEX IF EXISTS idx_role_permissions_role_id;
DROP TABLE IF EXISTS role_permissions;
DROP TABLE IF EXISTS roles;
