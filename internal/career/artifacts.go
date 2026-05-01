package career

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func RenderWorkspaceArtifact(workspace *Workspace, kind string) (string, string, error) {
	_, index, err := workspace.Status()
	if err != nil {
		return "", "", err
	}
	switch kind {
	case "jd-match":
		return "JD Match Report", renderJDMatch(workspace, index), nil
	case "resume-review":
		return "Resume Review", renderResumeReview(workspace, index), nil
	case "project-pitch":
		return "Project Pitch", renderProjectPitch(workspace, index), nil
	case "interview-review":
		return "Interview Review", renderInterviewReview(workspace, index), nil
	case "review-material":
		return "Review Material", renderReviewMaterial(workspace, index), nil
	default:
		return "", "", fmt.Errorf("unknown export kind %q", kind)
	}
}

func renderJDMatch(workspace *Workspace, index WorkspaceIndex) string {
	var b strings.Builder
	b.WriteString("# JD Match Report\n\n")
	writeItemsSection(&b, workspace, "Active JD Signals", filterItems(index, WorkspaceTypeJD), 2)
	writeItemsSection(&b, workspace, "Resume Evidence", filterItems(index, WorkspaceTypeResume), 1)
	writeItemsSection(&b, workspace, "Project Evidence", filterItems(index, WorkspaceTypeProject), 2)
	writeItemsSection(&b, workspace, "External Sources", filterItems(index, "search_source"), 3)
	b.WriteString("## Next Actions\n\n")
	b.WriteString("- Map each JD requirement to a resume bullet or project evidence path.\n")
	b.WriteString("- Mark missing claims as needing user confirmation instead of writing them as facts.\n")
	b.WriteString("- Prepare interview examples for the strongest matched project evidence.\n")
	return b.String()
}

func renderResumeReview(workspace *Workspace, index WorkspaceIndex) string {
	var b strings.Builder
	b.WriteString("# Resume Review\n\n")
	writeItemsSection(&b, workspace, "Current Resume Material", filterItems(index, WorkspaceTypeResume), 2)
	writeItemsSection(&b, workspace, "Target JD Context", filterItems(index, WorkspaceTypeJD), 1)
	b.WriteString("## Rewrite Rules\n\n")
	b.WriteString("- Keep confirmed facts separate from suggestions needing user confirmation.\n")
	b.WriteString("- Connect implementation details to engineering value, tradeoffs, reliability, and verification.\n")
	b.WriteString("- Do not add metrics unless the user provided them.\n")
	return b.String()
}

func renderProjectPitch(workspace *Workspace, index WorkspaceIndex) string {
	var b strings.Builder
	b.WriteString("# Project Pitch\n\n")
	writeItemsSection(&b, workspace, "Project Material", filterItems(index, WorkspaceTypeProject), 3)
	b.WriteString("## Talk Track Template\n\n")
	b.WriteString("1. Problem: what the project is solving.\n")
	b.WriteString("2. Architecture: main components and data flow.\n")
	b.WriteString("3. Tradeoffs: constraints, alternatives, and why this design was chosen.\n")
	b.WriteString("4. Verification: tests, evals, traces, demos, or operational signals.\n")
	b.WriteString("5. Lessons: what you would improve next based on evidence.\n")
	return b.String()
}

func renderInterviewReview(workspace *Workspace, index WorkspaceIndex) string {
	var b strings.Builder
	b.WriteString("# Interview Review\n\n")
	writeItemsSection(&b, workspace, "Interview Records", filterItems(index, WorkspaceTypeInterviewRecord), 3)
	writeItemsSection(&b, workspace, "Interview Experience Sources", filterItems(index, WorkspaceTypeInterviewExperience), 3)
	b.WriteString("## Review Checklist\n\n")
	b.WriteString("- Extract questions that appeared repeatedly.\n")
	b.WriteString("- Record weak answers and rewrite them with structure.\n")
	b.WriteString("- Link every project claim back to actual evidence.\n")
	return b.String()
}

func renderReviewMaterial(workspace *Workspace, index WorkspaceIndex) string {
	var b strings.Builder
	b.WriteString("# Review Material\n\n")
	writeItemsSection(&b, workspace, "Review Notes", filterItems(index, WorkspaceTypeReviewNote), 5)
	writeItemsSection(&b, workspace, "Interview Records", filterItems(index, WorkspaceTypeInterviewRecord), 2)
	b.WriteString("## Suggested Buckets\n\n")
	b.WriteString("- Agent runtime and tool calling\n")
	b.WriteString("- RAG retrieval, rerank, and citation\n")
	b.WriteString("- MCP and external tool integration\n")
	b.WriteString("- Backend reliability, observability, and evals\n")
	return b.String()
}

func writeItemsSection(b *strings.Builder, workspace *Workspace, title string, items []WorkspaceItem, limit int) {
	b.WriteString("## ")
	b.WriteString(title)
	b.WriteString("\n\n")
	if len(items) == 0 {
		b.WriteString("- No material saved yet.\n\n")
		return
	}
	for i, item := range items {
		if limit > 0 && i >= limit {
			break
		}
		b.WriteString("### ")
		b.WriteString(item.Title)
		b.WriteString("\n\n")
		if item.Path != "" {
			b.WriteString("Path: `")
			b.WriteString(item.Path)
			b.WriteString("`\n\n")
		}
		excerpt := readExcerpt(workspace, item.Path, 600)
		if excerpt == "" {
			excerpt = item.Summary
		}
		if excerpt != "" {
			b.WriteString(excerpt)
			b.WriteString("\n\n")
		}
	}
}

func filterItems(index WorkspaceIndex, itemType string) []WorkspaceItem {
	var out []WorkspaceItem
	for _, item := range index.Items {
		if item.Type == itemType {
			out = append(out, item)
		}
	}
	return out
}

func readExcerpt(workspace *Workspace, relPath string, limit int) string {
	if workspace == nil || strings.TrimSpace(relPath) == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(workspace.Root, filepath.FromSlash(relPath)))
	if err != nil {
		return ""
	}
	text := strings.TrimSpace(string(data))
	if limit > 0 && len([]rune(text)) > limit {
		return string([]rune(text)[:limit]) + "\n\n[excerpt truncated]"
	}
	return text
}
