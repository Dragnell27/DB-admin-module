-- Feature: module-builder
-- Migration 000006 (up): condiciones fijas de listado por módulo. Filtro en
-- memoria por columna + valores permitidos (coincidencia exacta, sin distinguir
-- mayúsculas), combinadas con AND. Se guardan como JSONB (lista de objetos
-- {field, values}); vacío = sin condiciones (se listan todas las filas).

ALTER TABLE module_definitions
    ADD COLUMN IF NOT EXISTS list_conditions JSONB NOT NULL DEFAULT '[]';
