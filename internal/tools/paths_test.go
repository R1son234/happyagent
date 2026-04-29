package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRootedPathResolverRejectsSymlinkEscapingRootOnRead(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()

	escapePath := filepath.Join(root, "escape")
	if err := os.Symlink(outside, escapePath); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	resolver, err := NewRootedPathResolver(root)
	if err != nil {
		t.Fatalf("NewRootedPathResolver() error = %v", err)
	}

	_, err = resolver.Resolve(filepath.Join("escape", "secret.txt"))
	if err == nil {
		t.Fatal("expected symlink escape to be rejected")
	}
}

func TestRootedPathResolverRejectsSymlinkEscapingRootOnWrite(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	linkPath := filepath.Join(root, "linked-dir")

	if err := os.Symlink(outside, linkPath); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	resolver, err := NewRootedPathResolver(root)
	if err != nil {
		t.Fatalf("NewRootedPathResolver() error = %v", err)
	}

	_, err = resolver.Resolve(filepath.Join("linked-dir", "new.txt"))
	if err == nil {
		t.Fatal("expected symlink escape to be rejected")
	}
}

func TestRootedPathResolverAllowsRegularChildPath(t *testing.T) {
	root := t.TempDir()
	resolver, err := NewRootedPathResolver(root)
	if err != nil {
		t.Fatalf("NewRootedPathResolver() error = %v", err)
	}

	resolved, err := resolver.Resolve(filepath.Join("nested", "file.txt"))
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	expected := filepath.Join(root, "nested", "file.txt")
	expected, err = filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("EvalSymlinks() error = %v", err)
	}
	expected = filepath.Join(expected, "nested", "file.txt")
	if resolved != expected {
		t.Fatalf("unexpected path: got %q want %q", resolved, expected)
	}
}
