-- Migration 000006 (down): elimina la columna de condiciones de listado.

ALTER TABLE module_definitions
    DROP COLUMN IF EXISTS list_conditions;
