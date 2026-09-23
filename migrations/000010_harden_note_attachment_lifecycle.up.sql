ALTER TABLE note_attachments
    ADD COLUMN IF NOT EXISTS unassociated_since timestamptz;

UPDATE note_attachments AS a
   SET unassociated_since = now()
 WHERE a.deleted_at IS NULL
   AND a.unassociated_since IS NULL
   AND NOT EXISTS (
       SELECT 1
         FROM note_attachment_relations AS r
        WHERE r.user_id = a.user_id
          AND r.attachment_id = a.id
   );

CREATE INDEX IF NOT EXISTS idx_note_attachments_cleanup
    ON note_attachments (unassociated_since, id) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_note_attachments_storage_path
    ON note_attachments (storage_path) WHERE storage_path <> '';

CREATE INDEX IF NOT EXISTS idx_note_change_log_created_at
    ON note_change_log (created_at);
