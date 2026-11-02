-- Feature: panel-admin-ott
-- Migration 000001 (up): create the `users` table for admin authentication/roles.
-- Schema per design.md Data Models (admin-backend tabla `users`). Requirement 22.1.

-- Needed for gen_random_uuid(); available in PostgreSQL 13+ core, kept explicit for older versions.
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS users (
    id             UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    username       VARCHAR(100) NOT NULL,
    password_hash  VARCHAR(255) NOT NULL,
    role           VARCHAR(20)  NOT NULL,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT users_role_check CHECK (role IN ('admin', 'viewer'))
);

-- `username` is the login identifier and must be unique.
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_username ON users (username);
