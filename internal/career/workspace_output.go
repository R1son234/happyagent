package career

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type UserOutputPaths struct {
	LatestMarkdown      string
	TimestampedMarkdown string
	LatestJSON          string
	TimestampedJSON     string
}

func (w *Workspace) WriteArtifact(kind string, title string, content string, now time.Time) (string, error) {
	if strings.TrimSpace(kind) == "" {
		return "", fmt.Errorf("artifact kind must not be empty")
	}
	if strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("artifact content must not be empty")
	}
	if now.IsZero() {
		now = time.Now()
	}
	dir, itemType := artifactDestination(kind)
	name := fmt.Sprintf("%s-%s.md", kind, now.Format("20060102-150405"))
	rel := filepath.Join(dir, name)
	abs := filepath.Join(w.Root, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", fmt.Errorf("create artifact dir: %w", err)
	}
	if strings.TrimSpace(title) != "" && !strings.HasPrefix(content, "# ") {
		content = "# " + title + "\n\n" + content
	}
	if err := os.WriteFile(abs, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
		return "", fmt.Errorf("write artifact: %w", err)
	}
	item := WorkspaceItem{
		ID:        fmt.Sprintf("%s-%s", kind, now.Format("20060102-150405")),
		Type:      itemType,
		Title:     title,
		Path:      filepath.ToSlash(rel),
		Tags:      []string{kind},
		CreatedAt: now,
		Summary:   summarizeMaterial(content),
	}
	if err := w.upsertIndexItem(item); err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

func (w *Workspace) LatestOutputPath(kind string, ext string) string {
	if strings.TrimSpace(ext) == "" {
		ext = ".md"
	}
	return filepath.ToSlash(filepath.Join("outputs", latestOutputName(kind)+ext))
}

func (w *Workspace) TimestampedOutputPath(kind string, ext string, now time.Time) string {
	if strings.TrimSpace(ext) == "" {
		ext = ".md"
	}
	if now.IsZero() {
		now = time.Now()
	}
	return filepath.ToSlash(filepath.Join("outputs", "runs", fmt.Sprintf("%s-%s%s", now.Format("20060102-150405"), latestOutputName(kind), ext)))
}

func (w *Workspace) WriteUserOutput(kind string, title string, markdown string, jsonContent []byte, now time.Time) (UserOutputPaths, error) {
	if strings.TrimSpace(kind) == "" {
		return UserOutputPaths{}, fmt.Errorf("user output kind must not be empty")
	}
	if strings.TrimSpace(markdown) == "" && len(jsonContent) == 0 {
		return UserOutputPaths{}, fmt.Errorf("user output content must not be empty")
	}
	if now.IsZero() {
		now = time.Now()
	}
	paths := UserOutputPaths{}
	if strings.TrimSpace(markdown) != "" {
		content := strings.TrimSpace(markdown)
		if strings.TrimSpace(title) != "" && !strings.HasPrefix(content, "# ") {
			content = "# " + title + "\n\n" + content
		}
		paths.LatestMarkdown = w.LatestOutputPath(kind, ".md")
		paths.TimestampedMarkdown = w.TimestampedOutputPath(kind, ".md", now)
		if err := w.writeWorkspaceText(paths.LatestMarkdown, content); err != nil {
			return UserOutputPaths{}, err
		}
		if err := w.writeWorkspaceText(paths.TimestampedMarkdown, content); err != nil {
			return UserOutputPaths{}, err
		}
	}
	if len(jsonContent) > 0 {
		content := append(trimTrailingNewlines(jsonContent), '\n')
		paths.LatestJSON = w.LatestOutputPath(kind, ".json")
		paths.TimestampedJSON = w.TimestampedOutputPath(kind, ".json", now)
		if err := w.writeWorkspaceBytes(paths.LatestJSON, content); err != nil {
			return UserOutputPaths{}, err
		}
		if err := w.writeWorkspaceBytes(paths.TimestampedJSON, content); err != nil {
			return UserOutputPaths{}, err
		}
	}
	return paths, nil
}

func artifactDestination(kind string) (string, string) {
	switch kind {
	case "project-pitch":
		return filepath.Join("prepare", "generated"), WorkspaceTypePrepare
	default:
		return filepath.Join("record", "generated"), WorkspaceTypeRecord
	}
}

func latestOutputName(kind string) string {
	switch strings.TrimSpace(kind) {
	case "report", "career-report", "analyze":
		return "latest-report"
	case "resume-review", "resume_review":
		return "latest-resume-review"
	case "interview-brief", "interview_brief":
		return "latest-interview-brief"
	case "gap-plan", "gap_plan":
		return "latest-gap-plan"
	case "interview-review", "interview_review":
		return "latest-interview-review"
	case "jd-match":
		return "latest-jd-match"
	case "project-pitch":
		return "latest-project-pitch"
	case "review-material":
		return "latest-review-material"
	default:
		return "latest-" + slug(kind)
	}
}

func trimTrailingNewlines(data []byte) []byte {
	return []byte(strings.TrimRight(string(data), "\n"))
}
