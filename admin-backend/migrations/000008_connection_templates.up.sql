-- Feature: connection-templates
-- Migration 000008 (up): plantillas de conexión reutilizables y la referencia
-- del módulo a su plantilla. Una plantilla define campos (host/puerto/usuario/
-- password/nombreBD…) marcados FIJO (valor cifrado en la plantilla) o VARIABLE
-- (lo aporta cada módulo). El DSN efectivo se ensambla EN MEMORIA al ejecutar.
--
-- ADVERTENCIA: el backfill de datos (DSN directo legado -> plantilla) NO lo hace
-- esta migración SQL (no puede descifrar secrets_enc); lo hace el comando
-- one-shot `cmd/migrate-connections`. El `.down.sql` por sí solo NO debe
-- ejecutarse DESPUÉS del backfill: dropearía connection_template_id,
-- secrets_enc_legacy y connection_templates dejando los módulos migrados sin
-- conexión (secrets_enc = Seal('{}')) y perdiendo el respaldo, de forma
-- silenciosa. El camino de reversión soportado tras un backfill es
-- `cmd/migrate-connections --rollback` (restaura datos ANTES de cualquier DROP).

CREATE EXTENSION IF NOT EXISTS pgcrypto;  -- gen_random_uuid()

-- 1) Plantilla de conexión. `fields` guarda SOLO la estructura (no sensible);
--    `secrets_enc` guarda el bundle cifrado de valores FIJOS (AES-GCM).
CREATE TABLE IF NOT EXISTS connection_templates (
    id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(150) NOT NULL UNIQUE,
    fields      JSONB        NOT NULL DEFAULT '[]',   -- estructura NO sensible
    secrets_enc BYTEA,                                -- AES-GCM: {claveFija -> valorFijo}
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- 2) FK del módulo a su plantilla. Nullable durante la ventana de migración
--    (módulos legados aún no migrados). ON DELETE RESTRICT implementa D-5 a
--    nivel de BD: no se puede borrar una plantilla referenciada.
ALTER TABLE module_data_sources
    ADD COLUMN IF NOT EXISTS connection_template_id UUID
        REFERENCES connection_templates (id) ON DELETE RESTRICT;

-- 3) Respaldo del DSN cifrado legado, OBLIGATORIO para la reversión segura del
--    backfill (Req. 7.5). El backfill lo copia ANTES de reescribir secrets_enc.
ALTER TABLE module_data_sources
    ADD COLUMN IF NOT EXISTS secrets_enc_legacy BYTEA;

-- 4) Ruta de menú libre del módulo (organizativa, profundidad ilimitada). Solo
--    presentación: no afecta ejecución ni RBAC (Req. 6).
ALTER TABLE module_definitions
    ADD COLUMN IF NOT EXISTS menu_path VARCHAR(512) NOT NULL DEFAULT '';

-- 5) Índice DELIBERADO de la FK: PostgreSQL no indexa columnas FK
--    automáticamente, y UsedByModules / el GROUP BY del listado / la
--    verificación de borrado consultan por esta columna en cada detalle,
--    listado y borrado de plantilla.
CREATE INDEX IF NOT EXISTS idx_module_data_sources_connection_template_id
    ON module_data_sources (connection_template_id);
