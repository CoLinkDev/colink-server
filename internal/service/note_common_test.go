package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"colink-server/internal/pkg"
)

func TestParseUUIDListRejectsDuplicates(t *testing.T) {
	id := uuid.New().String()
	_, err := parseUUIDList([]string{id, id})
	assertAppErrorCode(t, err, pkg.CodeInvalidParameter)
}

func TestNormalizeTagNameTrimsBeforeFolding(t *testing.T) {
	if got := normalizeTagName("  WoRk\u00a0"); got != normalizeTagName("work") {
		t.Fatalf("spaced tag normalized to %q", got)
	}
}

func TestValidateAttachmentURIs(t *testing.T) {
	id := uuid.New()
	tests := []struct {
		name     string
		markdown string
		ids      []uuid.UUID
		wantCode int
	}{
		{name: "no URI", markdown: "plain text"},
		{name: "associated", markdown: "![image](colink-attachment://" + id.String() + ")", ids: []uuid.UUID{id}},
		{name: "missing association", markdown: "colink-attachment://" + id.String(), wantCode: pkg.CodeInvalidNoteReference},
		{name: "invalid suffix", markdown: "colink-attachment://" + id.String() + "/content", ids: []uuid.UUID{id}, wantCode: pkg.CodeInvalidNoteReference},
		{name: "port suffix", markdown: "colink-attachment://" + id.String() + ":80", ids: []uuid.UUID{id}, wantCode: pkg.CodeInvalidNoteReference},
		{name: "host suffix", markdown: "colink-attachment://" + id.String() + ".example", ids: []uuid.UUID{id}, wantCode: pkg.CodeInvalidNoteReference},
		{name: "uppercase scheme", markdown: "COLINK-ATTACHMENT://" + id.String(), ids: []uuid.UUID{id}, wantCode: pkg.CodeInvalidNoteReference},
		{name: "malformed scheme", markdown: "colink-attachment:/" + id.String(), ids: []uuid.UUID{id}, wantCode: pkg.CodeInvalidNoteReference},
		{name: "uppercase", markdown: "colink-attachment://" + strings.ToUpper(id.String()), ids: []uuid.UUID{id}, wantCode: pkg.CodeInvalidNoteReference},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateAttachmentURIs(test.markdown, test.ids)
			if test.wantCode == 0 {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			assertAppErrorCode(t, err, test.wantCode)
		})
	}
}

func assertAppErrorCode(t *testing.T, err error, code int) {
	t.Helper()
	var appErr *pkg.AppError
	if !errors.As(err, &appErr) || appErr.Code != code {
		t.Fatalf("expected app error %d, got %v", code, err)
	}
}

func TestMapTagUniqueViolation(t *testing.T) {
	err := &pgconn.PgError{Code: "23505", ConstraintName: "idx_note_tags_user_normalized_active"}
	appErr := mapTagUniqueViolation(err)
	if appErr == nil || appErr.Code != pkg.CodeTagNameConflict {
		t.Fatalf("expected tag name conflict, got %v", appErr)
	}
}

func TestExceedsLimitDoesNotOverflow(t *testing.T) {
	max := int64(^uint64(0) >> 1)
	if !exceedsLimit(max, max, 1) {
		t.Fatal("expected overflowing sum to exceed limit")
	}
	if exceedsLimit(max, max-1, 1) {
		t.Fatal("expected exact limit to be accepted")
	}
}
