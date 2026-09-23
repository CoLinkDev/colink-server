package janitor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"colink-server/internal/config"
	"colink-server/internal/model"
	"colink-server/internal/repository"
)

var errInvalidStoragePath = errors.New("invalid attachment storage path")

type Janitor struct {
	db             *gorm.DB
	tokenRepo      *repository.TokenRepository
	ticketRepo     *repository.TicketRepository
	attachmentRepo *repository.NoteAttachmentRepository
	storageRepo    *repository.NoteStorageRepository
	changeLogRepo  *repository.NoteChangeLogRepository
	notesConfig    config.NotesConfig
	storageRoot    string
	interval       time.Duration
	log            *zap.Logger
}

func New(
	db *gorm.DB,
	tokenRepo *repository.TokenRepository,
	ticketRepo *repository.TicketRepository,
	attachmentRepo *repository.NoteAttachmentRepository,
	changeLogRepo *repository.NoteChangeLogRepository,
	notesConfig config.NotesConfig,
	interval time.Duration,
	log *zap.Logger,
) *Janitor {
	return &Janitor{
		db:             db,
		tokenRepo:      tokenRepo,
		ticketRepo:     ticketRepo,
		attachmentRepo: attachmentRepo,
		storageRepo:    repository.NewNoteStorageRepository(),
		changeLogRepo:  changeLogRepo,
		notesConfig:    notesConfig,
		storageRoot:    notesConfig.StoragePath,
		interval:       interval,
		log:            log,
	}
}

func (j *Janitor) Run(ctx context.Context) {
	j.cleanup()

	ticker := time.NewTicker(j.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			j.cleanup()
		}
	}
}

func (j *Janitor) cleanup() {
	now := time.Now().UTC()

	if err := j.tokenRepo.ExpireReusableTokens(now); err != nil {
		j.log.Warn("expire refresh token reuse windows", zap.Error(err))
	}
	if err := j.tokenRepo.DeleteExpired(now); err != nil {
		j.log.Warn("cleanup refresh tokens", zap.Error(err))
	}
	if err := j.ticketRepo.Cleanup(now); err != nil {
		j.log.Warn("cleanup ws tickets", zap.Error(err))
	}
	if err := j.compactChangeLog(now); err != nil {
		j.log.Warn("compact notes change log", zap.Error(err))
	}
	if err := j.removeExpiredUnassociatedAttachments(now); err != nil {
		j.log.Warn("remove unassociated note attachments", zap.Error(err))
	}
	if err := j.removeTombstonedAttachmentContent(); err != nil {
		j.log.Warn("remove tombstoned note attachment content", zap.Error(err))
	}
	if err := j.removeOrphanedAttachmentContent(now); err != nil {
		j.log.Warn("remove orphaned note attachment content", zap.Error(err))
	}
}

// compactChangeLog deletes events older than the retention window and moves
// the per-account compacted watermark forward, expiring cursors that point
// at removed entries.
func (j *Janitor) compactChangeLog(now time.Time) error {
	cutoff := now.Add(-j.notesConfig.ChangeLogRetention)
	return j.db.Transaction(func(tx *gorm.DB) error {
		return j.changeLogRepo.CompactBefore(tx, cutoff)
	})
}

// removeExpiredUnassociatedAttachments tombstones live attachments that no
// note references and whose retention window has passed, removing their
// content from storage. Tombstone rows are kept forever so attachment ids
// can never be reused.
func (j *Janitor) removeExpiredUnassociatedAttachments(now time.Time) error {
	cutoff := now.Add(-j.notesConfig.AttachmentRetention)

	for {
		var removed []model.NoteAttachment
		var selected int
		err := j.db.Transaction(func(tx *gorm.DB) error {
			attachments, err := j.attachmentRepo.ListUnassociatedOlderThan(tx, cutoff, 100)
			if err != nil {
				return err
			}
			selected = len(attachments)
			for _, attachment := range attachments {
				tombstoned, err := j.attachmentRepo.TombstoneIfUnreferenced(tx, attachment, now)
				if err != nil {
					return err
				}
			if tombstoned {
				if err := j.storageRepo.AddAttachment(tx, attachment.UserID, -attachment.Size); err != nil {
					return err
				}
				removed = append(removed, attachment)
					continue
				}
				if err := j.attachmentRepo.ClearUnassociatedState(tx, attachment.UserID, attachment.ID); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return err
		}

		for _, attachment := range removed {
			if err := j.removeAndClearStoragePath(attachment); err != nil {
				j.log.Warn("remove note attachment content", zap.String("path", attachment.StoragePath), zap.Error(err))
			}
		}
		if selected < 100 {
			return nil
		}
	}
}

func (j *Janitor) removeTombstonedAttachmentContent() error {
	for {
		attachments, err := j.attachmentRepo.ListTombstonedWithStoragePath(100)
		if err != nil {
			return err
		}
		if len(attachments) == 0 {
			return nil
		}
		cleared := 0
		for _, attachment := range attachments {
			removeErr := removeStorageFile(j.storageRoot, attachment.StoragePath)
			if removeErr != nil && !os.IsNotExist(removeErr) {
				j.log.Warn("retry note attachment content removal", zap.String("path", attachment.StoragePath), zap.Error(removeErr))
				if !errors.Is(removeErr, errInvalidStoragePath) {
					continue
				}
			}
			if err := j.attachmentRepo.ClearStoragePath(attachment.UserID, attachment.ID); err != nil {
				return err
			}
			cleared++
		}
		if len(attachments) < 100 || cleared == 0 {
			return nil
		}
	}
}

func (j *Janitor) removeAndClearStoragePath(attachment model.NoteAttachment) error {
	if err := removeStorageFile(j.storageRoot, attachment.StoragePath); err != nil && !os.IsNotExist(err) {
		if !errors.Is(err, errInvalidStoragePath) {
			return err
		}
		j.log.Warn("discard invalid note attachment storage path", zap.String("path", attachment.StoragePath), zap.Error(err))
	}

	return j.attachmentRepo.ClearStoragePath(attachment.UserID, attachment.ID)
}

// removeOrphanedAttachmentContent reconciles files left behind by interrupted
// uploads or account deletion. The retention delay prevents racing a file that
// has been moved into place while its database transaction is still committing.
func (j *Janitor) removeOrphanedAttachmentContent(now time.Time) error {
	root, err := filepath.Abs(j.storageRoot)
	if err != nil {
		return err
	}
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	cutoff := now.Add(-j.notesConfig.AttachmentRetention)
	type orphanCandidate struct {
		path     string
		relative string
	}
	candidates := make([]orphanCandidate, 0, 256)
	flush := func() error {
		if len(candidates) == 0 {
			return nil
		}
		paths := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			paths = append(paths, candidate.relative)
		}
		referencedPaths, err := j.attachmentRepo.FindStoragePaths(paths)
		if err != nil {
			return err
		}
		referenced := make(map[string]struct{}, len(referencedPaths))
		for _, path := range referencedPaths {
			referenced[filepath.Clean(path)] = struct{}{}
		}
		for _, candidate := range candidates {
			if _, exists := referenced[candidate.relative]; exists {
				continue
			}
			if err := os.Remove(candidate.path); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		candidates = candidates[:0]
		return nil
	}

	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !entry.Type().IsRegular() {
			return nil
		}

		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.ModTime().Before(cutoff) {
			return nil
		}

		candidates = append(candidates, orphanCandidate{path: path, relative: filepath.Clean(relative)})
		if len(candidates) == cap(candidates) {
			return flush()
		}
		return nil
	})
	if err != nil {
		return err
	}
	return flush()
}

func removeStorageFile(storageRoot string, relativePath string) error {
	if relativePath == "" {
		return nil
	}
	if filepath.IsAbs(relativePath) {
		return fmt.Errorf("%w: path must be relative: %s", errInvalidStoragePath, relativePath)
	}
	root, err := filepath.Abs(storageRoot)
	if err != nil {
		return err
	}
	target, err := filepath.Abs(filepath.Join(root, relativePath))
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(root, target)
	if err != nil {
		return err
	}
	if relative == ".." || filepath.IsAbs(relative) || len(relative) > 3 && relative[:3] == ".."+string(filepath.Separator) {
		return fmt.Errorf("%w: path escapes storage root: %s", errInvalidStoragePath, relativePath)
	}

	return os.Remove(target)
}
