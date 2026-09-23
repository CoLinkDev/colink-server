package integration_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

type noteAttachmentDTO struct {
	AttachmentID string    `json:"attachmentId"`
	Kind         string    `json:"kind"`
	FileName     string    `json:"fileName"`
	MediaType    string    `json:"mediaType"`
	Size         int64     `json:"size"`
	SHA256       string    `json:"sha256"`
	CreatedAt    time.Time `json:"createdAt"`
}

type noteDTO struct {
	NoteID      string              `json:"noteId"`
	Title       string              `json:"title"`
	Markdown    string              `json:"markdown"`
	TagIDs      []string            `json:"tagIds"`
	Attachments []noteAttachmentDTO `json:"attachments"`
	Revision    int64               `json:"revision"`
	CreatedAt   time.Time           `json:"createdAt"`
	UpdatedAt   time.Time           `json:"updatedAt"`
}

type noteSummaryDTO struct {
	NoteID          string    `json:"noteId"`
	Title           string    `json:"title"`
	TagIDs          []string  `json:"tagIds"`
	AttachmentCount int       `json:"attachmentCount"`
	Revision        int64     `json:"revision"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type notesListDTO struct {
	Notes         []noteSummaryDTO `json:"notes"`
	NextPageToken *string          `json:"nextPageToken"`
}

type tagDTO struct {
	TagID     string    `json:"tagId"`
	Name      string    `json:"name"`
	Revision  int64     `json:"revision"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type tagListDTO struct {
	Tags []tagDTO `json:"tags"`
}

type noteDeleteDTO struct {
	NoteID    string    `json:"noteId"`
	Revision  int64     `json:"revision"`
	DeletedAt time.Time `json:"deletedAt"`
}

type tagDeleteDTO struct {
	TagID     string    `json:"tagId"`
	Revision  int64     `json:"revision"`
	DeletedAt time.Time `json:"deletedAt"`
}

type snapshotDTO struct {
	Notes         []noteDTO `json:"notes"`
	Tags          []tagDTO  `json:"tags"`
	NextPageToken *string   `json:"nextPageToken"`
	Cursor        string    `json:"cursor"`
}

type changeEntryDTO struct {
	Type      string   `json:"type"`
	Operation string   `json:"operation"`
	Note      *noteDTO `json:"note,omitempty"`
	Tag       *tagDTO  `json:"tag,omitempty"`
	NoteID    *string  `json:"noteId,omitempty"`
	TagID     *string  `json:"tagId,omitempty"`
	Revision  int64    `json:"revision"`
}

type changesDTO struct {
	Changes    []changeEntryDTO `json:"changes"`
	NextCursor string           `json:"nextCursor"`
	HasMore    bool             `json:"hasMore"`
}

type storageDTO struct {
	UsedBytes          int64 `json:"usedBytes"`
	LimitBytes         int64 `json:"limitBytes"`
	RemainingBytes     int64 `json:"remainingBytes"`
	AttachmentBytes    int64 `json:"attachmentBytes"`
	MarkdownBytes      int64 `json:"markdownBytes"`
	MaxAttachmentBytes int64 `json:"maxAttachmentBytes"`
	MaxMarkdownBytes   int64 `json:"maxMarkdownBytes"`
}

func newNoteID() string {
	return uuid.NewString()
}

func newTagID() string {
	return uuid.NewString()
}

func newAttachmentID() string {
	return uuid.NewString()
}

func (a *testApp) uploadAttachment(t *testing.T, authorization string, attachmentID string, kind string, sha256Hex string, fileName string, content []byte) *http.Response {
	t.Helper()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if err := writer.WriteField("attachmentId", attachmentID); err != nil {
		t.Fatalf("write field: %v", err)
	}
	if err := writer.WriteField("kind", kind); err != nil {
		t.Fatalf("write field: %v", err)
	}
	if err := writer.WriteField("sha256", sha256Hex); err != nil {
		t.Fatalf("write field: %v", err)
	}
	part, err := writer.CreatePart(map[string][]string{
		"Content-Type":        {"text/plain"},
		"Content-Disposition": {fmt.Sprintf(`form-data; name="file"; filename="%s"`, fileName)},
	})
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write file part: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, a.server.URL+"/api/v1/note-attachments", body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	return resp
}

func digestOf(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func createTag(t *testing.T, a *testApp, authorization string, name string) tagDTO {
	t.Helper()

	resp := a.request(http.MethodPost, "/api/v1/note-tags", authorization, map[string]any{
		"tagId": newTagID(),
		"name":  name,
	})
	return decodeOK[tagDTO](t, resp)
}

func createNote(t *testing.T, a *testApp, authorization string, title string, markdown string, tagIDs []string, attachmentIDs []string) noteDTO {
	t.Helper()

	resp := a.request(http.MethodPost, "/api/v1/notes", authorization, map[string]any{
		"noteId":        newNoteID(),
		"title":         title,
		"markdown":      markdown,
		"tagIds":        tagIDs,
		"attachmentIds": attachmentIDs,
	})
	return decodeOK[noteDTO](t, resp)
}

func TestNotesFlow(t *testing.T) {
	a := newTestApp(t, 30*time.Second)
	defer a.close()

	auth := registerAndLogin(t, a)

	noteID := newNoteID()
	created := decodeOK[noteDTO](t, a.request(http.MethodPost, "/api/v1/notes", bearer(auth.Token), map[string]any{
		"noteId":        noteID,
		"title":         "Weekend Plans",
		"markdown":      "# Weekend Plans",
		"tagIds":        []string{},
		"attachmentIds": []string{},
	}))
	if created.NoteID != noteID || created.Revision != 1 {
		t.Fatalf("unexpected created note: %+v", created)
	}

	for _, missing := range []string{"noteId", "title", "markdown", "tagIds", "attachmentIds"} {
		request := map[string]any{
			"noteId":        newNoteID(),
			"title":         "",
			"markdown":      "",
			"tagIds":        []string{},
			"attachmentIds": []string{},
		}
		delete(request, missing)
		expectErrorCode(
			t,
			a.request(http.MethodPost, "/api/v1/notes", bearer(auth.Token), request),
			http.StatusBadRequest,
			4001,
		)
	}

	// Duplicate noteId is rejected.
	expectErrorCode(t, a.request(http.MethodPost, "/api/v1/notes", bearer(auth.Token), map[string]any{
		"noteId":        noteID,
		"title":         "Other",
		"markdown":      "",
		"tagIds":        []string{},
		"attachmentIds": []string{},
	}), http.StatusBadRequest, 4002)

	// Get returns the same note.
	got := decodeOK[noteDTO](t, a.request(http.MethodGet, "/api/v1/notes/"+noteID, bearer(auth.Token), nil))
	if got.Markdown != "# Weekend Plans" || got.Revision != 1 {
		t.Fatalf("unexpected note: %+v", got)
	}

	// Update without baseRevision conflicts.
	expectErrorCode(t, a.request(http.MethodPut, "/api/v1/notes/"+noteID, bearer(auth.Token), map[string]any{
		"baseRevision":  7,
		"title":         "x",
		"markdown":      "x",
		"tagIds":        []string{},
		"attachmentIds": []string{},
	}), http.StatusPreconditionFailed, 6002)

	// Successful update bumps revision.
	updated := decodeOK[noteDTO](t, a.request(http.MethodPut, "/api/v1/notes/"+noteID, bearer(auth.Token), map[string]any{
		"baseRevision":  1,
		"title":         "Weekend Plans v2",
		"markdown":      "# Weekend Plans v2",
		"tagIds":        []string{},
		"attachmentIds": []string{},
	}))
	if updated.Revision != 2 || updated.Title != "Weekend Plans v2" {
		t.Fatalf("unexpected updated note: %+v", updated)
	}
	storage := decodeOK[storageDTO](t, a.request(http.MethodGet, "/api/v1/notes/storage", bearer(auth.Token), nil))
	if storage.MarkdownBytes != int64(len(updated.Markdown)) {
		t.Fatalf("unexpected storage after note update: %+v", storage)
	}

	// Missing baseRevision is an invalid request body.
	expectErrorCode(t, a.request(http.MethodPut, "/api/v1/notes/"+noteID, bearer(auth.Token), map[string]any{
		"title":         "x",
		"markdown":      "x",
		"tagIds":        []string{},
		"attachmentIds": []string{},
	}), http.StatusBadRequest, 4001)

	// List returns the note summary.
	list := decodeOK[notesListDTO](t, a.request(http.MethodGet, "/api/v1/notes", bearer(auth.Token), nil))
	if len(list.Notes) != 1 || list.Notes[0].NoteID != noteID || list.NextPageToken != nil {
		t.Fatalf("unexpected list: %+v", list)
	}

	// Get with a nonexistent id returns 6001.
	expectErrorCode(t, a.request(http.MethodGet, "/api/v1/notes/"+newNoteID(), bearer(auth.Token), nil), http.StatusNotFound, 6001)

	// Delete requires matching baseRevision.
	expectErrorCode(t, a.request(http.MethodDelete, "/api/v1/notes/"+noteID+"?baseRevision=1", bearer(auth.Token), nil), http.StatusPreconditionFailed, 6002)

	deleted := decodeOK[noteDeleteDTO](t, a.request(http.MethodDelete, "/api/v1/notes/"+noteID+"?baseRevision=2", bearer(auth.Token), nil))
	if deleted.Revision != 3 {
		t.Fatalf("unexpected delete revision: %d", deleted.Revision)
	}
	storage = decodeOK[storageDTO](t, a.request(http.MethodGet, "/api/v1/notes/storage", bearer(auth.Token), nil))
	if storage.MarkdownBytes != 0 {
		t.Fatalf("deleted note still counts toward storage: %+v", storage)
	}

	// The deleted note is gone.
	expectErrorCode(t, a.request(http.MethodGet, "/api/v1/notes/"+noteID, bearer(auth.Token), nil), http.StatusNotFound, 6001)
	expectErrorCode(t, a.request(http.MethodDelete, "/api/v1/notes/"+noteID+"?baseRevision=3", bearer(auth.Token), nil), http.StatusNotFound, 6001)
}

func TestNotesTagFlow(t *testing.T) {
	a := newTestApp(t, 30*time.Second)
	defer a.close()

	auth := registerAndLogin(t, a)
	authorization := bearer(auth.Token)

	tagWork := createTag(t, a, authorization, "Work")
	if tagWork.Revision != 1 {
		t.Fatalf("unexpected tag revision: %d", tagWork.Revision)
	}
	expectErrorCode(t, a.request(http.MethodPost, "/api/v1/note-tags", authorization, map[string]any{
		"tagId": tagWork.TagID,
		"name":  tagWork.Name,
	}), http.StatusBadRequest, 4002)

	for _, request := range []map[string]any{
		{"name": "Missing ID"},
		{"tagId": newTagID()},
	} {
		expectErrorCode(t, a.request(http.MethodPost, "/api/v1/note-tags", authorization, request), http.StatusBadRequest, 4001)
	}

	// Duplicate names conflict after normalization and case folding.
	expectErrorCode(t, a.request(http.MethodPost, "/api/v1/note-tags", authorization, map[string]any{
		"tagId": newTagID(),
		"name":  "work",
	}), http.StatusConflict, 6004)

	// Display whitespace is preserved, but uniqueness ignores it.
	expectErrorCode(t, a.request(http.MethodPost, "/api/v1/note-tags", authorization, map[string]any{
		"tagId": newTagID(),
		"name":  " work ",
	}), http.StatusConflict, 6004)
	spacedTag := createTag(t, a, authorization, " Home ")
	if spacedTag.Name != " Home " {
		t.Fatalf("unexpected spaced tag name: %q", spacedTag.Name)
	}
	expectErrorCode(t, a.request(http.MethodPut, "/api/v1/note-tags/"+spacedTag.TagID, authorization, map[string]any{
		"baseRevision": 1,
		"name":         " WORK ",
	}), http.StatusConflict, 6004)

	// Empty name after trimming is invalid.
	expectErrorCode(t, a.request(http.MethodPost, "/api/v1/note-tags", authorization, map[string]any{
		"tagId": newTagID(),
		"name":  "   ",
	}), http.StatusBadRequest, 4002)
	expectErrorCode(t, a.request(http.MethodPost, "/api/v1/note-tags", authorization, map[string]any{
		"tagId": newTagID(),
		"name":  "",
	}), http.StatusBadRequest, 4002)
	expectErrorCode(t, a.request(http.MethodPut, "/api/v1/note-tags/"+tagWork.TagID, authorization, map[string]any{
		"name": "Office",
	}), http.StatusBadRequest, 4001)

	// Rename with wrong revision conflicts.
	expectErrorCode(t, a.request(http.MethodPut, "/api/v1/note-tags/"+tagWork.TagID, authorization, map[string]any{
		"baseRevision": 5,
		"name":         "Office",
	}), http.StatusPreconditionFailed, 6002)

	renamed := decodeOK[tagDTO](t, a.request(http.MethodPut, "/api/v1/note-tags/"+tagWork.TagID, authorization, map[string]any{
		"baseRevision": 1,
		"name":         "Office",
	}))
	if renamed.Revision != 2 || renamed.Name != "Office" {
		t.Fatalf("unexpected renamed tag: %+v", renamed)
	}

	// Rename does not affect note revisions.
	note := createNote(t, a, authorization, "Tagged", "body", []string{tagWork.TagID}, []string{})
	if len(note.TagIDs) != 1 || note.TagIDs[0] != tagWork.TagID {
		t.Fatalf("unexpected note tags: %+v", note.TagIDs)
	}

	// List with tag filter works.
	list := decodeOK[notesListDTO](t, a.request(http.MethodGet, "/api/v1/notes?tagId="+tagWork.TagID, authorization, nil))
	if len(list.Notes) != 1 || list.Notes[0].NoteID != note.NoteID {
		t.Fatalf("unexpected filtered list: %+v", list)
	}

	// Unknown tag filter is 6003.
	expectErrorCode(t, a.request(http.MethodGet, "/api/v1/notes?tagId="+newTagID(), authorization, nil), http.StatusNotFound, 6003)

	// Deleting the tag removes the association and bumps the note revision.
	tagDeleted := decodeOK[tagDeleteDTO](t, a.request(http.MethodDelete, "/api/v1/note-tags/"+tagWork.TagID+"?baseRevision=2", authorization, nil))
	if tagDeleted.Revision != 3 {
		t.Fatalf("unexpected tag delete revision: %d", tagDeleted.Revision)
	}

	afterTagDelete := decodeOK[noteDTO](t, a.request(http.MethodGet, "/api/v1/notes/"+note.NoteID, authorization, nil))
	if len(afterTagDelete.TagIDs) != 0 {
		t.Fatalf("expected tag removed, got %+v", afterTagDelete.TagIDs)
	}
	if afterTagDelete.Revision != note.Revision+1 {
		t.Fatalf("expected note revision bump: %d -> %d", note.Revision, afterTagDelete.Revision)
	}

	// The deleted tag is gone and notes cannot reference it anymore.
	tags := decodeOK[tagListDTO](t, a.request(http.MethodGet, "/api/v1/note-tags", authorization, nil))
	for _, tag := range tags.Tags {
		if tag.TagID == tagWork.TagID {
			t.Fatalf("deleted tag still listed: %+v", tag)
		}
	}
	expectErrorCode(t, a.request(http.MethodPut, "/api/v1/note-tags/"+tagWork.TagID, authorization, map[string]any{
		"baseRevision": 3,
		"name":         "Zombie",
	}), http.StatusNotFound, 6003)
	expectErrorCode(t, a.request(http.MethodPost, "/api/v1/notes", authorization, map[string]any{
		"noteId":        newNoteID(),
		"title":         "zombie",
		"markdown":      "",
		"tagIds":        []string{tagWork.TagID},
		"attachmentIds": []string{},
	}), http.StatusConflict, 6008)
}

func TestNotesTagDeleteUpdatesEveryNote(t *testing.T) {
	a := newTestApp(t, 30*time.Second)
	defer a.close()
	auth := registerAndLogin(t, a)
	authorization := bearer(auth.Token)
	tag := createTag(t, a, authorization, "Shared")
	noteIDs := make(map[string]bool)
	for i := 0; i < 3; i++ {
		note := createNote(t, a, authorization, fmt.Sprintf("Note %d", i), "body", []string{tag.TagID}, []string{})
		noteIDs[note.NoteID] = true
	}
	snapshot := decodeOK[snapshotDTO](t, a.request(http.MethodGet, "/api/v1/notes/sync/snapshot", authorization, nil))
	decodeOK[tagDeleteDTO](t, a.request(http.MethodDelete, "/api/v1/note-tags/"+tag.TagID+"?baseRevision=1", authorization, nil))
	changes := decodeOK[changesDTO](t, a.request(http.MethodGet, "/api/v1/notes/sync/changes?cursor="+snapshot.Cursor, authorization, nil))
	seen := make(map[string]bool)
	for _, change := range changes.Changes {
		if change.Note == nil || !noteIDs[change.Note.NoteID] {
			continue
		}
		if change.Note.Revision != 2 || len(change.Note.TagIDs) != 0 {
			t.Fatalf("unexpected note after tag deletion: %+v", change.Note)
		}
		seen[change.Note.NoteID] = true
	}
	if len(seen) != len(noteIDs) {
		t.Fatalf("missing note changes: got %d, want %d", len(seen), len(noteIDs))
	}
}

func TestNotesAttachmentsFlow(t *testing.T) {
	a := newTestApp(t, 30*time.Second)
	defer a.close()

	auth := registerAndLogin(t, a)
	authorization := bearer(auth.Token)

	content := []byte("hello")
	attachmentID := newAttachmentID()

	uploaded := decodeOK[noteAttachmentDTO](t, a.uploadAttachment(t, authorization, attachmentID, "file", digestOf(content), "hello.txt", content))
	if uploaded.Size != 5 || uploaded.SHA256 != digestOf(content) || uploaded.MediaType != "text/plain" {
		t.Fatalf("unexpected upload: %+v", uploaded)
	}

	// Content type is detected from bytes rather than trusted from the part.
	pngHeader := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	png := decodeOK[noteAttachmentDTO](t, a.uploadAttachment(t, authorization, newAttachmentID(), "image", digestOf(pngHeader), "image.png", pngHeader))
	if png.MediaType != "image/png" {
		t.Fatalf("unexpected detected media type: %q", png.MediaType)
	}
	emptyAttachmentID := newAttachmentID()
	decodeOK[noteAttachmentDTO](t, a.uploadAttachment(t, authorization, emptyAttachmentID, "file", digestOf(nil), "empty.bin", nil))
	emptyRangeReq, _ := http.NewRequest(http.MethodGet, a.server.URL+"/api/v1/note-attachments/"+emptyAttachmentID+"/content", nil)
	emptyRangeReq.Header.Set("Authorization", authorization)
	emptyRangeReq.Header.Set("Range", "bytes=-1")
	emptyRangeResp, err := http.DefaultClient.Do(emptyRangeReq)
	if err != nil {
		t.Fatal(err)
	}
	defer emptyRangeResp.Body.Close()
	if emptyRangeResp.StatusCode != http.StatusRequestedRangeNotSatisfiable {
		t.Fatalf("expected 416 for an empty attachment range, got %d", emptyRangeResp.StatusCode)
	}

	// Re-uploading the same id is rejected.
	expectErrorCode(t, a.uploadAttachment(t, authorization, attachmentID, "file", digestOf(content), "hello.txt", content), http.StatusConflict, 6011)

	// Digest mismatch is rejected.
	expectErrorCode(t, a.uploadAttachment(t, authorization, newAttachmentID(), "file", digestOf([]byte("other")), "other.txt", content), http.StatusUnprocessableEntity, 6010)

	// Invalid kind is rejected.
	expectErrorCode(t, a.uploadAttachment(t, authorization, newAttachmentID(), "movie", digestOf(content), "hello.txt", content), http.StatusBadRequest, 4002)

	// Oversized uploads use the Notes quota error envelope.
	oversized := bytes.Repeat([]byte("x"), 100*1024+1)
	expectErrorCode(t, a.uploadAttachment(t, authorization, newAttachmentID(), "file", digestOf(oversized), "large.bin", oversized), http.StatusRequestEntityTooLarge, 6007)

	// Duplicate sets and unassociated attachment URIs are rejected.
	expectErrorCode(t, a.request(http.MethodPost, "/api/v1/notes", authorization, map[string]any{
		"noteId":        newNoteID(),
		"title":         "duplicate",
		"markdown":      "",
		"tagIds":        []string{},
		"attachmentIds": []string{attachmentID, attachmentID},
	}), http.StatusBadRequest, 4002)
	expectErrorCode(t, a.request(http.MethodPost, "/api/v1/notes", authorization, map[string]any{
		"noteId":        newNoteID(),
		"title":         "missing association",
		"markdown":      "colink-attachment://" + attachmentID,
		"tagIds":        []string{},
		"attachmentIds": []string{},
	}), http.StatusConflict, 6008)

	// Metadata query.
	meta := decodeOK[noteAttachmentDTO](t, a.request(http.MethodGet, "/api/v1/note-attachments/"+attachmentID, authorization, nil))
	if meta.AttachmentID != attachmentID {
		t.Fatalf("unexpected metadata: %+v", meta)
	}

	// Reference the attachment from a note.
	markdown := "![Route](colink-attachment://" + attachmentID + ")"
	note := createNote(t, a, authorization, "With image", markdown, []string{}, []string{attachmentID})

	// Referencing query finds the note.
	refs := decodeOK[attachmentRefsDTO](t, a.request(http.MethodGet, "/api/v1/note-attachments/"+attachmentID+"/references", authorization, nil))
	if len(refs.NoteIDs) != 1 || refs.NoteIDs[0] != note.NoteID {
		t.Fatalf("unexpected references: %+v", refs.NoteIDs)
	}

	// Deleting a referenced attachment is rejected.
	expectErrorCode(t, a.request(http.MethodDelete, "/api/v1/note-attachments/"+attachmentID, authorization, nil), http.StatusConflict, 6006)

	// Download supports conditional requests and ranges.
	resp := a.request(http.MethodGet, "/api/v1/note-attachments/"+attachmentID+"/content", authorization, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("ETag") != `"`+digestOf(content)+`"` {
		t.Fatalf("unexpected download: %d %q", resp.StatusCode, resp.Header.Get("ETag"))
	}
	if resp.Header.Get("Accept-Ranges") != "bytes" {
		t.Fatalf("missing Accept-Ranges header")
	}
	body, _ := io.ReadAll(resp.Body)
	if !bytes.Equal(body, content) {
		t.Fatalf("unexpected body: %q", body)
	}

	// 304 on matching If-None-Match.
	req, _ := http.NewRequest(http.MethodGet, a.server.URL+"/api/v1/note-attachments/"+attachmentID+"/content", nil)
	req.Header.Set("Authorization", authorization)
	req.Header.Set("If-None-Match", `"`+digestOf(content)+`"`)
	notModified, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer notModified.Body.Close()
	if notModified.StatusCode != http.StatusNotModified {
		t.Fatalf("expected 304, got %d", notModified.StatusCode)
	}

	// 206 with single range.
	rangeReq, _ := http.NewRequest(http.MethodGet, a.server.URL+"/api/v1/note-attachments/"+attachmentID+"/content", nil)
	rangeReq.Header.Set("Authorization", authorization)
	rangeReq.Header.Set("Range", "bytes=1-3")
	rangeResp, err := http.DefaultClient.Do(rangeReq)
	if err != nil {
		t.Fatal(err)
	}
	defer rangeResp.Body.Close()
	if rangeResp.StatusCode != http.StatusPartialContent {
		t.Fatalf("expected 206, got %d", rangeResp.StatusCode)
	}
	if rangeResp.Header.Get("Content-Range") != "bytes 1-3/5" {
		t.Fatalf("unexpected Content-Range: %q", rangeResp.Header.Get("Content-Range"))
	}
	rangeBody, _ := io.ReadAll(rangeResp.Body)
	if string(rangeBody) != "ell" {
		t.Fatalf("unexpected range body: %q", rangeBody)
	}

	// Multi-range requests are rejected with 416.
	multiReq, _ := http.NewRequest(http.MethodGet, a.server.URL+"/api/v1/note-attachments/"+attachmentID+"/content", nil)
	multiReq.Header.Set("Authorization", authorization)
	multiReq.Header.Set("Range", "bytes=0-1,3-4")
	multiResp, err := http.DefaultClient.Do(multiReq)
	if err != nil {
		t.Fatal(err)
	}
	defer multiResp.Body.Close()
	if multiResp.StatusCode != http.StatusRequestedRangeNotSatisfiable {
		t.Fatalf("expected 416, got %d", multiResp.StatusCode)
	}

	// Storage accounting: markdown + attachments.
	storage := decodeOK[storageDTO](t, a.request(http.MethodGet, "/api/v1/notes/storage", authorization, nil))
	if storage.UsedBytes != storage.AttachmentBytes+storage.MarkdownBytes {
		t.Fatalf("storage accounting mismatch: %+v", storage)
	}
	if storage.AttachmentBytes != 5+png.Size || storage.MarkdownBytes != int64(len(markdown)) {
		t.Fatalf("unexpected storage: %+v", storage)
	}

	// Remove the association first, then delete the attachment.
	updatedNote := decodeOK[noteDTO](t, a.request(http.MethodPut, "/api/v1/notes/"+note.NoteID, authorization, map[string]any{
		"baseRevision":  note.Revision,
		"title":         "Without image",
		"markdown":      "text only",
		"tagIds":        []string{},
		"attachmentIds": []string{},
	}))
	if updatedNote.Revision != note.Revision+1 {
		t.Fatalf("unexpected revision: %d", updatedNote.Revision)
	}
	var state struct {
		UnassociatedSince *time.Time
	}
	if err := a.db.Raw(
		"SELECT unassociated_since FROM note_attachments WHERE user_id = ? AND id = ?",
		auth.UserID, attachmentID,
	).Scan(&state).Error; err != nil {
		t.Fatal(err)
	}
	if state.UnassociatedSince == nil || state.UnassociatedSince.Before(updatedNote.UpdatedAt.Add(-time.Second)) {
		t.Fatalf("unassociated retention did not start at unlink time: %v", state.UnassociatedSince)
	}

	decodeOK[any](t, a.request(http.MethodDelete, "/api/v1/note-attachments/"+attachmentID, authorization, nil))
	storage = decodeOK[storageDTO](t, a.request(http.MethodGet, "/api/v1/notes/storage", authorization, nil))
	if storage.AttachmentBytes != png.Size || storage.MarkdownBytes != int64(len("text only")) {
		t.Fatalf("unexpected storage after update and delete: %+v", storage)
	}

	// Tombstone: metadata, download, delete, and re-upload all fail.
	expectErrorCode(t, a.request(http.MethodGet, "/api/v1/note-attachments/"+attachmentID, authorization, nil), http.StatusNotFound, 6005)
	expectErrorCode(t, a.request(http.MethodGet, "/api/v1/note-attachments/"+attachmentID+"/content", authorization, nil), http.StatusNotFound, 6005)
	expectErrorCode(t, a.request(http.MethodDelete, "/api/v1/note-attachments/"+attachmentID, authorization, nil), http.StatusNotFound, 6005)
	expectErrorCode(t, a.uploadAttachment(t, authorization, attachmentID, "file", digestOf(content), "hello.txt", content), http.StatusConflict, 6011)
	expectErrorCode(t, a.request(http.MethodGet, "/api/v1/note-attachments/"+attachmentID+"/references", authorization, nil), http.StatusNotFound, 6005)
}

type attachmentRefsDTO struct {
	NoteIDs       []string `json:"noteIds"`
	NextPageToken *string  `json:"nextPageToken"`
}

func TestNotesSyncFlow(t *testing.T) {
	a := newTestApp(t, 30*time.Second)
	defer a.close()

	auth := registerAndLogin(t, a)
	authorization := bearer(auth.Token)

	// Full snapshot with no data issues a cursor.
	empty := decodeOK[snapshotDTO](t, a.request(http.MethodGet, "/api/v1/notes/sync/snapshot", authorization, nil))
	if len(empty.Notes) != 0 || len(empty.Tags) != 0 || empty.NextPageToken != nil || empty.Cursor == "" {
		t.Fatalf("unexpected empty snapshot: %+v", empty)
	}

	tag := createTag(t, a, authorization, "Sync")
	note1 := createNote(t, a, authorization, "One", "one", []string{tag.TagID}, []string{})
	note2 := createNote(t, a, authorization, "Two", "two", []string{}, []string{})

	// Paged snapshot: limit 1 forces multiple pages with a stable cursor.
	firstPage := decodeOK[snapshotDTO](t, a.request(http.MethodGet, "/api/v1/notes/sync/snapshot?limit=1", authorization, nil))
	if len(firstPage.Notes)+len(firstPage.Tags) != 1 || firstPage.NextPageToken == nil {
		t.Fatalf("unexpected first page: %+v", firstPage)
	}
	cursor := firstPage.Cursor

	secondPage := decodeOK[snapshotDTO](t, a.request(http.MethodGet, "/api/v1/notes/sync/snapshot?limit=1&pageToken="+*firstPage.NextPageToken, authorization, nil))
	thirdPage := decodeOK[snapshotDTO](t, a.request(http.MethodGet, "/api/v1/notes/sync/snapshot?limit=1&pageToken="+*secondPage.NextPageToken, authorization, nil))
	if thirdPage.NextPageToken != nil {
		t.Fatalf("expected final page, got %+v", thirdPage)
	}
	if thirdPage.Cursor != cursor {
		t.Fatalf("snapshot cursor changed mid-pagination: %q != %q", thirdPage.Cursor, cursor)
	}

	collected := map[string]bool{}
	for _, page := range [][]noteDTO{firstPage.Notes, secondPage.Notes, thirdPage.Notes} {
		for _, note := range page {
			collected[note.NoteID] = true
		}
	}
	if !collected[note1.NoteID] || !collected[note2.NoteID] {
		t.Fatalf("snapshot missed notes: %+v", collected)
	}

	// Incremental sync with no changes returns empty with the same cursor.
	changes := decodeOK[changesDTO](t, a.request(http.MethodGet, "/api/v1/notes/sync/changes?cursor="+cursor, authorization, nil))
	if len(changes.Changes) != 0 || changes.HasMore {
		t.Fatalf("unexpected changes: %+v", changes)
	}

	// Make changes: update note1, create note3, delete tag, rename? delete tag "Sync".
	updated := decodeOK[noteDTO](t, a.request(http.MethodPut, "/api/v1/notes/"+note1.NoteID, authorization, map[string]any{
		"baseRevision":  note1.Revision,
		"title":         "One v2",
		"markdown":      "one v2",
		"tagIds":        []string{},
		"attachmentIds": []string{},
	}))
	note3 := createNote(t, a, authorization, "Three", "three", []string{}, []string{})
	decodeOK[tagDeleteDTO](t, a.request(http.MethodDelete, "/api/v1/note-tags/"+tag.TagID+"?baseRevision=1", authorization, nil))

	next := decodeOK[changesDTO](t, a.request(http.MethodGet, "/api/v1/notes/sync/changes?cursor="+cursor, authorization, nil))
	if len(next.Changes) < 3 {
		t.Fatalf("expected at least 3 change events, got %+v", next.Changes)
	}
	if next.HasMore {
		t.Fatalf("unexpected hasMore")
	}

	sawNoteUpsert := false
	sawTagDelete := false
	for _, entry := range next.Changes {
		switch {
		case entry.Type == "note" && entry.Operation == "upsert" && entry.Note != nil && entry.Note.NoteID == note1.NoteID:
			sawNoteUpsert = true
			if entry.Note.Revision != updated.Revision || entry.Note.Markdown != "one v2" {
				t.Fatalf("unexpected upsert content: %+v", entry.Note)
			}
		case entry.Type == "note" && entry.Operation == "upsert" && entry.Note != nil && entry.Note.NoteID == note3.NoteID:
			// created
		case entry.Type == "note" && entry.Operation == "upsert" && entry.Note != nil && entry.Note.NoteID == note2.NoteID:
			t.Fatalf("note2 should not appear again: %+v", entry)
		case entry.Type == "tag" && entry.Operation == "delete" && entry.TagID != nil && *entry.TagID == tag.TagID:
			sawTagDelete = true
			if entry.Revision != 2 {
				t.Fatalf("unexpected tag delete revision: %d", entry.Revision)
			}
		}
	}
	if !sawNoteUpsert || !sawTagDelete {
		t.Fatalf("missing expected change events: %+v", next.Changes)
	}

	// Save next cursor and verify it advances.
	final := decodeOK[changesDTO](t, a.request(http.MethodGet, "/api/v1/notes/sync/changes?cursor="+next.NextCursor, authorization, nil))
	if len(final.Changes) != 0 || final.HasMore {
		t.Fatalf("unexpected trailing changes: %+v", final)
	}
}

func TestNotesIncrementalPagination(t *testing.T) {
	a := newTestApp(t, 30*time.Second)
	defer a.close()

	auth := registerAndLogin(t, a)
	authorization := bearer(auth.Token)

	snapshot := decodeOK[snapshotDTO](t, a.request(http.MethodGet, "/api/v1/notes/sync/snapshot", authorization, nil))

	created := make(map[string]bool)
	for i := 0; i < 5; i++ {
		note := createNote(t, a, authorization, fmt.Sprintf("Note %d", i), fmt.Sprintf("body %d", i), []string{}, []string{})
		created[note.NoteID] = true
	}
	sharedUpdatedAt := time.Date(2026, 9, 13, 8, 0, 0, 0, time.UTC)
	if err := a.db.Exec("UPDATE notes SET updated_at = ?", sharedUpdatedAt).Error; err != nil {
		t.Fatalf("align note timestamps: %v", err)
	}

	firstPage := decodeOK[notesListDTO](t, a.request(http.MethodGet, "/api/v1/notes?limit=2", authorization, nil))
	if firstPage.NextPageToken == nil {
		t.Fatal("expected first page token")
	}
	expectErrorCode(
		t,
		a.request(http.MethodGet, "/api/v1/notes?limit=3&pageToken="+*firstPage.NextPageToken, authorization, nil),
		http.StatusBadRequest,
		4002,
	)

	// Page through the incremental stream with limit 2 and verify that
	// every note arrives exactly once and pagination terminates.
	seen := map[string]bool{}
	cursor := snapshot.Cursor
	pages := 0
	for {
		page := decodeOK[changesDTO](t, a.request(http.MethodGet, "/api/v1/notes/sync/changes?cursor="+cursor+"&limit=2", authorization, nil))
		pages++
		for _, entry := range page.Changes {
			if entry.Type == "note" && entry.Operation == "upsert" && entry.Note != nil {
				if seen[entry.Note.NoteID] {
					t.Fatalf("duplicate note in incremental stream: %s", entry.Note.NoteID)
				}
				seen[entry.Note.NoteID] = true
			}
		}
		if !page.HasMore {
			break
		}
		cursor = page.NextCursor
		if pages > 10 {
			t.Fatalf("incremental pagination did not terminate")
		}
	}

	if len(seen) != 5 || pages != 3 {
		t.Fatalf("unexpected pagination: seen=%d pages=%d", len(seen), pages)
	}
}

func TestNotesModifiedDuringSnapshotPaginationIsNotLost(t *testing.T) {
	a := newTestApp(t, 30*time.Second)
	defer a.close()

	auth := registerAndLogin(t, a)
	authorization := bearer(auth.Token)

	noteA := createNote(t, a, authorization, "A", "a", []string{}, []string{})
	noteB := createNote(t, a, authorization, "B", "b", []string{}, []string{})

	// Start a paged snapshot; page one covers noteA only.
	firstPage := decodeOK[snapshotDTO](t, a.request(http.MethodGet, "/api/v1/notes/sync/snapshot?limit=1", authorization, nil))
	if len(firstPage.Notes) != 1 || firstPage.Notes[0].NoteID != noteA.NoteID {
		t.Fatalf("unexpected first page: %+v", firstPage)
	}

	// Modify noteB while the snapshot pagination is still in flight: its
	// change sequence moves beyond the snapshot sequence.
	updatedB := decodeOK[noteDTO](t, a.request(http.MethodPut, "/api/v1/notes/"+noteB.NoteID, authorization, map[string]any{
		"baseRevision":  noteB.Revision,
		"title":         "B v2",
		"markdown":      "b v2",
		"tagIds":        []string{},
		"attachmentIds": []string{},
	}))

	// The remaining snapshot pages must not leak the mid-flight revision.
	secondPage := decodeOK[snapshotDTO](t, a.request(http.MethodGet, "/api/v1/notes/sync/snapshot?limit=1&pageToken="+*firstPage.NextPageToken, authorization, nil))
	if secondPage.NextPageToken != nil {
		t.Fatalf("expected final snapshot page, got %+v", secondPage)
	}
	if secondPage.Cursor != firstPage.Cursor {
		t.Fatalf("snapshot cursor changed mid-pagination")
	}

	// Incremental after the snapshot cursor must deliver noteB's newer state.
	changes := decodeOK[changesDTO](t, a.request(http.MethodGet, "/api/v1/notes/sync/changes?cursor="+secondPage.Cursor, authorization, nil))
	foundUpdate := false
	for _, entry := range changes.Changes {
		if entry.Type == "note" && entry.Operation == "upsert" && entry.Note != nil && entry.Note.NoteID == noteB.NoteID {
			if entry.Note.Revision != updatedB.Revision || entry.Note.Markdown != "b v2" {
				t.Fatalf("unexpected incremental content: %+v", entry.Note)
			}
			foundUpdate = true
		}
	}
	if !foundUpdate {
		t.Fatalf("mid-snapshot modification lost: %+v", changes.Changes)
	}
}

func TestNotesUpdateAfterDelete(t *testing.T) {
	a := newTestApp(t, 30*time.Second)
	defer a.close()

	auth := registerAndLogin(t, a)
	authorization := bearer(auth.Token)

	note := createNote(t, a, authorization, "Doomed", "body", []string{}, []string{})
	deleted := decodeOK[noteDeleteDTO](t, a.request(http.MethodDelete, "/api/v1/notes/"+note.NoteID+"?baseRevision="+fmt.Sprint(note.Revision), authorization, nil))

	// Every write against the deleted note reports it as gone.
	expectErrorCode(t, a.request(http.MethodPut, "/api/v1/notes/"+note.NoteID, authorization, map[string]any{
		"baseRevision":  deleted.Revision,
		"title":         "zombie",
		"markdown":      "",
		"tagIds":        []string{},
		"attachmentIds": []string{},
	}), http.StatusNotFound, 6001)
}

func TestNotesSyncCursorExpiry(t *testing.T) {
	a := newTestApp(t, 30*time.Second)
	defer a.close()

	auth := registerAndLogin(t, a)
	authorization := bearer(auth.Token)

	note := createNote(t, a, authorization, "One", "one", []string{}, []string{})
	createNote(t, a, authorization, "Two", "two", []string{}, []string{})
	snapshot := decodeOK[snapshotDTO](t, a.request(http.MethodGet, "/api/v1/notes/sync/snapshot", authorization, nil))
	firstPage := decodeOK[snapshotDTO](t, a.request(http.MethodGet, "/api/v1/notes/sync/snapshot?limit=1", authorization, nil))
	if firstPage.NextPageToken == nil {
		t.Fatal("expected a snapshot continuation")
	}

	// Simulate compaction of every event the cursor has not consumed.
	if err := a.db.Exec(`DELETE FROM note_change_log`).Error; err != nil {
		t.Fatalf("compact change log: %v", err)
	}
	if err := a.db.Exec(
		`INSERT INTO note_sync_state (user_id, compacted_through_seq)
		 SELECT id, 1000000 FROM users
		 ON CONFLICT (user_id) DO UPDATE SET compacted_through_seq = 1000000`,
	).Error; err != nil {
		t.Fatalf("raise compaction watermark: %v", err)
	}

	expectErrorCode(t, a.request(http.MethodGet, "/api/v1/notes/sync/changes?cursor="+snapshot.Cursor, authorization, nil), http.StatusGone, 6009)
	expectErrorCode(t, a.request(http.MethodGet, "/api/v1/notes/sync/snapshot?limit=1&pageToken="+*firstPage.NextPageToken, authorization, nil), http.StatusGone, 6009)
	fresh := decodeOK[snapshotDTO](t, a.request(http.MethodGet, "/api/v1/notes/sync/snapshot", authorization, nil))
	if len(fresh.Notes) != 2 || fresh.Notes[0].NoteID != note.NoteID {
		t.Fatalf("fresh snapshot lost active notes after compaction: %+v", fresh.Notes)
	}
	changes := decodeOK[changesDTO](t, a.request(http.MethodGet, "/api/v1/notes/sync/changes?cursor="+fresh.Cursor, authorization, nil))
	if len(changes.Changes) != 0 || changes.HasMore {
		t.Fatalf("fresh snapshot cursor expired: %+v", changes)
	}
}

func TestNotesUploadFileBeforeFields(t *testing.T) {
	a := newTestApp(t, 30*time.Second)
	defer a.close()
	auth := registerAndLogin(t, a)
	content := []byte("file before metadata")
	attachmentID := newAttachmentID()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	file, err := writer.CreateFormFile("file", "early.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(content); err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{
		"attachmentId": attachmentID,
		"kind":         "file",
		"sha256":       digestOf(content),
	} {
		if err := writer.WriteField(name, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, a.server.URL+"/api/v1/note-attachments", body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", bearer(auth.Token))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	uploaded := decodeOK[noteAttachmentDTO](t, resp)
	if uploaded.AttachmentID != attachmentID || uploaded.Size != int64(len(content)) {
		t.Fatalf("unexpected upload: %+v", uploaded)
	}
}

func TestNotesCrossAccountIsolation(t *testing.T) {
	a := newTestApp(t, 30*time.Second)
	defer a.close()

	authA := registerAndLogin(t, a)
	authB := registerAndLogin(t, a)
	authorizationA := bearer(authA.Token)
	authorizationB := bearer(authB.Token)

	note := createNote(t, a, authorizationA, "Private", "secret", []string{}, []string{})
	tag := createTag(t, a, authorizationA, "PrivateTag")
	content := []byte("private file")
	attachmentID := newAttachmentID()
	decodeOK[noteAttachmentDTO](t, a.uploadAttachment(t, authorizationA, attachmentID, "file", digestOf(content), "private.txt", content))

	// Account B sees nothing: same errors as nonexistent resources.
	expectErrorCode(t, a.request(http.MethodGet, "/api/v1/notes/"+note.NoteID, authorizationB, nil), http.StatusNotFound, 6001)
	expectErrorCode(t, a.request(http.MethodDelete, "/api/v1/notes/"+note.NoteID+"?baseRevision=1", authorizationB, nil), http.StatusNotFound, 6001)
	expectErrorCode(t, a.request(http.MethodPut, "/api/v1/note-tags/"+tag.TagID, authorizationB, map[string]any{
		"baseRevision": 1,
		"name":         "Hijack",
	}), http.StatusNotFound, 6003)
	expectErrorCode(t, a.request(http.MethodDelete, "/api/v1/note-tags/"+tag.TagID+"?baseRevision=1", authorizationB, nil), http.StatusNotFound, 6003)
	expectErrorCode(t, a.request(http.MethodGet, "/api/v1/note-attachments/"+attachmentID, authorizationB, nil), http.StatusNotFound, 6005)
	expectErrorCode(t, a.request(http.MethodGet, "/api/v1/note-attachments/"+attachmentID+"/content", authorizationB, nil), http.StatusNotFound, 6005)
	expectErrorCode(t, a.request(http.MethodDelete, "/api/v1/note-attachments/"+attachmentID, authorizationB, nil), http.StatusNotFound, 6005)

	// B cannot reference A's resources.
	expectErrorCode(t, a.request(http.MethodPost, "/api/v1/notes", authorizationB, map[string]any{
		"noteId":        newNoteID(),
		"title":         "steal",
		"markdown":      "",
		"tagIds":        []string{tag.TagID},
		"attachmentIds": []string{attachmentID},
	}), http.StatusConflict, 6008)

	// B's snapshot does not contain A's resources.
	snapshot := decodeOK[snapshotDTO](t, a.request(http.MethodGet, "/api/v1/notes/sync/snapshot", authorizationB, nil))
	if len(snapshot.Notes) != 0 || len(snapshot.Tags) != 0 {
		t.Fatalf("snapshot leaked data: %+v", snapshot)
	}

	// Resource IDs are permanent only within an account. B can reuse all
	// three UUIDs without overwriting or observing A's resources.
	tagB := decodeOK[tagDTO](t, a.request(http.MethodPost, "/api/v1/note-tags", authorizationB, map[string]any{
		"tagId": tag.TagID,
		"name":  "PrivateTag",
	}))
	contentB := []byte("account B file")
	attachmentB := decodeOK[noteAttachmentDTO](t, a.uploadAttachment(
		t,
		authorizationB,
		attachmentID,
		"file",
		digestOf(contentB),
		"private-b.txt",
		contentB,
	))
	noteB := decodeOK[noteDTO](t, a.request(http.MethodPost, "/api/v1/notes", authorizationB, map[string]any{
		"noteId":        note.NoteID,
		"title":         "Private B",
		"markdown":      "account B",
		"tagIds":        []string{tagB.TagID},
		"attachmentIds": []string{attachmentB.AttachmentID},
	}))
	if noteB.Title != "Private B" || len(noteB.TagIDs) != 1 || len(noteB.Attachments) != 1 {
		t.Fatalf("unexpected account B note: %+v", noteB)
	}
	noteA := decodeOK[noteDTO](t, a.request(http.MethodGet, "/api/v1/notes/"+note.NoteID, authorizationA, nil))
	if noteA.Title != "Private" || noteA.Markdown != "secret" {
		t.Fatalf("account A note was overwritten: %+v", noteA)
	}
}

func TestNotesListPagination(t *testing.T) {
	a := newTestApp(t, 30*time.Second)
	defer a.close()

	auth := registerAndLogin(t, a)
	authorization := bearer(auth.Token)

	created := make(map[string]bool)
	for i := 0; i < 5; i++ {
		note := createNote(t, a, authorization, fmt.Sprintf("Note %d", i), fmt.Sprintf("body %d", i), []string{}, []string{})
		created[note.NoteID] = true
	}

	seen := map[string]bool{}
	pageToken := ""
	pages := 0
	for {
		path := "/api/v1/notes?limit=2"
		if pageToken != "" {
			path += "&pageToken=" + pageToken
		}
		page := decodeOK[notesListDTO](t, a.request(http.MethodGet, path, authorization, nil))
		pages++
		for _, summary := range page.Notes {
			if seen[summary.NoteID] {
				t.Fatalf("duplicate note across pages: %s", summary.NoteID)
			}
			seen[summary.NoteID] = true
		}
		if page.NextPageToken == nil {
			break
		}
		pageToken = *page.NextPageToken
		if pages > 10 {
			t.Fatalf("pagination did not terminate")
		}
	}

	if len(seen) != 5 || pages != 3 {
		t.Fatalf("unexpected pagination: seen=%d pages=%d", len(seen), pages)
	}
}

func registerAndLogin(t *testing.T, a *testApp) authResult {
	t.Helper()

	email := fmt.Sprintf("notes-%s@example.com", uuid.NewString())
	register := a.request(http.MethodPost, "/api/v1/auth/register", "", map[string]any{
		"email":    email,
		"username": "notesuser" + uuid.NewString()[:8],
		"password": "password123",
	})

	return decodeOK[authResult](t, register)
}
