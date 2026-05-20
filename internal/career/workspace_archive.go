package career

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

func (w *Workspace) ArchivePublicInterviewExperience(content string, now time.Time) (PublicInterviewArchiveResult, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return PublicInterviewArchiveResult{}, fmt.Errorf("public interview experience content must not be empty")
	}
	if now.IsZero() {
		now = time.Now()
	}
	experienceItem, err := w.AddMaterial(WorkspaceTypeExperiences, content, now)
	if err != nil {
		return PublicInterviewArchiveResult{}, err
	}
	result := PublicInterviewArchiveResult{ExperienceItem: experienceItem}
	if containsPrepareSignals(content) {
		prepareItem, err := w.AddMaterial(WorkspaceTypePrepare, content, now)
		if err != nil {
			return PublicInterviewArchiveResult{}, err
		}
		result.PrepareItem = prepareItem
	}
	_, index, err := w.Status()
	if err != nil {
		return PublicInterviewArchiveResult{}, err
	}
	ctx := w.buildReviewLibraryContext(experienceItem, index)
	result.Domain = ctx.Domain
	generatedPaths, err := w.writeExperienceReviewLibrary(ctx, experienceItem, now)
	if err != nil {
		return PublicInterviewArchiveResult{}, err
	}
	result.GeneratedPaths = generatedPaths
	result.MyInterviewRel = findGeneratedPathWithSuffix(generatedPaths, "面经链接与公司观察.md")
	recordRel := filepath.Join(WorkspaceInternalDir, "record", "imports", fmt.Sprintf("%s-public-interview-experience.md", now.Format("20060102-150405")))
	var b strings.Builder
	b.WriteString("# Public Interview Experience Import\n\n")
	b.WriteString(fmt.Sprintf("- source_path: %s\n", experienceItem.Path))
	b.WriteString(fmt.Sprintf("- domain: %s\n", result.Domain.Name))
	if result.MyInterviewRel != "" {
		b.WriteString(fmt.Sprintf("- observation_path: %s\n", result.MyInterviewRel))
	}
	if len(result.GeneratedPaths) > 0 {
		b.WriteString(fmt.Sprintf("- generated_paths: %s\n", strings.Join(result.GeneratedPaths, ", ")))
	}
	if result.PrepareItem.ID != "" {
		b.WriteString(fmt.Sprintf("- prepare_path: %s\n", result.PrepareItem.Path))
	}
	b.WriteString("- note: public interview experience; not a real user interview record\n")
	if err := w.writeWorkspaceText(recordRel, b.String()); err != nil {
		return PublicInterviewArchiveResult{}, err
	}
	result.RecordRel = filepath.ToSlash(recordRel)
	recordItem := WorkspaceItem{
		ID:        fmt.Sprintf("record-%s-public-interview-experience", now.Format("20060102-150405")),
		Type:      WorkspaceTypeRecord,
		Title:     "Public Interview Experience Import",
		Path:      result.RecordRel,
		Tags:      []string{"import", "public-interview-experience"},
		CreatedAt: now,
		Summary:   "Imported public interview experience and recorded derived workspace paths.",
	}
	if err := w.upsertIndexItem(recordItem); err != nil {
		return PublicInterviewArchiveResult{}, err
	}
	return result, nil
}

func findGeneratedPathWithSuffix(paths []string, suffix string) string {
	for _, path := range paths {
		if strings.HasSuffix(path, suffix) {
			return path
		}
	}
	return ""
}

func (w *Workspace) writeClassificationRecord(item WorkspaceItem, classification InputClassification, sourceLabel string, now time.Time, syncActions []string) (string, error) {
	if now.IsZero() {
		now = time.Now()
	}
	name := fmt.Sprintf("%s-classification-%s.md", now.Format("20060102-150405"), slug(item.ID))
	rel := filepath.Join(WorkspaceInternalDir, "record", "imports", name)
	var b strings.Builder
	b.WriteString("# Classification Record\n\n")
	if strings.TrimSpace(sourceLabel) != "" {
		b.WriteString(fmt.Sprintf("- input: %s\n", sourceLabel))
	}
	b.WriteString(fmt.Sprintf("- classified_type: %s\n", item.Type))
	b.WriteString(fmt.Sprintf("- confidence: %.2f\n", classification.Confidence))
	if classification.Reason != "" {
		b.WriteString(fmt.Sprintf("- reason: %s\n", classification.Reason))
	}
	if classification.RulePath != "" {
		b.WriteString(fmt.Sprintf("- rule_path: %s\n", classification.RulePath))
	}
	if len(classification.Signals) > 0 {
		b.WriteString(fmt.Sprintf("- matched_signals: %s\n", strings.Join(classification.Signals, ", ")))
	}
	b.WriteString(fmt.Sprintf("- destination: %s\n", item.Path))
	if item.Metadata.Original != "" {
		b.WriteString(fmt.Sprintf("- original: %s\n", item.Metadata.Original))
	}
	if pointer := activePointerName(item.Type); pointer != "" {
		b.WriteString(fmt.Sprintf("- active_pointer_updated: %s\n", pointer))
	} else {
		b.WriteString("- active_pointer_updated: none\n")
	}
	if len(syncActions) == 0 {
		b.WriteString("- sync_actions: none\n")
	} else {
		b.WriteString(fmt.Sprintf("- sync_actions: %s\n", strings.Join(syncActions, ", ")))
	}
	if err := w.writeWorkspaceText(rel, b.String()); err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

func containsPrepareSignals(content string) bool {
	lower := strings.ToLower(content)
	for _, signal := range []string{"项目", "project", "项目追问", "项目亮点", "项目难点", "技术方案", "证据口径"} {
		if strings.Contains(lower, strings.ToLower(signal)) {
			return true
		}
	}
	return false
}
