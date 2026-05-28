package background

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoreWritesJobOutputAndConsumesOnce(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	job := store.Start("shell")
	done := store.Complete(job.ID, "hello", nil)
	if done.Status != StatusComplete {
		t.Fatalf("unexpected status: %+v", done)
	}
	data, err := os.ReadFile(filepath.Join(root, done.LogPath))
	if err != nil {
		t.Fatalf("expected output log: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("unexpected log output: %q", string(data))
	}
	first := store.UnconsumedFinished()
	if len(first) != 1 || first[0].ID != job.ID {
		t.Fatalf("unexpected first notifications: %+v", first)
	}
	second := store.UnconsumedFinished()
	if len(second) != 0 {
		t.Fatalf("expected notification to be consumed, got %+v", second)
	}
}
