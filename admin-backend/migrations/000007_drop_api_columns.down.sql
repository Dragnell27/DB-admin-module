-- Migration 000007 (down): recrea las columnas API eliminadas con sus tipos
-- originales de la migración 000005 (reversible a nivel de ESQUEMA). Los DATOS
-- previos de estas columnas NO se restauran: la reversión solo devuelve la
-- forma de la tabla, no su contenido histórico (aceptable para esta migración).

ALTER TABLE module_data_sources
    ADD COLUMN IF NOT EXISTS base_url    TEXT,
    ADD COLUMN IF NOT EXISTS auth_scheme VARCHAR(24),
    ADD COLUMN IF NOT EXISTS auth_config JSONB NOT NULL DEFAULT '{}';

ALTER TABLE module_actions
    ADD COLUMN IF NOT EXISTS http_method VARCHAR(8),
    ADD COLUMN IF NOT EXISTS endpoint    TEXT;
