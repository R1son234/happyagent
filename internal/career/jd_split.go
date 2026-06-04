package career

import (
	"context"
	"fmt"
	"strings"
)

// splitJDIfNeeded checks if a saved JD item contains multiple job descriptions
// by calling the LLM split task. If it does, it removes the original item and
// saves each split JD as a separate workspace item.
func (s *CopilotService) splitJDIfNeeded(ctx context.Context, ws *Workspace, item WorkspaceItem, content string) ([]WorkspaceItem, error) {
	if strings.TrimSpace(content) == "" {
		return nil, nil
	}
	runner := s.structuredTaskRunner()
	result, err := runner.RunStructuredTask(ctx, StructuredTaskRequest{
		TaskName:      "split_jd",
		PromptVersion: PromptVersionJDSplit,
		Input:         buildJDSplitPrompt(item.Path, workspaceReadPath(ws, item.Path), content),
		SourcePaths:   []string{workspaceReadPath(ws, item.Path)},
	})
	if err != nil {
		return nil, fmt.Errorf("LLM JD split: %w", err)
	}
	parsed, err := parseJDSplitOutput(result.Output)
	if err != nil {
		return nil, fmt.Errorf("parse JD split output: %w", err)
	}
	if len(parsed.JobDescriptions) <= 1 {
		return nil, nil
	}
	var created []WorkspaceItem
	for _, jd := range parsed.JobDescriptions {
		body := renderSplitJDMarkdown(jd, item.Path)
		splitTitle := jd.Title
		if strings.TrimSpace(jd.Company) != "" {
			splitTitle = jd.Company + "-" + splitTitle
		}
		splitItem, addErr := ws.AddMaterialFromFile(WorkspaceFileInput{
			ItemType:           WorkspaceTypeJD,
			Title:              splitTitle,
			Text:               body,
			OriginalName:       item.Title + ".md",
			Now:                s.now(),
			Extractor:          "llm_jd_split",
			MIMEType:           "text/markdown",
			ExtractStatus:      "ok",
			ContentFingerprint: ContentFingerprint(body),
		})
		if addErr != nil {
			return created, fmt.Errorf("save split JD %q: %w", jd.Title, addErr)
		}
		created = append(created, splitItem)
	}
	return created, nil
}
