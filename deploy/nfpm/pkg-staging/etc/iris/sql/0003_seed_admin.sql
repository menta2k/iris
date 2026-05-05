-- 0003_seed_admin.sql
-- Seed the default admin user. Bootstrap so a fresh deploy can log in
-- with `admin / admin`. Operators MUST rotate the password immediately
-- via /v1/users on first login — the harness depends on these credentials
-- to set up scenarios. Idempotent: a row with username='admin' is left
-- alone on subsequent boots.
--
-- The bcrypt hash is for the literal string "admin" (cost 12, generated
-- via `go run ./scripts/hashpw admin`). Regenerate via the same command if
-- you want a different password baked into the seed.

INSERT INTO users (
  username, email, password_hash,
  active, failed_logins,
  created_at, updated_at
) VALUES (
  'admin', 'admin@kumo-ui.local',
  '$2a$12$6ZZW5uNp09fIvftJdPxdaePprVkezD0CoBQnTNY/K4YADrlStVGpu',
  TRUE, 0,
  NOW(), NOW()
)
ON CONFLICT (username) DO NOTHING;

-- Attach the admin role. user_roles is the join table; ent calls it that.
INSERT INTO user_roles (user_id, role_id)
SELECT u.id, r.id
FROM users u, roles r
WHERE u.username = 'admin' AND r.code = 'admin'
ON CONFLICT DO NOTHING;
