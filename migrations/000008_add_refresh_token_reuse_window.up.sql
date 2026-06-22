ALTER TABLE refresh_tokens
    ADD COLUMN rotated_at timestamptz,
    ADD COLUMN reuse_expires_at timestamptz,
    ADD COLUMN replacement_access_token text,
    ADD COLUMN replacement_refresh_token text,
    ADD COLUMN replacement_access_expires_at timestamptz,
    ADD COLUMN replacement_refresh_expires_at timestamptz;

CREATE INDEX IF NOT EXISTS idx_refresh_tokens_reuse_expires_at
    ON refresh_tokens (reuse_expires_at);
