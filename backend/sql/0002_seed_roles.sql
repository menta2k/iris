-- 0002_seed_roles.sql
-- Seed default roles. Idempotent: re-running is a no-op.

INSERT INTO roles (code, name, description, permissions, system, created_at, updated_at)
VALUES
  ('admin',    'Administrator', 'Full access',         '["*:*"]',                                                        TRUE, NOW(), NOW()),
  ('operator', 'Operator',      'Day-to-day ops',      '["kumo.policy:read","kumo.policy:write","kumo.queue:write","kumo.suppression:write","audit.log:read"]', TRUE, NOW(), NOW()),
  ('viewer',   'Viewer',        'Read-only dashboards', '["kumo.*:read","audit.log:read"]',                              TRUE, NOW(), NOW())
ON CONFLICT (code) DO NOTHING;
