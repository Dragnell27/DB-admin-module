-- Feature: panel-admin-ott
-- Migration 000001 (down): revert the `users` table creation. Requirement 22.1.

DROP INDEX IF EXISTS idx_users_username;
DROP TABLE IF EXISTS users;
