CREATE TABLE IF NOT EXISTS notes (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    id uuid NOT NULL,
    title text NOT NULL,
    markdown text NOT NULL,
    revision bigint NOT NULL DEFAULT 1,
    change_seq bigint NOT NULL DEFAULT 0,
    deleted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, id)
);

CREATE INDEX IF NOT EXISTS idx_notes_user_id ON notes (user_id);
CREATE INDEX IF NOT EXISTS idx_notes_user_updated ON notes (user_id, updated_at DESC, id ASC);
CREATE INDEX IF NOT EXISTS idx_notes_user_change_seq ON notes (user_id, change_seq ASC, id ASC);

CREATE TABLE IF NOT EXISTS note_tags (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    id uuid NOT NULL,
    name text NOT NULL,
    name_normalized text NOT NULL,
    revision bigint NOT NULL DEFAULT 1,
    change_seq bigint NOT NULL DEFAULT 0,
    deleted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, id)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_note_tags_user_normalized_active
    ON note_tags (user_id, name_normalized) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_note_tags_user_id ON note_tags (user_id);

CREATE TABLE IF NOT EXISTS note_tag_relations (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    note_id uuid NOT NULL,
    tag_id uuid NOT NULL,
    PRIMARY KEY (user_id, note_id, tag_id),
    FOREIGN KEY (user_id, note_id) REFERENCES notes(user_id, id) ON DELETE CASCADE,
    FOREIGN KEY (user_id, tag_id) REFERENCES note_tags(user_id, id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_note_tag_relations_user_tag ON note_tag_relations (user_id, tag_id);

CREATE TABLE IF NOT EXISTS note_attachments (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    id uuid NOT NULL,
    kind varchar(10) NOT NULL CHECK (kind IN ('image', 'file')),
    file_name text NOT NULL,
    media_type varchar(255) NOT NULL,
    size bigint NOT NULL,
    sha256 varchar(64) NOT NULL,
    storage_path text NOT NULL,
    deleted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, id)
);

CREATE INDEX IF NOT EXISTS idx_note_attachments_user ON note_attachments (user_id, deleted_at);

CREATE TABLE IF NOT EXISTS note_attachment_relations (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    note_id uuid NOT NULL,
    attachment_id uuid NOT NULL,
    PRIMARY KEY (user_id, note_id, attachment_id),
    FOREIGN KEY (user_id, note_id) REFERENCES notes(user_id, id) ON DELETE CASCADE,
    FOREIGN KEY (user_id, attachment_id) REFERENCES note_attachments(user_id, id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_note_attachment_relations_attachment ON note_attachment_relations (user_id, attachment_id);

CREATE TABLE IF NOT EXISTS note_change_log (
    seq bigserial PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    resource_type varchar(10) NOT NULL,
    resource_id uuid NOT NULL,
    operation varchar(10) NOT NULL,
    revision bigint NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_note_change_log_user_seq ON note_change_log (user_id, seq ASC);

CREATE TABLE IF NOT EXISTS note_sync_state (
    user_id uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    compacted_through_seq bigint NOT NULL DEFAULT 0
);
