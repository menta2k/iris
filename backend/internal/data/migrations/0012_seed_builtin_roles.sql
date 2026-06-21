-- 0012_seed_builtin_roles.sql
-- Seed the built-in roles so role assignment works on a fresh deployment.
-- user_roles references roles(id), and CreateUser links roles by name, so these
-- rows must exist for an assigned role to take effect.
--
-- NOTE: the *effective* permissions for built-in roles are resolved in code
-- (biz.BuiltinRolePermissions, keyed by role name) — not from the permissions
-- column below. The column is populated for documentation/inspection only.
INSERT INTO roles (name, description, permissions, builtin) VALUES
    ('owner',          'Full administrative access.',                 '{*}', true),
    ('operator',       'Manage outbound config, queues, and safety.', '{}',  true),
    ('security_admin', 'Manage users, roles, and audit.',             '{}',  true),
    ('viewer',         'Read-only access.',                           '{}',  true)
ON CONFLICT (name) DO NOTHING;
