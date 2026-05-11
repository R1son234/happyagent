package tools

import (
	"fmt"
	"os"
	"path/filepath"
)

type RootedPathResolver struct {
	root string
}

func NewRootedPathResolver(root string) (*RootedPathResolver, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve root path %q: %w", root, err)
	}
	realRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return nil, fmt.Errorf("evaluate root path %q: %w", absRoot, err)
	}

	return &RootedPathResolver{root: realRoot}, nil
}

func (r *RootedPathResolver) Resolve(path string) (string, error) {
	target := path
	if !filepath.IsAbs(target) {
		target = filepath.Join(r.root, target)
	}

	clean := filepath.Clean(target)
	resolved, err := r.resolveSymlinksWithinRoot(clean)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(r.root, resolved)
	if err != nil {
		return "", fmt.Errorf("calculate relative path for %q: %w", resolved, err)
	}
	if rel == ".." || len(rel) >= 3 && rel[:3] == "../" {
		return "", fmt.Errorf("path %q escapes root %q", path, r.root)
	}

	return resolved, nil
}

func (r *RootedPathResolver) Root() string {
	return r.root
}

func (r *RootedPathResolver) resolveSymlinksWithinRoot(target string) (string, error) {
	if _, err := os.Lstat(target); err == nil {
		resolved, err := filepath.EvalSymlinks(target)
		if err != nil {
			return "", fmt.Errorf("evaluate path %q: %w", target, err)
		}
		return filepath.Clean(resolved), nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("stat path %q: %w", target, err)
	}

	current := filepath.Clean(target)
	var unresolved []string
	for {
		if current == filepath.Dir(current) {
			return "", fmt.Errorf("path %q escapes root %q", target, r.root)
		}
		if _, err := os.Lstat(current); err == nil {
			resolvedParent, err := filepath.EvalSymlinks(current)
			if err != nil {
				return "", fmt.Errorf("evaluate path %q: %w", current, err)
			}
			resolved := filepath.Clean(resolvedParent)
			for i := len(unresolved) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, unresolved[i])
			}
			return resolved, nil
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("stat path %q: %w", current, err)
		}
		unresolved = append(unresolved, filepath.Base(current))
		current = filepath.Dir(current)
	}
}
