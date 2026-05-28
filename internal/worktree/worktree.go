package worktree

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

type Manager struct {
	root string
}

func NewManager(root string) (*Manager, error) {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	return &Manager{root: abs}, nil
}

func (m *Manager) Create(ctx context.Context, slug string, branch string) (string, error) {
	slug = cleanSlug(slug)
	if slug == "" {
		return "", fmt.Errorf("worktree slug cannot be empty")
	}
	if branch == "" {
		branch = "happyagent/" + slug
	}
	path := filepath.Join(m.root, ".happyagent", "worktrees", slug)
	args := []string{"worktree", "add", "-B", branch, path}
	if out, err := m.git(ctx, args...); err != nil {
		return "", fmt.Errorf("create worktree: %w: %s", err, out)
	}
	return path, nil
}

func (m *Manager) Remove(ctx context.Context, path string, discardDirty bool) error {
	resolved, err := m.safePath(path)
	if err != nil {
		return err
	}
	if !discardDirty {
		out, err := m.git(ctx, "-C", resolved, "status", "--porcelain")
		if err != nil {
			return fmt.Errorf("check worktree status: %w: %s", err, out)
		}
		if strings.TrimSpace(out) != "" {
			return fmt.Errorf("refusing to remove dirty worktree %s", resolved)
		}
	}
	args := []string{"worktree", "remove"}
	if discardDirty {
		args = append(args, "--force")
	}
	args = append(args, resolved)
	if out, err := m.git(ctx, args...); err != nil {
		return fmt.Errorf("remove worktree: %w: %s", err, out)
	}
	return nil
}

func (m *Manager) Enter(path string) (string, error) {
	return m.safePath(path)
}

func (m *Manager) Keep(path string) (string, error) {
	return m.safePath(path)
}

func (m *Manager) safePath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("worktree path cannot be empty")
	}
	resolved := path
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(m.root, resolved)
	}
	resolved = filepath.Clean(resolved)
	base := filepath.Join(m.root, ".happyagent", "worktrees")
	rel, err := filepath.Rel(base, resolved)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("worktree path %q must be under %s", path, base)
	}
	return resolved, nil
}

func (m *Manager) git(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = m.root
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	return strings.TrimSpace(output.String()), err
}

func cleanSlug(slug string) string {
	slug = strings.ToLower(strings.TrimSpace(slug))
	var b strings.Builder
	for _, r := range slug {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune('-')
		default:
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}
