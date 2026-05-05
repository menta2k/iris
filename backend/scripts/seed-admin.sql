-- One-shot seed for an initial admin user.
--
-- Generate the bcrypt hash on the host (cost 12) — DO NOT use psql to
-- compute the hash. From the backend/ directory:
--
--   go run ./scripts/hashpw -- 'YourStrongPassword123!'
--
-- Then substitute the printed $2a$12$… hash into the INSERT below and run:
--
--   psql 'postgres://iris:iris@127.0.0.1:5432/iris' \
--        -v hash="'$2a$12$...'" \
--        -f scripts/seed-admin.sql
--
-- The role insert is idempotent; the user insert will fail if `admin`
-- already exists.

INSERT INTO roles (code, name, description, permissions, system, created_at, updated_at)
VALUES (
  'admin', 'Administrator', 'Full access',
  '["*:*"]'::jsonb, TRUE, NOW(), NOW()
)
ON CONFLICT (code) DO NOTHING;

INSERT INTO users (
  username, email, display_name, password_hash, active,
  failed_logins, created_at, updated_at
)
VALUES (
  'admin', 'admin@local', 'Default Admin', :hash, TRUE,
  0, NOW(), NOW()
);

INSERT INTO user_roles (user_id, role_id)
SELECT u.id, r.id
FROM users u, roles r
WHERE u.username = 'admin' AND r.code = 'admin'
ON CONFLICT DO NOTHING;
