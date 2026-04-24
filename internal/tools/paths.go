package tools

import (
	"fmt"
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

	return &RootedPathResolver{root: absRoot}, nil
}

func (r *RootedPathResolver) Resolve(path string) (string, error) {
	target := path
	if !filepath.IsAbs(target) {
		target = filepath.Join(r.root, target)
	}

	clean := filepath.Clean(target)
	rel, err := filepath.Rel(r.root, clean)
	if err != nil {
		return "", fmt.Errorf("calculate relative path for %q: %w", clean, err)
	}
	if rel == ".." || len(rel) >= 3 && rel[:3] == "../" {
		return "", fmt.Errorf("path %q escapes root %q", path, r.root)
	}

	return clean, nil
}

func (r *RootedPathResolver) Root() string {
	return r.root
}
