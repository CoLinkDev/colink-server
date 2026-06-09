CREATE TABLE IF NOT EXISTS app_releases (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    platform varchar(20) NOT NULL,
    version varchar(50) NOT NULL,
    release_notes text NOT NULL DEFAULT '',
    published_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT chk_app_release_platform CHECK (platform IN ('android', 'windows')),
    CONSTRAINT uq_app_releases_platform_version UNIQUE (platform, version)
);

CREATE INDEX IF NOT EXISTS idx_app_releases_platform_published_at
    ON app_releases (platform, published_at DESC);

CREATE TABLE IF NOT EXISTS release_assets (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    release_id uuid NOT NULL REFERENCES app_releases(id) ON DELETE CASCADE,
    file_name varchar(255) NOT NULL,
    file_size bigint NOT NULL,
    file_path varchar(500) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT chk_release_asset_file_size CHECK (file_size >= 0),
    CONSTRAINT uq_release_assets_release_file UNIQUE (release_id, file_name)
);

CREATE INDEX IF NOT EXISTS idx_release_assets_release_id
    ON release_assets (release_id);
