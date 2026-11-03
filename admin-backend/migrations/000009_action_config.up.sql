-- Feature: module-builder (multi-table create)
-- Migration 000009 (up): config extendida por acción en JSONB. Persiste la
-- configuración de la acción que el modelo GORM plano no mapeaba: targetTable
-- (tabla objetivo del INSERT multi-tabla), uniqueness (columnas + mensaje) y
-- deleteTopTable (tope del borrado multi-tabla). Filas antiguas obtienen '{}'
-- (sin claves) => comportamiento idéntico a hoy (retrocompatibilidad).

ALTER TABLE module_actions
    ADD COLUMN IF NOT EXISTS config JSONB NOT NULL DEFAULT '{}';
