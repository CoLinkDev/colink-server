DROP INDEX IF EXISTS idx_refresh_tokens_reuse_expires_at;

ALTER TABLE refresh_tokens
    DROP COLUMN IF EXISTS replacement_refresh_expires_at,
    DROP COLUMN IF EXISTS replacement_access_expires_at,
    DROP COLUMN IF EXISTS replacement_refresh_token,
    DROP COLUMN IF EXISTS replacement_access_token,
    DROP COLUMN IF EXISTS reuse_expires_at,
    DROP COLUMN IF EXISTS rotated_at;
