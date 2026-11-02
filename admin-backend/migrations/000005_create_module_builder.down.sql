-- Migration 000005 (down): revierte el constructor de módulos low-code.
-- Elimina en orden inverso al up para respetar las claves foráneas.

DROP INDEX IF EXISTS idx_module_actions_module_id;
DROP TABLE IF EXISTS module_actions;
DROP TABLE IF EXISTS module_data_sources;
DROP TABLE IF EXISTS module_definitions;
