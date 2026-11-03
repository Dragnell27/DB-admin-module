-- Migration 000009 (down): elimina la columna de config extendida de la acción.

ALTER TABLE module_actions
    DROP COLUMN IF EXISTS config;
