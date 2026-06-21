-- dev_seed.sql
-- Seed data for local smoke testing. Safe to run repeatedly (idempotent).

-- Built-in roles ------------------------------------------------------------
INSERT INTO roles (name, description, permissions, builtin) VALUES
  ('owner', 'Full administrative access', ARRAY['*'], true),
  ('operator', 'Mail operations and configuration', ARRAY[
     'vmta:read','vmta:write','routing:read','routing:write','mail:read',
     'queue:read','queue:control','service:control','dkim:read','dkim:write',
     'suppression:read','suppression:write','webhook:read','webhook:write',
     'rspamd:read','dashboard:read'], true),
  ('security_admin', 'User and audit administration', ARRAY[
     'user:read','user:write','audit:read','dashboard:read','mail:read','queue:read'], true),
  ('viewer', 'Read-only access', ARRAY[
     'vmta:read','routing:read','mail:read','queue:read','dkim:read',
     'suppression:read','webhook:read','rspamd:read','dashboard:read',
     'user:read','audit:read'], true)
ON CONFLICT (name) DO NOTHING;

-- Seed administrator --------------------------------------------------------
INSERT INTO iris_users (email, display_name, status, mfa_required)
VALUES ('admin@localhost', 'Local Admin', 'active', false)
ON CONFLICT (email) DO NOTHING;

INSERT INTO user_roles (user_id, role_id)
SELECT u.id, r.id FROM iris_users u, roles r
WHERE u.email = 'admin@localhost' AND r.name = 'owner'
ON CONFLICT DO NOTHING;

-- Example listeners ---------------------------------------------------------
INSERT INTO listeners (name, ip_address, port, hostname) VALUES
  ('listener-a', '203.0.113.10', 25, 'mta-a.example.com'),
  ('listener-b', '203.0.113.11', 25, 'mta-b.example.com')
ON CONFLICT (name) DO NOTHING;

-- Example VMTAs (attached to listeners) and group ---------------------------
INSERT INTO vmtas (name, listener_id, status)
SELECT 'vmta-a', l.id, 'active' FROM listeners l WHERE l.name = 'listener-a'
ON CONFLICT (name) DO NOTHING;
INSERT INTO vmtas (name, listener_id, status)
SELECT 'vmta-b', l.id, 'active' FROM listeners l WHERE l.name = 'listener-b'
ON CONFLICT (name) DO NOTHING;

INSERT INTO vmta_groups (name, status) VALUES ('bulk-pool', 'active')
ON CONFLICT (name) DO NOTHING;

INSERT INTO vmta_group_members (group_id, vmta_id, weight)
SELECT g.id, v.id, 70 FROM vmta_groups g, vmtas v
WHERE g.name = 'bulk-pool' AND v.name = 'vmta-a'
ON CONFLICT DO NOTHING;
INSERT INTO vmta_group_members (group_id, vmta_id, weight)
SELECT g.id, v.id, 30 FROM vmta_groups g, vmtas v
WHERE g.name = 'bulk-pool' AND v.name = 'vmta-b'
ON CONFLICT DO NOTHING;

-- Example routing rule ------------------------------------------------------
INSERT INTO routing_rules (name, match_type, match_value, priority, target_type, target_id, status)
SELECT 'bulk-mailclass', 'mailclass', 'bulk', 100, 'vmta_group', g.id, 'active'
FROM vmta_groups g WHERE g.name = 'bulk-pool'
ON CONFLICT DO NOTHING;

-- Example suppression -------------------------------------------------------
INSERT INTO suppression_entries (type, value, reason, source, status)
VALUES ('domain', 'blocked.example', 'seed example', 'manual', 'active')
ON CONFLICT (type, value) DO NOTHING;

-- Example queue state -------------------------------------------------------
INSERT INTO mailclass_queues (mailclass, state, depth, oldest_message_age_seconds, last_observed_at)
VALUES ('bulk', 'running', 0, 0, now())
ON CONFLICT (mailclass) DO NOTHING;
