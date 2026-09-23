DROP INDEX IF EXISTS idx_note_change_log_created_at;
DROP INDEX IF EXISTS idx_note_attachments_storage_path;
DROP INDEX IF EXISTS idx_note_attachments_cleanup;

ALTER TABLE note_attachments
    DROP COLUMN IF EXISTS unassociated_since;
