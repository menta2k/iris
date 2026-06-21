-- TOTP MFA secret for a user. Stored base32-encoded; mfa_enrolled_at (existing)
-- marks the secret as confirmed/active. Empty means no enrollment in progress.
ALTER TABLE iris_users
    ADD COLUMN IF NOT EXISTS mfa_secret TEXT NOT NULL DEFAULT '';
