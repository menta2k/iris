-- 0011_user_password.sql
-- Add password storage for the login flow. The hash is a bcrypt digest; an
-- empty string means "no usable password" (login disabled for that account).
ALTER TABLE iris_users
    ADD COLUMN IF NOT EXISTS password_hash TEXT NOT NULL DEFAULT '';
