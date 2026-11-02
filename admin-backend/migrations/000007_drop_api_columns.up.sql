-- Feature: module-builder
-- Migration 000007 (up): remoción profunda del origen "API HTTP". El origen API
-- fue retirado del constructor (Fase 1) y no existen filas source_type='api'
-- (verificado: 0 filas), por lo que no se borran filas; solo se eliminan las
-- columnas específicas de API. secrets_enc y source_type se CONSERVAN
-- (compartidas con el origen PostgreSQL).

ALTER TABLE module_data_sources
    DROP COLUMN IF EXISTS base_url,
    DROP COLUMN IF EXISTS auth_scheme,
    DROP COLUMN IF EXISTS auth_config;

ALTER TABLE module_actions
    DROP COLUMN IF EXISTS http_method,
    DROP COLUMN IF EXISTS endpoint;
