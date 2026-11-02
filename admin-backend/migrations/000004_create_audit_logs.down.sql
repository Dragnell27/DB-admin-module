-- Migration 000004 (down): revierte la bitácora de auditoría.

DROP INDEX IF EXISTS idx_audit_logs_actor_name;
DROP INDEX IF EXISTS idx_audit_logs_created_at;
DROP INDEX IF EXISTS idx_audit_logs_resource;
DROP INDEX IF EXISTS idx_audit_logs_object_id;
DROP TABLE IF EXISTS audit_logs;
