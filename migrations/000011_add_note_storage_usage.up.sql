CREATE TABLE note_storage_usage (
    user_id uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    markdown_bytes bigint NOT NULL DEFAULT 0 CHECK (markdown_bytes >= 0),
    attachment_bytes bigint NOT NULL DEFAULT 0 CHECK (attachment_bytes >= 0)
);

INSERT INTO note_storage_usage (user_id, markdown_bytes, attachment_bytes)
SELECT u.id, COALESCE(n.markdown_bytes, 0), COALESCE(a.attachment_bytes, 0)
FROM users u
LEFT JOIN (
    SELECT user_id, SUM(octet_length(markdown)) AS markdown_bytes
    FROM notes WHERE deleted_at IS NULL GROUP BY user_id
) n ON n.user_id = u.id
LEFT JOIN (
    SELECT user_id, SUM(size) AS attachment_bytes
    FROM note_attachments WHERE deleted_at IS NULL GROUP BY user_id
) a ON a.user_id = u.id;
