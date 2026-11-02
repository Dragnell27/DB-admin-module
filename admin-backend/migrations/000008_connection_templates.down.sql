-- Feature: connection-templates
-- Migration 000008 (down): revierte EXCLUSIVAMENTE el esquema (DDL vacío de
-- datos). Reversa el orden del up: índice, menu_path, secrets_enc_legacy,
-- connection_template_id, tabla.
--
-- ADVERTENCIA: este .down.sql por sí solo NO debe ejecutarse DESPUÉS de un
-- backfill de datos. Si el backfill corrió, los módulos quedaron con
-- secrets_enc = Seal('{}') y su DSN legado solo vive en secrets_enc_legacy;
-- dropear estas columnas y la tabla PERDERÍA la conexión silenciosamente
-- (contrario a Req. 7.5). Tras un backfill, ejecutar primero
-- `cmd/migrate-connections --rollback` (restaura secrets_enc desde el respaldo y
-- suelta la FK) y SOLO DESPUÉS este .down.sql. Si el backfill nunca corrió, este
-- .down.sql es seguro (revierte únicamente el DDL).

DROP INDEX IF EXISTS idx_module_data_sources_connection_template_id;
ALTER TABLE module_definitions  DROP COLUMN IF EXISTS menu_path;
ALTER TABLE module_data_sources DROP COLUMN IF EXISTS secrets_enc_legacy;
ALTER TABLE module_data_sources DROP COLUMN IF EXISTS connection_template_id;
DROP TABLE IF EXISTS connection_templates;
