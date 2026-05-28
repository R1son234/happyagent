package tasks

import (
	"sync"
	"testing"
)

func TestTaskStoreBlocksClaimUntilDependenciesComplete(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	dep, err := store.Create(Task{ID: "dep", Title: "Dependency"})
	if err != nil {
		t.Fatalf("Create(dep) error = %v", err)
	}
	task, err := store.Create(Task{ID: "child", Title: "Child", BlockedBy: []string{dep.ID}})
	if err != nil {
		t.Fatalf("Create(child) error = %v", err)
	}
	if _, err := store.Claim(task.ID, "worker-a"); err == nil {
		t.Fatalf("expected blocked claim to fail")
	}
	if _, err := store.Claim(dep.ID, "worker-a"); err != nil {
		t.Fatalf("Claim(dep) error = %v", err)
	}
	if _, err := store.Complete(dep.ID, nil); err != nil {
		t.Fatalf("Complete(dep) error = %v", err)
	}
	claimed, err := store.Claim(task.ID, "worker-b")
	if err != nil {
		t.Fatalf("Claim(child) error = %v", err)
	}
	if claimed.Owner != "worker-b" || claimed.Status != StatusInProgress {
		t.Fatalf("unexpected claimed task: %+v", claimed)
	}
}

func TestTaskStoreRejectsDependencyCycles(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	if _, err := store.Create(Task{ID: "a", Title: "A"}); err != nil {
		t.Fatalf("Create(a) error = %v", err)
	}
	if _, err := store.Create(Task{ID: "b", Title: "B", BlockedBy: []string{"a"}}); err != nil {
		t.Fatalf("Create(b) error = %v", err)
	}
	if _, err := store.Update(Task{ID: "a", Title: "A", Status: StatusPending, BlockedBy: []string{"b"}}); err == nil {
		t.Fatalf("expected cycle update to fail")
	}
}

func TestTaskStoreAllowsOnlyOneConcurrentClaim(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	task, err := store.Create(Task{ID: "shared", Title: "Shared"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	var wg sync.WaitGroup
	successes := make(chan string, 2)
	for _, owner := range []string{"a", "b"} {
		wg.Add(1)
		go func(owner string) {
			defer wg.Done()
			if claimed, err := store.Claim(task.ID, owner); err == nil {
				successes <- claimed.Owner
			}
		}(owner)
	}
	wg.Wait()
	close(successes)
	if len(successes) != 1 {
		t.Fatalf("expected exactly one successful claim, got %d", len(successes))
	}
}
