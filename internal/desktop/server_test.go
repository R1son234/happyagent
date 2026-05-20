package desktop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"happyagent/internal/career"
)

func TestIsWithin(t *testing.T) {
	root := t.TempDir()
	if !isWithin(root, root) {
		t.Fatalf("root should be within itself")
	}
	if !isWithin(root, root+"/child/file.md") {
		t.Fatalf("child path should be within root")
	}
	if isWithin(root, root+"/../outside.md") {
		t.Fatalf("parent escape should not be within root")
	}
}

func TestPreviewKind(t *testing.T) {
	tests := map[string]string{
		".md":   "markdown",
		".txt":  "text",
		".json": "json",
		".pdf":  "pdf",
		".docx": "docx",
		".bin":  "unsupported",
	}
	for ext, want := range tests {
		if got := previewKind(ext); got != want {
			t.Fatalf("previewKind(%q) = %q, want %q", ext, got, want)
		}
	}
}

func TestBuildTreeShowsUserVisibleMaterialDirectories(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, career.WorkspaceDirResume, "resume.md"), "# Resume\n")
	mustWriteFile(t, filepath.Join(root, career.WorkspaceDirJD, "jd.md"), "# JD\n")
	mustWriteFile(t, filepath.Join(root, career.WorkspaceDirExperiences, "experience.md"), "# Experience\n")
	mustWriteFile(t, filepath.Join(root, career.WorkspaceDirResume, "metadata.json"), "{}\n")
	mustWriteFile(t, filepath.Join(root, career.WorkspaceDirResume, "extracted.md"), "internal\n")
	mustWriteFile(t, filepath.Join(root, career.WorkspaceInternalDir, "index.json"), "{}\n")
	mustWriteFile(t, filepath.Join(root, career.WorkspaceDirResume, "item-dir", "metadata.json"), "{}\n")
	mustWriteFile(t, filepath.Join(root, career.WorkspaceDirResume, "item-dir", "extracted.md"), "internal\n")

	materialDirs := map[string]bool{
		career.WorkspaceDirResume:      true,
		career.WorkspaceDirJD:          true,
		career.WorkspaceDirExperiences: true,
	}
	tree, err := buildTree(root, root, 5, materialDirs)
	if err != nil {
		t.Fatalf("buildTree() error = %v", err)
	}

	resumeDir, ok := findChild(tree, career.WorkspaceDirResume)
	if !ok || resumeDir.Kind != "directory" {
		t.Fatalf("expected visible resume directory, got %+v", tree.Children)
	}
	if _, ok := findChild(tree, career.WorkspaceDirJD); !ok {
		t.Fatalf("expected visible JD directory, got %+v", tree.Children)
	}
	if _, ok := findChild(tree, career.WorkspaceDirExperiences); !ok {
		t.Fatalf("expected visible experiences directory, got %+v", tree.Children)
	}
	if _, ok := findChild(resumeDir, "resume.md"); !ok {
		t.Fatalf("expected user-visible resume file, got %+v", resumeDir.Children)
	}
	for _, hidden := range []string{"metadata.json", "extracted.md", "item-dir"} {
		if _, ok := findChild(resumeDir, hidden); ok {
			t.Fatalf("expected %s to be hidden, got %+v", hidden, resumeDir.Children)
		}
	}
	if _, ok := findChild(tree, ".happyagent"); ok {
		t.Fatalf("expected internal .happyagent directory to be hidden")
	}
}

func TestNormalizeConfigJSONFormatsValidJSON(t *testing.T) {
	formatted, err := normalizeConfigJSON(`{"llm":{"model":"test"}}`)
	if err != nil {
		t.Fatalf("normalizeConfigJSON() error = %v", err)
	}
	got := string(formatted)
	if !strings.Contains(got, "\n  \"llm\": {") {
		t.Fatalf("expected indented JSON, got:\n%s", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Fatalf("expected trailing newline, got %q", got)
	}
}

func TestNormalizeConfigJSONRejectsInvalidJSON(t *testing.T) {
	if _, err := normalizeConfigJSON(`{"llm":`); err == nil {
		t.Fatalf("expected invalid JSON to be rejected")
	}
}

func mustWriteFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func findChild(node fileNode, name string) (fileNode, bool) {
	for _, child := range node.Children {
		if child.Name == name {
			return child, true
		}
	}
	return fileNode{}, false
}
