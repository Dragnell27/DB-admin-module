-- Feature: audit-log
-- Migration 000004 (up): bitácora de auditoría de las operaciones de escritura
-- del panel. Registra quién hizo qué, sobre qué objeto, cuándo, en qué país y
-- con qué resultado, más un snapshot (redaccionado) del payload de entrada.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS audit_logs (
    id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_id    UUID,                       -- id del usuario del panel (nullable por robustez)
    actor_name  VARCHAR(100) NOT NULL,      -- username del actor
    resource    VARCHAR(32)  NOT NULL,      -- customers/plans/subscriptions/cards/users/roles
    action      VARCHAR(32)  NOT NULL,      -- crear/editar/eliminar/cambiar/cancelar/reset-password
    object_id   VARCHAR(128),               -- id del objeto afectado (texto; puede ser vacío)
    country     VARCHAR(8),                 -- país efectivo (X-Country), si aplica
    outcome     VARCHAR(16)  NOT NULL,      -- success | error
    status_code INT          NOT NULL,      -- código HTTP de la respuesta
    error_code  VARCHAR(64),                -- errorCode normalizado si outcome=error
    message     TEXT,                       -- mensaje legible (opcional)
    payload     JSONB,                      -- snapshot del payload de entrada (redaccionado)
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- Índices de apoyo a los filtros de consulta de la bitácora.
CREATE INDEX IF NOT EXISTS idx_audit_logs_object_id  ON audit_logs (object_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_resource   ON audit_logs (resource);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at ON audit_logs (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_actor_name ON audit_logs (actor_name);
