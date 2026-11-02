-- Feature: admin-rbac (rediseño del modelo de permisos)
-- Migration 000003 (up): separa "qué puede hacer" (permiso recurso×acción en el
-- ROL) de "dónde puede hacerlo" (lista de países en el USUARIO). Los permisos
-- efectivos se calculan como el producto cartesiano (acciones del rol × países
-- del usuario) al resolver los permisos de un usuario.
--
-- Migración limpia (BD de desarrollo): se vacían roles/permisos/asignaciones y
-- se re-siembran con `cmd/seed`. No se preservan datos previos.

-- 1) Limpieza de datos previos (dependientes primero por las FKs).
DELETE FROM role_permissions;
DELETE FROM user_roles;
DELETE FROM roles;

-- 2) role_permissions pierde el país: ahora un permiso de rol es (recurso, acción).
ALTER TABLE role_permissions DROP CONSTRAINT IF EXISTS role_permissions_unique_triple;
ALTER TABLE role_permissions DROP COLUMN IF EXISTS country;
ALTER TABLE role_permissions
    ADD CONSTRAINT role_permissions_unique_pair UNIQUE (role_id, resource, action);

-- 3) El scope de países vive en el usuario: nueva tabla user_countries.
CREATE TABLE IF NOT EXISTS user_countries (
    user_id  UUID        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    country  VARCHAR(8)  NOT NULL,
    PRIMARY KEY (user_id, country)
);

CREATE INDEX IF NOT EXISTS idx_user_countries_user_id ON user_countries (user_id);
