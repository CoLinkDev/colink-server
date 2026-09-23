package janitor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveStorageFileRejectsEscapingPath(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(filepath.Dir(root), "outside.txt")
	if err := os.WriteFile(outside, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(outside) })

	if err := removeStorageFile(root, filepath.Join("..", filepath.Base(outside))); err == nil {
		t.Fatal("expected escaping path to be rejected")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("outside file was changed: %v", err)
	}
}

func TestRemoveStorageFileRemovesContainedFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "account", "attachment")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := removeStorageFile(root, filepath.Join("account", "attachment")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected file removal, got %v", err)
	}
}

func TestRemoveStorageFileRejectsAbsolutePath(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := removeStorageFile(root, outside); err == nil {
		t.Fatal("expected absolute path to be rejected")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("outside file was changed: %v", err)
	}
}
