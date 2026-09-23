package integration_test

import (
	"bytes"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func pauseFirstNotesQuery(t *testing.T, a *testApp) (<-chan struct{}, func()) {
	t.Helper()

	const callbackName = "test:pause-first-notes-query"
	started := make(chan struct{})
	resume := make(chan struct{})
	var resumeOnce sync.Once
	var paused atomic.Bool
	if err := a.db.Callback().Query().After("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		table := tx.Statement.Table
		if tx.Statement.Schema != nil {
			table = tx.Statement.Schema.Table
		}
		if table != "notes" || !paused.CompareAndSwap(false, true) {
			return
		}
		close(started)
		<-resume
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = a.db.Callback().Query().Remove(callbackName)
	})

	return started, func() {
		resumeOnce.Do(func() { close(resume) })
	}
}

func TestGetNoteUsesConsistentReadSnapshot(t *testing.T) {
	a := newTestApp(t, 30*time.Second)
	defer a.close()

	auth := registerAndLogin(t, a)
	authorization := bearer(auth.Token)
	firstTag := createTag(t, a, authorization, "First")
	secondTag := createTag(t, a, authorization, "Second")
	note := createNote(t, a, authorization, "Original", "original", []string{firstTag.TagID}, []string{})

	started, resume := pauseFirstNotesQuery(t, a)
	defer resume()
	responses := make(chan *http.Response, 1)
	go func() {
		responses <- a.request(http.MethodGet, "/api/v1/notes/"+note.NoteID, authorization, nil)
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("note read did not reach the pause point")
	}

	decodeOK[noteDTO](t, a.request(http.MethodPut, "/api/v1/notes/"+note.NoteID, authorization, map[string]any{
		"baseRevision":  note.Revision,
		"title":         "Updated",
		"markdown":      "updated",
		"tagIds":        []string{secondTag.TagID},
		"attachmentIds": []string{},
	}))
	resume()

	var got noteDTO
	select {
	case response := <-responses:
		got = decodeOK[noteDTO](t, response)
	case <-time.After(5 * time.Second):
		t.Fatal("note read did not complete")
	}
	if got.Revision != note.Revision || got.Markdown != note.Markdown || len(got.TagIDs) != 1 || got.TagIDs[0] != firstTag.TagID {
		t.Fatalf("note response mixed database revisions: %+v", got)
	}
}

func TestSnapshotPageUsesConsistentReadSnapshot(t *testing.T) {
	a := newTestApp(t, 30*time.Second)
	defer a.close()

	auth := registerAndLogin(t, a)
	authorization := bearer(auth.Token)
	firstTag := createTag(t, a, authorization, "First")
	secondTag := createTag(t, a, authorization, "Second")
	note := createNote(t, a, authorization, "Original", "original", []string{firstTag.TagID}, []string{})

	started, resume := pauseFirstNotesQuery(t, a)
	defer resume()
	responses := make(chan *http.Response, 1)
	go func() {
		responses <- a.request(http.MethodGet, "/api/v1/notes/sync/snapshot", authorization, nil)
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("snapshot read did not reach the pause point")
	}

	decodeOK[noteDTO](t, a.request(http.MethodPut, "/api/v1/notes/"+note.NoteID, authorization, map[string]any{
		"baseRevision":  note.Revision,
		"title":         "Updated",
		"markdown":      "updated",
		"tagIds":        []string{secondTag.TagID},
		"attachmentIds": []string{},
	}))
	resume()

	var snapshot snapshotDTO
	select {
	case response := <-responses:
		snapshot = decodeOK[snapshotDTO](t, response)
	case <-time.After(5 * time.Second):
		t.Fatal("snapshot read did not complete")
	}
	if len(snapshot.Notes) != 1 {
		t.Fatalf("unexpected snapshot notes: %+v", snapshot.Notes)
	}
	got := snapshot.Notes[0]
	if got.Revision != note.Revision || got.Markdown != note.Markdown || len(got.TagIDs) != 1 || got.TagIDs[0] != firstTag.TagID {
		t.Fatalf("snapshot response mixed database revisions: %+v", got)
	}
}

func TestNotesSnapshotCursorIsAccountScoped(t *testing.T) {
	a := newTestApp(t, 30*time.Second)
	defer a.close()

	authA := registerAndLogin(t, a)
	authB := registerAndLogin(t, a)

	// Model an account-A write that has allocated its global sequence but has
	// not committed yet. A committed account-B event receives a higher global
	// sequence and must not move account A's snapshot boundary past this write.
	tx := a.db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	if err := tx.Exec("SELECT id FROM users WHERE id = ? FOR UPDATE", authA.UserID).Error; err != nil {
		t.Fatal(err)
	}
	tagID := uuid.New()
	now := time.Now().UTC()
	if err := tx.Exec(
		`INSERT INTO note_tags (user_id, id, name, name_normalized, revision, change_seq, created_at, updated_at)
		 VALUES (?, ?, 'Pending A', 'pending a', 1, 0, ?, ?)`,
		authA.UserID, tagID, now, now,
	).Error; err != nil {
		t.Fatal(err)
	}
	var seq int64
	if err := tx.Raw(
		`INSERT INTO note_change_log (user_id, resource_type, resource_id, operation, revision, created_at)
		 VALUES (?, 'tag', ?, 'upsert', 1, ?) RETURNING seq`,
		authA.UserID, tagID, now,
	).Scan(&seq).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(
		"UPDATE note_tags SET change_seq = ? WHERE user_id = ? AND id = ?",
		seq, authA.UserID, tagID,
	).Error; err != nil {
		t.Fatal(err)
	}

	createTag(t, a, bearer(authB.Token), "Committed B")
	snapshot := decodeOK[snapshotDTO](t, a.request(
		http.MethodGet,
		"/api/v1/notes/sync/snapshot",
		bearer(authA.Token),
		nil,
	))
	if err := tx.Commit().Error; err != nil {
		t.Fatal(err)
	}

	changes := decodeOK[changesDTO](t, a.request(
		http.MethodGet,
		"/api/v1/notes/sync/changes?cursor="+snapshot.Cursor,
		bearer(authA.Token),
		nil,
	))
	if len(changes.Changes) != 1 || changes.Changes[0].Tag == nil || changes.Changes[0].Tag.TagID != tagID.String() {
		t.Fatalf("account-scoped snapshot cursor skipped the pending event: %+v", changes)
	}
}

func TestNotesTagCreateWaitsForAccountLock(t *testing.T) {
	a := newTestApp(t, 30*time.Second)
	defer a.close()

	auth := registerAndLogin(t, a)
	tx := a.db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	if err := tx.Exec("SELECT id FROM users WHERE id = ? FOR UPDATE", auth.UserID).Error; err != nil {
		t.Fatal(err)
	}

	requestBody := []byte(fmt.Sprintf(`{"tagId":%q,"name":"Serialized"}`, uuid.NewString()))
	started := make(chan struct{})
	responses := make(chan *http.Response, 1)
	errors := make(chan error, 1)
	go func() {
		req, err := http.NewRequest(http.MethodPost, a.server.URL+"/api/v1/note-tags", bytes.NewReader(requestBody))
		if err != nil {
			errors <- err
			return
		}
		req.Header.Set("Authorization", bearer(auth.Token))
		req.Header.Set("Content-Type", "application/json")
		close(started)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			errors <- err
			return
		}
		responses <- resp
	}()
	<-started

	select {
	case resp := <-responses:
		resp.Body.Close()
		t.Fatal("tag creation completed while the account lock was held")
	case err := <-errors:
		t.Fatal(err)
	case <-time.After(150 * time.Millisecond):
	}

	if err := tx.Rollback().Error; err != nil {
		t.Fatal(err)
	}
	select {
	case resp := <-responses:
		decodeOK[tagDTO](t, resp)
	case err := <-errors:
		t.Fatal(err)
	case <-time.After(5 * time.Second):
		t.Fatal("tag creation did not resume after releasing the account lock")
	}
}

func TestNotesStaleUpsertsCanProduceEmptyPages(t *testing.T) {
	a := newTestApp(t, 30*time.Second)
	defer a.close()

	auth := registerAndLogin(t, a)
	authorization := bearer(auth.Token)
	snapshot := decodeOK[snapshotDTO](t, a.request(http.MethodGet, "/api/v1/notes/sync/snapshot", authorization, nil))

	note := createNote(t, a, authorization, "Transient", "v1", []string{}, []string{})
	updated := decodeOK[noteDTO](t, a.request(http.MethodPut, "/api/v1/notes/"+note.NoteID, authorization, map[string]any{
		"baseRevision":  note.Revision,
		"title":         "Transient",
		"markdown":      "v2",
		"tagIds":        []string{},
		"attachmentIds": []string{},
	}))
	decodeOK[noteDeleteDTO](t, a.request(
		http.MethodDelete,
		"/api/v1/notes/"+note.NoteID+"?baseRevision="+fmt.Sprint(updated.Revision),
		authorization,
		nil,
	))

	first := decodeOK[changesDTO](t, a.request(http.MethodGet, "/api/v1/notes/sync/changes?cursor="+snapshot.Cursor+"&limit=1", authorization, nil))
	if len(first.Changes) != 0 || !first.HasMore || first.NextCursor == snapshot.Cursor {
		t.Fatalf("unexpected first suppressed page: %+v", first)
	}
	second := decodeOK[changesDTO](t, a.request(http.MethodGet, "/api/v1/notes/sync/changes?cursor="+first.NextCursor+"&limit=1", authorization, nil))
	if len(second.Changes) != 0 || !second.HasMore || second.NextCursor == first.NextCursor {
		t.Fatalf("unexpected second suppressed page: %+v", second)
	}
	third := decodeOK[changesDTO](t, a.request(http.MethodGet, "/api/v1/notes/sync/changes?cursor="+second.NextCursor+"&limit=1", authorization, nil))
	if len(third.Changes) != 1 || third.HasMore || third.Changes[0].Operation != "delete" || third.Changes[0].NoteID == nil || *third.Changes[0].NoteID != note.NoteID {
		t.Fatalf("missing convergent delete after suppressed upserts: %+v", third)
	}
}
